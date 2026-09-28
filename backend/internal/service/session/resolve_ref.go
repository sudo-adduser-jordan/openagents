package session

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/httpd/apierr"
)

// RefMatch records which half of a reference produced the answer, so a caller
// can tell an id that merely looked numeric from a number that was looked up.
type RefMatch string

const (
	// RefMatchID means the reference was an exact session id.
	RefMatchID RefMatch = "id"
	// RefMatchNum means a bare agent number was resolved to a session.
	RefMatchNum RefMatch = "num"
)

// ResolveRefResult is the canonical target of a user-supplied session reference.
type ResolveRefResult struct {
	// Ref echoes what the caller asked for, so a client can key its own state
	// on the token the user typed rather than the id it resolved to.
	Ref       string
	SessionID domain.SessionID
	ProjectID domain.ProjectID
	// Num is the owning project's agent number. Zero when the daemon predates
	// number addressing, which callers must treat as "unknown", not "number 0".
	Num       int64
	MatchedBy RefMatch
}

// ResolveRef turns a user-supplied session reference into a canonical session
// id. The reference is either a session id or a bare agent number ("5"), the
// number the sidebar shows.
//
// A number is unique only within its project — UNIQUE (project_id, num) in the
// schema — and a projectless session keeps its own number space, so a bare
// number without a project scope is only accepted when it is unique across every
// project. An ambiguous reference is reported with its candidates rather than
// resolved to whichever project happened to sort first.
//
// Precedence is id-first and absolute. An exact id match wins even when a
// different session holds that number, so a numeric session id keeps working;
// that is also why a project-scoped lookup refuses a mismatched id instead of
// falling through to the number.
//
// Retired numbers are reported distinctly. Retiring a session permanently
// reserves its number (NextSessionNum unions retired_session_nums) so a
// registered worktree path, PR conversation, or change_log row can never see it
// reused, which means a gap in the sequence is expected and a retired reference
// is permanently unresolvable — it must not be reported as never-existed.
func (s *Service) ResolveRef(ctx context.Context, ref string, project domain.ProjectID) (ResolveRefResult, error) {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return ResolveRefResult{}, apierr.Invalid("SESSION_REF_REQUIRED", "ref is required", nil)
	}
	// An exact id match is tried first and is never second-guessed.
	if found, ok, err := s.store.GetSessionRef(ctx, domain.SessionID(trimmed)); err != nil {
		return ResolveRefResult{}, fmt.Errorf("resolve session ref %s: %w", trimmed, err)
	} else if ok {
		if project != "" && found.ProjectID != project {
			return ResolveRefResult{}, apierr.Invalid("SESSION_REF_PROJECT_MISMATCH",
				fmt.Sprintf("session %s is not in project %s", trimmed, project), nil)
		}
		return resolved(trimmed, found, RefMatchID), nil
	}
	// Only an all-digit reference can be an agent number. Anything else is a
	// misspelled id and is reported as not found, never as a bad number.
	num, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return ResolveRefResult{}, apierr.NotFound("SESSION_REF_NOT_FOUND",
			fmt.Sprintf("no session with id %s", trimmed))
	}
	return s.resolveNum(ctx, trimmed, num, project)
}

func (s *Service) resolveNum(ctx context.Context, ref string, num int64, project domain.ProjectID) (ResolveRefResult, error) {
	refs, err := s.store.ListSessionRefsByNum(ctx, num)
	if err != nil {
		return ResolveRefResult{}, fmt.Errorf("resolve session num %d: %w", num, err)
	}
	if project != "" {
		// The unique constraint means at most one live session holds this
		// number in the named project; anything else belongs to another one.
		scoped := make([]domain.SessionNumRef, 0, 1)
		for _, candidate := range refs {
			if candidate.ProjectID == project {
				scoped = append(scoped, candidate)
			}
		}
		refs = scoped
	}
	switch len(refs) {
	case 1:
		return resolved(ref, refs[0], RefMatchNum), nil
	case 0:
		return ResolveRefResult{}, s.retiredOrMissing(ctx, ref, num, project)
	}
	return ResolveRefResult{}, ambiguousNum(num, refs)
}

// retiredOrMissing separates a number that can never resolve again from one that
// was never issued, so a retired reference never surfaces as a bare 404.
func (s *Service) retiredOrMissing(ctx context.Context, ref string, num int64, project domain.ProjectID) error {
	retired, err := s.store.ListRetiredSessionNumProjects(ctx, num)
	if err != nil {
		return fmt.Errorf("resolve retired session num %d: %w", num, err)
	}
	// Scoped, only the named project counts: another project having retired the
	// same number says nothing about whether this one ever issued it.
	mine := make([]domain.ProjectID, 0, len(retired))
	for _, owner := range retired {
		if project == "" || owner == project {
			mine = append(mine, owner)
		}
	}
	if len(mine) == 0 {
		if project == "" {
			return apierr.NotFound("SESSION_REF_NOT_FOUND",
				fmt.Sprintf("no session with id or number %s", ref))
		}
		return apierr.NotFound("SESSION_REF_NOT_FOUND",
			fmt.Sprintf("no session with id or number %s in project %s", ref, project))
	}
	if project != "" {
		return apierr.NotFound("SESSION_NUM_RETIRED",
			fmt.Sprintf("session number %s was retired; retired numbers are never reused", ref))
	}
	return apierr.NotFound("SESSION_NUM_RETIRED", fmt.Sprintf(
		"session number %s was retired; retired numbers are never reused (retired in %s)",
		ref, describeProjects(mine)))
}

func ambiguousNum(num int64, refs []domain.SessionNumRef) error {
	candidates := make([]map[string]any, 0, len(refs))
	descriptions := make([]string, 0, len(refs))
	for _, ref := range refs {
		candidates = append(candidates, map[string]any{
			"sessionId": string(ref.ID),
			"projectId": string(ref.ProjectID),
			"num":       ref.Num,
		})
		descriptions = append(descriptions, fmt.Sprintf("%s (%s)", ref.ID, describeProject(ref.ProjectID)))
	}
	return apierr.Conflict("SESSION_REF_AMBIGUOUS", fmt.Sprintf(
		"session number %d matches more than one session: %s; scope the request to a single project to choose one",
		num, strings.Join(descriptions, ", ")), map[string]any{"candidates": candidates})
}

func resolved(ref string, found domain.SessionNumRef, by RefMatch) ResolveRefResult {
	return ResolveRefResult{
		Ref:       ref,
		SessionID: found.ID,
		ProjectID: found.ProjectID,
		Num:       found.Num,
		MatchedBy: by,
	}
}

// describeProject names a scope for a human-facing message. A projectless
// session has no project to name, so it is described rather than shown blank.
func describeProject(project domain.ProjectID) string {
	if project == "" {
		return "no project"
	}
	return "project " + string(project)
}

func describeProjects(projects []domain.ProjectID) string {
	out := make([]string, 0, len(projects))
	for _, project := range projects {
		out = append(out, describeProject(project))
	}
	return strings.Join(out, ", ")
}
