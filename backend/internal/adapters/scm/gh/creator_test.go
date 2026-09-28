package gh

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

// stubGH installs a fake gh executable whose behavior is driven by
// OPEN_AGENTS_TEST_GH_MODE. It lets the adapter tests exercise auth failure,
// duplicate detection, and creation without network access.
func stubGH(t *testing.T, mode string) *Creator {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/sh
mode="$OPEN_AGENTS_TEST_GH_MODE"
if [ "$1" = "pr" ] && [ "$2" = "list" ]; then
  case "$mode" in
    list-found) echo '[{"url":"https://github.com/acme/repo/pull/7","number":7}]' ;;
    list-empty) echo '[]' ;;
    auth-failure) echo 'To get started with GitHub, please run:  gh auth login' >&2; exit 1 ;;
    *) echo "unexpected list mode: $mode" >&2; exit 2 ;;
  esac
  exit 0
fi
if [ "$1" = "pr" ] && [ "$2" = "create" ]; then
  case "$mode" in
    create-ok) echo 'https://github.com/acme/repo/pull/8' ;;
    create-exists) echo 'A pull request already exists for open-agents/sess-1: https://github.com/acme/repo/pull/7' >&2; exit 1 ;;
    auth-failure) echo 'gh: Not authenticated' >&2; exit 1 ;;
    *) echo "unexpected create mode: $mode" >&2; exit 2 ;;
  esac
  exit 0
fi
echo "unexpected args: $*" >&2
exit 2
`
	path := filepath.Join(dir, "gh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub gh: %v", err)
	}
	t.Setenv("OPEN_AGENTS_TEST_GH_MODE", mode)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return New(Options{Binary: "gh"})
}

func TestFindPRByHead_Found(t *testing.T) {
	c := stubGH(t, "list-found")
	url, number, found, err := c.FindPRByHead(context.Background(), t.TempDir(), "open-agents/sess-1")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if !found || url != "https://github.com/acme/repo/pull/7" || number != 7 {
		t.Errorf("got (%q, %d, %v), want (url, 7, true)", url, number, found)
	}
}

func TestFindPRByHead_Empty(t *testing.T) {
	c := stubGH(t, "list-empty")
	_, _, found, err := c.FindPRByHead(context.Background(), t.TempDir(), "open-agents/sess-1")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if found {
		t.Errorf("found = true, want false")
	}
}

func TestFindPRByHead_AuthFailure(t *testing.T) {
	c := stubGH(t, "auth-failure")
	_, _, _, err := c.FindPRByHead(context.Background(), t.TempDir(), "open-agents/sess-1")
	if !errors.Is(err, ports.ErrGHAuthMissing) {
		t.Fatalf("err = %v, want ErrGHAuthMissing", err)
	}
}

func TestFindPRByHead_MissingBinary(t *testing.T) {
	c := New(Options{Binary: "gh-definitely-not-installed"})
	_, _, _, err := c.FindPRByHead(context.Background(), t.TempDir(), "open-agents/sess-1")
	if !errors.Is(err, ports.ErrGHNotInstalled) {
		t.Fatalf("err = %v, want ErrGHNotInstalled", err)
	}
}

func TestCreatePR_Success(t *testing.T) {
	c := stubGH(t, "create-ok")
	got, err := c.CreatePR(context.Background(), t.TempDir(), "dev", "open-agents/sess-1", "title", "body")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !got.Created || got.URL != "https://github.com/acme/repo/pull/8" {
		t.Errorf("got %+v, want created PR 8", got)
	}
}

func TestCreatePR_AlreadyExists(t *testing.T) {
	c := stubGH(t, "create-exists")
	_, err := c.CreatePR(context.Background(), t.TempDir(), "dev", "open-agents/sess-1", "title", "body")
	if !errors.Is(err, ports.ErrGHPullRequestExists) {
		t.Fatalf("err = %v, want ErrGHPullRequestExists", err)
	}
}

func TestCreatePR_AuthFailure(t *testing.T) {
	c := stubGH(t, "auth-failure")
	_, err := c.CreatePR(context.Background(), t.TempDir(), "dev", "open-agents/sess-1", "title", "body")
	if !errors.Is(err, ports.ErrGHAuthMissing) {
		t.Fatalf("err = %v, want ErrGHAuthMissing", err)
	}
}
