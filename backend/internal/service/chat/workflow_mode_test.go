package chat_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	chatsvc "github.com/sudo-adduser-jordan/open-agents/backend/internal/service/chat"
)

func TestWorkflowModeToOpenCodeMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		kind domain.SessionKind
		mode domain.WorkflowMode
		want string
	}{
		{domain.KindWorker, domain.WorkflowModePlanning, "plan"},
		{domain.KindWorker, domain.WorkflowModeBuilding, "build"},
		{domain.KindWorker, domain.WorkflowModeManager, ""},
		{domain.KindWorker, "", ""},
		{domain.KindManager, domain.WorkflowModeManager, ""},
		{domain.KindManager, domain.WorkflowModePlanning, ""},
	}
	for _, tc := range cases {
		if got := chatsvc.WorkflowModeToOpenCodeMode(tc.kind, tc.mode); got != tc.want {
			t.Errorf("WorkflowModeToOpenCodeMode(%q, %q) = %q, want %q", tc.kind, tc.mode, got, tc.want)
		}
	}
}

func createWorkerSession(t *testing.T, st interface {
	CreateSession(ctx context.Context, rec domain.SessionRecord) (domain.SessionRecord, error)
}, kind domain.SessionKind) domain.SessionID {
	t.Helper()
	created, err := st.CreateSession(context.Background(), domain.SessionRecord{
		ProjectID: testProject, Kind: kind, Harness: domain.HarnessOpenCode,
		Mode: domain.SessionModeChat, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return created.ID
}

// Entering a stage drives the live provider session through the existing mode
// control, and the readback shows the stage-derived value.
func TestSyncProviderModeLiveController(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		stage domain.WorkflowMode
		want  string
	}{
		{domain.WorkflowModePlanning, "plan"},
		{domain.WorkflowModeBuilding, "build"},
	} {
		t.Run(string(tc.stage), func(t *testing.T) {
			ctx := context.Background()
			st := openStore(t)
			id := createWorkerSession(t, st, domain.KindWorker)
			conv := &modeConversation{fakeConversation: newFakeConversation(), mode: "build"}
			conv.providerConversationID = "opencode-thread"
			svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: uuid.NewString})
			cfg := chatsvc.StartConfig{SessionID: id, ProjectID: testProject, Kind: domain.KindWorker, Harness: domain.HarnessOpenCode, WorkspacePath: t.TempDir()}
			if _, err := svc.Start(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = svc.Stop(ctx, id) })

			if err := svc.SyncProviderMode(ctx, id, tc.stage); err != nil {
				t.Fatal(err)
			}
			if conv.mode != tc.want {
				t.Fatalf("provider mode = %q, want %q after sync into %q", conv.mode, tc.want, tc.stage)
			}
			options, err := svc.ConfigOptions(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, option := range options {
				if option.ID == "mode" && option.Current.Select == tc.want {
					found = true
				}
			}
			if !found {
				t.Fatalf("mode control readback = %+v, want current %q", options, tc.want)
			}
			stored, err := st.ConversationForSession(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Settings.OpenCodeMode != tc.want {
				t.Fatalf("stored mode = %q, want %q: the next restart must restore the stage value", stored.Settings.OpenCodeMode, tc.want)
			}
		})
	}
}

// Without a live controller the stage-derived choice is persisted, so the
// next start restores it instead of silently keeping a stale agent choice.
func TestSyncProviderModeWithoutLiveControllerPersists(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := openStore(t)
	id := createWorkerSession(t, st, domain.KindWorker)
	conv := &modeConversation{fakeConversation: newFakeConversation(), mode: "build"}
	conv.providerConversationID = "opencode-thread"
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: uuid.NewString})
	cfg := chatsvc.StartConfig{SessionID: id, ProjectID: testProject, Kind: domain.KindWorker, Harness: domain.HarnessOpenCode, WorkspacePath: t.TempDir()}
	if _, err := svc.Start(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if err := svc.Stop(ctx, id); err != nil {
		t.Fatal(err)
	}

	if err := svc.SyncProviderMode(ctx, id, domain.WorkflowModePlanning); err != nil {
		t.Fatal(err)
	}
	stored, err := st.ConversationForSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Settings.OpenCodeMode != "plan" {
		t.Fatalf("stored mode = %q, want plan", stored.Settings.OpenCodeMode)
	}
}

// Manager sessions never leave the manager stage, so there is nothing to bind.
func TestSyncProviderModeManagerIsNoop(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := openStore(t)
	id := createWorkerSession(t, st, domain.KindManager)
	conv := &modeConversation{fakeConversation: newFakeConversation(), mode: "build"}
	conv.providerConversationID = "opencode-thread"
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: uuid.NewString})
	cfg := chatsvc.StartConfig{SessionID: id, ProjectID: testProject, Kind: domain.KindManager, Harness: domain.HarnessOpenCode, WorkspacePath: t.TempDir()}
	if _, err := svc.Start(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Stop(ctx, id) })

	if err := svc.SyncProviderMode(ctx, id, domain.WorkflowModeManager); err != nil {
		t.Fatal(err)
	}
	if conv.mode != "build" {
		t.Fatalf("provider mode = %q, want untouched build", conv.mode)
	}
}

// A fresh worker controller starts in the provider mode its stage asks for,
// with no stage change required first.
func TestFreshStartAppliesStageMode(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		stage domain.WorkflowMode
		want  string
	}{
		{domain.WorkflowModePlanning, "plan"},
		{domain.WorkflowModeBuilding, "build"},
	} {
		t.Run(string(tc.stage), func(t *testing.T) {
			ctx := context.Background()
			st := openStore(t)
			id := createWorkerSession(t, st, domain.KindWorker)
			conv := &modeConversation{fakeConversation: newFakeConversation(), mode: "build"}
			conv.providerConversationID = "opencode-thread"
			svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: uuid.NewString})
			cfg := chatsvc.StartConfig{
				SessionID: id, ProjectID: testProject, Kind: domain.KindWorker,
				Harness: domain.HarnessOpenCode, WorkflowMode: tc.stage, WorkspacePath: t.TempDir(),
			}
			if _, err := svc.Start(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = svc.Stop(ctx, id) })

			if conv.mode != tc.want {
				t.Fatalf("provider mode = %q, want %q for a fresh %q worker", conv.mode, tc.want, tc.stage)
			}
			stored, err := st.ConversationForSession(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Settings.OpenCodeMode != tc.want {
				t.Fatalf("stored mode = %q, want %q", stored.Settings.OpenCodeMode, tc.want)
			}
		})
	}
}

// A provider without the mode control keeps its default: a fresh spawn must
// not fail for a control the provider never advertised.
func TestFreshStartWithoutModeControlKeepsDefault(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := openStore(t)
	id := createWorkerSession(t, st, domain.KindWorker)
	conv := newFakeConversation()
	conv.providerConversationID = "opencode-thread"
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: uuid.NewString})
	cfg := chatsvc.StartConfig{
		SessionID: id, ProjectID: testProject, Kind: domain.KindWorker,
		Harness: domain.HarnessOpenCode, WorkflowMode: domain.WorkflowModePlanning, WorkspacePath: t.TempDir(),
	}
	if _, err := svc.Start(ctx, cfg); err != nil {
		t.Fatalf("fresh start without a mode control failed: %v", err)
	}
	t.Cleanup(func() { _ = svc.Stop(ctx, id) })
	stored, err := st.ConversationForSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Settings.OpenCodeMode != "" {
		t.Fatalf("stored mode = %q, want empty: nothing was applied", stored.Settings.OpenCodeMode)
	}
}
