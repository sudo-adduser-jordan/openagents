package chat_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
	chatsvc "github.com/sudo-adduser-jordan/open-agents/backend/internal/service/chat"
)

type modeConversation struct {
	*fakeConversation
	mode   string
	setErr error
	ignore bool
}

func (c *modeConversation) ListConfigOptions(context.Context) ([]ports.ChatConfigOption, error) {
	return []ports.ChatConfigOption{{ID: "mode", Current: ports.ChatConfigOptionValue{Select: c.mode}}}, nil
}

func (c *modeConversation) SetConfigOption(ctx context.Context, _ string, value ports.ChatConfigOptionValue) ([]ports.ChatConfigOption, error) {
	if c.setErr != nil {
		return nil, c.setErr
	}
	if !c.ignore {
		c.mode = value.Select
	}
	return c.ListConfigOptions(ctx)
}

type liveModeConversation struct{ *modeConversation }

func (c *liveModeConversation) ReconnectedLive() bool { return true }

func openModeTestSession(t *testing.T, ctx context.Context, st interface {
	CreateSession(context.Context, domain.SessionRecord) (domain.SessionRecord, error)
},
) domain.SessionID {
	t.Helper()
	created, err := st.CreateSession(ctx, domain.SessionRecord{ProjectID: testProject, Kind: domain.KindWorker, Harness: domain.HarnessOpenCode, Mode: domain.SessionModeChat, CreatedAt: time.Now(), UpdatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return created.ID
}

func storedOpenCodeMode(t *testing.T, ctx context.Context, st interface {
	ConversationForSession(context.Context, domain.SessionID) (domain.ConversationRecord, error)
},
	id domain.SessionID,
) string {
	t.Helper()
	stored, err := st.ConversationForSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return stored.Settings.OpenCodeMode
}

// An explicit mode selection must be persisted even when the controller's
// in-memory settings already match the request while the durable row
// disagrees (e.g. a provider resync moved the live snapshot without a
// durable write). Otherwise PATCH returns 200 with a live value that the
// next controller publish silently reverts to the stale durable value.
func TestOpenCodeModeExplicitPatchRepairsDurableDivergence(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := openStore(t)
	id := openModeTestSession(t, ctx, st)
	conv := &modeConversation{fakeConversation: newFakeConversation(), mode: "build"}
	conv.providerConversationID = "opencode-thread"
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: uuid.NewString})
	cfg := chatsvc.StartConfig{SessionID: id, ProjectID: testProject, Harness: domain.HarnessOpenCode, WorkspacePath: t.TempDir()}
	if _, err := svc.Start(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Stop(ctx, id) })
	if _, err := svc.SetConfigOption(ctx, id, "mode", ports.ChatConfigOptionValue{Select: "build"}); err != nil {
		t.Fatal(err)
	}
	if got := storedOpenCodeMode(t, ctx, st, id); got != "build" {
		t.Fatalf("stored mode after explicit patch = %q, want %q", got, "build")
	}
	// Simulate the divergence: the durable row says plan while the live
	// controller still holds the explicit build.
	stored, err := st.ConversationForSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetConversationSettings(ctx, stored.ID, domain.ConversationSettings{OpenCodeMode: "plan"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	options, err := svc.SetConfigOption(ctx, id, "mode", ports.ChatConfigOptionValue{Select: "build"})
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range options {
		if option.ID == "mode" && option.Current.Select != "build" {
			t.Fatalf("live mode after explicit patch = %q, want %q", option.Current.Select, "build")
		}
	}
	if got := storedOpenCodeMode(t, ctx, st, id); got != "build" {
		t.Fatalf("stored mode after explicit re-patch = %q, want %q (PATCH 200 must durably stick)", got, "build")
	}
}

// A 200 that lies is worse than a 409 that tells the truth: when the provider
// accepts the mode RPC but never applies it, the explicit PATCH must fail
// instead of recording (live and durable) a mode the provider is not in.
func TestOpenCodeModePatchReportsUnconfirmedProvider(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := openStore(t)
	id := openModeTestSession(t, ctx, st)
	conv := &modeConversation{fakeConversation: newFakeConversation(), mode: "plan", ignore: true}
	conv.providerConversationID = "opencode-thread"
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: uuid.NewString})
	cfg := chatsvc.StartConfig{SessionID: id, ProjectID: testProject, Harness: domain.HarnessOpenCode, WorkspacePath: t.TempDir()}
	if _, err := svc.Start(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Stop(ctx, id) })
	if _, err := svc.SetConfigOption(ctx, id, "mode", ports.ChatConfigOptionValue{Select: "build"}); !errors.Is(err, chatsvc.ErrProviderRefused) {
		t.Fatalf("SetConfigOption err = %v, want ErrProviderRefused", err)
	}
	if got := storedOpenCodeMode(t, ctx, st, id); got == "build" {
		t.Fatalf("stored mode = %q after unconfirmed provider: a mode the provider is not in must not persist", got)
	}
}

// The durable explicit selection must be re-applied when a live provider
// reconnects, not only on a cold publish: a daemon restart rebuilds the
// driver snapshot from the stale host-captured setup while the same provider
// process keeps running, and skipping the restore leaves the live catalog
// showing the stale value.
func TestOpenCodeModeReassertedOnLiveReconnect(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := openStore(t)
	id := openModeTestSession(t, ctx, st)
	first := &modeConversation{fakeConversation: newFakeConversation(), mode: "build"}
	first.providerConversationID = "opencode-thread"
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: first}}, Log: slog.New(slog.DiscardHandler), NewID: uuid.NewString})
	cfg := chatsvc.StartConfig{SessionID: id, ProjectID: testProject, Harness: domain.HarnessOpenCode, WorkspacePath: t.TempDir()}
	if _, err := svc.Start(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetConfigOption(ctx, id, "mode", ports.ChatConfigOptionValue{Select: "build"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Stop(ctx, id); err != nil {
		t.Fatal(err)
	}
	live := &liveModeConversation{&modeConversation{fakeConversation: newFakeConversation(), mode: "plan"}}
	live.providerConversationID = "opencode-thread"
	resumed := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: live}}, Reader: fullSnapshotReader(st), Log: slog.New(slog.DiscardHandler), NewID: uuid.NewString})
	t.Cleanup(func() { _ = resumed.Stop(ctx, id) })
	cfg.ProviderConversationID = "opencode-thread"
	if _, err := resumed.Start(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	if live.mode != "build" {
		t.Fatalf("reconnected provider mode = %q, want explicit %q", live.mode, "build")
	}
}

// The exact observed sequence: explicit build, one turn, still build — live
// via the config-options catalog and durable via the conversation row.
func TestOpenCodeModeSurvivesTurn(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	st := openStore(t)
	id := openModeTestSession(t, ctx, st)
	conv := &modeConversation{fakeConversation: newFakeConversation(), mode: "build"}
	conv.providerConversationID = "opencode-thread"
	svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: uuid.NewString})
	cfg := chatsvc.StartConfig{SessionID: id, ProjectID: testProject, Harness: domain.HarnessOpenCode, WorkspacePath: t.TempDir()}
	if _, err := svc.Start(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Stop(ctx, id) })
	if _, err := svc.SetConfigOption(ctx, id, "mode", ports.ChatConfigOptionValue{Select: "build"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, id, ports.ChatUserMessage{Text: "hello", Origin: domain.MessageOriginHuman}); err != nil {
		t.Fatal(err)
	}
	options, err := svc.ConfigOptions(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, option := range options {
		if option.ID == "mode" && option.Current.Select != "build" {
			t.Fatalf("live mode after turn = %q, want %q", option.Current.Select, "build")
		}
	}
	if got := storedOpenCodeMode(t, ctx, st, id); got != "build" {
		t.Fatalf("stored mode after turn = %q, want %q", got, "build")
	}
}
func TestOpenCodeModeSurvivesControllerRestart(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"plan", "build", "open-agents-plan-project-1"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			st := openStore(t)
			created, err := st.CreateSession(ctx, domain.SessionRecord{ProjectID: testProject, Kind: domain.KindWorker, Harness: domain.HarnessOpenCode, Mode: domain.SessionModeChat, CreatedAt: time.Now(), UpdatedAt: time.Now()})
			if err != nil {
				t.Fatal(err)
			}
			id := created.ID
			makeService := func(conv ports.ChatConversation) *chatsvc.Service {
				return chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: uuid.NewString})
			}
			first := &modeConversation{fakeConversation: newFakeConversation(), mode: "build"}
			first.providerConversationID = "opencode-thread"
			svc := makeService(first)
			cfg := chatsvc.StartConfig{SessionID: id, ProjectID: testProject, Harness: domain.HarnessOpenCode, WorkspacePath: t.TempDir()}
			if _, err := svc.Start(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = svc.Stop(ctx, id) })
			if _, err := svc.SetConfigOption(ctx, id, "mode", ports.ChatConfigOptionValue{Select: mode}); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.SetTurnSettings(ctx, id, domain.ConversationSettings{Model: "another-model"}); err != nil {
				t.Fatal(err)
			}
			stored, err := st.ConversationForSession(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Settings.OpenCodeMode != mode {
				t.Fatalf("stored mode = %q", stored.Settings.OpenCodeMode)
			}
			if err := svc.Stop(ctx, id); err != nil {
				t.Fatal(err)
			}
			second := &modeConversation{fakeConversation: newFakeConversation(), mode: "build"}
			second.providerConversationID = "opencode-thread"
			svc = makeService(second)
			cfg.ProviderConversationID = "opencode-thread"
			if _, err := svc.Start(ctx, cfg); err != nil {
				t.Fatal(err)
			}
			if second.mode != mode {
				t.Fatalf("resumed mode = %q, want %q", second.mode, mode)
			}
		})
	}
}

func TestOpenCodeModeRestoreFailureDoesNotPublishController(t *testing.T) {
	t.Parallel()
	for _, silent := range []bool{false, true} {
		t.Run(map[bool]string{false: "rejected", true: "not confirmed"}[silent], func(t *testing.T) {
			ctx := context.Background()
			st := openStore(t)
			conversation, err := st.CreateConversation(ctx, "saved-plan", domain.ConversationScopeProject, testProject, testSession, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if err := st.SetConversationSettings(ctx, conversation.ID, domain.ConversationSettings{OpenCodeMode: "plan"}, time.Now()); err != nil {
				t.Fatal(err)
			}
			conv := &modeConversation{fakeConversation: newFakeConversation(), mode: "build", ignore: silent}
			if !silent {
				conv.setErr = errors.New("mode unavailable")
			}
			conv.providerConversationID = "opencode-thread"
			svc := chatsvc.New(chatsvc.Options{Store: st, Sessions: st, Drivers: fakeRegistry{driver: fakeDriver{conv: conv}}, Log: slog.New(slog.DiscardHandler), NewID: uuid.NewString})
			t.Cleanup(func() { _ = svc.Stop(ctx, testSession) })
			_, err = svc.Start(ctx, chatsvc.StartConfig{SessionID: testSession, ProjectID: testProject, Harness: domain.HarnessOpenCode, WorkspacePath: t.TempDir(), ProviderConversationID: "opencode-thread"})
			if err == nil {
				t.Fatal("expected mode restoration error")
			}
			if _, err := svc.Controller(testSession); err == nil {
				t.Fatal("published a controller after failed Plan restoration")
			}
		})
	}
}
