package cli

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// resolveSessionRefResponse mirrors the daemon's ResolveSessionRefResponse body
// for GET /api/v1/sessions/resolve. The CLI keeps its own copy so it need not
// import httpd.
type resolveSessionRefResponse struct {
	Ref       string `json:"ref"`
	SessionID string `json:"sessionId"`
	ProjectID string `json:"projectId"`
	Num       int64  `json:"num"`
	MatchedBy string `json:"matchedBy"`
}

// resolvedSessionRef is a target the rest of the CLI can use directly: always a
// concrete session id, plus the number to show the user.
type resolvedSessionRef struct {
	SessionID string
	ProjectID string
	Num       int64
	MatchedBy string
}

// resolveSessionRefOptions selects the project scope used to disambiguate a bare
// agent number.
type resolveSessionRefOptions struct {
	// Project is an explicit --project value; it wins over everything else.
	Project string
	// FromSessionID is the sending session whose project should be assumed. It is
	// best-effort: any failure simply leaves the number unscoped.
	FromSessionID string
}

// resolveSessionRef turns a user-supplied target into a concrete session id.
//
// Only an all-digit target can be an agent number, so a nonnumeric target is
// returned unchanged without a round trip — it cannot have been a number, and
// asking would only add a failure mode to the common case.
func (c *commandContext) resolveSessionRef(
	ctx context.Context,
	ref string,
	opts resolveSessionRefOptions,
) (resolvedSessionRef, error) {
	trimmed := strings.TrimSpace(ref)
	if !isAgentNumRef(trimmed) {
		return resolvedSessionRef{SessionID: trimmed}, nil
	}
	project, err := c.resolveRefProject(ctx, opts)
	if err != nil {
		return resolvedSessionRef{}, err
	}
	return c.requestSessionRef(ctx, trimmed, project)
}

// requestSessionRef asks the daemon to resolve a reference. Callers that already
// know the reference needs the daemon — including a reference that is an id, when
// the point is to learn the number it owns — use this directly.
func (c *commandContext) requestSessionRef(ctx context.Context, ref, project string) (resolvedSessionRef, error) {
	query := url.Values{}
	query.Set("ref", ref)
	if project != "" {
		query.Set("project", project)
	}
	var out resolveSessionRefResponse
	if err := c.getJSON(ctx, "sessions/resolve?"+query.Encode(), &out); err != nil {
		return resolvedSessionRef{}, refResolutionError(err)
	}
	return resolvedSessionRef{
		SessionID: out.SessionID,
		ProjectID: out.ProjectID,
		Num:       out.Num,
		MatchedBy: out.MatchedBy,
	}, nil
}

// isAgentNumRef reports whether ref could be an agent number. Only digits count:
// an id may contain digits, and an id is tried first anyway, so gating on
// "starts with a digit" would just add pointless lookups.
func isAgentNumRef(ref string) bool {
	if ref == "" {
		return false
	}
	_, err := strconv.ParseInt(ref, 10, 64)
	return err == nil
}

// resolveRefProject applies the scope precedence: an explicit --project, then
// OPEN_AGENTS_PROJECT_ID, then the sending session's own project. Falling back to
// no scope is deliberate — the daemon then requires the number to be unique
// across every project and reports a candidate list if it is not, which beats
// guessing a project here.
func (c *commandContext) resolveRefProject(ctx context.Context, opts resolveSessionRefOptions) (string, error) {
	if project := strings.TrimSpace(opts.Project); project != "" {
		return project, nil
	}
	if project := strings.TrimSpace(os.Getenv("OPEN_AGENTS_PROJECT_ID")); project != "" {
		return project, nil
	}
	if sender := strings.TrimSpace(opts.FromSessionID); sender != "" {
		var sess sessionResponse
		if err := c.getJSON(ctx, "sessions/"+url.PathEscape(sender), &sess); err == nil {
			if project := strings.TrimSpace(sess.Session.ProjectID); project != "" {
				return project, nil
			}
		}
	}
	return "", nil
}

// refResolutionError converts a resolver failure into the right CLI outcome.
//
// An ambiguous number is the user's mistake rather than a daemon failure: the
// request was well formed, but "5" does not identify one session. Exiting 2 with
// a usage error tells a script the fix is a flag, not a retry. Every other code
// is a real failure and keeps its envelope, request id, and exit 1.
func refResolutionError(err error) error {
	var responseErr apiResponseError
	if errors.As(err, &responseErr) && responseErr.ErrorBody.Code == "SESSION_REF_AMBIGUOUS" {
		return usageError{responseErr}
	}
	return err
}

// errRouteNotFound and errSessionNotFound are the two ways a daemon too old to
// have GET /sessions/resolve reports the missing route. Only these are treated
// as "this daemon cannot resolve numbers"; a real 404 from a resolver that
// exists (an unknown or retired number) must reach the user unchanged.
const (
	errRouteNotFound   = "ROUTE_NOT_FOUND"
	errSessionNotFound = "SESSION_NOT_FOUND"
)

// isUnresolvableByOldDaemon reports whether err means the daemon has no resolver
// route. The ref is then passed through verbatim, which is the pre-numbering
// behavior: a bare number simply does not exist as an id, and the send attempt
// reports it. Callers must not extend this to any 404 — SESSION_REF_NOT_FOUND
// and SESSION_NUM_RETIRED come from a resolver that does exist and are answers,
// not missing capability.
func isUnresolvableByOldDaemon(err error) bool {
	if errors.Is(err, errDaemonUnavailable) {
		return true
	}
	var responseErr apiResponseError
	if !errors.As(err, &responseErr) || responseErr.StatusCode != http.StatusNotFound {
		return false
	}
	return responseErr.ErrorBody.Code == errRouteNotFound || responseErr.ErrorBody.Code == errSessionNotFound
}

// resolveSendTarget resolves a --session target for a command that addresses an
// existing session, falling back to the literal value against a daemon that
// predates number addressing.
//
// The fallback is only for capability, never for a resolver's answer: a daemon
// that resolves and reports SESSION_REF_NOT_FOUND knows the number is not a
// session, and substituting the literal number would only produce a second,
// worse error.
func (c *commandContext) resolveSendTarget(ctx context.Context, ref string, opts resolveSessionRefOptions) (resolvedSessionRef, error) {
	resolved, err := c.resolveSessionRef(ctx, ref, opts)
	if err == nil {
		return resolved, nil
	}
	if isUnresolvableByOldDaemon(err) {
		return resolvedSessionRef{SessionID: strings.TrimSpace(ref)}, nil
	}
	return resolvedSessionRef{}, err
}

// agentNumPrefix renders the "[from N] " sender prefix, falling back to the
// full session id when the sending session's number is unknown.
//
// Only digits are shown. A number is what the board shows and what a person
// would type back, so "[from 6]" is addressable where "[from openagents-6]" is
// not, and a resolved number may legitimately be small enough to look like a
// session id.
func agentNumPrefix(sessionID string, num int64) string {
	if num > 0 {
		return "[from " + strconv.FormatInt(num, 10) + "] "
	}
	return "[from " + sessionID + "] "
}
