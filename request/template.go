package request

import (
	"crypto/rand"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"time"
)

// placeholder matches {{name}} in params, headers, and bodies.
var placeholder = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// Expand replaces {{name}} from vars. {{uuid}} is a new id per occurrence,
// {{now}} is the same UTC timestamp for the whole string, and {{index}} is
// the 1-based step index when index > 0. Unknown names are left in place.
func Expand(s string, vars map[string]string, index int) string {
	return expand(s, vars, index, false)
}

// ExpandPath is Expand with each substituted value path-escaped.
func ExpandPath(s string, vars map[string]string, index int) string {
	return expand(s, vars, index, true)
}

func expand(s string, vars map[string]string, index int, escape bool) string {
	if s == "" || !placeholder.MatchString(s) {
		return s
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return placeholder.ReplaceAllStringFunc(s, func(match string) string {
		name := placeholder.FindStringSubmatch(match)[1]
		val, ok := builtin(name, vars, index, now)
		if !ok {
			return match
		}
		if escape {
			return url.PathEscape(val)
		}
		return val
	})
}

func builtin(name string, vars map[string]string, index int, now string) (string, bool) {
	switch name {
	case "uuid":
		return newUUID(), true
	case "now":
		return now, true
	case "index":
		if index > 0 {
			return strconv.Itoa(index), true
		}
		return "", false
	default:
		v, ok := vars[name]
		return v, ok
	}
}

// ExpandMap returns a copy with values expanded. Nil stays nil.
func ExpandMap(in map[string]string, vars map[string]string, index int) map[string]string {
	if len(in) == 0 {
		return in
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = Expand(v, vars, index)
	}
	return out
}

// ExpandInput returns a copy of in with {{name}} filled from vars.
// The path string is path-escaped; path params are escaped later by BuildURL.
func ExpandInput(in ExecuteInput, vars map[string]string, index int) ExecuteInput {
	in.Path = ExpandPath(in.Path, vars, index)
	in.PathParams = ExpandMap(in.PathParams, vars, index)
	in.Query = ExpandMap(in.Query, vars, index)
	in.Headers = ExpandMap(in.Headers, vars, index)
	in.Body = Expand(in.Body, vars, index)
	in.JWT = Expand(in.JWT, vars, index)
	in.CustomAuth = Expand(in.CustomAuth, vars, index)
	in.BaseURL = Expand(in.BaseURL, vars, index)
	in.ContentType = Expand(in.ContentType, vars, index)
	return in
}

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("id-%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
