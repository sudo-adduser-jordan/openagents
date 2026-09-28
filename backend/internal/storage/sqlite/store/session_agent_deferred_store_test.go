package store_test

import (
	"context"
	"encoding/json"
	"testing"
)

// A session staged by `spawn --no-start` has to remember that its agent was
// never launched. The fact is durable rather than inferred from a missing
// runtime handle, because a launch in flight looks the same and the distinction
// has to survive a daemon restart.
func TestSessionRoundTripsAgentDeferred(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "deferred")

	rec := sampleRecord("deferred")
	rec.DisplayName = "staged task"
	rec.AgentDeferred = true
	staged, err := s.CreateSession(ctx, rec)
	if err != nil {
		t.Fatalf("create staged session: %v", err)
	}
	if !staged.AgentDeferred {
		t.Fatalf("CreateSession returned AgentDeferred=false, want true")
	}

	got, found, err := s.GetSession(ctx, staged.ID)
	if err != nil {
		t.Fatalf("get staged session: %v", err)
	}
	if !found {
		t.Fatalf("GetSession did not find %s", staged.ID)
	}
	if !got.AgentDeferred {
		t.Errorf("GetSession AgentDeferred=false, want true")
	}

	byProject, err := s.ListSessions(ctx, "deferred")
	if err != nil {
		t.Fatalf("list by project: %v", err)
	}
	if len(byProject) != 1 {
		t.Fatalf("ListSessionsByProject returned %d sessions, want 1", len(byProject))
	}
	if !byProject[0].AgentDeferred {
		t.Errorf("ListSessionsByProject AgentDeferred=false, want true")
	}

	all, err := s.ListAllSessions(ctx)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("ListAllSessions returned %d sessions, want 1", len(all))
	}
	if !all[0].AgentDeferred {
		t.Errorf("ListAllSessions AgentDeferred=false, want true")
	}

	// Starting the session is the same column flipping back to false.
	started := got
	started.AgentDeferred = false
	if err := s.UpdateSession(ctx, started); err != nil {
		t.Fatalf("update session: %v", err)
	}
	reread, _, err := s.GetSession(ctx, staged.ID)
	if err != nil {
		t.Fatalf("re-get session: %v", err)
	}
	if reread.AgentDeferred {
		t.Errorf("AgentDeferred=true after the start update, want false")
	}
}

// The column defaults to 0, so the ordinary create path needs the caller to set
// nothing and every pre-existing row keeps its old meaning.
func TestSessionDefaultsToNotDeferred(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "not-deferred")

	created, err := s.CreateSession(ctx, sampleRecord("not-deferred"))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if created.AgentDeferred {
		t.Errorf("CreateSession AgentDeferred=true for an ordinary session, want false")
	}

	got, _, err := s.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.AgentDeferred {
		t.Errorf("GetSession AgentDeferred=true for an ordinary session, want false")
	}
}

// The board has to learn that a session went from "not started" to started, or
// the Start agent control would linger after the agent is already running. The
// trigger carries `agentDeferred` alongside the state fields, so a subscriber
// can react to the flip without re-reading the session.
func TestStartingADeferredSessionEmitsAgentDeferredChangeEvent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "deferred-event")

	rec := sampleRecord("deferred-event")
	rec.AgentDeferred = true
	staged, err := s.CreateSession(ctx, rec)
	if err != nil {
		t.Fatalf("create staged session: %v", err)
	}

	// A write that leaves the flag alone must not look like a start. Prompt is
	// not one of the trigger's listed columns, so this is the suppressed case.
	staged.Metadata.Prompt = "only the prompt changed"
	if err := s.UpdateSession(ctx, staged); err != nil {
		t.Fatalf("metadata-only update: %v", err)
	}

	staged.AgentDeferred = false
	if err := s.UpdateSession(ctx, staged); err != nil {
		t.Fatalf("update session: %v", err)
	}

	evs, err := s.EventsAfter(ctx, 0, 100)
	if err != nil {
		t.Fatalf("events after: %v", err)
	}
	var starts int
	for _, e := range evs {
		if e.Type != "session_updated" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(e.Payload), &payload); err != nil {
			t.Fatalf("session_updated payload JSON: %v", err)
		}
		// The rename alone has no agentDeferred key, because the WHEN guard
		// suppressed the event entirely; the start must carry it as a bool.
		if v, ok := payload["agentDeferred"]; ok {
			if _, isBool := v.(bool); !isBool {
				t.Fatalf("agentDeferred payload type = %T, want bool", v)
			}
			starts++
		}
	}
	if starts != 1 {
		t.Fatalf("session_updated events carrying agentDeferred = %d, want exactly 1", starts)
	}
}
