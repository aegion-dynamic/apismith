package request

import (
	"fmt"
	"sort"
	"strings"
)

// StepResult is one step of a sequence run.
type StepResult struct {
	Index      int               `json:"index"`
	ID         string            `json:"id"`
	Method     string            `json:"method"`
	Path       string            `json:"path"`
	URL        string            `json:"url,omitempty"`
	Status     int               `json:"status"`
	StatusText string            `json:"status_text,omitempty"`
	DurationMS int64             `json:"duration_ms"`
	OK         bool              `json:"ok"`
	Skipped    bool              `json:"skipped,omitempty"`
	Error      string            `json:"error,omitempty"`
	Body       string            `json:"body,omitempty"`
	Captured   map[string]string `json:"captured,omitempty"`
}

// RunResult is the outcome of a sequence.
type RunResult struct {
	OK        bool              `json:"ok"`
	Steps     []StepResult      `json:"steps"`
	Variables map[string]string `json:"variables"`
}

// RunOptions is the session a sequence runs with. JWT is not stored.
type RunOptions struct {
	BaseURL    string
	Production bool
	Confirm    bool
	AuthMode   AuthMode
	JWT        string
	CustomAuth string
	Vars       map[string]string
}

// RunSequence sends steps in order. A step that misses its expected status,
// or whose capture path is missing, stops the run. Later steps are skipped.
// Captures are written into the returned variable table.
func RunSequence(exec *Executor, lib *Library, seq Sequence, opt RunOptions) RunResult {
	if exec == nil {
		exec = NewExecutor()
	}
	working := cloneMap(opt.Vars)
	results := make([]StepResult, 0, len(seq.Steps))
	failed := false
	for i, step := range seq.Steps {
		res := StepResult{Index: i + 1, ID: step.ID, Method: step.Method, Path: step.Path}
		if strings.TrimSpace(res.ID) == "" {
			res.ID = fmt.Sprintf("step-%d", i+1)
		}
		if failed {
			res.Skipped = true
			res.Method = step.Method
			res.Path = step.Path
			results = append(results, res)
			continue
		}
		resolved, err := lib.ResolveStep(step)
		if err != nil {
			res.OK = false
			res.Error = err.Error()
			failed = true
			results = append(results, res)
			continue
		}
		res.Method = resolved.Method
		res.Path = resolved.Path
		in := ExecuteInput{
			Method:            resolved.Method,
			Path:              resolved.Path,
			PathParams:        resolved.PathParams,
			Query:             resolved.Query,
			Headers:           resolved.Headers,
			Body:              resolved.Body,
			ContentType:       resolved.ContentType,
			AuthMode:          resolved.AuthMode,
			JWT:               opt.JWT,
			CustomAuth:        opt.CustomAuth,
			ConfirmProduction: opt.Confirm,
		}
		if in.AuthMode == "" {
			in.AuthMode = opt.AuthMode
		}
		if in.AuthMode == "" {
			in.AuthMode = AuthJWT
		}
		expanded := ExpandInput(in, working, i+1)
		out := exec.Execute(expanded, opt.BaseURL, opt.Production)
		if strings.Contains(strings.ToLower(out.ContentType), "json") {
			out.Body = PrettyJSON(out.Body)
		}
		res.URL = out.URL
		res.Status = out.Status
		res.StatusText = out.StatusText
		res.DurationMS = out.DurationMS
		res.Body = out.Body
		if !StatusOK(out.Status, out.Error, resolved.Expect) {
			res.OK = false
			res.Error = statusFailure(out, resolved.Expect)
			failed = true
			results = append(results, res)
			continue
		}
		captured, err := captureAll(out.Body, resolved.Capture)
		if err != nil {
			res.OK = false
			res.Error = err.Error()
			failed = true
			results = append(results, res)
			continue
		}
		for k, v := range captured {
			working[k] = v
		}
		res.OK = true
		res.Captured = captured
		results = append(results, res)
	}
	return RunResult{OK: !failed, Steps: results, Variables: working}
}

func captureAll(body string, capture map[string]string) (map[string]string, error) {
	if len(capture) == 0 {
		return nil, nil
	}
	names := make([]string, 0, len(capture))
	for name := range capture {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make(map[string]string, len(names))
	for _, name := range names {
		path := capture[name]
		val, err := LookupJSON(body, path)
		if err != nil {
			return nil, fmt.Errorf("capture %s (%s): %w", name, path, err)
		}
		out[name] = val
	}
	return out, nil
}

func statusFailure(out ExecuteOutput, expect string) string {
	if out.Error != "" && out.Status == 0 {
		return out.Error
	}
	want := strings.TrimSpace(expect)
	if want == "" {
		want = "2xx"
	}
	if out.Error != "" {
		return fmt.Sprintf("expected %s, got %d: %s", want, out.Status, out.Error)
	}
	return fmt.Sprintf("expected %s, got %d", want, out.Status)
}
