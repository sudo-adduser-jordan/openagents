package settings

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
)

type stubStore struct {
	snapshot Snapshot
	getErr   error
	setErr   error
	setMode  domain.SessionMode
}

func (s *stubStore) GetAppSettings(context.Context) (Snapshot, error) {
	if s.getErr != nil {
		return Snapshot{}, s.getErr
	}
	return s.snapshot, nil
}

func (s *stubStore) SetDefaultSessionMode(_ context.Context, mode domain.SessionMode, _ time.Time) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.setMode = mode
	return nil
}

// An unreadable preference must not stop work: spawns fall back to the
// compatibility default rather than failing.
func TestDefaultSessionModeFallsBackOnReadError(t *testing.T) {
	svc := New(&stubStore{getErr: errors.New("db down")}, nil, nil)
	if got := svc.DefaultSessionMode(context.Background()); got != domain.DefaultSessionMode {
		t.Fatalf("DefaultSessionMode = %q, want %q", got, domain.DefaultSessionMode)
	}
}

func TestSetDefaultSessionModeRejectsInvalid(t *testing.T) {
	svc := New(&stubStore{}, nil, nil)
	if _, err := svc.SetDefaultSessionMode(context.Background(), domain.SessionMode("bogus")); err == nil {
		t.Fatal("SetDefaultSessionMode(bogus) = nil error, want error")
	}
}
