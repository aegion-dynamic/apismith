package request

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// Curl renders a shell command for in. The Authorization value is never the real token.
// JWT mode references $JWT so the command can be run as JWT=... curl ...
func Curl(baseURL string, in ExecuteInput) string {
	target, err := BuildURL(baseURL, in.Path, in.PathParams, in.Query)
	if err != nil {
		target = baseURL + in.Path
	}
	var b strings.Builder
	fmt.Fprintf(&b, "curl -X %s %s", strings.ToUpper(in.Method), shellQuote(target))

	keys := make([]string, 0, len(in.Headers))
	for k := range in.Headers {
		if sensitiveHeaders[strings.ToLower(k)] {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, " \\\n  -H %s", shellQuote(k+": "+in.Headers[k]))
	}
	if in.ContentType != "" && !headerSet(in.Headers, "Content-Type") {
		fmt.Fprintf(&b, " \\\n  -H %s", shellQuote("Content-Type: "+in.ContentType))
	}

	switch in.AuthMode {
	case AuthNone:
	case AuthCustom:
		if in.CustomAuth != "" {
			fmt.Fprintf(&b, " \\\n  -H %s", shellQuote("Authorization: "+maskValue(in.CustomAuth)))
		}
	default:
		if in.JWT != "" {
			b.WriteString(" \\\n  -H \"Authorization: Bearer $JWT\"")
		}
	}

	if in.Body != "" && strings.ToUpper(in.Method) != http.MethodGet && strings.ToUpper(in.Method) != http.MethodHead {
		fmt.Fprintf(&b, " \\\n  --data-raw %s", shellQuote(in.Body))
	}
	return b.String()
}

// CallCommand renders an apismith call invocation. The JWT is omitted;
// call mints its own access token unless --no-auth is set.
func CallCommand(in ExecuteInput) string {
	var b strings.Builder
	fmt.Fprintf(&b, "apismith call %s %s", strings.ToUpper(in.Method), shellQuote(in.Path))
	if in.Environment != "" {
		fmt.Fprintf(&b, " --env %s", shellQuote(in.Environment))
	}
	if strings.TrimSpace(in.BaseURL) != "" {
		fmt.Fprintf(&b, " --base-url %s", shellQuote(in.BaseURL))
	}
	writeKV(&b, "--path", in.PathParams)
	writeKV(&b, "--query", in.Query)
	keys := make([]string, 0, len(in.Headers))
	for k := range in.Headers {
		if sensitiveHeaders[strings.ToLower(k)] {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, " --header %s", shellQuote(k+"="+in.Headers[k]))
	}
	if in.Body != "" {
		fmt.Fprintf(&b, " --body %s", shellQuote(in.Body))
	}
	if in.AuthMode == AuthNone {
		b.WriteString(" --no-auth")
	}
	return b.String()
}

func writeKV(b *strings.Builder, flag string, m map[string]string) {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(b, " %s %s", flag, shellQuote(k+"="+m[k]))
	}
}

func headerSet(h map[string]string, name string) bool {
	for k := range h {
		if strings.EqualFold(k, name) {
			return true
		}
	}
	return false
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
