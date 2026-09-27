package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
)

func TestManagerReengagementPersistence(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "p")
	rec := sampleRecord("p")
	rec.Kind = domain.KindManager
	rec, err := s.CreateSession(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	if err := s.ScheduleManagerReengagement(ctx, rec.ID, now.Add(time.Minute), now); err != nil {
		t.Fatal(err)
	}
	state, ok, err := s.GetManagerReengagement(ctx, rec.ID)
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if state.AttemptCount != 0 || state.State != domain.ManagerReengagementActive {
		t.Fatalf("initial state = %#v", state)
	}
	state, err = s.RecordManagerReengagementAttempt(ctx, rec.ID, now.Add(2*time.Minute), now, 3)
	if err != nil {
		t.Fatal(err)
	}
	if state.AttemptCount != 1 || state.State != domain.ManagerReengagementActive {
		t.Fatalf("attempt state = %#v", state)
	}
	if err := s.MarkManagerReengagementProgress(ctx, rec.ID, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.ScheduleManagerReengagement(ctx, rec.ID, now.Add(5*time.Minute), now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	state, _, err = s.GetManagerReengagement(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.AttemptCount != 0 || state.ProgressSinceAttempt || !state.NextAttemptAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("reset state = %#v", state)
	}
	if _, err := s.CompleteManagerReengagement(ctx, rec.ID, now.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkManagerReengagementProgress(ctx, rec.ID, now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := s.ScheduleManagerReengagement(ctx, rec.ID, now.Add(10*time.Minute), now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	state, _, err = s.GetManagerReengagement(ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.State != domain.ManagerReengagementCompleted {
		t.Fatalf("completed state was rearmed: %#v", state)
	}
}

func TestOrchestratorAttentionDeliveryPersistence(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	seedProject(t, s, "p")
	rec := sampleRecord("p")
	rec.Kind = domain.KindManager
	rec, err := s.CreateSession(ctx, rec)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	if err := s.ScheduleManagerReengagement(ctx, rec.ID, now, now); err != nil {
		t.Fatal(err)
	}
	state, err := s.RecordManagerReengagementAttempt(ctx, rec.ID, now.Add(time.Minute), now, 1)
	if err != nil {
		t.Fatal(err)
	}
	if state.State != domain.ManagerReengagementExhausted || state.AttentionNotified {
		t.Fatalf("exhausted state = %#v", state)
	}
	pending, err := s.ListPendingManagerAttention(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].SessionID != rec.ID {
		t.Fatalf("pending attention = %#v", pending)
	}
	marked, err := s.MarkManagerAttentionNotified(ctx, rec.ID, now.Add(time.Second))
	if err != nil || !marked {
		t.Fatalf("mark notified: marked=%v err=%v", marked, err)
	}
	pending, err = s.ListPendingManagerAttention(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("pending attention after delivery = %#v", pending)
	}
}
