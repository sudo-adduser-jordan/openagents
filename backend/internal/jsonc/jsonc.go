// Package jsonc handles the JSON-with-comments-and-trailing-commas dialect that
// opencode's own config file uses.
//
// The only thing this package is used for is *validation*: a file has to be
// understood well enough to reject a broken one before it is written back. It is
// never used to produce the file. Open Agents writes the user's bytes verbatim,
// because re-serializing a hand-edited config silently deletes every comment in
// it, and a config file whose comments vanish is a config file nobody trusts.
package jsonc

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

// Strip returns text with comments removed and trailing commas dropped, so that
// encoding/json can parse it. The result is for inspection only.
//
// Newlines are preserved so a reported error still points at the right line, and
// whitespace where a comment stood is left behind; the contract is "parses as
// JSON", not "byte-identical apart from comments".
//
// Comments and commas are removed everywhere except inside a JSON string, where
// both are ordinary characters: a URL like "https://example.com" is not a
// comment, and a comma inside a string is not a trailing one.
func Strip(text string) string {
	var out strings.Builder
	out.Grow(len(text))

	runes := []rune(text)
	for i := 0; i < len(runes); {
		switch c := runes[i]; {
		case c == '"':
			// Copy the string literal whole. Escapes are consumed so that an
			// escaped quote does not look like the end of the literal.
			out.WriteRune(c)
			i++
			for i < len(runes) {
				if runes[i] == '\\' && i+1 < len(runes) {
					out.WriteRune(runes[i])
					out.WriteRune(runes[i+1])
					i += 2
					continue
				}
				out.WriteRune(runes[i])
				if runes[i] == '"' {
					i++
					break
				}
				i++
			}
		case c == '/' && i+1 < len(runes) && runes[i+1] == '/':
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(runes) && runes[i+1] == '*':
			i += 2
			for i < len(runes) {
				if runes[i] == '*' && i+1 < len(runes) && runes[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
		default:
			out.WriteRune(c)
			i++
		}
	}
	return dropTrailingCommas(out.String())
}

// dropTrailingCommas removes a comma that is followed only by whitespace before
// the next closing brace or bracket.
func dropTrailingCommas(text string) string {
	runes := []rune(text)
	var out strings.Builder
	out.Grow(len(text))

	for i := 0; i < len(runes); i++ {
		if runes[i] != ',' {
			out.WriteRune(runes[i])
			continue
		}
		// Look ahead past whitespace for a closer; a comma followed by one is
		// trailing and legal in JSONC but not in JSON.
		j := i + 1
		for j < len(runes) && unicode.IsSpace(runes[j]) {
			j++
		}
		if j < len(runes) && (runes[j] == '}' || runes[j] == ']') {
			continue
		}
		out.WriteRune(',')
	}
	return out.String()
}

// Validate reports whether text is a well-formed JSONC document.
//
// An unterminated block comment or string is reported with the offset where it
// opened, because "invalid JSON" tells a user nothing about a file they were
// mid-way through typing.
func Validate(text string) error {
	runes := []rune(text)
	for i := 0; i < len(runes); {
		switch c := runes[i]; {
		case c == '"':
			closed := false
			i++
			for i < len(runes) {
				if runes[i] == '\\' && i+1 < len(runes) {
					i += 2
					continue
				}
				if runes[i] == '"' {
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return fmt.Errorf("unterminated string starting at byte %d", i)
			}
		case c == '/' && i+1 < len(runes) && runes[i+1] == '*':
			open := i
			i += 2
			closed := false
			for i < len(runes) {
				if runes[i] == '*' && i+1 < len(runes) && runes[i+1] == '/' {
					i += 2
					closed = true
					break
				}
				i++
			}
			if !closed {
				return fmt.Errorf("unterminated block comment starting at byte %d", open)
			}
		default:
			i++
		}
	}

	var probe any
	stripped := Strip(text)
	// An absent or blank config is a valid config: opencode treats it as "no
	// overrides", and refusing to save one would make the editor unusable until
	// the user typed something.
	if strings.TrimSpace(stripped) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(stripped), &probe); err != nil {
		return err
	}
	return nil
}

// Decode parses text into a value, for reading a document that is known to be
// valid. Comments and trailing commas are not preserved: use this to inspect,
// never to write.
func Decode(text string, into any) error {
	if err := json.Unmarshal([]byte(Strip(text)), into); err != nil {
		return err
	}
	return nil
}
