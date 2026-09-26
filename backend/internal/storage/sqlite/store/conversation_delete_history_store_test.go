package store_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/storage/sqlite/store"
)

// A prefix trim removes everything strictly before the anchor: the anchor turn
// and everything after it survive with their original sequences.
func TestDeleteHistoryBeforeRemovesOnlyThePrefix(t *testing.T) {
	s, session, conversation := conversationFixture(t)
	ctx := context.Background()

	seedTurn(t, s, conversation, session, "turn-1", "first", histClock)
	seedTurn(t, s, conversation, session, "turn-2", "second", histClock.Add(time.Minute))
	seedTurn(t, s, conversation, session, "turn-3", "third", histClock.Add(2*time.Minute))

	messages, activities, err := s.DeleteHistoryBefore(ctx, conversation, "turn-2")
	if err != nil {
		t.Fatalf("DeleteHistoryBefore: %v", err)
	}
	// Each seeded turn leaves one user message and one activity behind.
	if messages != 1 || activities != 1 {
		t.Fatalf("deleted = %d messages / %d activities, want 1 and 1", messages, activities)
	}

	snapshot, err := s.LoadConversationSnapshot(ctx, conversation)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if got := texts(snapshot.Messages); len(got) != 2 || got[0] != "second" || got[1] != "third" {
		t.Fatalf("messages = %#v, want [second third]", got)
	}
	for _, activity := range snapshot.Activities {
		if activity.TurnID == "turn-1" {
			t.Fatalf("prefix activity survived: %+v", activity)
		}
	}

	// Turn rows survive the trim: the operation deletes rendered prose, not
	// durable facts, so retry lineage and the rolled-back filters keep working.
	if len(snapshot.Turns) != 3 {
		t.Fatalf("turns = %d, want all 3 still readable", len(snapshot.Turns))
	}

	// Sequences are immutable: survivors keep their original positions.
	for _, message := range snapshot.Messages {
		if message.Text == "second" && message.Sequence != 3 {
			t.Errorf("surviving message sequence = %d, want its original 3", message.Sequence)
		}
	}
}

// Deleting up to the first turn removes nothing: there is no prefix.
func TestDeleteHistoryBeforeTheFirstTurnDeletesNothing(t *testing.T) {
	s, session, conversation := conversationFixture(t)
	ctx := context.Background()

	seedTurn(t, s, conversation, session, "turn-1", "first", histClock)
	seedTurn(t, s, conversation, session, "turn-2", "second", histClock.Add(time.Minute))

	messages, activities, err := s.DeleteHistoryBefore(ctx, conversation, "turn-1")
	if err != nil {
		t.Fatalf("DeleteHistoryBefore: %v", err)
	}
	if messages != 0 || activities != 0 {
		t.Fatalf("deleted = %d messages / %d activities, want nothing", messages, activities)
	}
}

// Deleting up to the last turn clears everything before it.
func TestDeleteHistoryBeforeTheLastTurnKeepsOnlyTheAnchor(t *testing.T) {
	s, session, conversation := conversationFixture(t)
	ctx := context.Background()

	seedTurn(t, s, conversation, session, "turn-1", "first", histClock)
	seedTurn(t, s, conversation, session, "turn-2", "second", histClock.Add(time.Minute))
	seedTurn(t, s, conversation, session, "turn-3", "third", histClock.Add(2*time.Minute))

	messages, activities, err := s.DeleteHistoryBefore(ctx, conversation, "turn-3")
	if err != nil {
		t.Fatalf("DeleteHistoryBefore: %v", err)
	}
	if messages != 2 || activities != 2 {
		t.Fatalf("deleted = %d messages / %d activities, want 2 and 2", messages, activities)
	}

	snapshot, err := s.LoadConversationSnapshot(ctx, conversation)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if got := texts(snapshot.Messages); len(got) != 1 || got[0] != "third" {
		t.Fatalf("messages = %#v, want [third]", got)
	}
}

// A turn from another conversation must not anchor a range operation here.
func TestDeleteHistoryBeforeRefusesATurnFromAnotherConversation(t *testing.T) {
	s, session, conversation := conversationFixture(t)
	ctx := context.Background()

	seedTurn(t, s, conversation, session, "turn-1", "first", histClock)

	otherRec := sampleRecord("hist")
	otherRec.Mode = domain.SessionModeChat
	otherSession, err := s.CreateSession(ctx, otherRec)
	if err != nil {
		t.Fatalf("create second session: %v", err)
	}
	other, err := s.CreateConversation(ctx, "conv-2", domain.ConversationScopeSession, "hist", otherSession.ID, histClock)
	if err != nil {
		t.Fatalf("create second conversation: %v", err)
	}
	seedTurn(t, s, other.ID, otherSession.ID, "turn-elsewhere", "elsewhere", histClock)

	if _, _, err := s.DeleteHistoryBefore(ctx, conversation, "turn-elsewhere"); !errors.Is(err, store.ErrConversationTurnNotFound) {
		t.Fatalf("err = %v, want ErrConversationTurnNotFound", err)
	}
}

func TestDeleteHistoryBeforeRefusesAnUnknownTurn(t *testing.T) {
	s, _, conversation := conversationFixture(t)
	ctx := context.Background()

	if _, _, err := s.DeleteHistoryBefore(ctx, conversation, "turn-nope"); !errors.Is(err, store.ErrConversationTurnNotFound) {
		t.Fatalf("err = %v, want ErrConversationTurnNotFound", err)
	}
}
