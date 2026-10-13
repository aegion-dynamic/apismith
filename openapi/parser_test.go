package openapi

import (
	"path/filepath"
	"testing"
)

func TestLoadFixture(t *testing.T) {
	cat, err := Load(filepath.Join("testdata", "fixture.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cat.Title != "Console Fixture" {
		t.Fatalf("title: %q", cat.Title)
	}
	if len(cat.Endpoints) != 4 {
		t.Fatalf("expected 4 endpoints, got %d", len(cat.Endpoints))
	}

	var create, login, getByID *Endpoint
	for i := range cat.Endpoints {
		ep := &cat.Endpoints[i]
		switch ep.ID {
		case "POST /users":
			create = ep
		case "POST /auth/login":
			login = ep
		case "GET /users/{id}":
			getByID = ep
		}
	}
	if create == nil || login == nil || getByID == nil {
		t.Fatalf("missing endpoints: create=%v login=%v get=%v", create, login, getByID)
	}
	if !create.AuthRequired {
		t.Fatal("POST /users should require auth")
	}
	if login.AuthRequired {
		t.Fatal("POST /auth/login should be public")
	}
	if create.RequestBody == nil || create.RequestBody.ExampleJSON == "" {
		t.Fatal("expected generated request body example")
	}
	schema := cat.ResponseSchema("POST", "/users", 201)
	if schema == nil || schema.Type != "object" {
		t.Fatalf("response schema: %+v", schema)
	}
	if getByID.Parameters[0].Name != "id" || getByID.Parameters[0].In != "path" {
		t.Fatalf("path param: %+v", getByID.Parameters)
	}
}
