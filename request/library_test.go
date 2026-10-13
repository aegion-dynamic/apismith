package request

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLibraryRoundTrip(t *testing.T) {
	lib, err := OpenLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	err = lib.PutSaved(SavedRequest{
		Name:     "Get me",
		Method:   "get",
		Path:     "/users/me",
		Headers:  map[string]string{"Authorization": "Bearer secret", "X-Debug": "1"},
		AuthMode: AuthJWT,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := lib.GetSaved("Get me")
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != "GET" {
		t.Fatalf("method: %s", got.Method)
	}
	if _, ok := got.Headers["Authorization"]; ok {
		t.Fatal("authorization header was stored")
	}
	if got.Headers["X-Debug"] != "1" {
		t.Fatalf("headers: %#v", got.Headers)
	}
	b, err := os.ReadFile(filepath.Join(lib.dir, "saved", "get-me.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "secret") {
		t.Fatalf("secret in file:\n%s", b)
	}

	seq := Sequence{
		Name: "signup",
		Steps: []Step{
			{ID: "create", Method: "POST", Path: "/users", Body: `{"email":"{{email}}"}`, Expect: "201", Capture: map[string]string{"userId": "$.id"}},
			{Saved: "Get me", Expect: "200"},
		},
	}
	if err := lib.PutSequence(seq); err != nil {
		t.Fatal(err)
	}
	loaded, err := lib.GetSequence("signup")
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Steps) != 2 || loaded.Steps[1].Saved != "Get me" {
		t.Fatalf("%+v", loaded.Steps)
	}
	resolved, err := lib.ResolveStep(loaded.Steps[1])
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Method != "GET" || resolved.Path != "/users/me" {
		t.Fatalf("resolved: %+v", resolved)
	}
	if err := lib.DeleteSaved("Get me"); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.GetSaved("Get me"); err == nil {
		t.Fatal("expected delete")
	}
}

func TestHistoryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")
	h := NewHistory(2)
	if err := h.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	h.Add(HistoryItem{Method: "GET", Path: "/a", Headers: map[string]string{"Authorization": "Bearer secret"}})
	h.Add(HistoryItem{Method: "GET", Path: "/b"})
	h.Add(HistoryItem{Method: "GET", Path: "/c"})

	again := NewHistory(2)
	if err := again.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	items := again.List()
	if len(items) != 2 || items[0].Path != "/c" || items[1].Path != "/b" {
		t.Fatalf("reloaded: %+v", items)
	}
	for _, it := range items {
		if _, ok := it.Headers["Authorization"]; ok {
			t.Fatal("authorization stored in history")
		}
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "secret") {
		t.Fatalf("secret in history file: %s", raw)
	}
}

func TestRunSequenceCapture(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":"42"}`))
	})
	mux.HandleFunc("/users/42", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("auth %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"42","ok":true}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	seq := Sequence{
		Name: "signup",
		Steps: []Step{
			{ID: "create", Method: "POST", Path: "/users", Body: `{"email":"{{email}}"}`, Expect: "201", Capture: map[string]string{"userId": "$.id"}, AuthMode: AuthNone},
			{ID: "get", Method: "GET", Path: "/users/{id}", PathParams: map[string]string{"id": "{{userId}}"}, Expect: "200", AuthMode: AuthJWT},
			{ID: "nope", Method: "GET", Path: "/missing", Expect: "200", AuthMode: AuthNone},
			{ID: "later", Method: "GET", Path: "/users/42", AuthMode: AuthNone},
		},
	}
	result := RunSequence(NewExecutor(), nil, seq, RunOptions{
		BaseURL:  srv.URL,
		AuthMode: AuthJWT,
		JWT:      "tok",
		Vars:     map[string]string{"email": "a@b.com"},
	})
	if result.OK {
		t.Fatal("expected failure on missing route")
	}
	if len(result.Steps) != 4 {
		t.Fatalf("steps: %+v", result.Steps)
	}
	if !result.Steps[0].OK || result.Steps[0].Captured["userId"] != "42" {
		t.Fatalf("create: %+v", result.Steps[0])
	}
	if !result.Steps[1].OK || !strings.Contains(result.Steps[1].URL, "/users/42") {
		t.Fatalf("get: %+v", result.Steps[1])
	}
	if result.Steps[2].OK || result.Steps[2].Skipped {
		t.Fatalf("missing should fail: %+v", result.Steps[2])
	}
	if !result.Steps[3].Skipped {
		t.Fatalf("later step should be skipped: %+v", result.Steps[3])
	}
	if result.Variables["userId"] != "42" || result.Variables["email"] != "a@b.com" {
		t.Fatalf("vars: %#v", result.Variables)
	}
}
