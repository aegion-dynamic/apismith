package openapi

import "testing"

func TestMatchBody(t *testing.T) {
	schema := &SchemaView{
		Type:     "object",
		Required: []string{"id"},
		Properties: map[string]*SchemaView{
			"id":   {Type: "string"},
			"n":    {Type: "integer"},
			"tags": {Type: "array", Items: &SchemaView{Type: "string"}},
		},
	}
	ok, notes := MatchBody(`{"id":"a","n":1,"tags":["x"],"extra":true}`, schema)
	if !ok || len(notes) != 0 {
		t.Fatalf("ok=%v notes=%v", ok, notes)
	}
	ok, notes = MatchBody(`{"n":1}`, schema)
	if ok || len(notes) == 0 {
		t.Fatal("expected missing id")
	}
	ok, notes = MatchBody(`{"id":"a","n":1.5}`, schema)
	if ok {
		t.Fatalf("expected integer mismatch: %v", notes)
	}
	ok, notes = MatchBody(`not-json`, schema)
	if ok || notes[0] != "response is not JSON" {
		t.Fatalf("notes: %v", notes)
	}
}
