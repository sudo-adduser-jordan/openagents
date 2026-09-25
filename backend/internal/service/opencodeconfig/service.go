// Package opencodeconfig reads and writes the user's own opencode configuration
// so the desktop app can show and edit it.
//
// This file is opencode's, not Open Agents'. Open Agents never writes it on its
// own initiative -- there is no background sync, and nothing here runs on spawn.
// Every write comes from an explicit user save, because the consequence of
// getting it wrong is that opencode will not start.
package opencodeconfig

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/adapters/agent/hookutil"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/jsonc"
)

// ConfigName is opencode's own config file name. It is JSONC: opencode reads
// comments and trailing commas, and so does this package.
const ConfigName = "opencode.jsonc"

// backupSuffix is appended to the previous contents before an overwrite. A config
// file that silently loses a hand-written permission rule is worse than one that
// was never edited, so every write leaves the prior text recoverable.
const backupSuffix = ".open-agents.bak"

// Document is what the UI reads and writes.
type Document struct {
	// Path is absolute, so the UI can tell the user which file it is editing.
	Path string `json:"path"`
	// Exists is false when the user has no config yet, in which case Content is
	// empty and the editor starts from a documented skeleton rather than "{}".
	Exists bool `json:"exists"`
	// Content is the file's exact bytes. It is never reformatted: comments and
	// the user's layout survive a save untouched.
	Content string `json:"content"`
	// Warning is set when the file exists but does not parse. The content is
	// still returned so the user can see and repair it, rather than being met
	// with an error and no file.
	Warning string `json:"warning,omitempty"`
}

// WriteRequest is the PUT body. The document travels as a string so the daemon
// can hand the user's exact bytes to the file rather than a re-serialized copy.
type WriteRequest struct {
	Content string `json:"content"`
}

// Skeleton is offered when there is no config to edit. It documents the shape
// rather than an empty object, because a blank editor teaches nothing.
const Skeleton = `{
  "$schema": "https://opencode.ai/config.json",
  "permission": {
    "bash": {
      "*": "deny",
      "open-agents *": "allow"
    }
  }
}
`

// Service resolves and edits the config.
type Service struct {
	// home resolves the user's home directory. Injected so tests can point at a
	// scratch tree instead of the real config.
	home func() (string, error)
}

// New builds the service.
func New(home func() (string, error)) *Service {
	if home == nil {
		home = os.UserHomeDir
	}
	return &Service{home: home}
}

// Path returns the absolute path of the user's opencode config.
func (s *Service) Path() (string, error) {
	dir, err := s.home()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(dir, ".config", "opencode", ConfigName), nil
}

// Read returns the current document.
//
// A parse failure is reported through Warning rather than as an error, because
// the user still needs to see the file to fix it. Returning an error here would
// leave the editor empty with no way to recover the text.
func (s *Service) Read(_ context.Context) (Document, error) {
	path, err := s.Path()
	if err != nil {
		return Document{}, err
	}
	doc := Document{Path: path}

	data, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		doc.Content = Skeleton
		return doc, nil
	case err != nil:
		return Document{}, fmt.Errorf("read %s: %w", path, err)
	}

	doc.Exists = true
	doc.Content = string(data)
	if err := jsonc.Validate(doc.Content); err != nil {
		doc.Warning = fmt.Sprintf("This file is not valid JSONC and will not load: %v", err)
	}
	return doc, nil
}

// Write replaces the config with the user's text, verbatim.
//
// The text is validated first and rejected rather than written when it does not
// parse, so a typo cannot leave the user with a config opencode refuses to read.
// The prior contents are kept alongside, because a validated-but-wrong file is
// still a file the user cannot undo from here.
func (s *Service) Write(_ context.Context, content string) (Document, error) {
	path, err := s.Path()
	if err != nil {
		return Document{}, err
	}
	if err := jsonc.Validate(content); err != nil {
		return Document{}, fmt.Errorf("%s is not valid JSONC: %w", ConfigName, err)
	}
	// Normalize only the final newline, which editors add and which is invisible
	// either way. Everything else is the user's.
	body := content
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}

	if previous, readErr := os.ReadFile(path); readErr == nil {
		if err := hookutil.AtomicWriteFile(path+backupSuffix, previous, 0o600); err != nil {
			return Document{}, fmt.Errorf("back up %s: %w", ConfigName, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return Document{}, fmt.Errorf("create opencode config directory: %w", err)
	}
	if err := hookutil.AtomicWriteFile(path, []byte(body), 0o600); err != nil {
		return Document{}, fmt.Errorf("write %s: %w", ConfigName, err)
	}
	return Document{Path: path, Exists: true, Content: body}, nil
}
