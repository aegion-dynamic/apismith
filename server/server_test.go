package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"aegion-dynamic/apismith/environments"
	"aegion-dynamic/apismith/openapi"
)

func TestConsoleSavedVarsSequenceAndHistory(t *testing.T) {
	var seenPath, seenAuth, seenType string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenAuth = r.Header.Get("Authorization")
		seenType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"7"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"7"}`))
	}))
	defer upstream.Close()

	dir := t.TempDir()
	cfg := &environments.Config{
		Path:               filepath.Join(dir, "environments.yaml"),
		DefaultEnvironment: "dev",
		Environments: []environments.Environment{{
			ID: "dev", Name: "DEV", BaseURL: upstream.URL,
		}},
	}
	cat := &openapi.Catalog{
		Title: "t",
		Endpoints: []openapi.Endpoint{{
			Method: "GET",
			Path:   "/items/{id}",
			Responses: []openapi.ResponseInfo{{
				Status: "200",
				Schema: &openapi.SchemaView{
					Type:     "object",
					Required: []string{"id"},
					Properties: map[string]*openapi.SchemaView{
						"id": {Type: "string"},
					},
				},
			}},
		}, {
			Method: "POST",
			Path:   "/items",
		}},
	}
	console := httptest.NewServer(New(cfg, cat).Handler())
	defer console.Close()

	putJSON(t, console.URL+"/api/variables", `{"itemId":"7"}`)

	res := postJSON(t, console.URL+"/api/request", `{
		"environment":"dev",
		"method":"GET",
		"path":"/items/{id}",
		"path_params":{"id":"{{itemId}}"},
		"auth_mode":"jwt",
		"jwt":"real-token",
		"body_type":"text",
		"body":"hello"
	}`)
	if res["response"].(map[string]any)["schema_ok"] != true {
		t.Fatalf("schema: %#v", res["response"])
	}
	if seenPath != "/items/7" {
		t.Fatalf("path: %s", seenPath)
	}
	if seenAuth != "Bearer real-token" {
		t.Fatalf("auth: %s", seenAuth)
	}

	putJSON(t, console.URL+"/api/saved", `{
		"name":"Create item",
		"method":"POST",
		"path":"/items",
		"headers":{"Authorization":"Bearer secret","X-Debug":"1"},
		"body":"{\"email\":\"{{email}}\"}",
		"auth_mode":"none"
	}`)
	saved := getJSON(t, console.URL+"/api/saved").([]any)
	if len(saved) != 1 {
		t.Fatalf("saved: %#v", saved)
	}
	raw, _ := json.Marshal(saved)
	if strings.Contains(string(raw), "secret") {
		t.Fatalf("secret returned: %s", raw)
	}

	snip := postJSON(t, console.URL+"/api/snippet", `{
		"environment":"dev",
		"method":"GET",
		"path":"/items/{id}",
		"path_params":{"id":"{{itemId}}"},
		"auth_mode":"jwt",
		"jwt":"real-token"
	}`)
	curl := snip["curl"].(string)
	if strings.Contains(curl, "real-token") || !strings.Contains(curl, "$JWT") || !strings.Contains(curl, "/items/7") {
		t.Fatalf("curl: %s", curl)
	}

	putJSON(t, console.URL+"/api/variables", `{"email":"a@b.com"}`)
	run := postJSON(t, console.URL+"/api/run", `{
		"environment":"dev",
		"auth_mode":"none",
		"sequence":{
			"name":"make",
			"steps":[
				{"id":"create","method":"POST","path":"/items","body":"{\"email\":\"{{email}}\"}","auth_mode":"none","expect":"201","capture":{"id":"$.id"}},
				{"id":"get","method":"GET","path":"/items/{id}","path_params":{"id":"{{id}}"},"auth_mode":"jwt","expect":"200"}
			]
		},
		"jwt":"real-token"
	}`)
	if run["ok"] != true {
		t.Fatalf("run: %#v", run)
	}
	if seenPath != "/items/7" || seenAuth != "Bearer real-token" {
		t.Fatalf("after run path=%s auth=%s type=%s", seenPath, seenAuth, seenType)
	}

	again := httptest.NewServer(New(cfg, cat).Handler())
	defer again.Close()
	hist := getJSON(t, again.URL+"/api/history").([]any)
	if len(hist) < 2 {
		t.Fatalf("history did not persist: %#v", hist)
	}
	blob, _ := json.Marshal(hist)
	if strings.Contains(string(blob), "real-token") || strings.Contains(string(blob), "secret") {
		t.Fatalf("token persisted in history: %s", blob)
	}
}

func postJSON(t *testing.T, url, body string) map[string]any {
	t.Helper()
	res, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		t.Fatalf("POST %s: %s %s", url, res.Status, raw)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func putJSON(t *testing.T, url, body string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPut, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		t.Fatalf("PUT %s: %s %s", url, res.Status, raw)
	}
}

func getJSON(t *testing.T, url string) any {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		t.Fatalf("GET %s: %s %s", url, res.Status, raw)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
