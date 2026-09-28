package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/storage/sqlite"
)

// refStore seeds projects so the addressing lookups run against the real schema,
// including the cross-project number collision that UNIQUE (project_id, num)
// allows and a single project forbids.
func refStore(t *testing.T) *sqlite.Store {
	t.Helper()
	s := newTestStore(t)
	for _, project := range []string{"alpha", "beta"} {
		seedProject(t, s, project)
	}
	return s
}

// seedRefSession creates a session and returns it with its assigned number,
// which is read back through the addressing lookup because a session record
// deliberately carries no number.
func seedRefSession(t *testing.T, s *sqlite.Store, rec domain.SessionRecord) (domain.SessionRecord, int64) {
	t.Helper()
	ctx := context.Background()
	session, err := s.CreateSession(ctx, rec)
	if err != nil {
		t.Fatalf("CreateSession(%s): %v", rec.ID, err)
	}
	ref, ok, err := s.GetSessionRef(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSessionRef(%s): %v", session.ID, err)
	}
	if !ok {
		t.Fatalf("GetSessionRef(%s) missed a session that was just created", session.ID)
	}
	if ref.Num <= 0 {
		t.Fatalf("num = %d for %s, want a positive agent number", ref.Num, session.ID)
	}
	return session, ref.Num
}

func alphaRecord() domain.SessionRecord { return sampleRecord("alpha") }

func TestGetSessionRefReturnsTheAddressingTriple(t *testing.T) {
	ctx := context.Background()
	s := refStore(t)
	session, num := seedRefSession(t, s, alphaRecord())

	got, ok, err := s.GetSessionRef(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetSessionRef: %v", err)
	}
	if !ok {
		t.Fatal("GetSessionRef reported a miss for a session that exists")
	}
	want := domain.SessionNumRef{ID: session.ID, ProjectID: "alpha", Num: num}
	if got != want {
		t.Errorf("ref = %+v, want %+v", got, want)
	}
}

func TestGetSessionRefMissesAnUnknownID(t *testing.T) {
	s := refStore(t)
	_, ok, err := s.GetSessionRef(context.Background(), "alpha-nope")
	if err != nil {
		t.Fatalf("GetSessionRef: %v", err)
	}
	// A miss must stay a miss: a caller that ignored the flag would otherwise
	// address the zero session.
	if ok {
		t.Error("GetSessionRef reported a hit for an unknown id")
	}
}

func TestListSessionRefsByNumReturnsEveryProjectHoldingTheNumber(t *testing.T) {
	ctx := context.Background()
	s := refStore(t)
	alpha, alphaNum := seedRefSession(t, s, alphaRecord())
	beta, betaNum := seedRefSession(t, s, sampleRecord("beta"))
	if betaNum != alphaNum {
		t.Fatalf("fixture assumption broken: beta num = %d, alpha num = %d", betaNum, alphaNum)
	}

	refs, err := s.ListSessionRefsByNum(ctx, alphaNum)
	if err != nil {
		t.Fatalf("ListSessionRefsByNum: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("refs = %+v, want both projects holding num %d", refs, alphaNum)
	}
	// Stable order, so a collision is always reported the same way.
	want := []domain.SessionNumRef{
		{ID: alpha.ID, ProjectID: "alpha", Num: alphaNum},
		{ID: beta.ID, ProjectID: "beta", Num: betaNum},
	}
	if refs[0] != want[0] || refs[1] != want[1] {
		t.Errorf("refs = %+v, want %+v", refs, want)
	}
}

func TestListSessionRefsByNumMissesAFreeNumber(t *testing.T) {
	s := refStore(t)
	refs, err := s.ListSessionRefsByNum(context.Background(), 9999)
	if err != nil {
		t.Fatalf("ListSessionRefsByNum: %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("refs = %+v, want none", refs)
	}
}

func TestListRetiredSessionNumProjectsReportsEveryRetiringProject(t *testing.T) {
	ctx := context.Background()
	s := refStore(t)
	alpha, alphaNum := seedRefSession(t, s, alphaRecord())
	beta, _ := seedRefSession(t, s, sampleRecord("beta"))
	retireRefSession(t, s, alpha)
	retireRefSession(t, s, beta)

	projects, err := s.ListRetiredSessionNumProjects(ctx, alphaNum)
	if err != nil {
		t.Fatalf("ListRetiredSessionNumProjects: %v", err)
	}
	want := []domain.ProjectID{"alpha", "beta"}
	if len(projects) != len(want) || projects[0] != want[0] || projects[1] != want[1] {
		t.Errorf("projects = %v, want %v", projects, want)
	}

}

func TestListRetiredSessionNumProjectsReportsTheProjectlessScope(t *testing.T) {
	ctx := context.Background()
	s := refStore(t)
	rec := sampleRecord("")
	rec.ID = "bare-1"
	rec.Metadata = domain.SessionMetadata{WorkspacePath: "/ws/bare"}
	session, num := seedRefSession(t, s, rec)
	retireRefSession(t, s, session)

	projects, err := s.ListRetiredSessionNumProjects(ctx, num)
	if err != nil {
		t.Fatalf("ListRetiredSessionNumProjects: %v", err)
	}
	// A projectless session retires its number under the empty project, which is
	// how the resolver recognizes the standalone namespace.
	if len(projects) != 1 || projects[0] != "" {
		t.Errorf("projects = %#v, want a single empty project", projects)
	}
}

func TestListRetiredSessionNumProjectsMissesALiveNumber(t *testing.T) {
	s := refStore(t)
	_, num := seedRefSession(t, s, alphaRecord())
	projects, err := s.ListRetiredSessionNumProjects(context.Background(), num)
	if err != nil {
		t.Fatalf("ListRetiredSessionNumProjects: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("projects = %v, want none for a live number", projects)
	}
}

func retireRefSession(t *testing.T, s *sqlite.Store, session domain.SessionRecord) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	session.IsTerminated = true
	session.Activity.State = domain.ActivityExited
	session.UpdatedAt = now
	if err := s.UpdateSession(ctx, session); err != nil {
		t.Fatalf("UpdateSession(%s): %v", session.ID, err)
	}
	if _, err := s.RetireSession(ctx, session.ID, now); err != nil {
		t.Fatalf("RetireSession(%s): %v", session.ID, err)
	}
}
