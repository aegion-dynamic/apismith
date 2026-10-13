package request

import (
	"strings"
	"testing"
	"time"
)

func TestExpand(t *testing.T) {
	vars := map[string]string{"userId": "abc", "email": "a@b.com"}
	got := Expand("id={{userId}} email={{ email }}", vars, 0)
	if got != "id=abc email=a@b.com" {
		t.Fatalf("got %q", got)
	}
	if Expand("{{missing}}", vars, 0) != "{{missing}}" {
		t.Fatal("unknown placeholder should stay")
	}
	if Expand("{{index}}", nil, 3) != "3" {
		t.Fatal("index")
	}
	if Expand("{{index}}", nil, 0) != "{{index}}" {
		t.Fatal("index without a step should stay")
	}
	now := Expand("{{now}} {{now}}", nil, 1)
	parts := strings.Split(now, " ")
	if len(parts) != 2 || parts[0] != parts[1] {
		t.Fatalf("now should be stable in one string: %q", now)
	}
	if _, err := time.Parse(time.RFC3339, parts[0]); err != nil {
		t.Fatal(err)
	}
	ids := strings.Split(Expand("{{uuid}} {{uuid}}", nil, 1), " ")
	if len(ids) != 2 || ids[0] == ids[1] || len(ids[0]) < 32 {
		t.Fatalf("uuid: %v", ids)
	}
	escaped := ExpandPath("/files/{{userId}}", map[string]string{"userId": "a/b"}, 1)
	if escaped != "/files/a%2Fb" {
		t.Fatalf("path escape: %s", escaped)
	}
}

func TestLookupJSON(t *testing.T) {
	body := `{"id":"abc","n":2,"user":{"email":"a@b.com"},"items":[{"id":"i1"}]}`
	cases := map[string]string{
		"$.id":          "abc",
		"$.n":           "2",
		"$.user.email":  "a@b.com",
		"$.items[0].id": "i1",
		"$.items.0.id":  "i1",
	}
	for path, want := range cases {
		got, err := LookupJSON(body, path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if got != want {
			t.Fatalf("%s: got %q", path, got)
		}
	}
	if _, err := LookupJSON(body, "$.nope"); err == nil {
		t.Fatal("expected missing path")
	}
	if _, err := LookupJSON("not json", "$.id"); err == nil {
		t.Fatal("expected non-json error")
	}
}

func TestStatusOK(t *testing.T) {
	if !StatusOK(201, "", "") {
		t.Fatal("2xx should pass with empty expect")
	}
	if StatusOK(404, "", "") {
		t.Fatal("404 should fail with empty expect")
	}
	if !StatusOK(404, "", "404") || !StatusOK(404, "", "4xx") {
		t.Fatal("explicit expect")
	}
	if StatusOK(0, "dial tcp", "200") {
		t.Fatal("transport error")
	}
}

func TestCurlOmitsToken(t *testing.T) {
	cmd := Curl("http://localhost:8080/api/v1", ExecuteInput{
		Method:     "POST",
		Path:       "/users/{id}",
		PathParams: map[string]string{"id": "42"},
		Headers:    map[string]string{"X-Request-Id": "abc", "Authorization": "Bearer real-secret"},
		Body:       `{"email":"a@b.com"}`,
		AuthMode:   AuthJWT,
		JWT:        "real-secret-token",
	})
	if strings.Contains(cmd, "real-secret") {
		t.Fatalf("token leaked: %s", cmd)
	}
	if !strings.Contains(cmd, "$JWT") {
		t.Fatalf("missing $JWT: %s", cmd)
	}
	if !strings.Contains(cmd, "/users/42") {
		t.Fatalf("path: %s", cmd)
	}
	call := CallCommand(ExecuteInput{
		Environment: "dev",
		Method:      "GET",
		Path:        "/users/me",
		AuthMode:    AuthNone,
		JWT:         "real-secret-token",
	})
	if strings.Contains(call, "real-secret") || !strings.Contains(call, "--no-auth") {
		t.Fatalf("call: %s", call)
	}
}
