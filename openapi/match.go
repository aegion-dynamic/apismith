package openapi

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const maxSchemaNotes = 12

// ResponseSchema returns the JSON schema for a status code, then a class
// such as 2XX, then default. Nil when the spec has no schema for that response.
func (c *Catalog) ResponseSchema(method, path string, status int) *SchemaView {
	if c == nil || status == 0 {
		return nil
	}
	ep, _, err := c.LookupMethodPath(method, path)
	if err != nil || ep == nil {
		return nil
	}
	code := strconv.Itoa(status)
	var class, def *SchemaView
	for i := range ep.Responses {
		r := &ep.Responses[i]
		if r.Schema == nil {
			continue
		}
		if r.Status == code {
			return r.Schema
		}
		if len(r.Status) == 3 && strings.EqualFold(r.Status[1:], "xx") && r.Status[:1] == code[:1] {
			class = r.Schema
		}
		if r.Status == "default" {
			def = r.Schema
		}
	}
	if class != nil {
		return class
	}
	return def
}

// MatchBody checks a JSON body against a simplified OpenAPI schema.
// Extra properties are allowed. The bool is false when notes is non-empty.
func MatchBody(body string, schema *SchemaView) (bool, []string) {
	if schema == nil {
		return true, nil
	}
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return false, []string{"response is not JSON"}
	}
	var notes []string
	matchValue("$", v, schema, &notes)
	return len(notes) == 0, notes
}

func matchValue(path string, v any, schema *SchemaView, notes *[]string) {
	if schema == nil || len(*notes) >= maxSchemaNotes {
		return
	}
	if v == nil {
		if schema.Nullable {
			return
		}
		addNote(notes, path+": null")
		return
	}
	typ := schema.Type
	if typ == "" && len(schema.Properties) > 0 {
		typ = "object"
	}
	switch typ {
	case "object":
		obj, ok := v.(map[string]any)
		if !ok {
			addNote(notes, fmt.Sprintf("%s: expected object", path))
			return
		}
		for _, name := range schema.Required {
			if _, exists := obj[name]; !exists {
				addNote(notes, fmt.Sprintf("%s.%s: required", path, name))
			}
		}
		for name, prop := range schema.Properties {
			child, exists := obj[name]
			if !exists {
				continue
			}
			childPath := path + "." + name
			if path == "$" {
				childPath = "$." + name
			}
			matchValue(childPath, child, prop, notes)
		}
	case "array":
		arr, ok := v.([]any)
		if !ok {
			addNote(notes, fmt.Sprintf("%s: expected array", path))
			return
		}
		if schema.Items == nil {
			return
		}
		for i, item := range arr {
			if len(*notes) >= maxSchemaNotes {
				return
			}
			matchValue(fmt.Sprintf("%s[%d]", path, i), item, schema.Items, notes)
		}
	case "string":
		if _, ok := v.(string); !ok {
			addNote(notes, fmt.Sprintf("%s: expected string", path))
		}
	case "integer":
		n, ok := v.(float64)
		if !ok || n != math.Trunc(n) || math.IsInf(n, 0) {
			addNote(notes, fmt.Sprintf("%s: expected integer", path))
		}
	case "number":
		if _, ok := v.(float64); !ok {
			addNote(notes, fmt.Sprintf("%s: expected number", path))
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			addNote(notes, fmt.Sprintf("%s: expected boolean", path))
		}
	}
}

func addNote(notes *[]string, msg string) {
	if len(*notes) >= maxSchemaNotes {
		return
	}
	*notes = append(*notes, msg)
}
