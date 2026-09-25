package opencodeconfig_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/service/opencodeconfig"
)

func serviceFor(t *testing.T) (*opencodeconfig.Service, string) {
	t.Helper()
	root := t.TempDir()
	return opencodeconfig.New(func() (string, error) { return root, nil }), root
}

func writeConfig(t *testing.T, root, content string) string {
	t.Helper()
	dir := filepath.Join(root, ".config", "opencode")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, opencodeconfig.ConfigName)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadOffersASkeletonWhenThereIsNoConfig(t *testing.T) {
	s, _ := serviceFor(t)
	doc, err := s.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Exists {
		t.Fatal("Exists = true for a missing file")
	}
	if !strings.Contains(doc.Content, "permission") {
		t.Fatalf("skeleton does not document the shape: %q", doc.Content)
	}
	if _, err := os.Stat(doc.Path); !os.IsNotExist(err) {
		t.Fatalf("reading created the file: %v", err)
	}
}

// A file with comments and trailing commas must come back byte-identical. Any
// reformatting here would silently delete the user's own notes.
func TestReadReturnsTheFileVerbatim(t *testing.T) {
	s, root := serviceFor(t)
	original := `{
  // my own note
  "$schema": "https://opencode.ai/config.json",
  "permission": {
    "bash": {
      "git push *": "deny",
    }
  }
}`
	path := writeConfig(t, root, original)

	doc, err := s.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Content != original {
		t.Fatalf("content was reformatted\n got: %q\nwant: %q", doc.Content, original)
	}
	if doc.Warning != "" {
		t.Fatalf("a valid JSONC file warned: %s", doc.Warning)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil || string(onDisk) != original {
		t.Fatalf("read mutated the file: %q %v", onDisk, err)
	}
}

// A broken file must still be readable, or the user cannot see what to fix.
func TestReadWarnsButStillReturnsBrokenContent(t *testing.T) {
	s, root := serviceFor(t)
	broken := `{"permission": {`
	writeConfig(t, root, broken)

	doc, err := s.Read(context.Background())
	if err != nil {
		t.Fatalf("Read errored instead of warning: %v", err)
	}
	if doc.Content != broken {
		t.Fatalf("content = %q, want the broken text so it can be repaired", doc.Content)
	}
	if doc.Warning == "" {
		t.Fatal("a broken file produced no warning")
	}
}

func TestWritePersistsCommentsVerbatim(t *testing.T) {
	s, _ := serviceFor(t)
	content := `{
  // keep me
  "permission": {
    "bash": {
      "rm *": "ask",
    }
  }
}`

	doc, err := s.Write(context.Background(), content)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Warning != "" {
		t.Fatalf("saved a file it then called invalid: %s", doc.Warning)
	}

	reread, err := s.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if reread.Content != content+"\n" {
		t.Fatalf("round trip changed the text\n got: %q\nwant: %q", reread.Content, content+"\n")
	}
	if !strings.Contains(reread.Content, "// keep me") {
		t.Fatal("the comment did not survive the save")
	}
}

func TestWriteRejectsInvalidJSONCWithoutTouchingTheFile(t *testing.T) {
	s, root := serviceFor(t)
	original := `{"permission": {}}`
	path := writeConfig(t, root, original)

	if _, err := s.Write(context.Background(), `{"permission": {`); err == nil {
		t.Fatal("Write accepted a broken document")
	}
	onDisk, err := os.ReadFile(path)
	if err != nil || string(onDisk) != original {
		t.Fatalf("a rejected write changed the file: %q %v", onDisk, err)
	}
}

func TestWriteBacksUpThePreviousContents(t *testing.T) {
	s, root := serviceFor(t)
	original := `{"permission": {"bash": {"git push *": "deny"}}}`
	path := writeConfig(t, root, original)

	if _, err := s.Write(context.Background(), `{"permission": {"bash": {"rm *": "ask"}}}`); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + ".open-agents.bak")
	if err != nil {
		t.Fatalf("no backup was kept: %v", err)
	}
	if string(backup) != original {
		t.Fatalf("backup = %q, want the prior contents", backup)
	}
}

func TestWriteAcceptsAnEmptyConfig(t *testing.T) {
	s, _ := serviceFor(t)
	if _, err := s.Write(context.Background(), ""); err != nil {
		t.Fatalf("an empty config was rejected: %v", err)
	}
	doc, err := s.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(doc.Content) != "" {
		t.Fatalf("content = %q, want empty", doc.Content)
	}
}

func TestPathPointsAtTheUsersOpencodeConfig(t *testing.T) {
	s, root := serviceFor(t)
	path, err := s.Path()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, ".config", "opencode", "opencode.jsonc")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}
