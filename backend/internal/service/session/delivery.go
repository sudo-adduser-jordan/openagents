package session

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/httpd/apierr"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

// deliveryTargetBranch is the local and remote base branch for board delivery.
// The local button merges into the local dev ref; the remote button opens its
// pull request with --base dev. It is a constant, not configuration, so both
// buttons can never disagree about where the work lands.
const deliveryTargetBranch = "dev"

// MergeLocalOutcome is the verified end-of-life delivery of a session branch
// into the local checkout: merged, verified, branch removed, session
// terminated (and therefore archived on the board).
type MergeLocalOutcome struct {
	Session       domain.Session `json:"session"`
	TargetBranch  string         `json:"targetBranch"`
	TargetHeadSHA string         `json:"targetHeadSha"`
	AlreadyMerged bool           `json:"alreadyMerged"`
	BranchRemoved bool           `json:"branchRemoved"`
}

// CreatePROutcome is the remote delivery of a session branch: pushed to
// origin with exactly one pull request against dev. The session stays alive
// for review.
type CreatePROutcome struct {
	Session domain.Session `json:"session"`
	URL     string         `json:"prUrl"`
	Number  int            `json:"prNumber"`
	Created bool           `json:"created"`
}

// MergeSessionLocal merges the session's branch into the local dev checkout
// and, only on verified success, removes the branch and terminates the
// session. The strict order is merge → verify → remove branch → terminate: a
// failure at any step stops the chain and the session stays alive. Merge and
// verification live in the delivery adapter; termination is the manager Kill
// every other destructive card action uses, so the card lands in archive the
// same way.
//
// An already-terminated session may still merge: Approve kills the worker
// first but preserves its worktree and branch for this later step, so the
// merge is verified the same way and teardown is simply skipped.
func (s *Service) MergeSessionLocal(ctx context.Context, id domain.SessionID) (MergeLocalOutcome, error) {
	rec, ok, err := s.store.GetSession(ctx, id)
	if err != nil {
		return MergeLocalOutcome{}, fmt.Errorf("get session %s for local merge: %w", id, err)
	}
	if !ok {
		return MergeLocalOutcome{}, apierr.NotFound("SESSION_NOT_FOUND", "Unknown session")
	}
	branch := strings.TrimSpace(rec.Metadata.Branch)
	if branch == "" {
		return MergeLocalOutcome{}, apierr.Invalid("SESSION_BRANCH_UNKNOWN", "Session has no branch to merge", nil)
	}
	if s.delivery == nil {
		return MergeLocalOutcome{}, apierr.NotImplemented("DELIVERY_UNAVAILABLE", "Local merge is not configured")
	}
	res, err := s.delivery.MergeSessionBranchLocal(ctx, rec.ProjectID, branch, deliveryTargetBranch)
	if err != nil {
		return MergeLocalOutcome{}, mapDeliveryError(branch, err)
	}
	if rec.IsTerminated {
		sess, err := s.Get(ctx, id)
		if err != nil {
			return MergeLocalOutcome{}, fmt.Errorf("read merged session %s: %w", id, err)
		}
		return MergeLocalOutcome{
			Session:       sess,
			TargetBranch:  res.TargetBranch,
			TargetHeadSHA: res.TargetHeadSHA,
			AlreadyMerged: res.AlreadyMerged,
			BranchRemoved: res.BranchRemoved,
		}, nil
	}
	if s.manager == nil {
		return MergeLocalOutcome{}, apierr.Internal("TERMINATE_UNAVAILABLE", "Session termination is not configured")
	}
	// The branch is merged, verified, and removed. Terminate last: the card
	// moves to archive only once there is nothing left to lose.
	killed, err := s.manager.Kill(ctx, id)
	if err != nil {
		return MergeLocalOutcome{}, apierr.Internal("TERMINATE_FAILED", fmt.Sprintf("Branch merged into %s but termination failed: %v", res.TargetBranch, err))
	}
	if !killed {
		return MergeLocalOutcome{}, apierr.NotFound("SESSION_NOT_FOUND", "Unknown session")
	}
	sess, err := s.Get(ctx, id)
	if err != nil {
		return MergeLocalOutcome{}, fmt.Errorf("read merged session %s: %w", id, err)
	}
	return MergeLocalOutcome{
		Session:       sess,
		TargetBranch:  res.TargetBranch,
		TargetHeadSHA: res.TargetHeadSHA,
		AlreadyMerged: res.AlreadyMerged,
		BranchRemoved: res.BranchRemoved,
	}, nil
}

// mapDeliveryError renders delivery-adapter sentinels as stable card-facing
// codes. Anything unrecognized stays a 500 so internal git details never leak.
func mapDeliveryError(branch string, err error) error {
	details := map[string]any{"branch": branch, "targetBranch": deliveryTargetBranch}
	switch {
	case errors.Is(err, ports.ErrWorkspaceDirty):
		return apierr.Conflict("WORKSPACE_DIRTY", "Project checkout has uncommitted changes; commit or stash them, then merge again", details)
	case errors.Is(err, ports.ErrDeliveryNotOnTargetBranch):
		return apierr.Conflict("CHECKOUT_NOT_ON_DEV", "Project checkout is not on dev; switch it to dev, then merge again", details)
	case errors.Is(err, ports.ErrDeliveryMergeConflict):
		return apierr.Conflict("LOCAL_MERGE_CONFLICT", fmt.Sprintf("Session branch conflicts with dev: %v. Resolve or abort the merge in the project checkout, then merge again", err), details)
	case errors.Is(err, ports.ErrDeliveryBranchNotFound):
		return apierr.NotFound("SESSION_BRANCH_NOT_FOUND", "Session branch is not in the project repository")
	default:
		return fmt.Errorf("merge session branch %s: %w", branch, err)
	}
}

// CreateSessionPR pushes the session branch to origin and opens exactly one
// pull request against dev, returning its URL. Duplicate protection runs
// before any mutation: durable daemon PR facts first (no subprocess), then
// the provider listing. A creation race that loses still returns the existing
// PR instead of failing, so double-clicks can never create two PRs.
func (s *Service) CreateSessionPR(ctx context.Context, id domain.SessionID) (CreatePROutcome, error) {
	rec, ok, err := s.store.GetSession(ctx, id)
	if err != nil {
		return CreatePROutcome{}, fmt.Errorf("get session %s for pull request: %w", id, err)
	}
	if !ok {
		return CreatePROutcome{}, apierr.NotFound("SESSION_NOT_FOUND", "Unknown session")
	}
	if rec.IsTerminated {
		return CreatePROutcome{}, apierr.Conflict("SESSION_TERMINATED", "Terminated sessions cannot open pull requests", nil)
	}
	branch := strings.TrimSpace(rec.Metadata.Branch)
	if branch == "" {
		return CreatePROutcome{}, apierr.Invalid("SESSION_BRANCH_UNKNOWN", "Session has no branch to push", nil)
	}
	worktree := strings.TrimSpace(rec.Metadata.WorkspacePath)
	if worktree == "" {
		return CreatePROutcome{}, apierr.Invalid("SESSION_WORKSPACE_UNKNOWN", "Session has no workspace to push from", nil)
	}
	if s.delivery == nil || s.prCreator == nil {
		return CreatePROutcome{}, apierr.NotImplemented("DELIVERY_UNAVAILABLE", "Pull request creation is not configured")
	}
	if existing, ok, err := s.findDurableSessionPR(ctx, id, branch); err != nil {
		return CreatePROutcome{}, err
	} else if ok {
		return s.prOutcome(ctx, id, existing.URL, existing.Number, false)
	}
	remoteURL, remoteNumber, found, err := s.prCreator.FindPRByHead(ctx, worktree, branch)
	if err != nil {
		return CreatePROutcome{}, mapGHError(err)
	}
	if found {
		return s.prOutcome(ctx, id, remoteURL, remoteNumber, false)
	}
	if err := s.delivery.PushSessionBranch(ctx, rec.ProjectID, worktree, branch); err != nil {
		if errors.Is(err, ports.ErrDeliveryPushRejected) {
			return CreatePROutcome{}, apierr.Conflict("PUSH_REJECTED", "Origin rejected the push (the remote branch diverged); reconcile it, then try again",
				map[string]any{"branch": branch})
		}
		return CreatePROutcome{}, fmt.Errorf("push session branch %s: %w", branch, err)
	}
	created, err := s.prCreator.CreatePR(ctx, worktree, deliveryTargetBranch, branch, prTitle(rec, branch), prBody(id))
	if err != nil {
		if errors.Is(err, ports.ErrGHPullRequestExists) {
			// Lost a creation race: re-list and return the winner instead of
			// failing, so the card still learns exactly one URL.
			if url, number, found, listErr := s.prCreator.FindPRByHead(ctx, worktree, branch); listErr == nil && found {
				return s.prOutcome(ctx, id, url, number, false)
			}
		}
		return CreatePROutcome{}, mapGHError(err)
	}
	return s.prOutcome(ctx, id, created.URL, created.Number, true)
}

// AutoDeliveryOutcome reports what one automatic delivery attempt did, so the
// head observer can log and back off without re-deriving the decision.
type AutoDeliveryOutcome struct {
	// Delivered is true only when this attempt actually pushed and opened (or
	// found) the pull request. A skip leaves the durable delivered-head fact
	// untouched so a later commit can still be delivered.
	Delivered bool
	// Reason names why the session was skipped. Empty when Delivered is true.
	Reason string
	// URL is the delivered pull request, when there is one.
	URL string
}

// automaticDeliveryReason explains a skip. These are the load-bearing answers
// to "why did the agent's commit not start delivery?" and are also the strings
// tests assert on.
const (
	autoReasonNoChange      = "head already delivered"
	autoReasonNotEligible   = "session not eligible for automatic delivery"
	autoReasonNotConfigured = "delivery not configured"
	autoReasonNoCommit      = "no commit on the session branch"
	// autoReasonNotReviewed names the stage-discipline skip: the worker
	// committed before its plan was reviewed and approved through the
	// planning-to-building transition, so the commit earns no pull request.
	autoReasonNotReviewed = "plan not approved"
)

// EligibleForAutoDelivery reports whether a commit on this session's branch
// should start delivery on its own, without the user pressing Commit.
//
// The gate is deliberately narrow and mirrors the sessions that currently
// receive the Commit button: a live worker in building mode that already has a
// branch and a workspace, and that the daemon has not already observed a
// pull request for. Planning-mode and manager sessions are excluded because
// building mode is the user's explicit "let this session ship work" signal, and
// a manager session delivers through its workers rather than its own branch.
//
// Building mode alone is not enough: the worker's plan must also have been
// reviewed while uncommitted and approved through the planning-to-building
// stage transition (the durable plan-approved fact). A commit made before that
// approval — the commit-before-review behavior the stage discipline forbids —
// earns no pull request. The skip leaves the delivered-head fact alone, so the
// same commit is still delivered once the approval lands; nothing is silently
// dropped.
//
// This decides only whether to *start* delivery. It never selects a board
// column: the card still moves because a pull request exists, and every column
// is derived from daemon-observed PR facts.
func EligibleForAutoDelivery(rec domain.SessionRecord, prs []domain.PRFacts) (bool, string) {
	if rec.IsTerminated {
		return false, autoReasonNotEligible
	}
	if rec.Kind != domain.KindWorker {
		return false, autoReasonNotEligible
	}
	if rec.WorkflowMode != domain.WorkflowModeBuilding {
		return false, autoReasonNotEligible
	}
	if !rec.PlanApproved {
		return false, autoReasonNotReviewed
	}
	if strings.TrimSpace(rec.Metadata.Branch) == "" || strings.TrimSpace(rec.Metadata.WorkspacePath) == "" {
		return false, autoReasonNotEligible
	}
	// A tracked pull request already means delivery happened. Checking the
	// durable facts first is what makes repeated polls cheap and keeps the
	// trigger from opening a second PR for the same session.
	for _, pr := range prs {
		if !pr.Closed && !pr.Merged {
			return false, autoReasonNotEligible
		}
	}
	return true, ""
}

// DeliverSessionHead runs one automatic delivery attempt for a commit the
// daemon just observed at headSHA. It is the automatic counterpart of the
// Commit button and deliberately reuses CreateSessionPR, so both paths share
// the same push, the same exactly-one-PR de-duplication, and the same target
// branch.
//
// The durable delivered-head fact is written only after delivery succeeds. A
// failure returns the error and leaves the fact alone, so the same commit is
// retried on a later poll instead of being silently skipped.
func (s *Service) DeliverSessionHead(ctx context.Context, id domain.SessionID, headSHA string) (AutoDeliveryOutcome, error) {
	headSHA = strings.TrimSpace(headSHA)
	rec, ok, err := s.store.GetSession(ctx, id)
	if err != nil {
		return AutoDeliveryOutcome{}, fmt.Errorf("get session %s for automatic delivery: %w", id, err)
	}
	if !ok {
		return AutoDeliveryOutcome{Reason: autoReasonNotEligible}, nil
	}
	if headSHA == "" {
		return AutoDeliveryOutcome{Reason: autoReasonNoCommit}, nil
	}
	// The head the observer saw must still be the fact we recorded, otherwise
	// the commit is already known and re-delivering would be a redundant push.
	if rec.DeliveredHeadSHA == headSHA {
		return AutoDeliveryOutcome{Reason: autoReasonNoChange}, nil
	}
	if s.delivery == nil || s.prCreator == nil {
		return AutoDeliveryOutcome{Reason: autoReasonNotConfigured}, nil
	}
	prs, err := s.store.ListPRFactsForSession(ctx, id)
	if err != nil {
		return AutoDeliveryOutcome{}, fmt.Errorf("list pull request facts for session %s: %w", id, err)
	}
	if eligible, reason := EligibleForAutoDelivery(rec, prs); !eligible {
		return AutoDeliveryOutcome{Reason: reason}, nil
	}
	outcome, err := s.CreateSessionPR(ctx, id)
	if err != nil {
		return AutoDeliveryOutcome{}, err
	}
	if _, err := s.store.SetSessionDeliveredHeadSHA(ctx, id, headSHA, s.now()); err != nil {
		// The pull request exists but the fact did not stick. Return the
		// success anyway: the PR is real, and the next poll's durable PR-fact
		// check makes a duplicate attempt harmless.
		return AutoDeliveryOutcome{Delivered: true, URL: outcome.URL}, fmt.Errorf("record delivered head for session %s: %w", id, err)
	}
	return AutoDeliveryOutcome{Delivered: true, URL: outcome.URL}, nil
}

// sessionPRExisting is one non-terminal pull request already attributed to a
// session branch.
type sessionPRExisting struct {
	URL    string
	Number int
}

// findDurableSessionPR returns the session's tracked open pull request for a
// branch, if the daemon has already observed one. Terminal PRs (merged,
// closed) do not count: opening a follow-up after one landed is legitimate.
func (s *Service) findDurableSessionPR(ctx context.Context, id domain.SessionID, branch string) (sessionPRExisting, bool, error) {
	prs, err := s.store.ListPRsBySession(ctx, id)
	if err != nil {
		return sessionPRExisting{}, false, fmt.Errorf("list session pull requests %s: %w", id, err)
	}
	for _, pr := range prs {
		if pr.Merged || pr.Closed || pr.SourceBranch != branch {
			continue
		}
		url := strings.TrimSpace(pr.HTMLURL)
		if url == "" {
			url = strings.TrimSpace(pr.URL)
		}
		if url == "" {
			continue
		}
		return sessionPRExisting{URL: url, Number: pr.Number}, true, nil
	}
	return sessionPRExisting{}, false, nil
}

func (s *Service) prOutcome(ctx context.Context, id domain.SessionID, url string, number int, created bool) (CreatePROutcome, error) {
	sess, err := s.Get(ctx, id)
	if err != nil {
		return CreatePROutcome{}, fmt.Errorf("read session %s: %w", id, err)
	}
	return CreatePROutcome{Session: sess, URL: url, Number: number, Created: created}, nil
}

// mapGHError renders gh failures as stable card-facing codes: missing binary
// is unconfigured (501), missing auth names the fix (403), everything else is
// a 500 without leaking subprocess output.
func mapGHError(err error) error {
	switch {
	case errors.Is(err, ports.ErrGHNotInstalled):
		return apierr.NotImplemented("GH_NOT_INSTALLED", "The gh CLI is not installed; install it to open pull requests")
	case errors.Is(err, ports.ErrGHAuthMissing):
		return apierr.Forbidden("GH_AUTH_MISSING", "gh is not authenticated; run `gh auth login`, then try again")
	default:
		return fmt.Errorf("pull request operation: %w", err)
	}
}

func prTitle(rec domain.SessionRecord, branch string) string {
	if title := strings.TrimSpace(rec.DisplayName); title != "" {
		return title
	}
	return branch
}

func prBody(id domain.SessionID) string {
	return fmt.Sprintf("Opened from Open Agents session %s.", string(id))
}
