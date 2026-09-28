package gitworktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

// setupDeliveryRepo seeds an origin + project checkout on the dev branch with
// user identity configured, mirroring setupOriginClone but on dev.
func setupDeliveryRepo(t *testing.T, git, tmp string) (origin, repo string) {
	t.Helper()
	origin = filepath.Join(tmp, "origin.git")
	seed := filepath.Join(tmp, "seed")
	repo = filepath.Join(tmp, "repo")
	run(t, git, "init", "--bare", origin)
	run(t, git, "init", seed)
	runGit(t, git, seed, "config", "user.email", "open-agents@example.com")
	runGit(t, git, seed, "config", "user.name", "Open Agents")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	runGit(t, git, seed, "add", "README.md")
	runGit(t, git, seed, "commit", "-m", "seed")
	runGit(t, git, seed, "branch", "-M", "dev")
	runGit(t, git, seed, "remote", "add", "origin", origin)
	runGit(t, git, seed, "push", "-u", "origin", "dev")
	runGit(t, git, origin, "symbolic-ref", "HEAD", "refs/heads/dev")
	run(t, git, "clone", origin, repo)
	runGit(t, git, repo, "config", "user.email", "open-agents@example.com")
	runGit(t, git, repo, "config", "user.name", "Open Agents")
	runGit(t, git, repo, "checkout", "dev")
	return origin, repo
}

// addSessionWorktree creates a session branch with one committed file and
// registers a linked worktree for it, mirroring a live session workspace.
func addSessionWorktree(t *testing.T, git, repo, branch, filename, content string) (worktreePath, tip string) {
	t.Helper()
	worktreePath = filepath.Join(filepath.Dir(repo), "wt-"+strings.ReplaceAll(branch, "/", "-"))
	runGit(t, git, repo, "worktree", "add", "-b", branch, worktreePath, "dev")
	if err := os.WriteFile(filepath.Join(worktreePath, filename), []byte(content), 0o644); err != nil {
		t.Fatalf("write session file: %v", err)
	}
	runGit(t, git, worktreePath, "add", filename)
	runGit(t, git, worktreePath, "commit", "-m", "session work")
	tip = gitOutput(t, git, worktreePath, "rev-parse", "HEAD")
	return worktreePath, tip
}

func newDeliveryWorkspace(t *testing.T, git, repo, managed string) *Workspace {
	t.Helper()
	ws, err := New(Options{Binary: git, ManagedRoot: managed, RepoResolver: StaticRepoResolver{"proj": repo}})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	return ws
}

func TestMergeSessionBranchLocal_Success(t *testing.T) {
	git := requireGit(t)
	tmp := t.TempDir()
	_, repo := setupDeliveryRepo(t, git, tmp)
	_, tip := addSessionWorktree(t, git, repo, "open-agents/sess-1", "SESSION.md", "session work\n")
	ws := newDeliveryWorkspace(t, git, repo, filepath.Join(tmp, "managed"))

	res, err := ws.MergeSessionBranchLocal(context.Background(), "proj", "open-agents/sess-1", "dev")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if res.TargetBranch != "dev" {
		t.Errorf("TargetBranch = %q, want dev", res.TargetBranch)
	}
	if res.AlreadyMerged {
		t.Errorf("AlreadyMerged = true, want false for a new branch")
	}
	if !res.BranchRemoved {
		t.Errorf("BranchRemoved = false, want true")
	}
	devHead := gitOutput(t, git, repo, "rev-parse", "dev")
	if res.TargetHeadSHA != devHead {
		t.Errorf("TargetHeadSHA = %q, want dev HEAD %q", res.TargetHeadSHA, devHead)
	}
	// The session tip must now be reachable from dev.
	runGit(t, git, repo, "merge-base", "--is-ancestor", tip, "dev")
	// The branch ref is gone ...
	exists, err := ws.refExists(context.Background(), repo, "refs/heads/open-agents/sess-1")
	if err != nil {
		t.Fatalf("ref check: %v", err)
	}
	if exists {
		t.Errorf("session branch ref still exists after merge")
	}
	// ... and the session worktree is detached so nothing checks it out.
	records, err := ws.listRecords(context.Background(), repo)
	if err != nil {
		t.Fatalf("worktree list: %v", err)
	}
	for _, rec := range records {
		if rec.Branch == "open-agents/sess-1" && !rec.Detached {
			t.Errorf("worktree %q still checks out the removed branch", rec.Path)
		}
	}
	if content, err := os.ReadFile(filepath.Join(repo, "SESSION.md")); err != nil || string(content) != "session work\n" {
		t.Errorf("merged file content = %q, err = %v", content, err)
	}
}

func TestMergeSessionBranchLocal_DirtyCheckout(t *testing.T) {
	git := requireGit(t)
	tmp := t.TempDir()
	_, repo := setupDeliveryRepo(t, git, tmp)
	addSessionWorktree(t, git, repo, "open-agents/sess-1", "SESSION.md", "session work\n")
	devBefore := gitOutput(t, git, repo, "rev-parse", "dev")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatalf("dirty the checkout: %v", err)
	}
	ws := newDeliveryWorkspace(t, git, repo, filepath.Join(tmp, "managed"))

	_, err := ws.MergeSessionBranchLocal(context.Background(), "proj", "open-agents/sess-1", "dev")
	if !errors.Is(err, ports.ErrWorkspaceDirty) {
		t.Fatalf("err = %v, want ErrWorkspaceDirty", err)
	}
	if devAfter := gitOutput(t, git, repo, "rev-parse", "dev"); devAfter != devBefore {
		t.Errorf("dev moved from %q to %q on a refused merge", devBefore, devAfter)
	}
	exists, refErr := ws.refExists(context.Background(), repo, "refs/heads/open-agents/sess-1")
	if refErr != nil || !exists {
		t.Errorf("session branch missing after refused merge (exists=%v, err=%v)", exists, refErr)
	}
}

func TestMergeSessionBranchLocal_NotOnTarget(t *testing.T) {
	git := requireGit(t)
	tmp := t.TempDir()
	_, repo := setupDeliveryRepo(t, git, tmp)
	addSessionWorktree(t, git, repo, "open-agents/sess-1", "SESSION.md", "session work\n")
	runGit(t, git, repo, "branch", "main", "dev")
	runGit(t, git, repo, "checkout", "main")
	ws := newDeliveryWorkspace(t, git, repo, filepath.Join(tmp, "managed"))

	_, err := ws.MergeSessionBranchLocal(context.Background(), "proj", "open-agents/sess-1", "dev")
	if !errors.Is(err, ports.ErrDeliveryNotOnTargetBranch) {
		t.Fatalf("err = %v, want ErrDeliveryNotOnTargetBranch", err)
	}
}

func TestMergeSessionBranchLocal_Conflict(t *testing.T) {
	git := requireGit(t)
	tmp := t.TempDir()
	_, repo := setupDeliveryRepo(t, git, tmp)
	worktreePath, _ := addSessionWorktree(t, git, repo, "open-agents/sess-1", "SHARED.md", "session side\n")
	// Advance dev on the same path with different content: a true conflict.
	if err := os.WriteFile(filepath.Join(repo, "SHARED.md"), []byte("dev side\n"), 0o644); err != nil {
		t.Fatalf("write dev file: %v", err)
	}
	runGit(t, git, repo, "add", "SHARED.md")
	runGit(t, git, repo, "commit", "-m", "dev side")
	if err := os.WriteFile(filepath.Join(worktreePath, "SHARED.md"), []byte("session side v2\n"), 0o644); err != nil {
		t.Fatalf("write session file: %v", err)
	}
	runGit(t, git, worktreePath, "add", "SHARED.md")
	runGit(t, git, worktreePath, "commit", "-m", "session side v2")
	devBefore := gitOutput(t, git, repo, "rev-parse", "dev")
	ws := newDeliveryWorkspace(t, git, repo, filepath.Join(tmp, "managed"))

	_, err := ws.MergeSessionBranchLocal(context.Background(), "proj", "open-agents/sess-1", "dev")
	if !errors.Is(err, ports.ErrDeliveryMergeConflict) {
		t.Fatalf("err = %v, want ErrDeliveryMergeConflict", err)
	}
	// Recoverable: MERGE_HEAD is intact for `git merge --abort`.
	if _, statErr := os.Stat(gitDirPath(t, git, repo, "MERGE_HEAD")); statErr != nil {
		t.Errorf("MERGE_HEAD missing after conflicted merge: %v", statErr)
	}
	if devAfter := gitOutput(t, git, repo, "rev-parse", "dev"); devAfter != devBefore {
		t.Errorf("dev moved from %q to %q on a conflicted merge", devBefore, devAfter)
	}
	exists, refErr := ws.refExists(context.Background(), repo, "refs/heads/open-agents/sess-1")
	if refErr != nil || !exists {
		t.Errorf("session branch missing after conflicted merge (exists=%v, err=%v)", exists, refErr)
	}
	conflicted := gitOutput(t, git, repo, "diff", "--name-only", "--diff-filter=U")
	if !strings.Contains(conflicted, "SHARED.md") {
		t.Errorf("conflicted files = %q, want SHARED.md", conflicted)
	}
	runGit(t, git, repo, "merge", "--abort")
}

func TestMergeSessionBranchLocal_AlreadyMerged(t *testing.T) {
	git := requireGit(t)
	tmp := t.TempDir()
	_, repo := setupDeliveryRepo(t, git, tmp)
	worktreePath, _ := addSessionWorktree(t, git, repo, "open-agents/sess-1", "SESSION.md", "session work\n")
	// Merge by hand first: the button must treat this as verified success.
	runGit(t, git, repo, "merge", "--no-edit", "open-agents/sess-1")
	ws := newDeliveryWorkspace(t, git, repo, filepath.Join(tmp, "managed"))

	res, err := ws.MergeSessionBranchLocal(context.Background(), "proj", "open-agents/sess-1", "dev")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !res.AlreadyMerged {
		t.Errorf("AlreadyMerged = false, want true")
	}
	if !res.BranchRemoved {
		t.Errorf("BranchRemoved = false, want true")
	}
	exists, refErr := ws.refExists(context.Background(), repo, "refs/heads/open-agents/sess-1")
	if refErr != nil || exists {
		t.Errorf("session branch still exists after already-merged path (exists=%v, err=%v)", exists, refErr)
	}
	_ = worktreePath
}

func TestMergeSessionBranchLocal_UnknownBranch(t *testing.T) {
	git := requireGit(t)
	tmp := t.TempDir()
	_, repo := setupDeliveryRepo(t, git, tmp)
	ws := newDeliveryWorkspace(t, git, repo, filepath.Join(tmp, "managed"))

	_, err := ws.MergeSessionBranchLocal(context.Background(), "proj", "open-agents/nope", "dev")
	if !errors.Is(err, ports.ErrDeliveryBranchNotFound) {
		t.Fatalf("err = %v, want ErrDeliveryBranchNotFound", err)
	}
}

func TestPushSessionBranch_Success(t *testing.T) {
	git := requireGit(t)
	tmp := t.TempDir()
	origin, repo := setupDeliveryRepo(t, git, tmp)
	worktreePath, tip := addSessionWorktree(t, git, repo, "open-agents/sess-1", "SESSION.md", "session work\n")
	ws := newDeliveryWorkspace(t, git, repo, filepath.Join(tmp, "managed"))

	if err := ws.PushSessionBranch(context.Background(), "proj", worktreePath, "open-agents/sess-1"); err != nil {
		t.Fatalf("push: %v", err)
	}
	if pushed := gitOutput(t, git, origin, "rev-parse", "refs/heads/open-agents/sess-1"); pushed != tip {
		t.Errorf("origin branch = %q, want session tip %q", pushed, tip)
	}
}

func TestPushSessionBranch_Rejected(t *testing.T) {
	git := requireGit(t)
	tmp := t.TempDir()
	_, repo := setupDeliveryRepo(t, git, tmp)
	worktreePath, _ := addSessionWorktree(t, git, repo, "open-agents/sess-1", "SESSION.md", "session work\n")
	ws := newDeliveryWorkspace(t, git, repo, filepath.Join(tmp, "managed"))
	if err := ws.PushSessionBranch(context.Background(), "proj", worktreePath, "open-agents/sess-1"); err != nil {
		t.Fatalf("initial push: %v", err)
	}
	// Diverge the remote: a second push of the same local history must be
	// rejected, not force-pushed.
	other := filepath.Join(tmp, "other")
	run(t, git, "clone", gitOutput(t, git, repo, "remote", "get-url", "origin"), other)
	runGit(t, git, other, "config", "user.email", "open-agents@example.com")
	runGit(t, git, other, "config", "user.name", "Open Agents")
	runGit(t, git, other, "fetch", "origin", "open-agents/sess-1:open-agents/sess-1")
	runGit(t, git, other, "checkout", "open-agents/sess-1")
	if err := os.WriteFile(filepath.Join(other, "OTHER.md"), []byte("diverged\n"), 0o644); err != nil {
		t.Fatalf("write diverged file: %v", err)
	}
	runGit(t, git, other, "add", "OTHER.md")
	runGit(t, git, other, "commit", "-m", "diverged")
	runGit(t, git, other, "push", "origin", "open-agents/sess-1")

	err := ws.PushSessionBranch(context.Background(), "proj", worktreePath, "open-agents/sess-1")
	if !errors.Is(err, ports.ErrDeliveryPushRejected) {
		t.Fatalf("err = %v, want ErrDeliveryPushRejected", err)
	}
}

func gitDirPath(t *testing.T, git, repo, name string) string {
	t.Helper()
	gitDir := gitOutput(t, git, repo, "rev-parse", "--git-dir")
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repo, gitDir)
	}
	return filepath.Join(gitDir, name)
}
