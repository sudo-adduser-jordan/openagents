package session

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/httpd/apierr"
)

// refFixture is a store holding sessions with the numbers the resolver cares
// about: a bare session, a project-scoped one, a shared number across two
// projects, and a number retired in one project but still live in another.
type refFixture struct {
	*fakeStore
	service *Service
}

func newRefFixture(t *testing.T) *refFixture {
	t.Helper()
	store := newFakeStore()
	add := func(id string, project domain.ProjectID, num int64) {
		store.sessions[domain.SessionID(id)] = domain.SessionRecord{ID: domain.SessionID(id), ProjectID: project}
		store.nums[domain.SessionID(id)] = num
	}
	add("openagents-5", "openagents", 5)
	add("acme-5", "acme", 5)
	add("demo-7", "demo", 7)
	add("bare-9", "", 9)
	store.retiredNums[4] = []domain.ProjectID{"openagents"}
	return &refFixture{store, New(nil, store)}
}

func TestResolveRefResolvesExactID(t *testing.T) {
	f := newRefFixture(t)
	got, err := f.service.ResolveRef(context.Background(), "demo-7", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SessionID != "demo-7" || got.MatchedBy != RefMatchID {
		t.Errorf("result = %+v, want demo-7 matched by id", got)
	}
	if got.ProjectID != "demo" || got.Num != 7 {
		t.Errorf("result = %+v, want the session's project and number", got)
	}
	if got.Ref != "demo-7" {
		t.Errorf("Ref = %q, want the requested reference echoed back", got.Ref)
	}
}

func TestResolveRefTrimsSurroundingWhitespace(t *testing.T) {
	f := newRefFixture(t)
	got, err := f.service.ResolveRef(context.Background(), "  5  ", "acme")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SessionID != "acme-5" {
		t.Errorf("SessionID = %q, want acme-5", got.SessionID)
	}
}

func TestResolveRefIDWinsOverANumericCollision(t *testing.T) {
	f := newRefFixture(t)
	// "5" is a number held by two sessions, but it is also nothing here: the id
	// branch is tried first so a numeric id keeps addressing its own session.
	add := f.sessions[domain.SessionID("5")]
	add.ID = "5"
	f.sessions["5"] = add
	f.nums["5"] = 5

	got, err := f.service.ResolveRef(context.Background(), "5", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SessionID != "5" || got.MatchedBy != RefMatchID {
		t.Errorf("result = %+v, want the exact id 5 to win", got)
	}
}

func TestResolveRefIDInAnotherProjectIsRejectedNotRebound(t *testing.T) {
	f := newRefFixture(t)
	// demo-7 is a real id, but it is not in "acme". Falling through to the
	// number 7 would silently retarget a different session, so this must fail.
	_, err := f.service.ResolveRef(context.Background(), "demo-7", "acme")
	assertAPIError(t, err, "SESSION_REF_PROJECT_MISMATCH", "session demo-7 is not in project acme")
}

func TestResolveRefResolvesNumberWithinProject(t *testing.T) {
	f := newRefFixture(t)
	got, err := f.service.ResolveRef(context.Background(), "5", "acme")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SessionID != "acme-5" || got.MatchedBy != RefMatchNum {
		t.Errorf("result = %+v, want acme-5 matched by num", got)
	}
}

func TestResolveRefResolvesUnambiguousNumberWithoutProject(t *testing.T) {
	f := newRefFixture(t)
	got, err := f.service.ResolveRef(context.Background(), "7", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SessionID != "demo-7" || got.MatchedBy != RefMatchNum {
		t.Errorf("result = %+v, want demo-7 matched by num", got)
	}
}

func TestResolveRefReportsAmbiguousNumberWithCandidates(t *testing.T) {
	f := newRefFixture(t)
	_, err := f.service.ResolveRef(context.Background(), "5", "")
	assertAPIError(t, err, "SESSION_REF_AMBIGUOUS",
		"session number 5 matches more than one session: acme-5 (project acme), openagents-5 (project openagents); scope the request to a single project to choose one")
}

func TestResolveRefAmbiguityCarriesCandidateDetails(t *testing.T) {
	f := newRefFixture(t)
	_, err := f.service.ResolveRef(context.Background(), "5", "")
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want an API error", err)
	}
	candidates, ok := apiErr.Details["candidates"].([]map[string]any)
	if !ok {
		t.Fatalf("details = %#v, want a candidates list", apiErr.Details)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidates = %#v, want both matches", candidates)
	}
	// The candidates are what a client needs to offer a choice, so they must
	// carry the id, the project, and the number.
	first := candidates[0]
	if first["sessionId"] != "acme-5" || first["projectId"] != "acme" || first["num"] != int64(5) {
		t.Errorf("candidate = %#v, want the full addressing triple", first)
	}
}

func TestResolveRefReportsRetiredNumberDistinctly(t *testing.T) {
	f := newRefFixture(t)
	_, err := f.service.ResolveRef(context.Background(), "4", "openagents")
	assertAPIError(t, err, "SESSION_NUM_RETIRED",
		"session number 4 was retired; retired numbers are never reused")
}

func TestResolveRefUnscopedRetiredNumberNamesItsProject(t *testing.T) {
	f := newRefFixture(t)
	_, err := f.service.ResolveRef(context.Background(), "4", "")
	// The unscoped message has to say where the number went, since the caller
	// did not name a project.
	if !strings.Contains(err.Error(), "retired in project openagents") {
		t.Errorf("err = %v, want the retired project named", err)
	}
}

func TestResolveRefRetiredInAnotherProjectIsStillNotFound(t *testing.T) {
	f := newRefFixture(t)
	// Number 4 was retired in openagents only. Asking about it in "acme" is a
	// plain miss: acme never issued it, and never will.
	_, err := f.service.ResolveRef(context.Background(), "4", "acme")
	assertAPIError(t, err, "SESSION_REF_NOT_FOUND", "no session with id or number 4 in project acme")
}

func TestResolveRefMissingNumberIsNotFound(t *testing.T) {
	f := newRefFixture(t)
	_, err := f.service.ResolveRef(context.Background(), "42", "")
	assertAPIError(t, err, "SESSION_REF_NOT_FOUND", "no session with id or number 42")
}

func TestResolveRefUnknownIDIsNotFound(t *testing.T) {
	f := newRefFixture(t)
	// Not a number, so it can only be a misspelled id.
	_, err := f.service.ResolveRef(context.Background(), "openagents-6", "")
	assertAPIError(t, err, "SESSION_REF_NOT_FOUND", "no session with id openagents-6")
}

func TestResolveRefBlankRefIsAUsageError(t *testing.T) {
	f := newRefFixture(t)
	_, err := f.service.ResolveRef(context.Background(), "   ", "")
	assertAPIError(t, err, "SESSION_REF_REQUIRED", "ref is required")
}

func TestResolveRefResolvesProjectlessSession(t *testing.T) {
	f := newRefFixture(t)
	// A projectless session keeps its own number space, so 9 is unambiguous.
	got, err := f.service.ResolveRef(context.Background(), "9", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.SessionID != "bare-9" || got.ProjectID != "" {
		t.Errorf("result = %+v, want bare-9 with no project", got)
	}
}

func TestResolveRefPropagatesStoreFailure(t *testing.T) {
	f := newRefFixture(t)
	// A storage failure is not an answer: it must not be reported as "no such
	// session", which would tell the user their agent does not exist.
	f.getSessionErr = errors.New("disk on fire")
	_, err := f.service.ResolveRef(context.Background(), "demo-7", "")
	if err == nil {
		t.Fatal("expected the store failure to propagate")
	}
	if !strings.Contains(err.Error(), "disk on fire") {
		t.Errorf("err = %v, want the storage cause", err)
	}
	var apiErr *apierr.Error
	if errors.As(err, &apiErr) {
		t.Errorf("err = %v, want a wrapped error rather than a user-facing 404", err)
	}
}

func assertAPIError(t *testing.T, err error, code, message string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected %s, got nil", code)
	}
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want an API error", err)
	}
	if apiErr.Code != code {
		t.Errorf("code = %q, want %q (err: %v)", apiErr.Code, code, err)
	}
	if apiErr.Message != message {
		t.Errorf("message = %q, want %q", apiErr.Message, message)
	}
}
