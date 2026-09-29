package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
)

// Plan approval is the stage-discipline review gate: the planning-to-building
// transition records that the plan was reviewed while uncommitted, and only
// then may the head observer hand a commit to the remote.
func TestSessionWorkflowModeRoundTripsPlanApproved(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "approved")

	created, err := s.CreateSession(ctx, sampleRecord("approved"))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if created.PlanApproved {
		t.Error("a new worker reports PlanApproved=true, want false: nothing has been reviewed yet")
	}

	ok, err := s.SetSessionWorkflowMode(ctx, created.ID, domain.WorkflowModeBuilding, time.Unix(1000, 0).UTC())
	if err != nil {
		t.Fatalf("set workflow mode: %v", err)
	}
	if !ok {
		t.Fatal("SetSessionWorkflowMode reported no row for an existing session")
	}
	got, found, err := s.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if !found {
		t.Fatalf("GetSession did not find %s", created.ID)
	}
	if !got.PlanApproved {
		t.Error("PlanApproved=false after planning->building, want the transition to record the approval")
	}

	// Sending the worker back to planning revokes the approval.
	ok, err = s.SetSessionWorkflowMode(ctx, created.ID, domain.WorkflowModePlanning, time.Unix(2000, 0).UTC())
	if err != nil {
		t.Fatalf("set workflow mode: %v", err)
	}
	if !ok {
		t.Fatal("SetSessionWorkflowMode reported no row for an existing session")
	}
	got, _, err = s.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("re-get session: %v", err)
	}
	if got.PlanApproved {
		t.Error("PlanApproved=true after building->planning, want the approval revoked")
	}

	all, err := s.ListAllSessions(ctx)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 1 || all[0].PlanApproved {
		t.Errorf("ListAllSessions = %+v, want the revocation to survive the list path too", all)
	}
}

// A manager never leaves its own stage, so no stage-derived approval exists
// for it.
func TestSessionWorkflowModeManagerStaysUnapproved(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "mgr")
	rec := sampleRecord("mgr")
	rec.Kind = domain.KindManager
	created, err := s.CreateSession(ctx, rec)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	ok, err := s.SetSessionWorkflowMode(ctx, created.ID, domain.WorkflowModeManager, time.Now().UTC())
	if err != nil {
		t.Fatalf("set workflow mode: %v", err)
	}
	if !ok {
		t.Fatal("SetSessionWorkflowMode reported no row for an existing session")
	}
	got, _, err := s.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.PlanApproved {
		t.Error("a manager reports PlanApproved=true, want false: managers have no plan/build posture")
	}
}

// The focused write exists precisely so a full-record save cannot clear a real
// approval. A record read before the transition carries a stale false value;
// replaying it must not erase the review.
func TestFullRecordUpdateDoesNotClearPlanApproved(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "stale-approval")

	created, err := s.CreateSession(ctx, sampleRecord("stale-approval"))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := s.SetSessionWorkflowMode(ctx, created.ID, domain.WorkflowModeBuilding, time.Unix(1000, 0).UTC()); err != nil {
		t.Fatalf("set workflow mode: %v", err)
	}

	// stale is the record as it looked before the approval.
	stale := created
	stale.DisplayName = "renamed by a stale writer"
	if err := s.UpdateSession(ctx, stale); err != nil {
		t.Fatalf("update session: %v", err)
	}

	got, _, err := s.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("re-get session: %v", err)
	}
	if got.DisplayName != "renamed by a stale writer" {
		t.Errorf("DisplayName=%q, want the update to have applied", got.DisplayName)
	}
	if !got.PlanApproved {
		t.Error("PlanApproved=false after a full-record update, want true to survive")
	}
}

func TestSetSessionWorkflowModeUnknownSessionPlanApproved(t *testing.T) {
	s := newTestStore(t)
	ok, err := s.SetSessionWorkflowMode(context.Background(), "nope", domain.WorkflowModeBuilding, time.Now().UTC())
	if err != nil {
		t.Fatalf("set workflow mode: %v", err)
	}
	if ok {
		t.Error("ok = true for an unknown session, want false")
	}
}

// The fact is internal bookkeeping and must not leak into the API read model.
func TestPlanApprovedIsNotSerialized(t *testing.T) {
	encoded, err := json.Marshal(domain.SessionRecord{ID: "s1", PlanApproved: true})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, leaked := decoded["PlanApproved"]; leaked {
		t.Errorf("PlanApproved is exposed on the wire: %s", encoded)
	}
	if _, leaked := decoded["planApproved"]; leaked {
		t.Errorf("PlanApproved is exposed on the wire: %s", encoded)
	}
}
