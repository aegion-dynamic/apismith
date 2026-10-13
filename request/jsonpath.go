package request

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// LookupJSON reads one value from a JSON body.
// Paths look like $.id, $.user.email, or $.items[0].id.
// Strings are returned unquoted. Objects and arrays are compact JSON.
func LookupJSON(body, path string) (string, error) {
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return "", fmt.Errorf("response is not JSON")
	}
	tokens, err := jsonTokens(path)
	if err != nil {
		return "", err
	}
	cur := v
	for _, tok := range tokens {
		switch node := cur.(type) {
		case map[string]any:
			next, ok := node[tok]
			if !ok {
				return "", fmt.Errorf("missing %q", tok)
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(tok)
			if err != nil || i < 0 || i >= len(node) {
				return "", fmt.Errorf("bad index %q", tok)
			}
			cur = node[i]
		default:
			return "", fmt.Errorf("cannot read %q from a %s", tok, jsonKind(cur))
		}
	}
	return stringifyJSON(cur), nil
}

func jsonTokens(path string) ([]string, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "$" {
		return nil, nil
	}
	if strings.HasPrefix(path, "$") {
		path = path[1:]
	}
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return nil, nil
	}
	var tokens []string
	for i := 0; i < len(path); {
		switch path[i] {
		case '.':
			i++
		case '[':
			end := strings.IndexByte(path[i:], ']')
			if end < 0 {
				return nil, fmt.Errorf("invalid path %q", path)
			}
			inner := strings.TrimSpace(path[i+1 : i+end])
			inner = strings.Trim(inner, `"'`)
			if inner == "" {
				return nil, fmt.Errorf("invalid path %q", path)
			}
			tokens = append(tokens, inner)
			i += end + 1
		default:
			j := i
			for j < len(path) && path[j] != '.' && path[j] != '[' {
				j++
			}
			tokens = append(tokens, path[i:j])
			i = j
		}
	}
	return tokens, nil
}

func stringifyJSON(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return string(b)
	}
}

func jsonKind(v any) string {
	switch v.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", v)
	}
}
