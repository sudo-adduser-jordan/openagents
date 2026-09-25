package jsonc

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStripRemovesCommentsAndTrailingCommas(t *testing.T) {
	// Shaped like a real opencode config: a $schema, nested permission rules,
	// both comment styles, and a trailing comma on the last rule.
	input := `{
  // which schema
  "$schema": "https://opencode.ai/config.json",
  /* block
     comment */
  "permission": {
    "bash": {
      "sudo *": "deny",   // inline
      "rm *": "ask",
    },
  },
}`
	// The contract is that it parses as JSON with the comments gone, not that
	// the bytes match modulo comments: a line comment leaves its indentation and
	// its newline behind so error line numbers stay right.
	var decoded struct {
		Schema    string `json:"$schema"`
		Permission struct {
			Bash map[string]string `json:"bash"`
		} `json:"permission"`
	}
	if err := Decode(input, &decoded); err != nil {
		t.Fatalf("Decode: %v\nstripped: %s", err, Strip(input))
	}
	if decoded.Schema != "https://opencode.ai/config.json" {
		t.Fatalf("schema = %q", decoded.Schema)
	}
	want := map[string]string{"sudo *": "deny", "rm *": "ask"}
	if !reflect.DeepEqual(decoded.Permission.Bash, want) {
		t.Fatalf("bash = %#v, want %#v", decoded.Permission.Bash, want)
	}
}

// The whole point of treating comments as string-safe: a URL contains "//" and a
// path can contain a comma. Both must survive untouched.
func TestStripLeavesCommentLikeTextInsideStrings(t *testing.T) {
	input := `{"url": "https://example.com/a//b", "note": "a, b, c", "glob": "*//*"}`
	if got := Strip(input); got != input {
		t.Fatalf("Strip rewrote string contents\n got: %q\nwant: %q", got, input)
	}
	var decoded map[string]string
	if err := Decode(input, &decoded); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"url": "https://example.com/a//b", "note": "a, b, c", "glob": "*//*",
	}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("decoded = %#v, want %#v", decoded, want)
	}
}

func TestStripHandlesEscapedQuote(t *testing.T) {
	input := `{"a": "he said \"hi // not a comment", "b": 1}`
	var decoded map[string]any
	if err := Decode(input, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["a"] != `he said "hi // not a comment` {
		t.Fatalf("a = %v", decoded["a"])
	}
}

func TestValidateRejectsBrokenDocuments(t *testing.T) {
	for name, input := range map[string]string{
		"unbalanced brace":       `{"a": 1`,
		"unterminated string":    `{"a": "never closed`,
		"unterminated comment":   `{"a": 1} /* trailing`,
		"not an object":          `[1, 2, 3] trailing garbage`,
		"bad token after object": `{"a": 1} oops`,
	} {
		if err := Validate(input); err == nil {
			t.Fatalf("%s: Validate accepted %q", name, input)
		}
	}
}

func TestValidateAcceptsWellFormedJSONC(t *testing.T) {
	for name, input := range map[string]string{
		"plain json":       `{"a": 1}`,
		"line comments":    "{\n// hi\n\"a\": 1\n}",
		"block comments":   `{/* hi */ "a": 1}`,
		"trailing commas":  `{"a": [1, 2, 3,],}`,
		"empty":            `{}`,
		"only whitespace":  "  \n\t ",
		"empty containers": `{"a": {}, "b": []}`,
	} {
		if err := Validate(input); err != nil {
			t.Fatalf("%s: Validate rejected %q: %v", name, input, err)
		}
	}
}

// The machine this was written on has a real opencode config with a permission
// block. If the stripper cannot read that, the feature is broken where it
// matters, so keep a copy of the shape as a fixture rather than a sketch.
func TestValidateReadsARealisticOpencodeConfig(t *testing.T) {
	input := `{
  "$schema": "https://opencode.ai/config.json",
  "permission": {
    "bash": {
      "sudo *": "deny",
      "sudo docker *": "ask",
      "rm *": "ask",
      "gh *": "ask",
      "git push *": "deny",
    }
  }
}`
	if err := Validate(input); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	var decoded struct {
		Permission map[string]map[string]string `json:"permission"`
	}
	if err := Decode(input, &decoded); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"sudo *": "deny", "sudo docker *": "ask", "rm *": "ask",
		"gh *": "ask", "git push *": "deny",
	}
	if !reflect.DeepEqual(decoded.Permission["bash"], want) {
		t.Fatalf("bash rules = %#v, want %#v", decoded.Permission["bash"], want)
	}
}

// If a config is readable on this machine, the stripper has to handle it.
func TestValidateReadsTheDevelopersOwnConfigWhenPresent(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	path := filepath.Join(home, ".config", "opencode", "opencode.jsonc")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("no opencode config to check")
	}
	if err := Validate(string(data)); err != nil {
		t.Fatalf("the local opencode config does not validate: %v", err)
	}
}
