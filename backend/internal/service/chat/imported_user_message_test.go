package chat_test

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
	chatsvc "github.com/sudo-adduser-jordan/open-agents/backend/internal/service/chat"
)

// A provider that mints a fresh user-message id on every session/load (opencode
// does) can replay a user message whose turn.started Open Agents either deduped
// against a prior replay or rewrote onto another durable turn while the message
// itself was not. The message then names a turn with no row.
//
// Importing that one message used to hard-fail the whole native-history restore,
// so resume never published a controller and the conversation stayed wedged.
// The import must adopt the enclosing turn instead of aborting.
func TestNativeHistoryImportAdoptsTurnForUnbackedUserMessage(t *testing.T) {
	t.Parallel()
	st := openStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 27, 12, 52, 30, 0, time.UTC)

	rec, found, err := st.GetSession(ctx, testSession)
	if err != nil || !found {
		t.Fatalf("load session: found=%v err=%v", found, err)
	}
	rec.Metadata.ConversationCheckpointState = domain.ConversationCheckpointLegacy
	if err := st.UpdateSession(ctx, rec); err != nil {
		t.Fatalf("seed checkpoint state: %v", err)
	}

	conversation, err := st.CreateConversation(ctx, "conversation",
		domain.ConversationScopeSession, testProject, testSession, now)
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if err := st.ClaimChatControllerGeneration(ctx, testSession, "generation"); err != nil {
		t.Fatalf("ClaimChatControllerGeneration: %v", err)
	}
	// One durable turn so the replay reconciliation takes its mapping path.
	created, err := st.AppendUserMessage(ctx, conversation.ID, testSession, "generation",
		domain.ConversationMessage{
			ID: "durable-user", Text: "first prompt", Origin: domain.MessageOriginHuman,
			ClientMessageID: "durable-client",
		}, "durable-turn", now, domain.WorkflowMode(""))
	if err != nil || !created {
		t.Fatalf("AppendUserMessage: created=%v err=%v", created, err)
	}
	if err := st.BindTurnToProvider(ctx, "durable-turn", "durable-provider-turn", now); err != nil {
		t.Fatalf("BindTurnToProvider: %v", err)
	}
	if err := st.SettleTurn(ctx, conversation.ID, "durable-provider-turn",
		domain.TurnStateRecovered, "", now); err != nil {
		t.Fatalf("SettleTurn: %v", err)
	}

	var idSeq atomic.Int64
	conv := &nativeHistoryConversation{
		fakeConversation: newFakeConversation(),
		events: []ports.ChatEvent{
			// No turn.started for this turn: its identity churned across replays.
			{
				Kind:            ports.ChatEventUserMessageCompleted,
				ProviderEventID: "replay-user",
				ProviderTurnID:  "acp-history-turn:scope:msg-new",
				ProviderItemID:  "replay-item", ClientMessageID: "replay-item",
				Text: "second prompt",
			},
		},
	}
	svc := chatsvc.New(chatsvc.Options{
		Store: st, Sessions: st,
		Reader:  fullSnapshotReader(st),
		Drivers: fakeRegistry{driver: fakeDriver{conv: conv}},
		Log:     slog.New(slog.DiscardHandler),
		Now:     func() time.Time { return now },
		NewID: func() string {
			return fmt.Sprintf("imported-id-%d", idSeq.Add(1))
		},
	})
	t.Cleanup(func() { _ = svc.Stop(context.Background(), testSession) })

	controller, err := svc.Start(ctx, chatsvc.StartConfig{
		SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessOpenCode,
		WorkspacePath: t.TempDir(), ProviderConversationID: "thread-1",
		HistoryMode: ports.ChatHistoryRequired,
	})
	if err != nil {
		t.Fatalf("Start must tolerate a replayed user message with no durable turn: %v", err)
	}

	snapshot, err := st.LoadConversationSnapshot(ctx, controller.ConversationID())
	if err != nil {
		t.Fatalf("LoadConversationSnapshot: %v", err)
	}
	if len(snapshot.Messages) != 2 {
		t.Fatalf("imported messages = %#v, want the durable prompt and the replayed one", snapshot.Messages)
	}
	if got := snapshot.Messages[1].Text; got != "second prompt" {
		t.Fatalf("imported user message = %q, want second prompt", got)
	}
	if len(snapshot.Turns) != 2 {
		t.Fatalf("imported turns = %#v, want the durable turn and the adopted one", snapshot.Turns)
	}
}
