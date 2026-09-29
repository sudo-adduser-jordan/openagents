package head

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

type fakeStore struct {
	sessions []domain.SessionRecord
	err      error
}

func (f *fakeStore) ListAllSessions(context.Context) ([]domain.SessionRecord, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]domain.SessionRecord(nil), f.sessions...), nil
}

type fakeDeliverer struct {
	outcomes map[string]Outcome
	errs     map[string]error
	calls    []string
}

func (f *fakeDeliverer) DeliverSessionHead(_ context.Context, id domain.SessionID, headSHA string) (Outcome, error) {
	f.calls = append(f.calls, string(id)+"@"+headSHA)
	if err, ok := f.errs[string(id)]; ok {
		return Outcome{}, err
	}
	return f.outcomes[string(id)], nil
}

type fakeWorkspaces struct {
	heads      map[string]string
	observeErr map[string]error
	calls      []string
	infos      []ports.WorkspaceInfo
}

func (f *fakeWorkspaces) ObserveWorkspace(_ context.Context, info ports.WorkspaceInfo) (ports.WorkspaceObservation, error) {
	f.calls = append(f.calls, info.Path)
	f.infos = append(f.infos, info)
	if err, ok := f.observeErr[info.Path]; ok {
		return ports.WorkspaceObservation{}, err
	}
	return ports.WorkspaceObservation{Path: info.Path, HeadSHA: f.heads[info.Path]}, nil
}

func worker(id, worktree, headSHA string) domain.SessionRecord {
	return domain.SessionRecord{
		ID:               domain.SessionID(id),
		ProjectID:        "proj-1",
		Kind:             domain.KindWorker,
		WorkflowMode:     domain.WorkflowModeBuilding,
		PlanApproved:     true,
		Metadata:         domain.SessionMetadata{Branch: "open-agents/" + id, WorkspacePath: worktree},
		DeliveredHeadSHA: headSHA,
	}
}

func newTestObserver(store *fakeStore, d *fakeDeliverer, ws *fakeWorkspaces, now *time.Time) *Observer {
	return New(store, d, ws, Config{
		Clock:          func() time.Time { return *now },
		FailureBackoff: time.Minute,
	})
}

func TestPoll_DeliversNewCommit(t *testing.T) {
	now := time.Now()
	store := &fakeStore{sessions: []domain.SessionRecord{worker("s1", "/wt/s1", "old")}}
	d := &fakeDeliverer{outcomes: map[string]Outcome{"s1": {Delivered: true, URL: "u"}}}
	ws := &fakeWorkspaces{heads: map[string]string{"/wt/s1": "new"}}

	if err := newTestObserver(store, d, ws, &now).Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(d.calls) != 1 || d.calls[0] != "s1@new" {
		t.Fatalf("delivery calls = %v, want one call for the new commit", d.calls)
	}
}

// The observer must route the read through the session's project: without the
// project id the workspace router resolves the scratch adapter, which has no
// HEAD to report, and every poll silently skips delivery.
func TestPoll_ForwardsProjectIDToWorkspaceObservation(t *testing.T) {
	now := time.Now()
	store := &fakeStore{sessions: []domain.SessionRecord{worker("s1", "/wt/s1", "old")}}
	d := &fakeDeliverer{outcomes: map[string]Outcome{"s1": {Delivered: true, URL: "u"}}}
	ws := &fakeWorkspaces{heads: map[string]string{"/wt/s1": "new"}}

	if err := newTestObserver(store, d, ws, &now).Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(ws.infos) != 1 {
		t.Fatalf("workspace observations = %d, want 1", len(ws.infos))
	}
	if ws.infos[0].ProjectID != domain.ProjectID("proj-1") || ws.infos[0].Branch != "open-agents/s1" {
		t.Fatalf("workspace info = %+v, want the session project and branch", ws.infos[0])
	}
}

func TestPoll_SkipsAlreadyDeliveredCommit(t *testing.T) {
	now := time.Now()
	store := &fakeStore{sessions: []domain.SessionRecord{worker("s1", "/wt/s1", "same")}}
	d := &fakeDeliverer{}
	ws := &fakeWorkspaces{heads: map[string]string{"/wt/s1": "same"}}

	if err := newTestObserver(store, d, ws, &now).Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(d.calls) != 0 {
		t.Fatalf("delivery calls = %v, want none: the commit was already delivered", d.calls)
	}
	// The worktree is still read -- the head cannot be known without asking git --
	// but the durable fact is what turns that reading into a no-op.
	if len(ws.calls) != 1 {
		t.Errorf("workspace calls = %v, want one read of the worktree", ws.calls)
	}
}

func TestPoll_FailureBacksOffThenRetriesAfterWindow(t *testing.T) {
	now := time.Now()
	store := &fakeStore{sessions: []domain.SessionRecord{worker("s1", "/wt/s1", "")}}
	d := &fakeDeliverer{errs: map[string]error{"s1": errors.New("origin down")}}
	ws := &fakeWorkspaces{heads: map[string]string{"/wt/s1": "new"}}
	o := newTestObserver(store, d, ws, &now)

	if err := o.Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(d.calls) != 1 {
		t.Fatalf("delivery calls = %d, want 1 on the first pass", len(d.calls))
	}
	// Still inside the backoff window: no second attempt.
	if err := o.Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(d.calls) != 1 {
		t.Fatalf("delivery calls = %d, want the backoff to suppress a retry", len(d.calls))
	}
	// Past the window: the same commit is retried, because the fact never stuck.
	now = now.Add(2 * time.Minute)
	if err := o.Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(d.calls) != 2 {
		t.Fatalf("delivery calls = %d, want a retry after the backoff", len(d.calls))
	}
}

func TestPoll_UnreadableWorkspaceIsNotAFailure(t *testing.T) {
	now := time.Now()
	store := &fakeStore{sessions: []domain.SessionRecord{worker("s1", "/wt/s1", "")}}
	d := &fakeDeliverer{}
	ws := &fakeWorkspaces{observeErr: map[string]error{"/wt/s1": errors.New("worktree gone")}}
	o := newTestObserver(store, d, ws, &now)

	if err := o.Poll(context.Background()); err != nil {
		t.Fatalf("poll returned %v, want an unreadable worktree to be skipped silently", err)
	}
	if _, backed := o.backoffUntil["s1"]; backed {
		t.Error("an unreadable worktree armed the failure backoff; it must retry freely")
	}
}

func TestPoll_SuccessClearsBackoff(t *testing.T) {
	now := time.Now()
	store := &fakeStore{sessions: []domain.SessionRecord{worker("s1", "/wt/s1", "")}}
	d := &fakeDeliverer{
		outcomes: map[string]Outcome{"s1": {Delivered: true}},
		errs:     map[string]error{"s1": errors.New("transient")},
	}
	ws := &fakeWorkspaces{heads: map[string]string{"/wt/s1": "new"}}
	o := newTestObserver(store, d, ws, &now)

	_ = o.Poll(context.Background())
	if _, ok := o.backoffUntil["s1"]; !ok {
		t.Fatal("want a backoff after the failure")
	}
	delete(d.errs, "s1")
	now = now.Add(2 * time.Minute)
	_ = o.Poll(context.Background())
	if _, ok := o.backoffUntil["s1"]; ok {
		t.Error("a successful delivery must clear the backoff")
	}
}

// The observer must not treat a session it cannot possibly deliver as work: the
// pre-filter is cheap, and the authoritative gate lives in the delivery policy.
func TestPoll_SkipsNonEligibleSessionsWithoutObserving(t *testing.T) {
	now := time.Now()
	planning := worker("s1", "/wt/s1", "")
	planning.WorkflowMode = domain.WorkflowModePlanning
	manager := worker("s2", "/wt/s2", "")
	manager.Kind = domain.KindManager
	dead := worker("s3", "/wt/s3", "")
	dead.IsTerminated = true
	store := &fakeStore{sessions: []domain.SessionRecord{planning, manager, dead}}
	d := &fakeDeliverer{}
	ws := &fakeWorkspaces{heads: map[string]string{"/wt/s1": "a", "/wt/s2": "b", "/wt/s3": "c"}}

	if err := newTestObserver(store, d, ws, &now).Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(ws.calls) != 0 || len(d.calls) != 0 {
		t.Fatalf("workspace calls = %v, delivery calls = %v, want none", ws.calls, d.calls)
	}
}

// A building-stage worker commit with no recorded plan approval is not work
// the observer should even read a worktree for: the authoritative gate lives in
// the delivery policy, and this pre-filter only avoids the git call.
func TestPoll_SkipsUnapprovedPlanWithoutObserving(t *testing.T) {
	now := time.Now()
	unapproved := worker("s1", "/wt/s1", "")
	unapproved.PlanApproved = false
	store := &fakeStore{sessions: []domain.SessionRecord{unapproved}}
	d := &fakeDeliverer{}
	ws := &fakeWorkspaces{heads: map[string]string{"/wt/s1": "new"}}

	if err := newTestObserver(store, d, ws, &now).Poll(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	if len(ws.calls) != 0 || len(d.calls) != 0 {
		t.Fatalf("workspace calls = %v, delivery calls = %v, want none", ws.calls, d.calls)
	}
}

func TestPoll_StoreFailurePropagates(t *testing.T) {
	now := time.Now()
	o := newTestObserver(&fakeStore{err: errors.New("db down")}, &fakeDeliverer{}, &fakeWorkspaces{}, &now)
	if err := o.Poll(context.Background()); err == nil {
		t.Fatal("err = nil, want the store failure surfaced to the poll loop")
	}
}

func TestPoll_CancelledContextStops(t *testing.T) {
	now := time.Now()
	o := newTestObserver(&fakeStore{}, &fakeDeliverer{}, &fakeWorkspaces{}, &now)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := o.Poll(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestStart_ReturnsOpenChannelThenClosesOnCancel(t *testing.T) {
	now := time.Now()
	store := &fakeStore{sessions: []domain.SessionRecord{worker("s1", "/wt/s1", "")}}
	d := &fakeDeliverer{outcomes: map[string]Outcome{"s1": {Delivered: true}}}
	ws := &fakeWorkspaces{heads: map[string]string{"/wt/s1": "new"}}
	o := New(store, d, ws, Config{Tick: time.Hour, Clock: func() time.Time { return now }})

	ctx, cancel := context.WithCancel(context.Background())
	done := o.Start(ctx)
	select {
	case <-done:
		t.Fatal("observer loop exited immediately; it did not start")
	default:
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("observer loop did not stop after cancel")
	}
}

func TestNew_AppliesDefaults(t *testing.T) {
	o := New(&fakeStore{}, &fakeDeliverer{}, &fakeWorkspaces{}, Config{})
	if o.tick != DefaultTickInterval || o.failureBackoff != DefaultFailureBackoff {
		t.Errorf("tick = %v, backoff = %v, want production defaults", o.tick, o.failureBackoff)
	}
	if o.clock == nil || o.logger == nil || o.backoffUntil == nil {
		t.Error("New left a dependency nil; a nil map would panic on first backoff")
	}
}

// A nil wiring must be inert rather than a panic, so a partially configured
// daemon still starts.
func TestPoll_NilDependenciesAreInert(t *testing.T) {
	if err := New(nil, nil, nil, Config{}).Poll(context.Background()); err != nil {
		t.Fatalf("err = %v, want nil for unwired dependencies", err)
	}
}
