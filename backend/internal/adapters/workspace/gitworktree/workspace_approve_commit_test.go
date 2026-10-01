package gitworktree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

// TestWorkspaceIntegrationCommitUncommitted is the approve-path counterpart to
// the preserve round-trip: unstaged working-tree changes become a real commit
// so a follow-up Destroy sees a clean tree instead of refusing with
// ErrWorkspaceDirty. A clean worktree reports committed=false, and unresolved
// merge conflicts refuse with ErrWorkspaceDirty (callers fall back to
// dirty-preserve, never force-delete).
func TestWorkspaceIntegrationCommitUncommitted(t *testing.T) {
	git := requireGit(t)
	tmp := t.TempDir()
	repo := setupOriginClone(t, git, tmp)
	root := filepath.Join(tmp, "managed")
	ws, err := New(Options{Binary: git, ManagedRoot: root, RepoResolver: StaticRepoResolver{"proj": repo}})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()
	cfg := ports.WorkspaceConfig{ProjectID: "proj", SessionID: "sess-approve", Branch: "feature/approve"}

	info, err := ws.Create(ctx, cfg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Clean worktree: nothing to commit.
	if committed, err := ws.CommitUncommitted(ctx, info); err != nil || committed {
		t.Fatalf("clean CommitUncommitted = (%v, %v), want (false, nil)", committed, err)
	}

	// Dirty worktree: tracked edit plus a new untracked file.
	if err := os.WriteFile(filepath.Join(info.Path, "README.md"), []byte("approved work\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	if err := os.WriteFile(filepath.Join(info.Path, "new-file.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write new file: %v", err)
	}
	committed, err := ws.CommitUncommitted(ctx, info)
	if err != nil {
		t.Fatalf("CommitUncommitted: %v", err)
	}
	if !committed {
		t.Fatal("CommitUncommitted = false for dirty worktree, want true")
	}
	// The tree must now be clean: Destroy proceeds instead of refusing.
	if _, err := ws.DestroyReclaim(ctx, ports.WorkspaceInfo{Path: info.Path, SessionID: info.SessionID, ProjectID: info.ProjectID, RepoPath: repo}); err != nil {
		t.Fatalf("DestroyReclaim after approve commit: %v", err)
	}
	if _, err := os.Stat(info.Path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("worktree path still exists after DestroyReclaim")
	}
}

// TestWorkspaceIntegrationCommitUncommittedConflictRefuses leaves conflict
// markers in the tree and asserts the approve auto-commit refuses with
// ErrWorkspaceDirty rather than committing a conflicted state.
func TestWorkspaceIntegrationCommitUncommittedConflictRefuses(t *testing.T) {
	git := requireGit(t)
	tmp := t.TempDir()
	repo := setupOriginClone(t, git, tmp)
	root := filepath.Join(tmp, "managed")
	ws, err := New(Options{Binary: git, ManagedRoot: root, RepoResolver: StaticRepoResolver{"proj": repo}})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ctx := context.Background()
	cfg := ports.WorkspaceConfig{ProjectID: "proj", SessionID: "sess-conflict", Branch: "feature/conflict"}

	info, err := ws.Create(ctx, cfg)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Engineer an unresolved merge conflict: commit one side, then merge a
	// conflicting change from a second worktree.
	if err := os.WriteFile(filepath.Join(info.Path, "README.md"), []byte("ours\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, git, info.Path, "commit", "-am", "ours")
	other := filepath.Join(tmp, "other")
	runGit(t, git, repo, "worktree", "add", "--detach", other)
	t.Cleanup(func() { runGit(t, git, repo, "worktree", "remove", "--force", other) })
	runGit(t, git, other, "checkout", "--detach", "HEAD")
	if err := os.WriteFile(filepath.Join(other, "README.md"), []byte("theirs\n"), 0o644); err != nil {
		t.Fatalf("write other README: %v", err)
	}
	runGit(t, git, other, "commit", "-am", "theirs")
	theirsOut, err := exec.Command(git, "-C", other, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		t.Fatalf("rev-parse: %v\n%s", err, theirsOut)
	}
	// The merge is expected to conflict (non-zero exit); that is the state
	// under test, not a setup failure.
	if out, err := exec.Command(git, "-C", info.Path, "merge", "--no-edit", strings.TrimSpace(string(theirsOut))).CombinedOutput(); err == nil {
		t.Fatalf("merge unexpectedly succeeded:\n%s", out)
	}

	if _, err := ws.CommitUncommitted(ctx, info); !errors.Is(err, ports.ErrWorkspaceDirty) {
		t.Fatalf("CommitUncommitted err = %v, want ErrWorkspaceDirty", err)
	}
	runGit(t, git, info.Path, "merge", "--abort")
}
