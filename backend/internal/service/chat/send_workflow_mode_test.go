package chat_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/storage/sqlite/store"
)

// The send-mode gap: each human message must durably carry the workflow mode
// that was active when it was sent, so the timeline can color it after later
// mode switches. The conversation-level mode is current-only, so a
// frontend-only change would color old messages with the current mode.
func TestSendRecordsResolvedWorkflowModeOnTurn(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := context.Background()

	first, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text:            "first prompt",
		ClientMessageID: "wf-mode-first",
		Origin:          domain.MessageOriginHuman,
	})
	if err != nil {
		t.Fatalf("Send first: %v", err)
	}
	// The harness session is a manager with no explicit mode, which resolves
	// to the coordinating manager stage.
	if first.WorkflowMode != domain.WorkflowModeManager {
		t.Fatalf("first send workflow = %q, want %q", first.WorkflowMode, domain.WorkflowModeManager)
	}

	if _, err := h.st.SetSessionWorkflowMode(ctx, testSession, domain.WorkflowModePlanning, h.now()); err != nil {
		t.Fatalf("switch to planning: %v", err)
	}
	second, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text:            "second prompt",
		ClientMessageID: "wf-mode-second",
		Origin:          domain.MessageOriginHuman,
	})
	if err != nil {
		t.Fatalf("Send second: %v", err)
	}
	if second.WorkflowMode != domain.WorkflowModePlanning {
		t.Fatalf("second send workflow = %q, want %q", second.WorkflowMode, domain.WorkflowModePlanning)
	}

	// Switching mode afterwards must change neither recorded mode.
	if _, err := h.st.SetSessionWorkflowMode(ctx, testSession, domain.WorkflowModeManager, h.now()); err != nil {
		t.Fatalf("switch back to manager: %v", err)
	}
	snapshot, err := h.st.LoadConversationSnapshot(ctx, h.ctrl.ConversationID())
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	byID := make(map[string]domain.ConversationTurn, len(snapshot.Turns))
	for _, turn := range snapshot.Turns {
		byID[turn.ID] = turn
	}
	if got := byID[first.ID].WorkflowMode; got != domain.WorkflowModeManager {
		t.Fatalf("durable first turn workflow = %q, want %q", got, domain.WorkflowModeManager)
	}
	if got := byID[second.ID].WorkflowMode; got != domain.WorkflowModePlanning {
		t.Fatalf("durable second turn workflow = %q, want %q", got, domain.WorkflowModePlanning)
	}
}

// An edit revises the source prompt rather than sending a new one, so the
// replacement turn inherits the source turn's recorded send-mode even when the
// session has since moved to another mode.
func TestEditMessageInheritsSourceTurnWorkflowMode(t *testing.T) {
	t.Parallel()
	h, _, _ := newEditHarness(t, true)
	ctx := context.Background()
	first := completeTurn(t, h, "original prompt", "provider-turn-1")
	h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool { return len(s.Messages) == 2 })

	if _, err := h.st.SetSessionWorkflowMode(ctx, testSession, domain.WorkflowModePlanning, h.now()); err != nil {
		t.Fatalf("switch to planning: %v", err)
	}
	result, err := h.svc.EditMessage(ctx, testSession, first, ports.ChatUserMessage{
		Text:            "edited prompt",
		ClientMessageID: "wf-edit-1",
		Origin:          domain.MessageOriginHuman,
	})
	if err != nil {
		t.Fatalf("EditMessage: %v", err)
	}
	if result.Turn.WorkflowMode != domain.WorkflowModeManager {
		t.Fatalf("replacement turn workflow = %q, want inherited %q",
			result.Turn.WorkflowMode, domain.WorkflowModeManager)
	}
	snapshot, err := h.st.LoadConversationSnapshot(ctx, h.ctrl.ConversationID())
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	replacement, ok := turnByID(snapshot, result.Turn.ID)
	if !ok || replacement.WorkflowMode != domain.WorkflowModeManager {
		t.Fatalf("durable replacement workflow = %+v, want inherited manager", replacement)
	}
}

// Retrying a failed turn re-sends its prompt now, so the new turn records the
// mode active at retry time rather than inheriting the failed attempt's mode.
func TestRetryTurnRecordsCurrentWorkflowMode(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := context.Background()

	turn, err := h.svc.Send(ctx, testSession, ports.ChatUserMessage{
		Text:            "retry me",
		ClientMessageID: "wf-retry-first",
		Origin:          domain.MessageOriginHuman,
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	h.conv.emit(ports.ChatEvent{
		Kind:           ports.ChatEventTurnCompleted,
		ProviderTurnID: turn.ProviderTurnID,
		TurnState:      domain.TurnStateFailed,
		Err:            errors.New("stream disconnected before completion"),
	})
	failedTurnSnapshot(t, h, turn.ID)

	if _, err := h.st.SetSessionWorkflowMode(ctx, testSession, domain.WorkflowModePlanning, h.now()); err != nil {
		t.Fatalf("switch to planning: %v", err)
	}
	retried, err := h.svc.RetryTurn(ctx, testSession, turn.ID)
	if err != nil {
		t.Fatalf("RetryTurn: %v", err)
	}
	if retried.WorkflowMode != domain.WorkflowModePlanning {
		t.Fatalf("retried turn workflow = %q, want %q", retried.WorkflowMode, domain.WorkflowModePlanning)
	}
	after := h.awaitSnapshot(t, func(s store.ConversationSnapshot) bool {
		_, ok := turnByID(s, retried.ID)
		return ok
	})
	original, ok := turnByID(after, turn.ID)
	if !ok || original.WorkflowMode != domain.WorkflowModeManager {
		t.Fatalf("original turn workflow = %+v, want it to stay manager", original)
	}
}
