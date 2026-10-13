package request

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// SavedRequest is one named call. Tokens are not stored.
type SavedRequest struct {
	Name        string            `json:"name" yaml:"name"`
	Method      string            `json:"method" yaml:"method"`
	Path        string            `json:"path" yaml:"path"`
	PathParams  map[string]string `json:"path_params,omitempty" yaml:"path_params,omitempty"`
	Query       map[string]string `json:"query,omitempty" yaml:"query,omitempty"`
	Headers     map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	Body        string            `json:"body,omitempty" yaml:"body,omitempty"`
	BodyType    string            `json:"body_type,omitempty" yaml:"body_type,omitempty"`
	ContentType string            `json:"content_type,omitempty" yaml:"content_type,omitempty"`
	AuthMode    AuthMode          `json:"auth_mode,omitempty" yaml:"auth_mode,omitempty"`
}

// Step is one call in a sequence. Method and path make the step portable.
// Saved is used only when method and path are empty, to pull a library entry at run time.
type Step struct {
	ID          string            `json:"id" yaml:"id"`
	Saved       string            `json:"saved,omitempty" yaml:"saved,omitempty"`
	Method      string            `json:"method" yaml:"method"`
	Path        string            `json:"path" yaml:"path"`
	PathParams  map[string]string `json:"path_params,omitempty" yaml:"path_params,omitempty"`
	Query       map[string]string `json:"query,omitempty" yaml:"query,omitempty"`
	Headers     map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	Body        string            `json:"body,omitempty" yaml:"body,omitempty"`
	BodyType    string            `json:"body_type,omitempty" yaml:"body_type,omitempty"`
	ContentType string            `json:"content_type,omitempty" yaml:"content_type,omitempty"`
	AuthMode    AuthMode          `json:"auth_mode,omitempty" yaml:"auth_mode,omitempty"`
	Expect      string            `json:"expect,omitempty" yaml:"expect,omitempty"`
	Capture     map[string]string `json:"capture,omitempty" yaml:"capture,omitempty"`
}

// Sequence is an ordered list of calls.
type Sequence struct {
	Name  string `json:"name" yaml:"name"`
	Steps []Step `json:"steps" yaml:"steps"`
}

// Library stores saved requests and sequences as YAML beside the environments file.
type Library struct {
	dir string
}

// OpenLibrary creates config/library/saved and config/library/sequences.
func OpenLibrary(dir string) (*Library, error) {
	if strings.TrimSpace(dir) == "" {
		return &Library{}, nil
	}
	for _, sub := range []string{"saved", "sequences"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, fmt.Errorf("library: %w", err)
		}
	}
	return &Library{dir: dir}, nil
}

func (l *Library) ready() error {
	if l == nil || l.dir == "" {
		return fmt.Errorf("library directory is not available")
	}
	return nil
}

// ListSaved returns saved requests ordered by name.
func (l *Library) ListSaved() ([]SavedRequest, error) {
	if err := l.ready(); err != nil {
		return nil, err
	}
	return readAll[SavedRequest](filepath.Join(l.dir, "saved"))
}

// PutSaved writes one request. Authorization and cookie headers are dropped.
func (l *Library) PutSaved(req SavedRequest) error {
	if err := l.ready(); err != nil {
		return err
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := validName(req.Name); err != nil {
		return err
	}
	req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	req.Path = strings.TrimSpace(req.Path)
	if req.Method == "" || req.Path == "" {
		return fmt.Errorf("method and path are required")
	}
	req.Headers = stripSensitive(req.Headers)
	return writeNamed(filepath.Join(l.dir, "saved"), req.Name, req)
}

// GetSaved reads one saved request by name.
func (l *Library) GetSaved(name string) (SavedRequest, error) {
	items, err := l.ListSaved()
	if err != nil {
		return SavedRequest{}, err
	}
	for _, it := range items {
		if it.Name == name {
			return it, nil
		}
	}
	return SavedRequest{}, fmt.Errorf("saved request %q not found", name)
}

// DeleteSaved removes a saved request by name.
func (l *Library) DeleteSaved(name string) error {
	if err := l.ready(); err != nil {
		return err
	}
	return deleteNamed[SavedRequest](filepath.Join(l.dir, "saved"), name)
}

// ListSequences returns sequences ordered by name.
func (l *Library) ListSequences() ([]Sequence, error) {
	if err := l.ready(); err != nil {
		return nil, err
	}
	return readAll[Sequence](filepath.Join(l.dir, "sequences"))
}

// PutSequence writes one sequence.
func (l *Library) PutSequence(seq Sequence) error {
	if err := l.ready(); err != nil {
		return err
	}
	seq.Name = strings.TrimSpace(seq.Name)
	if err := validName(seq.Name); err != nil {
		return err
	}
	if len(seq.Steps) == 0 {
		return fmt.Errorf("a sequence needs at least one step")
	}
	for i := range seq.Steps {
		seq.Steps[i].Method = strings.ToUpper(strings.TrimSpace(seq.Steps[i].Method))
		seq.Steps[i].Headers = stripSensitive(seq.Steps[i].Headers)
		if strings.TrimSpace(seq.Steps[i].ID) == "" {
			seq.Steps[i].ID = fmt.Sprintf("step-%d", i+1)
		}
		if seq.Steps[i].Method == "" || strings.TrimSpace(seq.Steps[i].Path) == "" {
			if strings.TrimSpace(seq.Steps[i].Saved) == "" {
				return fmt.Errorf("step %d needs a method and path", i+1)
			}
		}
	}
	return writeNamed(filepath.Join(l.dir, "sequences"), seq.Name, seq)
}

// GetSequence reads one sequence by name.
func (l *Library) GetSequence(name string) (Sequence, error) {
	items, err := l.ListSequences()
	if err != nil {
		return Sequence{}, err
	}
	for _, it := range items {
		if it.Name == name {
			return it, nil
		}
	}
	return Sequence{}, fmt.Errorf("sequence %q not found", name)
}

// DeleteSequence removes a sequence by name.
func (l *Library) DeleteSequence(name string) error {
	if err := l.ready(); err != nil {
		return err
	}
	return deleteNamed[Sequence](filepath.Join(l.dir, "sequences"), name)
}

// LoadSequenceFile reads a sequence YAML file from any path.
func LoadSequenceFile(path string) (Sequence, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Sequence{}, fmt.Errorf("read sequence: %w", err)
	}
	var seq Sequence
	if err := yaml.Unmarshal(b, &seq); err != nil {
		return Sequence{}, fmt.Errorf("parse sequence: %w", err)
	}
	if strings.TrimSpace(seq.Name) == "" {
		seq.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if len(seq.Steps) == 0 {
		return Sequence{}, fmt.Errorf("sequence %q has no steps", seq.Name)
	}
	return seq, nil
}

// ResolveStep fills method and path from a saved request when the step omitted them.
func (l *Library) ResolveStep(step Step) (Step, error) {
	if strings.TrimSpace(step.Method) != "" && strings.TrimSpace(step.Path) != "" {
		step.Method = strings.ToUpper(step.Method)
		return step, nil
	}
	name := strings.TrimSpace(step.Saved)
	if name == "" {
		return Step{}, fmt.Errorf("step %q needs a method and path", step.ID)
	}
	if l == nil {
		return Step{}, fmt.Errorf("step %q references saved request %q but no library is configured", step.ID, name)
	}
	saved, err := l.GetSaved(name)
	if err != nil {
		return Step{}, err
	}
	out := Step{
		ID:          step.ID,
		Saved:       name,
		Method:      saved.Method,
		Path:        saved.Path,
		PathParams:  cloneMap(saved.PathParams),
		Query:       cloneMap(saved.Query),
		Headers:     cloneMap(saved.Headers),
		Body:        saved.Body,
		BodyType:    saved.BodyType,
		ContentType: saved.ContentType,
		AuthMode:    saved.AuthMode,
		Expect:      step.Expect,
		Capture:     step.Capture,
	}
	if step.Method != "" {
		out.Method = strings.ToUpper(step.Method)
	}
	if step.Path != "" {
		out.Path = step.Path
	}
	if step.Body != "" {
		out.Body = step.Body
	}
	if step.BodyType != "" {
		out.BodyType = step.BodyType
	}
	if step.ContentType != "" {
		out.ContentType = step.ContentType
	}
	if step.AuthMode != "" {
		out.AuthMode = step.AuthMode
	}
	if len(step.PathParams) > 0 {
		out.PathParams = step.PathParams
	}
	if len(step.Query) > 0 {
		out.Query = step.Query
	}
	if len(step.Headers) > 0 {
		out.Headers = step.Headers
	}
	return out, nil
}

type named interface {
	itemName() string
}

func (s SavedRequest) itemName() string { return s.Name }
func (s Sequence) itemName() string     { return s.Name }

func readAll[T named](dir string) ([]T, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []T
	seen := map[string]bool{}
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".yaml") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, ent.Name()))
		if err != nil {
			return nil, err
		}
		var item T
		if err := yaml.Unmarshal(b, &item); err != nil {
			return nil, fmt.Errorf("%s: %w", ent.Name(), err)
		}
		if seen[item.itemName()] {
			continue
		}
		seen[item.itemName()] = true
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].itemName()) < strings.ToLower(out[j].itemName())
	})
	if out == nil {
		out = []T{}
	}
	return out, nil
}

func writeNamed(dir, name string, v any) error {
	path, err := pathForName(dir, name)
	if err != nil {
		return err
	}
	b, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func deleteNamed[T named](dir, name string) error {
	name = strings.TrimSpace(name)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	found := false
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, ent.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var item T
		if err := yaml.Unmarshal(b, &item); err != nil {
			continue
		}
		if item.itemName() != name {
			continue
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		found = true
	}
	if !found {
		return fmt.Errorf("%q not found", name)
	}
	return nil
}

func pathForName(dir, name string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, ent.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		var probe struct {
			Name string `yaml:"name"`
		}
		if err := yaml.Unmarshal(b, &probe); err != nil {
			continue
		}
		if probe.Name == name {
			return path, nil
		}
	}
	base := slug(name)
	candidate := filepath.Join(dir, base+".yaml")
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate, nil
	}
	for i := 2; i < 1000; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s-%d.yaml", base, i))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not allocate a file name for %q", name)
}

func validName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > 80 {
		return fmt.Errorf("name is too long")
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("name cannot contain slashes")
	}
	return nil
}

func slug(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		return "item"
	}
	return s
}

func stripSensitive(h map[string]string) map[string]string {
	if len(h) == 0 {
		return h
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		if sensitiveHeaders[strings.ToLower(k)] {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
