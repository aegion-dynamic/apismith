package request

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 8 << 20 // 8 MiB

var sensitiveHeaders = map[string]bool{
	"authorization":        true,
	"cookie":               true,
	"set-cookie":           true,
	"x-api-key":            true,
	"x-amz-security-token": true,
	"proxy-authorization":  true,
}

// AuthMode controls how the Authorization header is applied.
type AuthMode string

const (
	AuthJWT    AuthMode = "jwt"
	AuthNone   AuthMode = "none"
	AuthCustom AuthMode = "custom"
)

// ExecuteInput is a fully described outbound API call.
type ExecuteInput struct {
	Environment       string            `json:"environment"`
	Method            string            `json:"method"`
	Path              string            `json:"path"`
	PathParams        map[string]string `json:"path_params"`
	Query             map[string]string `json:"query"`
	Headers           map[string]string `json:"headers"`
	Body              string            `json:"body"`
	BodyType          string            `json:"body_type,omitempty"`
	ContentType       string            `json:"content_type,omitempty"`
	AuthMode          AuthMode          `json:"auth_mode"`
	JWT               string            `json:"jwt"`
	CustomAuth        string            `json:"custom_authorization"`
	ConfirmProduction bool              `json:"confirm_production"`
	// BaseURL optionally overrides the selected environment's base URL
	// for this request (hosted server, tunnel, etc.).
	BaseURL string `json:"base_url,omitempty"`
}

// ExecuteOutput is the captured HTTP response (body truncated if huge).
type ExecuteOutput struct {
	OK            bool              `json:"ok"`
	URL           string            `json:"url"`
	Status        int               `json:"status"`
	StatusText    string            `json:"status_text"`
	DurationMS    int64             `json:"duration_ms"`
	Headers       map[string]string `json:"headers"`
	Body          string            `json:"body"`
	ContentType   string            `json:"content_type"`
	Truncated     bool              `json:"truncated"`
	Error         string            `json:"error,omitempty"`
	RequestMasked map[string]string `json:"request_headers_masked"`
	// SchemaOK is set when the spec has a JSON schema for this status.
	// Nil means the spec has nothing to check.
	SchemaOK    *bool    `json:"schema_ok,omitempty"`
	SchemaNotes []string `json:"schema_notes,omitempty"`
}

// Executor sends HTTP requests to a configured base URL.
type Executor struct {
	Client *http.Client
}

// NewExecutor returns an executor with a sensible timeout.
func NewExecutor() *Executor {
	return &Executor{
		Client: &http.Client{Timeout: 30 * time.Second},
	}
}

// ResolveBaseURL returns override if set, otherwise configured.
// Both must be an absolute http(s) URL with a host.
func ResolveBaseURL(configured, override string) (string, error) {
	chosen := strings.TrimSpace(override)
	if chosen == "" {
		chosen = strings.TrimSpace(configured)
	}
	return NormalizeBaseURL(chosen)
}

// NormalizeBaseURL trims trailing slashes and checks the URL is http(s).
func NormalizeBaseURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return "", fmt.Errorf("base url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid base url %q: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("base url must be http or https, got %q", raw)
	}
	if u.Host == "" {
		return "", fmt.Errorf("invalid base url %q: missing host", raw)
	}
	return raw, nil
}

// BuildURL joins base URL, path template, path params, and query string.
func BuildURL(baseURL, pathTemplate string, pathParams, query map[string]string) (string, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	path := substitutePath(pathTemplate, pathParams)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	raw := baseURL + path
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid url %s: %w", raw, err)
	}
	if len(query) > 0 {
		q := u.Query()
		for k, v := range query {
			if strings.TrimSpace(k) == "" || v == "" {
				continue
			}
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

func substitutePath(template string, params map[string]string) string {
	out := template
	for k, v := range params {
		out = strings.ReplaceAll(out, "{"+k+"}", url.PathEscape(v))
	}
	return out
}

// Execute performs the request. JWT values are never copied into logs.
func (e *Executor) Execute(in ExecuteInput, baseURL string, production bool) ExecuteOutput {
	if production && !in.ConfirmProduction {
		return ExecuteOutput{
			OK:    false,
			Error: "production environment requires confirm_production=true",
		}
	}

	target, err := BuildURL(baseURL, in.Path, in.PathParams, in.Query)
	if err != nil {
		return ExecuteOutput{OK: false, Error: err.Error()}
	}

	var body io.Reader
	if in.Body != "" && strings.ToUpper(in.Method) != http.MethodGet && strings.ToUpper(in.Method) != http.MethodHead {
		body = strings.NewReader(in.Body)
	}

	req, err := http.NewRequest(strings.ToUpper(in.Method), target, body)
	if err != nil {
		return ExecuteOutput{OK: false, URL: target, Error: err.Error()}
	}

	for k, v := range in.Headers {
		if strings.TrimSpace(k) == "" {
			continue
		}
		req.Header.Set(k, v)
	}

	switch in.AuthMode {
	case AuthNone:
		req.Header.Del("Authorization")
	case AuthCustom:
		if in.CustomAuth != "" {
			req.Header.Set("Authorization", in.CustomAuth)
		}
	default:
		if in.JWT != "" {
			value := in.JWT
			if !strings.HasPrefix(strings.ToLower(value), "bearer ") {
				value = "Bearer " + value
			}
			req.Header.Set("Authorization", value)
		}
	}

	if req.Header.Get("Content-Type") == "" && in.ContentType != "" {
		req.Header.Set("Content-Type", in.ContentType)
	}
	if body != nil && req.Header.Get("Content-Type") == "" {
		switch in.BodyType {
		case "text":
			req.Header.Set("Content-Type", "text/plain; charset=utf-8")
		case "file":
			req.Header.Set("Content-Type", "application/octet-stream")
		default:
			req.Header.Set("Content-Type", "application/json")
		}
	}

	masked := MaskHeaders(req.Header)

	start := time.Now()
	resp, err := e.Client.Do(req)
	duration := time.Since(start).Milliseconds()
	if err != nil {
		return ExecuteOutput{
			OK:            false,
			URL:           target,
			DurationMS:    duration,
			Error:         err.Error(),
			RequestMasked: masked,
		}
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxResponseBytes+1)
	buf, err := io.ReadAll(limited)
	if err != nil {
		return ExecuteOutput{
			OK:            false,
			URL:           target,
			Status:        resp.StatusCode,
			StatusText:    resp.Status,
			DurationMS:    duration,
			Error:         err.Error(),
			RequestMasked: masked,
			Headers:       flattenHeaders(resp.Header),
		}
	}
	truncated := len(buf) > maxResponseBytes
	if truncated {
		buf = buf[:maxResponseBytes]
	}

	out := ExecuteOutput{
		OK:            resp.StatusCode < 400,
		URL:           target,
		Status:        resp.StatusCode,
		StatusText:    resp.Status,
		DurationMS:    duration,
		Headers:       flattenHeaders(resp.Header),
		Body:          string(buf),
		ContentType:   resp.Header.Get("Content-Type"),
		Truncated:     truncated,
		RequestMasked: masked,
	}
	return out
}

// MaskHeaders copies headers with secrets redacted.
func MaskHeaders(h http.Header) map[string]string {
	out := map[string]string{}
	for k, vals := range h {
		joined := strings.Join(vals, ", ")
		if sensitiveHeaders[strings.ToLower(k)] {
			out[k] = maskValue(joined)
			continue
		}
		out[k] = joined
	}
	return out
}

func maskValue(v string) string {
	if strings.HasPrefix(strings.ToLower(v), "bearer ") {
		return "Bearer ********"
	}
	if v == "" {
		return ""
	}
	return "********"
}

func flattenHeaders(h http.Header) map[string]string {
	out := map[string]string{}
	for k, vals := range h {
		out[k] = strings.Join(vals, ", ")
	}
	return out
}

// PrettyJSON attempts to pretty-print a JSON body. Returns the original on failure.
func PrettyJSON(body string) string {
	trimmed := bytes.TrimSpace([]byte(body))
	if len(trimmed) == 0 {
		return body
	}
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return body
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, trimmed, "", "  "); err != nil {
		return body
	}
	return buf.String()
}
