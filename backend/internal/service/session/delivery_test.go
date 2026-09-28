package session

import (
	"context"
	"errors"
	"testing"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/httpd/apierr"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

type fakeDelivery struct {
	mergeResult ports.LocalMergeResult
	mergeErr    error
	mergeCalls  []mergeCall
	pushErr     error
	pushCalls   []pushCall
}

type mergeCall struct {
	project domain.ProjectID
	branch  string
	target  string
}

type pushCall struct {
	project  domain.ProjectID
	worktree string
	branch   string
}

func (f *fakeDelivery) MergeSessionBranchLocal(_ context.Context, projectID domain.ProjectID, sessionBranch, targetBranch string) (ports.LocalMergeResult, error) {
	f.mergeCalls = append(f.mergeCalls, mergeCall{projectID, sessionBranch, targetBranch})
	return f.mergeResult, f.mergeErr
}

func (f *fakeDelivery) PushSessionBranch(_ context.Context, projectID domain.ProjectID, worktreePath, branch string) error {
	f.pushCalls = append(f.pushCalls, pushCall{projectID, worktreePath, branch})
	return f.pushErr
}

type fakePRCreator struct {
	findURL     string
	findNumber  int
	findFound   bool
	findErr     error
	findCalls   int
	created     ports.CreatedPullRequest
	createErr   error
	createRace  bool
	createCalls int
}

func (f *fakePRCreator) FindPRByHead(_ context.Context, _, _ string) (string, int, bool, error) {
	f.findCalls++
	return f.findURL, f.findNumber, f.findFound, f.findErr
}

func (f *fakePRCreator) CreatePR(_ context.Context, _, _, _, _, _ string) (ports.CreatedPullRequest, error) {
	f.createCalls++
	if f.createRace {
		// The race resolved between create and re-list: the second find sees it.
		f.findURL = "https://github.com/acme/repo/pull/7"
		f.findNumber = 7
		f.findFound = true
		return ports.CreatedPullRequest{}, ports.ErrGHPullRequestExists
	}
	return f.created, f.createErr
}

func deliverySession() domain.SessionRecord {
	return domain.SessionRecord{
		ID:        "sess-1",
		ProjectID: "proj-1",
		Kind:      domain.KindWorker,
		Metadata: domain.SessionMetadata{
			Branch:        "open-agents/sess-1",
			WorkspacePath: "/tmp/wt-sess-1",
		},
	}
}

func newDeliveryService(st *fakeStore, fc *fakeCommander, delivery *fakeDelivery, creator *fakePRCreator) *Service {
	return NewWithDeps(Deps{Manager: fc, Store: st, Delivery: delivery, PRCreator: creator})
}

func apierrCode(t *testing.T, err error) string {
	t.Helper()
	var apiErr *apierr.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want an apierr.Error", err)
	}
	return apiErr.Code
}

func TestMergeSessionLocal_SuccessTerminates(t *testing.T) {
	st := newFakeStore()
	st.sessions["sess-1"] = deliverySession()
	fc := &fakeCommander{}
	delivery := &fakeDelivery{mergeResult: ports.LocalMergeResult{TargetBranch: "dev", TargetHeadSHA: "abc123", BranchRemoved: true}}
	svc := newDeliveryService(st, fc, delivery, &fakePRCreator{})

	out, err := svc.MergeSessionLocal(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if len(delivery.mergeCalls) != 1 || delivery.mergeCalls[0].branch != "open-agents/sess-1" || delivery.mergeCalls[0].target != "dev" {
		t.Fatalf("merge calls = %+v, want one merge of the session branch into dev", delivery.mergeCalls)
	}
	if len(fc.killed) != 1 || fc.killed[0] != "sess-1" {
		t.Fatalf("killed = %v, want [sess-1]: success must terminate", fc.killed)
	}
	if out.TargetBranch != "dev" || out.TargetHeadSHA != "abc123" || !out.BranchRemoved {
		t.Errorf("outcome = %+v, want dev/abc123/removed", out)
	}
}

func TestMergeSessionLocal_DirtyNeverTerminates(t *testing.T) {
	st := newFakeStore()
	st.sessions["sess-1"] = deliverySession()
	fc := &fakeCommander{}
	delivery := &fakeDelivery{mergeErr: ports.ErrWorkspaceDirty}
	svc := newDeliveryService(st, fc, delivery, &fakePRCreator{})

	_, err := svc.MergeSessionLocal(context.Background(), "sess-1")
	if code := apierrCode(t, err); code != "WORKSPACE_DIRTY" {
		t.Fatalf("code = %q, want WORKSPACE_DIRTY", code)
	}
	if len(fc.killed) != 0 {
		t.Fatalf("killed = %v, want none: a failed merge must never terminate", fc.killed)
	}
}

func TestMergeSessionLocal_ConflictNeverTerminates(t *testing.T) {
	st := newFakeStore()
	st.sessions["sess-1"] = deliverySession()
	fc := &fakeCommander{}
	delivery := &fakeDelivery{mergeErr: ports.ErrDeliveryMergeConflict}
	svc := newDeliveryService(st, fc, delivery, &fakePRCreator{})

	_, err := svc.MergeSessionLocal(context.Background(), "sess-1")
	if code := apierrCode(t, err); code != "LOCAL_MERGE_CONFLICT" {
		t.Fatalf("code = %q, want LOCAL_MERGE_CONFLICT", code)
	}
	if len(fc.killed) != 0 {
		t.Fatalf("killed = %v, want none", fc.killed)
	}
}

func TestMergeSessionLocal_UnknownSession(t *testing.T) {
	svc := newDeliveryService(newFakeStore(), &fakeCommander{}, &fakeDelivery{}, &fakePRCreator{})
	_, err := svc.MergeSessionLocal(context.Background(), "nope")
	if code := apierrCode(t, err); code != "SESSION_NOT_FOUND" {
		t.Fatalf("code = %q, want SESSION_NOT_FOUND", code)
	}
}

func TestMergeSessionLocal_NoBranch(t *testing.T) {
	st := newFakeStore()
	rec := deliverySession()
	rec.Metadata.Branch = ""
	st.sessions["sess-1"] = rec
	svc := newDeliveryService(st, &fakeCommander{}, &fakeDelivery{}, &fakePRCreator{})
	_, err := svc.MergeSessionLocal(context.Background(), "sess-1")
	if code := apierrCode(t, err); code != "SESSION_BRANCH_UNKNOWN" {
		t.Fatalf("code = %q, want SESSION_BRANCH_UNKNOWN", code)
	}
}

func TestCreateSessionPR_Success(t *testing.T) {
	st := newFakeStore()
	st.sessions["sess-1"] = deliverySession()
	fc := &fakeCommander{}
	delivery := &fakeDelivery{}
	creator := &fakePRCreator{created: ports.CreatedPullRequest{URL: "https://github.com/acme/repo/pull/8", Number: 8, Created: true}}
	svc := newDeliveryService(st, fc, delivery, creator)

	out, err := svc.CreateSessionPR(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !out.Created || out.URL != "https://github.com/acme/repo/pull/8" || out.Number != 8 {
		t.Errorf("outcome = %+v, want created PR 8", out)
	}
	if len(delivery.pushCalls) != 1 || delivery.pushCalls[0].worktree != "/tmp/wt-sess-1" {
		t.Fatalf("push calls = %+v, want one push from the session worktree", delivery.pushCalls)
	}
	if creator.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", creator.createCalls)
	}
	if len(fc.killed) != 0 {
		t.Fatalf("killed = %v, want none: the remote button leaves the session alive", fc.killed)
	}
}

func TestCreateSessionPR_DurableDuplicateSkipsPushAndCreate(t *testing.T) {
	st := newFakeStore()
	st.sessions["sess-1"] = deliverySession()
	st.prs["sess-1"] = []domain.PullRequest{{
		URL: "https://github.com/acme/repo/pull/7", HTMLURL: "https://github.com/acme/repo/pull/7",
		Number: 7, SourceBranch: "open-agents/sess-1",
	}}
	delivery := &fakeDelivery{}
	creator := &fakePRCreator{}
	svc := newDeliveryService(st, &fakeCommander{}, delivery, creator)

	out, err := svc.CreateSessionPR(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.Created || out.URL != "https://github.com/acme/repo/pull/7" {
		t.Errorf("outcome = %+v, want existing PR 7 uncreated", out)
	}
	if len(delivery.pushCalls) != 0 || creator.createCalls != 0 || creator.findCalls != 0 {
		t.Errorf("duplicate must skip push/create/list (push=%d create=%d find=%d)",
			len(delivery.pushCalls), creator.createCalls, creator.findCalls)
	}
}

func TestCreateSessionPR_RemoteDuplicateSkipsPushAndCreate(t *testing.T) {
	st := newFakeStore()
	st.sessions["sess-1"] = deliverySession()
	delivery := &fakeDelivery{}
	creator := &fakePRCreator{findURL: "https://github.com/acme/repo/pull/7", findNumber: 7, findFound: true}
	svc := newDeliveryService(st, &fakeCommander{}, delivery, creator)

	out, err := svc.CreateSessionPR(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.Created || out.URL != "https://github.com/acme/repo/pull/7" {
		t.Errorf("outcome = %+v, want existing PR 7 uncreated", out)
	}
	if len(delivery.pushCalls) != 0 || creator.createCalls != 0 {
		t.Errorf("duplicate must skip push/create (push=%d create=%d)", len(delivery.pushCalls), creator.createCalls)
	}
}

func TestCreateSessionPR_CreateRaceReturnsExisting(t *testing.T) {
	st := newFakeStore()
	st.sessions["sess-1"] = deliverySession()
	delivery := &fakeDelivery{}
	creator := &fakePRCreator{createRace: true}
	svc := newDeliveryService(st, &fakeCommander{}, delivery, creator)

	out, err := svc.CreateSessionPR(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if out.Created || out.URL != "https://github.com/acme/repo/pull/7" {
		t.Errorf("outcome = %+v, want the raced PR 7, not an error", out)
	}
}

func TestCreateSessionPR_AuthMissingBeforePush(t *testing.T) {
	st := newFakeStore()
	st.sessions["sess-1"] = deliverySession()
	delivery := &fakeDelivery{}
	creator := &fakePRCreator{findErr: ports.ErrGHAuthMissing}
	svc := newDeliveryService(st, &fakeCommander{}, delivery, creator)

	_, err := svc.CreateSessionPR(context.Background(), "sess-1")
	if code := apierrCode(t, err); code != "GH_AUTH_MISSING" {
		t.Fatalf("code = %q, want GH_AUTH_MISSING", code)
	}
	if len(delivery.pushCalls) != 0 {
		t.Fatalf("push calls = %d, want 0: auth is checked before mutating", len(delivery.pushCalls))
	}
}

func TestCreateSessionPR_PushRejectedSkipsCreate(t *testing.T) {
	st := newFakeStore()
	st.sessions["sess-1"] = deliverySession()
	delivery := &fakeDelivery{pushErr: ports.ErrDeliveryPushRejected}
	creator := &fakePRCreator{}
	svc := newDeliveryService(st, &fakeCommander{}, delivery, creator)

	_, err := svc.CreateSessionPR(context.Background(), "sess-1")
	if code := apierrCode(t, err); code != "PUSH_REJECTED" {
		t.Fatalf("code = %q, want PUSH_REJECTED", code)
	}
	if creator.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", creator.createCalls)
	}
}

// autoDeliverySession is a session that qualifies for automatic delivery: a
// live worker in building mode with a branch and a workspace.
func autoDeliverySession() domain.SessionRecord {
	rec := deliverySession()
	rec.WorkflowMode = domain.WorkflowModeBuilding
	return rec
}

func TestDeliverSessionHead_CommitsAndRecordsHead(t *testing.T) {
	st := newFakeStore()
	st.sessions["sess-1"] = autoDeliverySession()
	delivery := &fakeDelivery{}
	creator := &fakePRCreator{created: ports.CreatedPullRequest{URL: "https://github.com/acme/repo/pull/8", Number: 8, Created: true}}
	svc := newDeliveryService(st, &fakeCommander{}, delivery, creator)

	out, err := svc.DeliverSessionHead(context.Background(), "sess-1", "head1")
	if err != nil {
		t.Fatalf("deliver head: %v", err)
	}
	if !out.Delivered || out.URL != "https://github.com/acme/repo/pull/8" {
		t.Errorf("outcome = %+v, want delivered PR 8", out)
	}
	if len(delivery.pushCalls) != 1 {
		t.Fatalf("push calls = %d, want 1: the commit must be handed to the remote", len(delivery.pushCalls))
	}
	if got := st.sessions["sess-1"].DeliveredHeadSHA; got != "head1" {
		t.Errorf("DeliveredHeadSHA = %q, want head1: a delivered commit must be remembered", got)
	}
}

// The same head observed twice must not push twice. This is the property that
// makes polling safe at all.
func TestDeliverSessionHead_AlreadyDeliveredHeadSkipsEverything(t *testing.T) {
	st := newFakeStore()
	rec := autoDeliverySession()
	rec.DeliveredHeadSHA = "head1"
	st.sessions["sess-1"] = rec
	delivery := &fakeDelivery{}
	creator := &fakePRCreator{}
	svc := newDeliveryService(st, &fakeCommander{}, delivery, creator)

	out, err := svc.DeliverSessionHead(context.Background(), "sess-1", "head1")
	if err != nil {
		t.Fatalf("deliver head: %v", err)
	}
	if out.Delivered || out.Reason != autoReasonNoChange {
		t.Errorf("outcome = %+v, want skip %q", out, autoReasonNoChange)
	}
	if len(delivery.pushCalls) != 0 || creator.createCalls != 0 {
		t.Errorf("a known commit must not push (push=%d create=%d)", len(delivery.pushCalls), creator.createCalls)
	}
}

func TestDeliverSessionHead_FailedDeliveryStaysRetryable(t *testing.T) {
	st := newFakeStore()
	st.sessions["sess-1"] = autoDeliverySession()
	delivery := &fakeDelivery{pushErr: errors.New("origin unreachable")}
	creator := &fakePRCreator{}
	svc := newDeliveryService(st, &fakeCommander{}, delivery, creator)

	if _, err := svc.DeliverSessionHead(context.Background(), "sess-1", "head1"); err == nil {
		t.Fatal("deliver head succeeded, want a push failure")
	}
	if got := st.sessions["sess-1"].DeliveredHeadSHA; got != "" {
		t.Errorf("DeliveredHeadSHA = %q, want empty: a failed attempt must stay retryable", got)
	}
}

// A recorded head that is lost in storage must not cost the user the pull
// request they already have.
func TestDeliverSessionHead_LostFactStillReportsDeliveredPR(t *testing.T) {
	st := newFakeStore()
	st.sessions["sess-1"] = autoDeliverySession()
	st.setDeliveredHeadErr = errors.New("disk full")
	creator := &fakePRCreator{created: ports.CreatedPullRequest{URL: "https://github.com/acme/repo/pull/8", Number: 8, Created: true}}
	svc := newDeliveryService(st, &fakeCommander{}, &fakeDelivery{}, creator)

	out, err := svc.DeliverSessionHead(context.Background(), "sess-1", "head1")
	if err == nil {
		t.Fatal("err = nil, want the failed durable write reported")
	}
	if !out.Delivered || out.URL != "https://github.com/acme/repo/pull/8" {
		t.Errorf("outcome = %+v, want the real PR reported alongside the error", out)
	}
}

func TestDeliverSessionHead_UnknownSessionIsSkipped(t *testing.T) {
	delivery := &fakeDelivery{}
	svc := newDeliveryService(newFakeStore(), &fakeCommander{}, delivery, &fakePRCreator{})
	out, err := svc.DeliverSessionHead(context.Background(), "nope", "head1")
	if err != nil {
		t.Fatalf("deliver head: %v", err)
	}
	if out.Delivered || out.Reason != autoReasonNotEligible {
		t.Errorf("outcome = %+v, want a skip", out)
	}
	if len(delivery.pushCalls) != 0 {
		t.Errorf("push calls = %d, want 0 for an unknown session", len(delivery.pushCalls))
	}
}

func TestDeliverSessionHead_NoCommitSkips(t *testing.T) {
	st := newFakeStore()
	st.sessions["sess-1"] = autoDeliverySession()
	delivery := &fakeDelivery{}
	svc := newDeliveryService(st, &fakeCommander{}, delivery, &fakePRCreator{})

	out, err := svc.DeliverSessionHead(context.Background(), "sess-1", "  ")
	if err != nil {
		t.Fatalf("deliver head: %v", err)
	}
	if out.Delivered || out.Reason != autoReasonNoCommit {
		t.Errorf("outcome = %+v, want skip %q", out, autoReasonNoCommit)
	}
	if len(delivery.pushCalls) != 0 {
		t.Errorf("push calls = %d, want 0 with no commit", len(delivery.pushCalls))
	}
}

func TestEligibleForAutoDelivery(t *testing.T) {
	openPR := []domain.PRFacts{{URL: "u", Closed: false, Merged: false}}
	mergedPR := []domain.PRFacts{{URL: "u", Closed: false, Merged: true}}
	cases := []struct {
		name  string
		mut   func(*domain.SessionRecord)
		prs   []domain.PRFacts
		want  bool
		whyOK string
	}{
		{name: "building worker with branch and workspace", want: true},
		{name: "terminated", mut: func(r *domain.SessionRecord) { r.IsTerminated = true }},
		{name: "manager kind", mut: func(r *domain.SessionRecord) { r.Kind = domain.KindManager }},
		{name: "planning mode", mut: func(r *domain.SessionRecord) { r.WorkflowMode = domain.WorkflowModePlanning }},
		{name: "manager workflow mode", mut: func(r *domain.SessionRecord) { r.WorkflowMode = domain.WorkflowModeManager }},
		{name: "no branch", mut: func(r *domain.SessionRecord) { r.Metadata.Branch = "" }},
		{name: "no workspace", mut: func(r *domain.SessionRecord) { r.Metadata.WorkspacePath = "" }},
		{name: "already has an open PR", prs: openPR},
		{name: "terminal PR does not block", prs: mergedPR, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := autoDeliverySession()
			if tc.mut != nil {
				tc.mut(&rec)
			}
			got, reason := EligibleForAutoDelivery(rec, tc.prs)
			if got != tc.want {
				t.Fatalf("eligible = %v (reason %q), want %v", got, reason, tc.want)
			}
			if got && reason != "" {
				t.Errorf("eligible session carries reason %q, want empty", reason)
			}
			if !got && reason == "" {
				t.Error("ineligible session must say why")
			}
		})
	}
}
