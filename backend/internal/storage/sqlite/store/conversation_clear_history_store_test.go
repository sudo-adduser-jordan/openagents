package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/storage/sqlite"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/storage/sqlite/sqlitetest"
)

func clearHistoryFixture(t *testing.T) (*sqlite.Store, *sql.DB, domain.SessionRecord, domain.ConversationRecord) {
	t.Helper()
	ctx := context.Background()
	dataDir := t.TempDir()
	s := sqlitetest.MustOpenAt(t, dataDir)
	seedProject(t, s, "clear-history")

	rec := sampleRecord("clear-history")
	rec.Mode = domain.SessionModeChat
	rec.Metadata.ProviderConversationID = "thread-root"
	rec.Metadata.ControllerGeneration = "generation-root"
	session, err := s.CreateSession(ctx, rec)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	conversation, err := s.CreateConversation(
		ctx, "conversation-clear", domain.ConversationScopeSession,
		"clear-history", session.ID, testNow,
	)
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if _, err := s.AppendUserMessage(ctx, conversation.ID, session.ID, "generation-root", domain.ConversationMessage{
		ID:     "message-original",
		Origin: domain.MessageOriginHuman,
		Text:   "the original task",
	}, "turn-1", testNow); err != nil {
		t.Fatalf("append message: %v", err)
	}

	raw, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "open-agents.db"))
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	t.Cleanup(func() { _ = raw.Close() })
	return s, raw, session, conversation
}

// Clearing history is what stops one task's narrative leaking into the next. It
// works by dropping the provider handle on the branch, not by deleting anything,
// so the agent forgets while the user's own transcript survives.
func TestClearHistoryDropsProviderContextAndKeepsTranscript(t *testing.T) {
	ctx := context.Background()
	s, raw, session, conversation := clearHistoryFixture(t)

	cleared, err := s.ClearHistory(ctx, conversation.ID, domain.ConversationActivity{
		ID:             "activity-clear",
		Kind:           domain.ActivityKindSystem,
		Status:         domain.ActivityStatusCompleted,
		Summary:        "History cleared.",
		ProviderItemID: "open-agents-context-reset:clear-history-1",
	}, testNow.Add(time.Minute))
	if err != nil {
		t.Fatalf("ClearHistory: %v", err)
	}

	branch, err := s.ConversationBranch(ctx, conversation.ID, cleared.ActiveBranchID)
	if err != nil {
		t.Fatalf("ConversationBranch: %v", err)
	}
	if branch.ProviderConversationID != "" {
		t.Fatalf("provider conversation = %q, want it cleared", branch.ProviderConversationID)
	}

	var text string
	if err := raw.QueryRow(`
SELECT m.text FROM conversation_messages m
WHERE m.conversation_id = ? AND m.id = 'message-original'`,
		conversation.ID,
	).Scan(&text); err != nil {
		t.Fatalf("read back the message: %v", err)
	}
	if text != "the original task" {
		t.Fatalf("message = %q, want the transcript preserved", text)
	}
	_ = session
}

// The boundary and the provider-handle reset must land together. Written
// separately, a reader could see a conversation whose history is intact while the
// agent has already forgotten it.
func TestClearHistoryRecordsBoundaryInTheSameWrite(t *testing.T) {
	ctx := context.Background()
	s, raw, _, conversation := clearHistoryFixture(t)

	cleared, err := s.ClearHistory(ctx, conversation.ID, domain.ConversationActivity{
		ID:             "activity-clear",
		Kind:           domain.ActivityKindSystem,
		Status:         domain.ActivityStatusCompleted,
		Summary:        "History cleared.",
		ProviderItemID: "open-agents-context-reset:clear-history-1",
	}, testNow.Add(time.Minute))
	if err != nil {
		t.Fatalf("ClearHistory: %v", err)
	}
	if cleared.LatestSequence <= conversation.LatestSequence {
		t.Fatalf("latest sequence = %d, want it past the pre-clear %d", cleared.LatestSequence, conversation.LatestSequence)
	}

	var summary, kind string
	var sequence int64
	if err := raw.QueryRow(`
SELECT summary, kind, sequence FROM conversation_activities
WHERE conversation_id = ? AND provider_item_id = 'open-agents-context-reset:clear-history-1'`,
		conversation.ID,
	).Scan(&summary, &kind, &sequence); err != nil {
		t.Fatalf("read back the boundary: %v", err)
	}
	if summary != "History cleared." || kind != string(domain.ActivityKindSystem) {
		t.Fatalf("boundary = %q/%q", kind, summary)
	}
	if sequence != cleared.LatestSequence {
		t.Fatalf("boundary sequence = %d, want the conversation head %d", sequence, cleared.LatestSequence)
	}
}
