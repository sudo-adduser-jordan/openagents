package store_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
)

// The head observer has to survive a daemon restart without re-pushing a commit
// it already delivered, so "the commit the daemon handed to the remote" is a
// durable fact rather than in-memory observer state.
func TestSessionRoundTripsDeliveredHeadSHA(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "delivered")

	created, err := s.CreateSession(ctx, sampleRecord("delivered"))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if created.DeliveredHeadSHA != "" {
		t.Errorf("a new session reports DeliveredHeadSHA=%q, want empty: nothing has been delivered yet", created.DeliveredHeadSHA)
	}

	ok, err := s.SetSessionDeliveredHeadSHA(ctx, created.ID, "abc123", time.Unix(1000, 0).UTC())
	if err != nil {
		t.Fatalf("set delivered head: %v", err)
	}
	if !ok {
		t.Fatal("SetSessionDeliveredHeadSHA reported no row for an existing session")
	}

	got, found, err := s.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if !found {
		t.Fatalf("GetSession did not find %s", created.ID)
	}
	if got.DeliveredHeadSHA != "abc123" {
		t.Errorf("GetSession DeliveredHeadSHA=%q, want abc123", got.DeliveredHeadSHA)
	}

	all, err := s.ListAllSessions(ctx)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 1 || all[0].DeliveredHeadSHA != "abc123" {
		t.Errorf("ListAllSessions = %+v, want the delivered head to survive the list path too", all)
	}
}

// The focused write exists precisely so a full-record save cannot clear a real
// delivery. A record read before the fact was observed carries a stale empty
// value; replaying it must not erase the record of a delivered commit.
func TestFullRecordUpdateDoesNotClearDeliveredHeadSHA(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "stale-replay")

	created, err := s.CreateSession(ctx, sampleRecord("stale-replay"))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := s.SetSessionDeliveredHeadSHA(ctx, created.ID, "abc123", time.Unix(1000, 0).UTC()); err != nil {
		t.Fatalf("set delivered head: %v", err)
	}

	// stale is the record as it looked before the delivery.
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
	if got.DeliveredHeadSHA != "abc123" {
		t.Errorf("DeliveredHeadSHA=%q after a full-record update, want abc123 to survive", got.DeliveredHeadSHA)
	}
}

func TestSetSessionDeliveredHeadSHAUnknownSession(t *testing.T) {
	s := newTestStore(t)
	ok, err := s.SetSessionDeliveredHeadSHA(context.Background(), "nope", "abc123", time.Now().UTC())
	if err != nil {
		t.Fatalf("set delivered head: %v", err)
	}
	if ok {
		t.Error("ok = true for an unknown session, want false")
	}
}

// The fact is internal bookkeeping and must not leak into the API read model.
func TestDeliveredHeadSHAIsNotSerialized(t *testing.T) {
	encoded, err := json.Marshal(domain.SessionRecord{ID: "s1", DeliveredHeadSHA: "abc123"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, leaked := decoded["DeliveredHeadSHA"]; leaked {
		t.Errorf("DeliveredHeadSHA is exposed on the wire: %s", encoded)
	}
}
