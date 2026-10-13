package openapi

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// Catalog is the console's view of an OpenAPI document.
type Catalog struct {
	Title       string     `json:"title"`
	Version     string     `json:"version"`
	Description string     `json:"description"`
	SpecPath    string     `json:"spec_path"`
	Tags        []Tag      `json:"tags"`
	Endpoints   []Endpoint `json:"endpoints"`
}

// Tag is an OpenAPI tag used to group endpoints.
type Tag struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// Endpoint is one HTTP operation extracted from the spec.
type Endpoint struct {
	ID           string         `json:"id"`
	Method       string         `json:"method"`
	Path         string         `json:"path"`
	OperationID  string         `json:"operation_id,omitempty"`
	Summary      string         `json:"summary,omitempty"`
	Description  string         `json:"description,omitempty"`
	Tags         []string       `json:"tags"`
	AuthRequired bool           `json:"auth_required"`
	Deprecated   bool           `json:"deprecated,omitempty"`
	Parameters   []Parameter    `json:"parameters"`
	RequestBody  *RequestBody   `json:"request_body,omitempty"`
	Responses    []ResponseInfo `json:"responses"`
}

// Parameter is a path, query, or header parameter.
type Parameter struct {
	Name        string      `json:"name"`
	In          string      `json:"in"`
	Required    bool        `json:"required"`
	Description string      `json:"description,omitempty"`
	Schema      *SchemaView `json:"schema,omitempty"`
	Example     any         `json:"example,omitempty"`
}

// RequestBody describes the JSON (or other) payload the operation accepts.
type RequestBody struct {
	Required    bool        `json:"required"`
	Description string      `json:"description,omitempty"`
	ContentType string      `json:"content_type"`
	Schema      *SchemaView `json:"schema,omitempty"`
	Example     any         `json:"example,omitempty"`
	ExampleJSON string      `json:"example_json,omitempty"`
}

// ResponseInfo is a compact description of an operation response.
// Schema is kept for the console's response check and is not sent to the browser.
type ResponseInfo struct {
	Status      string      `json:"status"`
	Description string      `json:"description,omitempty"`
	ContentType string      `json:"content_type,omitempty"`
	Schema      *SchemaView `json:"-"`
}

// Load parses an OpenAPI 3 document and returns the catalog used by the UI.
func Load(path string) (*Catalog, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = true
	doc, err := loader.LoadFromFile(path)
	if err != nil {
		return nil, fmt.Errorf("load OpenAPI spec %s: %w", path, err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		// Some in-progress specs fail strict validation; still extract operations.
		_ = err
	}
	return catalogFromDoc(path, doc), nil
}

func catalogFromDoc(path string, doc *openapi3.T) *Catalog {
	cat := &Catalog{SpecPath: path}
	if doc.Info != nil {
		cat.Title = doc.Info.Title
		cat.Version = doc.Info.Version
		cat.Description = doc.Info.Description
	}

	tagDesc := map[string]string{}
	for _, t := range doc.Tags {
		if t == nil {
			continue
		}
		tagDesc[t.Name] = t.Description
		cat.Tags = append(cat.Tags, Tag{Name: t.Name, Description: t.Description})
	}

	if doc.Paths == nil {
		return cat
	}

	seenTags := map[string]bool{}
	for _, t := range cat.Tags {
		seenTags[t.Name] = true
	}

	for _, pathName := range sortedKeys(doc.Paths.Map()) {
		item := doc.Paths.Value(pathName)
		if item == nil {
			continue
		}
		for _, method := range []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"} {
			op := item.GetOperation(method)
			if op == nil {
				continue
			}
			ep := endpointFrom(doc, method, pathName, item, op)
			cat.Endpoints = append(cat.Endpoints, ep)
			for _, tag := range ep.Tags {
				if !seenTags[tag] {
					seenTags[tag] = true
					cat.Tags = append(cat.Tags, Tag{Name: tag})
				}
			}
		}
	}

	sort.SliceStable(cat.Endpoints, func(i, j int) bool {
		ti, tj := firstTag(cat.Endpoints[i]), firstTag(cat.Endpoints[j])
		if ti != tj {
			return ti < tj
		}
		if cat.Endpoints[i].Path != cat.Endpoints[j].Path {
			return cat.Endpoints[i].Path < cat.Endpoints[j].Path
		}
		return cat.Endpoints[i].Method < cat.Endpoints[j].Method
	})
	return cat
}

func endpointFrom(doc *openapi3.T, method, path string, item *openapi3.PathItem, op *openapi3.Operation) Endpoint {
	tags := append([]string{}, op.Tags...)
	if len(tags) == 0 {
		tags = []string{"untagged"}
	}
	ep := Endpoint{
		ID:           method + " " + path,
		Method:       method,
		Path:         path,
		OperationID:  op.OperationID,
		Summary:      op.Summary,
		Description:  strings.TrimSpace(op.Description),
		Tags:         tags,
		AuthRequired: requiresAuth(doc, op),
		Deprecated:   op.Deprecated,
		Parameters:   collectParams(item, op),
		Responses:    collectResponses(op),
	}
	ep.RequestBody = collectRequestBody(op)
	return ep
}

func requiresAuth(doc *openapi3.T, op *openapi3.Operation) bool {
	if op.Security != nil {
		return len(*op.Security) > 0
	}
	return len(doc.Security) > 0
}

func collectParams(item *openapi3.PathItem, op *openapi3.Operation) []Parameter {
	seen := map[string]bool{}
	var out []Parameter
	add := func(p *openapi3.Parameter) {
		if p == nil {
			return
		}
		key := p.In + ":" + p.Name
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, Parameter{
			Name:        p.Name,
			In:          p.In,
			Required:    p.Required || p.In == "path",
			Description: p.Description,
			Schema:      viewFromRef(p.Schema, nil),
			Example:     firstExample(p.Example, exampleFromMap(p.Examples), schemaExample(p.Schema)),
		})
	}
	for _, ref := range item.Parameters {
		if ref != nil {
			add(ref.Value)
		}
	}
	for _, ref := range op.Parameters {
		if ref != nil {
			add(ref.Value)
		}
	}
	return out
}

func collectRequestBody(op *openapi3.Operation) *RequestBody {
	if op.RequestBody == nil || op.RequestBody.Value == nil {
		return nil
	}
	rb := op.RequestBody.Value
	ct, media := preferredMedia(rb.Content)
	if media == nil {
		return &RequestBody{Required: rb.Required, Description: rb.Description}
	}
	schema := viewFromRef(media.Schema, nil)
	example := firstExample(media.Example, exampleFromMap(media.Examples), exampleFromSchemaView(schema))
	return &RequestBody{
		Required:    rb.Required,
		Description: rb.Description,
		ContentType: ct,
		Schema:      schema,
		Example:     example,
		ExampleJSON: mustJSON(example),
	}
}

func collectResponses(op *openapi3.Operation) []ResponseInfo {
	if op.Responses == nil {
		return nil
	}
	var out []ResponseInfo
	for _, code := range sortedKeys(op.Responses.Map()) {
		ref := op.Responses.Value(code)
		desc := ""
		if ref != nil && ref.Value != nil && ref.Value.Description != nil {
			desc = *ref.Value.Description
		}
		info := ResponseInfo{Status: code, Description: desc}
		if ref != nil && ref.Value != nil {
			ct, media := preferredMedia(ref.Value.Content)
			if media != nil {
				info.ContentType = ct
				info.Schema = viewFromRef(media.Schema, nil)
			}
		}
		out = append(out, info)
	}
	return out
}

func preferredMedia(content openapi3.Content) (string, *openapi3.MediaType) {
	if content == nil {
		return "", nil
	}
	for _, ct := range []string{"application/json", "application/problem+json"} {
		if m := content.Get(ct); m != nil {
			return ct, m
		}
	}
	for ct, m := range content {
		return ct, m
	}
	return "", nil
}

func firstTag(ep Endpoint) string {
	if len(ep.Tags) == 0 {
		return "untagged"
	}
	return ep.Tags[0]
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func exampleFromMap(examples openapi3.Examples) any {
	if len(examples) == 0 {
		return nil
	}
	keys := sortedKeys(examples)
	ex := examples[keys[0]]
	if ex != nil && ex.Value != nil {
		return ex.Value.Value
	}
	return nil
}

func schemaExample(ref *openapi3.SchemaRef) any {
	if ref == nil || ref.Value == nil {
		return nil
	}
	return ref.Value.Example
}

func firstExample(values ...any) any {
	for _, v := range values {
		if v != nil {
			return v
		}
	}
	return nil
}
