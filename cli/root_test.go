package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestHelpListsActions(t *testing.T) {
	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"ui", "jwt", "ls", "call", "run", "Notes:", "Examples:", "OpenAPI spec"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q:\n%s", want, out)
		}
	}
}

func TestCallHelpMentionsBaseURL(t *testing.T) {
	cmd := newRootCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"call", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"--base-url", "hosted"} {
		if !strings.Contains(out, want) {
			t.Fatalf("call help missing %q:\n%s", want, out)
		}
	}
}

func TestActionHelpHasNotes(t *testing.T) {
	for _, action := range []string{"ui", "jwt", "ls", "call", "run"} {
		cmd := newRootCmd()
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{action, "--help"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("%s help: %v", action, err)
		}
		out := buf.String()
		if !strings.Contains(out, "Notes:") || !strings.Contains(out, "Examples:") {
			t.Fatalf("%s help missing notes/examples:\n%s", action, out)
		}
	}
}
