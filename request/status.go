package request

import (
	"strconv"
	"strings"
)

// StatusOK reports whether a response satisfies expect.
// An empty expect accepts any 2xx or 3xx, matching a single call with no --expect.
// expect may be a code ("404") or a class ("2xx"). A transport error never matches.
func StatusOK(status int, transportErr, expect string) bool {
	if transportErr != "" {
		return false
	}
	expect = strings.TrimSpace(expect)
	if expect == "" {
		return status >= 200 && status < 400
	}
	if strings.HasSuffix(strings.ToLower(expect), "xx") && len(expect) == 3 {
		class, err := strconv.Atoi(expect[:1])
		if err != nil {
			return false
		}
		return status/100 == class
	}
	want, err := strconv.Atoi(expect)
	if err != nil {
		return false
	}
	return status == want
}
