package lifecycle

import (
	"context"
	"testing"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
)

// markSpawned is the single commit point every launch passes through, so it is
// the one place that can clear a staged session's "not started yet" fact. If it
// did not, a board would keep reading "Not started" on a session that is now
// running.
func TestMarkSpawnedClearsAgentDeferred(t *testing.T) {
	m, st, _ := newManager()
	ctx := context.Background()

	st.sessions["mer-1"] = domain.SessionRecord{
		ID:            "mer-1",
		ProjectID:     "mer",
		AgentDeferred: true,
		Metadata: domain.SessionMetadata{
			Branch:        "b",
			WorkspacePath: "/ws",
			Prompt:        "the original brief",
		},
	}

	if err := m.MarkSpawned(ctx, "mer-1", domain.SessionMetadata{
		Branch:          "b",
		WorkspacePath:   "/ws",
		RuntimeHandleID: "h1",
		RuntimeLaunchID: "launch-1",
	}); err != nil {
		t.Fatal(err)
	}

	got := st.sessions["mer-1"]
	if got.AgentDeferred {
		t.Error("AgentDeferred=true after MarkSpawned, want false: the agent is launching now")
	}
	// The staged task's own facts have to survive the start, or the session would
	// lose the brief it was created for.
	if got.Metadata.Prompt != "the original brief" {
		t.Errorf("Prompt = %q after MarkSpawned, want the original brief", got.Metadata.Prompt)
	}
	if got.Metadata.WorkspacePath != "/ws" {
		t.Errorf("WorkspacePath = %q after MarkSpawned, want /ws", got.Metadata.WorkspacePath)
	}
}

// An ordinary spawn has nothing to clear, so the commit must leave the flag off
// rather than flip it on.
func TestMarkSpawnedLeavesOrdinarySessionNotDeferred(t *testing.T) {
	m, st, _ := newManager()
	ctx := context.Background()

	st.sessions["mer-1"] = domain.SessionRecord{ID: "mer-1", ProjectID: "mer"}
	if err := m.MarkSpawned(ctx, "mer-1", domain.SessionMetadata{
		WorkspacePath:   "/ws",
		RuntimeHandleID: "h1",
	}); err != nil {
		t.Fatal(err)
	}
	if st.sessions["mer-1"].AgentDeferred {
		t.Error("AgentDeferred=true for an ordinary spawn, want false")
	}
}
