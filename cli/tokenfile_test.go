package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadTokenFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("  abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readTokenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "abc" {
		t.Fatalf("got %q", got)
	}

	empty := filepath.Join(dir, "empty")
	if err := os.WriteFile(empty, []byte(" \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readTokenFile(empty); err == nil {
		t.Fatal("expected empty token file to fail")
	}
}
