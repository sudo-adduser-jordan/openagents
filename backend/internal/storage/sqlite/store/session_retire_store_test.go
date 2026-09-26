package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/cdc"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/storage/sqlite"
)

func retireFixture(t *testing.T) (*sqlite.Store, domain.SessionRecord) {
	t.Helper()
	ctx := context.Background()
	s := newTestStore(t)
	seedProject(t, s, "mer")
	rec := sampleRecord("mer")
	session, err := s.CreateSession(ctx, rec)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	session.IsTerminated = true
	session.Activity.State = domain.ActivityExited
	session.UpdatedAt = time.Now().UTC()
	if err := s.UpdateSession(ctx, session); err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}
	return s, session
}

func TestRetireSessionRemovesATerminatedRow(t *testing.T) {
	ctx := context.Background()
	s, session := retireFixture(t)

	removed, err := s.RetireSession(ctx, session.ID, time.Now().UTC())
	if err != nil {
		t.Fatalf("RetireSession: %v", err)
	}
	if !removed {
		t.Fatal("RetireSession removed nothing for a terminated session")
	}
	if _, ok, err := s.GetSession(ctx, session.ID); err != nil || ok {
		t.Fatalf("session still present after retiring: ok=%v err=%v", ok, err)
	}
}

// A retired number must never come back. A reused id would silently re-point a
// registered worktree path, a PR conversation, or a change_log row at an
// unrelated session. The hazard is retiring the *highest* number: that is the
// one MAX(num)+1 would hand straight back out.
func TestRetiredSessionNumberIsNotReused(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedProject(t, s, "mer")
	first, err := s.CreateSession(ctx, sampleRecord("mer"))
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	highest, err := s.CreateSession(ctx, sampleRecord("mer"))
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if highest.ID == first.ID {
		t.Fatalf("two sessions share the id %q", first.ID)
	}
	retiredID := highest.ID

	highest.IsTerminated = true
	highest.UpdatedAt = time.Now().UTC()
	if err := s.UpdateSession(ctx, highest); err != nil {
		t.Fatalf("UpdateSession: %v", err)
	}
	if _, err := s.RetireSession(ctx, retiredID, time.Now().UTC()); err != nil {
		t.Fatalf("RetireSession: %v", err)
	}

	next, err := s.CreateSession(ctx, sampleRecord("mer"))
	if err != nil {
		t.Fatalf("CreateSession after retiring: %v", err)
	}
	if next.ID == retiredID {
		t.Fatalf("reused the retired session id %q", retiredID)
	}
}

// A delete has to reach connected clients, or every board keeps showing a card
// for a session that no longer exists.
func TestRetiringASessionEmitsAChangeEvent(t *testing.T) {
	ctx := context.Background()
	s, session := retireFixture(t)
	if _, err := s.RetireSession(ctx, session.ID, time.Now().UTC()); err != nil {
		t.Fatalf("RetireSession: %v", err)
	}
	events, err := s.EventsAfter(ctx, 0, 200)
	if err != nil {
		t.Fatalf("EventsAfter: %v", err)
	}
	found := false
	for _, event := range events {
		if event.Type == cdc.EventSessionUpdated && strings.Contains(string(event.Payload), `"retired":true`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("no retirement event in the change log: %+v", events)
	}
}

func TestRetireSessionRefusesALiveSession(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	seedProject(t, s, "mer")
	live, err := s.CreateSession(ctx, sampleRecord("mer"))
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if _, err := s.RetireSession(ctx, live.ID, time.Now().UTC()); !errors.Is(err, ports.ErrSessionNotTerminated) {
		t.Fatalf("err = %v, want ErrSessionNotTerminated", err)
	}
	if _, ok, err := s.GetSession(ctx, live.ID); err != nil || !ok {
		t.Fatalf("a refused retire removed the session: ok=%v err=%v", ok, err)
	}
}

func TestRetireSessionIsBenignForAnUnknownID(t *testing.T) {
	s := newTestStore(t)
	removed, err := s.RetireSession(context.Background(), "mer-999", time.Now().UTC())
	if err != nil || removed {
		t.Fatalf("unknown id = (%v, %v), want (false, nil)", removed, err)
	}
}
