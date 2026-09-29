package sessionmanager

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

func TestIsProviderHostTransportFailure(t *testing.T) {
	t.Parallel()
	transport := func() error {
		return fmt.Errorf("restore mer-1: resume chat: restore OpenCode mode %q: %w",
			"build", fmt.Errorf("set ACP session config option %q: %w", "mode", context.DeadlineExceeded))
	}
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"canceled caller", context.Canceled, false},
		{"deadline wrapped", fmt.Errorf("resume chat: %w", context.DeadlineExceeded), true},
		{"acp payload deadline", transport(), true},
		{"closed connection", errors.New("read tcp 127.0.0.1:1->127.0.0.1:2: use of closed network connection"), true},
		{"connection reset", errors.New("read: connection reset by peer"), true},
		{"connection refused", errors.New("dial tcp: connection refused"), true},
		{"broken pipe", errors.New("write: broken pipe"), true},
		{"unexpected eof", errors.New("read frame: unexpected eof"), true},
		{"inconclusive preserved", fmt.Errorf("reconcile mer-1: %w", ports.ErrChatRecoveryInconclusive), false},
		{"resume refused", fmt.Errorf("resume agent mer-1: resume chat: %w: provider rejected the load", ports.ErrChatResumeFailed), false},
		{"bad config option", fmt.Errorf("set mode: %w", ports.ErrChatConfigOptionInvalid), false},
		{"auth required", fmt.Errorf("initialize: %w", ports.ErrChatAuthRequired), false},
		{"incompatible driver", fmt.Errorf("probe: %w", ports.ErrChatDriverIncompatible), false},
		{"unavailable driver", fmt.Errorf("spawn: %w", ports.ErrChatDriverUnavailable), false},
		{"ordinary failure", errors.New("boom"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isProviderHostTransportFailure(tc.err); got != tc.want {
				t.Fatalf("isProviderHostTransportFailure(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// flakyTransportLauncher fails its first StartChat with a wedged-host
// transport error, then behaves like the recording launcher.
type flakyTransportLauncher struct {
	*recordingLauncher
	calls int
}

func (l *flakyTransportLauncher) StartChat(ctx context.Context, cfg ChatStart) (ChatStarted, error) {
	l.calls++
	if l.calls == 1 {
		return ChatStarted{}, fmt.Errorf("restore OpenCode mode %q: set ACP session config option %q: %w",
			"build", "mode", context.DeadlineExceeded)
	}
	return l.recordingLauncher.StartChat(ctx, cfg)
}

func transportFailureRecord(id domain.SessionID) domain.SessionRecord {
	return domain.SessionRecord{
		ID: id, ProjectID: chatTestProject, Kind: domain.KindWorker,
		Harness: domain.HarnessOpenCode, Mode: domain.SessionModeChat,
		Activity: domain.Activity{State: domain.ActivityActive},
		Metadata: domain.SessionMetadata{
			Branch: "open-agents/" + string(id) + "/root", WorkspacePath: "/ws/" + string(id),
			ProviderConversationID: "01a03c61-23a9-7111-95e9-2bacb04eb064",
		},
	}
}

func TestReconcileLive_TransportFailureRetriesOnceWithFreshHost(t *testing.T) {
	t.Parallel()
	base := &recordingLauncher{}
	launcher := &flakyTransportLauncher{recordingLauncher: base}
	m, st, _ := newChatManager(t, launcher)
	rec := transportFailureRecord("mer-1")
	st.sessions[rec.ID] = rec

	if err := m.reconcileLive(context.Background(), rec); err != nil {
		t.Fatalf("reconcileLive: %v", err)
	}
	if launcher.calls != 2 {
		t.Fatalf("StartChat calls = %d, want 2 (failed attempt plus one retry)", launcher.calls)
	}
	got := st.sessions[rec.ID]
	if got.IsTerminated || got.Metadata.ControllerGeneration != "gen-1" {
		t.Fatalf("retry did not publish a live controller: %+v", got.Metadata)
	}
}

func TestReconcileLive_InconclusiveFailureDoesNotRetry(t *testing.T) {
	t.Parallel()
	launcher := &recordingLauncher{
		startErr: fmt.Errorf("resume chat: %w: provider rejected the load", ports.ErrChatResumeFailed),
	}
	m, st, _ := newChatManager(t, launcher)
	rec := transportFailureRecord("mer-2")
	st.sessions[rec.ID] = rec

	err := m.reconcileLive(context.Background(), rec)
	if !errors.Is(err, ports.ErrChatResumeFailed) {
		t.Fatalf("reconcileLive error = %v, want ErrChatResumeFailed", err)
	}
	if len(launcher.started) != 1 {
		t.Fatalf("StartChat calls = %d, want 1 (no retry on provider verdicts)", len(launcher.started))
	}
}

func TestReconcileLive_DeferredTransportFailureDoesNotRetry(t *testing.T) {
	t.Parallel()
	base := &recordingLauncher{}
	launcher := &flakyTransportLauncher{recordingLauncher: base}
	m, st, _ := newChatManager(t, launcher)
	rec := transportFailureRecord("mer-3")
	rec.AgentDeferred = true
	rec.Metadata.Prompt = "staged work"
	st.sessions[rec.ID] = rec

	if err := m.reconcileLive(context.Background(), rec); err == nil {
		t.Fatal("deferred transport failure unexpectedly succeeded")
	}
	if launcher.calls != 1 {
		t.Fatalf("StartChat calls = %d, want 1 (staged sessions never retry: the prompt may be delivered)", launcher.calls)
	}
}
