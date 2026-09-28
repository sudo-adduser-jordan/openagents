package gitworktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

var _ ports.SessionBranchDelivery = (*Workspace)(nil)

// MergeSessionBranchLocal merges the session branch into the local target
// branch of the project's repository: merge, verify the session tip is an
// ancestor of the target, detach the session worktree, delete the branch ref.
// Every precondition is checked before anything moves: a dirty checkout, a
// checkout not on the target branch, or an in-progress merge aborts without
// touching refs. A conflict leaves MERGE_HEAD and conflict markers intact so
// the user can resolve (`git merge --continue`) or abandon
// (`git merge --abort`) it; nothing is committed and no branch is deleted.
func (w *Workspace) MergeSessionBranchLocal(ctx context.Context, projectID domain.ProjectID, sessionBranch, targetBranch string) (ports.LocalMergeResult, error) {
	branch := strings.TrimSpace(sessionBranch)
	target := strings.TrimSpace(targetBranch)
	if branch == "" || target == "" {
		return ports.LocalMergeResult{}, fmt.Errorf("%w: session and target branches are required", ports.ErrWorkspaceBranchInvalid)
	}
	repo, err := w.repoPath(projectID)
	if err != nil {
		return ports.LocalMergeResult{}, err
	}
	if err := w.validateBranch(ctx, repo, branch); err != nil {
		return ports.LocalMergeResult{}, err
	}
	if err := w.validateBranch(ctx, repo, target); err != nil {
		return ports.LocalMergeResult{}, err
	}
	// The merge runs in the project checkout, so it lands on HEAD. Refuse
	// unless HEAD is exactly the target: merging anywhere else would put the
	// session's work on a branch nobody asked for.
	current, err := w.currentBranch(ctx, repo)
	if err != nil || current != target {
		return ports.LocalMergeResult{}, fmt.Errorf("gitworktree: checkout is on %q, want %q: %w", current, target, ports.ErrDeliveryNotOnTargetBranch)
	}
	// Never merge over uncommitted work: a conflicted merge on a dirty tree
	// mixes the user's edits with the session's, and neither side stays
	// recoverable on its own.
	dirty, err := w.isDirty(ctx, repo)
	if err != nil {
		return ports.LocalMergeResult{}, err
	}
	if dirty {
		return ports.LocalMergeResult{}, fmt.Errorf("gitworktree: refusing to merge into dirty checkout %q: %w", repo, ports.ErrWorkspaceDirty)
	}
	if inProgress, err := w.sequencerActive(ctx, repo); err != nil {
		return ports.LocalMergeResult{}, err
	} else if inProgress {
		return ports.LocalMergeResult{}, fmt.Errorf("gitworktree: a merge is already in progress in %q: %w", repo, ports.ErrDeliveryMergeConflict)
	}
	tip, err := w.revParse(ctx, repo, "refs/heads/"+branch)
	if err != nil {
		return ports.LocalMergeResult{}, fmt.Errorf("gitworktree: session branch %q: %w", branch, ports.ErrDeliveryBranchNotFound)
	}
	alreadyMerged, err := w.isAncestor(ctx, repo, tip, target)
	if err != nil {
		return ports.LocalMergeResult{}, err
	}
	if !alreadyMerged {
		if err := w.mergeIntoCurrent(ctx, repo, branch); err != nil {
			return ports.LocalMergeResult{}, err
		}
		// Verify, don't trust the exit code alone: the merge must have left
		// the session tip reachable from the target.
		merged, err := w.isAncestor(ctx, repo, tip, target)
		if err != nil {
			return ports.LocalMergeResult{}, err
		}
		if !merged {
			return ports.LocalMergeResult{}, fmt.Errorf("gitworktree: session tip %q is not reachable from %q after merge", tip, target)
		}
	}
	// git refuses to delete a branch checked out in a linked worktree, so
	// detach the session worktree first. Detaching moves no files and changes
	// no content: the worktree HEAD stays on the same commit, only the branch
	// pointer is released. Safe-delete (-d) then refuses if the tip is somehow
	// not merged, which is the last guard before the ref disappears.
	if err := w.detachSessionWorktree(ctx, repo, branch, tip); err != nil {
		return ports.LocalMergeResult{}, err
	}
	if _, err := w.run(ctx, w.binary, branchDeleteArgs(repo, branch)...); err != nil {
		return ports.LocalMergeResult{}, fmt.Errorf("gitworktree: delete branch %q: %w", branch, err)
	}
	head, err := w.revParse(ctx, repo, target)
	if err != nil {
		return ports.LocalMergeResult{}, fmt.Errorf("gitworktree: read %q HEAD: %w", target, err)
	}
	return ports.LocalMergeResult{TargetBranch: target, TargetHeadSHA: head, AlreadyMerged: alreadyMerged, BranchRemoved: true}, nil
}

// PushSessionBranch pushes the session branch from its worktree to the origin
// remote without force. A non-fast-forward remote rejects the push instead of
// being rewritten; the local state is untouched.
func (w *Workspace) PushSessionBranch(ctx context.Context, projectID domain.ProjectID, worktreePath, branch string) error {
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return fmt.Errorf("%w: session branch is required", ports.ErrWorkspaceBranchInvalid)
	}
	if _, err := w.repoPath(projectID); err != nil {
		return err
	}
	worktree, err := physicalAbs(worktreePath)
	if err != nil {
		return fmt.Errorf("gitworktree: worktree path: %w", err)
	}
	if _, err := w.run(ctx, w.binary, pushBranchArgs(worktree, branch)...); err != nil {
		if isPushRejected(err) {
			return fmt.Errorf("gitworktree: push %q: %w", branch, ports.ErrDeliveryPushRejected)
		}
		return fmt.Errorf("gitworktree: push %q: %w", branch, err)
	}
	return nil
}

// currentBranch reports the checkout's current branch, or "" on detached HEAD.
func (w *Workspace) currentBranch(ctx context.Context, repo string) (string, error) {
	out, err := w.run(ctx, w.binary, symbolicRefShortArgs(repo)...)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return "", nil
		}
		return "", fmt.Errorf("gitworktree: current branch: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// sequencerActive reports whether a merge/cherry-pick/revert is in progress
// in the checkout.
func (w *Workspace) sequencerActive(ctx context.Context, repo string) (bool, error) {
	for _, name := range []string{"MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD"} {
		out, err := w.run(ctx, w.binary, gitPathArgs(repo, name)...)
		if err != nil {
			return false, fmt.Errorf("gitworktree: sequencer state: %w", err)
		}
		candidate := strings.TrimSpace(string(out))
		if candidate == "" {
			continue
		}
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(repo, candidate)
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return true, nil
		}
	}
	return false, nil
}

// isAncestor reports whether tip is reachable from target.
func (w *Workspace) isAncestor(ctx context.Context, repo, tip, target string) (bool, error) {
	if err := w.runMergeBase(ctx, repo, tip, target); err == nil {
		return true, nil
	} else {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return false, nil
		}
		return false, fmt.Errorf("gitworktree: merge-base %q %q: %w", tip, target, err)
	}
}

func (w *Workspace) runMergeBase(ctx context.Context, repo, tip, target string) error {
	_, err := w.run(ctx, w.binary, mergeBaseAncestorArgs(repo, tip, target)...)
	return err
}

// mergeIntoCurrent merges the session branch into the checkout's HEAD. On
// conflict it returns ErrDeliveryMergeConflict carrying the conflicted files,
// with MERGE_HEAD and markers left intact for manual recovery.
func (w *Workspace) mergeIntoCurrent(ctx context.Context, repo, branch string) error {
	if _, err := w.run(ctx, w.binary, mergeBranchArgs(repo, branch)...); err == nil {
		return nil
	} else {
		inProgress, seqErr := w.sequencerActive(ctx, repo)
		if seqErr != nil {
			return fmt.Errorf("gitworktree: merge %q failed: %w", branch, err)
		}
		if !inProgress {
			// No sequencer state: the merge failed without starting one
			// (locked index, missing identity, ...). Nothing to recover.
			return fmt.Errorf("gitworktree: merge %q: %w", branch, err)
		}
		files := w.conflictedFiles(ctx, repo)
		if len(files) == 0 {
			return fmt.Errorf("gitworktree: merge %q conflicts; resolve or abort in %q: %w", branch, repo, ports.ErrDeliveryMergeConflict)
		}
		return fmt.Errorf("gitworktree: merge %q conflicts in %s; resolve or abort in %q: %w", branch, strings.Join(files, ", "), repo, ports.ErrDeliveryMergeConflict)
	}
}

// conflictedFiles lists paths with unresolved merge markers, best-effort: an
// empty list never hides the conflict itself, only the file names.
func (w *Workspace) conflictedFiles(ctx context.Context, repo string) []string {
	out, err := w.run(ctx, w.binary, conflictedFilesArgs(repo)...)
	if err != nil {
		return nil
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if file := strings.TrimSpace(line); file != "" {
			files = append(files, file)
		}
	}
	return files
}

// detachSessionWorktree detaches every linked worktree currently checking out
// the session branch, so the branch ref becomes deletable. Worktrees already
// detached or on other branches are untouched.
func (w *Workspace) detachSessionWorktree(ctx context.Context, repo, branch, tip string) error {
	records, err := w.listRecords(ctx, repo)
	if err != nil {
		return err
	}
	for _, rec := range records {
		if rec.Branch != branch || rec.Detached || rec.Path == "" {
			continue
		}
		if _, err := w.run(ctx, w.binary, checkoutDetachArgs(rec.Path)...); err != nil {
			return fmt.Errorf("gitworktree: detach worktree %q: %w", rec.Path, err)
		}
	}
	return nil
}

// isPushRejected reports whether a push failure is a remote rejection
// (non-fast-forward) rather than a connectivity or configuration failure.
func isPushRejected(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "[rejected]") ||
		strings.Contains(msg, "non-fast-forward") ||
		strings.Contains(msg, "fetch first")
}
