package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/storage/sqlite/gen"
)

// ScheduleManagerReengagement schedules or resets re-engagement after an idle transition.
func (s *Store) ScheduleManagerReengagement(ctx context.Context, id domain.SessionID, next, now time.Time) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.qw.ScheduleManagerReengagement(ctx, gen.ScheduleManagerReengagementParams{
		SessionID:     string(id),
		NextAttemptAt: next,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
}

// EnsureManagerReengagement creates missing durable state for an idle manager.
func (s *Store) EnsureManagerReengagement(ctx context.Context, id domain.SessionID, next, now time.Time) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.qw.EnsureManagerReengagement(ctx, gen.EnsureManagerReengagementParams{
		SessionID:     string(id),
		NextAttemptAt: next,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
}

// MarkManagerReengagementProgress records productive work after an attempt.
func (s *Store) MarkManagerReengagementProgress(ctx context.Context, id domain.SessionID, now time.Time) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err := s.qw.MarkManagerReengagementProgress(ctx, gen.MarkManagerReengagementProgressParams{
		SessionID: string(id),
		UpdatedAt: now,
	})
	return err
}

// ListDueManagerReengagements returns active re-engagements due by now.
func (s *Store) ListDueManagerReengagements(ctx context.Context, now time.Time) ([]domain.ManagerReengagement, error) {
	rows, err := s.qr.ListDueManagerReengagements(ctx, now)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ManagerReengagement, 0, len(rows))
	for _, row := range rows {
		out = append(out, managerReengagementFromRow(row))
	}
	return out, nil
}

// GetManagerReengagement loads durable re-engagement state for a session.
func (s *Store) GetManagerReengagement(ctx context.Context, id domain.SessionID) (domain.ManagerReengagement, bool, error) {
	row, err := s.qr.GetManagerReengagement(ctx, string(id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ManagerReengagement{}, false, nil
	}
	if err != nil {
		return domain.ManagerReengagement{}, false, err
	}
	return managerReengagementFromRow(row), true, nil
}

// RecordManagerReengagementAttempt advances the attempt count and retry state.
func (s *Store) RecordManagerReengagementAttempt(ctx context.Context, id domain.SessionID, next, now time.Time, maxAttempts int) (domain.ManagerReengagement, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	row, err := s.qw.RecordManagerReengagementAttempt(ctx, gen.RecordManagerReengagementAttemptParams{
		SessionID:     string(id),
		NextAttemptAt: next,
		LastAttemptAt: sql.NullTime{Time: now, Valid: true},
		AttemptCount:  int64(maxAttempts),
		UpdatedAt:     now,
	})
	if err != nil {
		return domain.ManagerReengagement{}, fmt.Errorf("record manager re-engagement attempt: %w", err)
	}
	return managerReengagementFromRow(row), nil
}

// ListPendingManagerAttention returns exhausted loops whose terminal
// human-attention notification has not been delivered.
func (s *Store) ListPendingManagerAttention(ctx context.Context) ([]domain.ManagerReengagement, error) {
	rows, err := s.qr.ListPendingManagerAttention(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.ManagerReengagement, 0, len(rows))
	for _, row := range rows {
		out = append(out, managerReengagementFromRow(row))
	}
	return out, nil
}

// MarkManagerAttentionNotified records successful delivery of the
// terminal human-attention notification.
func (s *Store) MarkManagerAttentionNotified(ctx context.Context, id domain.SessionID, now time.Time) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	rows, err := s.qw.MarkManagerAttentionNotified(ctx, gen.MarkManagerAttentionNotifiedParams{
		SessionID: string(id),
		UpdatedAt: now,
	})
	return rows > 0, err
}

// CompleteManagerReengagement permanently marks a session's loop complete.
func (s *Store) CompleteManagerReengagement(ctx context.Context, id domain.SessionID, now time.Time) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	rows, err := s.qw.CompleteManagerReengagement(ctx, gen.CompleteManagerReengagementParams{
		SessionID:     string(id),
		NextAttemptAt: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	return rows > 0, err
}

func managerReengagementFromRow(row gen.ManagerReengagement) domain.ManagerReengagement {
	var lastAttempt time.Time
	if row.LastAttemptAt.Valid {
		lastAttempt = row.LastAttemptAt.Time
	}
	return domain.ManagerReengagement{
		SessionID:            domain.SessionID(row.SessionID),
		AttemptCount:         int(row.AttemptCount),
		NextAttemptAt:        row.NextAttemptAt,
		LastAttemptAt:        lastAttempt,
		ProgressSinceAttempt: row.ProgressSinceAttempt,
		AttentionNotified:    row.AttentionNotified,
		State:                domain.ManagerReengagementState(row.State),
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
	}
}
