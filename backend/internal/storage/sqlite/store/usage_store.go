package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aoagents/agent-orchestrator/backend/internal/domain"
	"github.com/aoagents/agent-orchestrator/backend/internal/storage/sqlite/gen"
)

// UpsertUsageBinding records or refreshes the association between an AO
// session and a native root session/thread.
func (s *Store) UpsertUsageBinding(ctx context.Context, rec domain.UsageBindingRecord) (domain.UsageBindingRecord, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	row, err := s.qw.UpsertUsageBinding(ctx, gen.UpsertUsageBindingParams{
		SessionID:      rec.SessionID,
		Harness:        rec.Harness,
		NativeRootID:   rec.NativeRootID,
		InitialModelID: strings.TrimSpace(rec.InitialModelID),
		ProviderHint:   strings.TrimSpace(rec.ProviderHint),
		State:          usageBindingStateOrDefault(rec.State),
		LastErrorCode:  rec.LastErrorCode,
		UpdatedAt:      timeOrNow(rec.UpdatedAt),
	})
	if err != nil {
		return domain.UsageBindingRecord{}, fmt.Errorf("upsert usage binding for session %s root %q: %w", rec.SessionID, rec.NativeRootID, err)
	}
	return usageBindingFromGen(row), nil
}

// GetUsageBinding returns one binding, or ok=false when absent.
func (s *Store) GetUsageBinding(ctx context.Context, sessionID domain.SessionID, harness domain.AgentHarness, nativeRootID string) (domain.UsageBindingRecord, bool, error) {
	row, err := s.qr.GetUsageBindingBySessionHarnessRoot(ctx, gen.GetUsageBindingBySessionHarnessRootParams{
		SessionID:    sessionID,
		Harness:      harness,
		NativeRootID: nativeRootID,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.UsageBindingRecord{}, false, nil
	}
	if err != nil {
		return domain.UsageBindingRecord{}, false, fmt.Errorf("get usage binding for session %s root %q: %w", sessionID, nativeRootID, err)
	}
	return usageBindingFromGen(row), true, nil
}

// ListUsageBindingsForSession returns every native usage binding for a session.
func (s *Store) ListUsageBindingsForSession(ctx context.Context, sessionID domain.SessionID) ([]domain.UsageBindingRecord, error) {
	rows, err := s.qr.ListUsageBindingsForSession(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list usage bindings for session %s: %w", sessionID, err)
	}
	out := make([]domain.UsageBindingRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, usageBindingFromGen(row))
	}
	return out, nil
}

// FinalizeUsageBindingsForSessionLaunch atomically moves a live session's
// bindings into finalization only while its durable runtime generation and
// session revision match the lifecycle observation.
func (s *Store) FinalizeUsageBindingsForSessionLaunch(
	ctx context.Context,
	sessionID domain.SessionID,
	expectedRuntimeLaunchID string,
	expectedSessionRevision int64,
	at time.Time,
) ([]domain.UsageBindingRecord, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	rows, err := s.qw.FinalizeUsageBindingsForSessionLaunch(ctx, gen.FinalizeUsageBindingsForSessionLaunchParams{
		SessionID:               sessionID,
		ExpectedRuntimeLaunchID: expectedRuntimeLaunchID,
		ExpectedSessionRevision: expectedSessionRevision,
		FinalizedAt:             timeOrNow(at),
	})
	if err != nil {
		return nil, fmt.Errorf("finalize usage bindings for session %s launch %q: %w", sessionID, expectedRuntimeLaunchID, err)
	}
	out := make([]domain.UsageBindingRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, usageBindingFromGen(row))
	}
	return out, nil
}

// UpdateUsageBindingState updates only the binding lifecycle/error state.
func (s *Store) UpdateUsageBindingState(ctx context.Context, id int64, state domain.UsageBindingState, lastErrorCode string, at time.Time) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.UpdateUsageBinding(ctx, gen.UpdateUsageBindingParams{
		ID:            id,
		State:         usageBindingStateOrDefault(state),
		LastErrorCode: lastErrorCode,
		UpdatedAt:     timeOrNow(at),
	})
	if err != nil {
		return false, fmt.Errorf("update usage binding %d: %w", id, err)
	}
	return n > 0, nil
}

// UpdateUsageBindingErrorCode refreshes binding diagnostics without changing
// lifecycle state chosen by a concurrent finalization or ingestion pass.
func (s *Store) UpdateUsageBindingErrorCode(ctx context.Context, id int64, lastErrorCode string, at time.Time) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.UpdateUsageBinding(ctx, gen.UpdateUsageBindingParams{
		ID:            id,
		State:         "",
		LastErrorCode: lastErrorCode,
		UpdatedAt:     timeOrNow(at),
	})
	if err != nil {
		return false, fmt.Errorf("update usage binding %d error code: %w", id, err)
	}
	return n > 0, nil
}

// CompleteUsageBindingIfSettled atomically completes a finalizing binding only
// when every registered source generation is complete.
func (s *Store) CompleteUsageBindingIfSettled(ctx context.Context, bindingID int64, at time.Time) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.CompleteUsageBindingIfSettled(ctx, gen.CompleteUsageBindingIfSettledParams{
		UsageBindingID: bindingID,
		UpdatedAt:      timeOrNow(at),
	})
	if err != nil {
		return false, fmt.Errorf("complete settled usage binding %d: %w", bindingID, err)
	}
	return n > 0, nil
}

// InsertUsageSource records a physical JSONL source generation. Repeated calls
// for the same binding/path/generation return the existing row.
func (s *Store) InsertUsageSource(ctx context.Context, rec domain.UsageSourceRecord) (domain.UsageSourceRecord, error) {
	if err := validateParserStateObject(rec.ParserStateJSON); err != nil {
		return domain.UsageSourceRecord{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	row, err := s.qw.InsertUsageSource(ctx, usageSourceInsertParams(rec))
	if err != nil {
		return domain.UsageSourceRecord{}, fmt.Errorf("insert usage source for binding %d generation %d: %w", rec.BindingID, rec.Generation, err)
	}
	return usageSourceFromGen(row), nil
}

// ReplaceUsageSource atomically retires one source generation and inserts its
// replacement so startup or watcher reconciliation can never observe a gap.
func (s *Store) ReplaceUsageSource(
	ctx context.Context,
	oldSourceID int64,
	oldErrorCode string,
	rec domain.UsageSourceRecord,
	at time.Time,
) (domain.UsageSourceRecord, error) {
	if err := validateParserStateObject(rec.ParserStateJSON); err != nil {
		return domain.UsageSourceRecord{}, err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	var replaced domain.UsageSourceRecord
	err := s.inTx(ctx, "replace usage source", func(q *gen.Queries) error {
		n, err := q.UpdateUsageSourceLifecycle(ctx, gen.UpdateUsageSourceLifecycleParams{
			ID:            oldSourceID,
			State:         domain.UsageSourceComplete,
			FailureCount:  sql.NullInt64{},
			LastErrorCode: oldErrorCode,
			NextRetryAt:   sql.NullTime{},
			UpdatedAt:     timeOrNow(at),
		})
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("usage source %d not found", oldSourceID)
		}
		row, err := q.InsertUsageSource(ctx, usageSourceInsertParams(rec))
		if err != nil {
			return err
		}
		replaced = usageSourceFromGen(row)
		return nil
	})
	if err != nil {
		return domain.UsageSourceRecord{}, fmt.Errorf("replace usage source %d: %w", oldSourceID, err)
	}
	return replaced, nil
}

// ListUsageSourcesForBinding returns all source generations for one native
// session binding.
func (s *Store) ListUsageSourcesForBinding(ctx context.Context, bindingID int64) ([]domain.UsageSourceRecord, error) {
	rows, err := s.qr.ListUsageSourcesForBinding(ctx, bindingID)
	if err != nil {
		return nil, fmt.Errorf("list usage sources for binding %d: %w", bindingID, err)
	}
	out := make([]domain.UsageSourceRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, usageSourceFromGen(row))
	}
	return out, nil
}

// ListWatchableUsageSources returns the latest source generation for every
// provider artifact attached to a resumable session. Watch registrations are
// rebuilt from this durable inventory after daemon and watcher restarts.
func (s *Store) ListWatchableUsageSources(ctx context.Context) ([]domain.UsageSourceRecord, error) {
	rows, err := s.qr.ListWatchableUsageSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("list watchable usage sources: %w", err)
	}
	out := make([]domain.UsageSourceRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, usageSourceFromGen(row))
	}
	return out, nil
}

// HasPendingUsageDiscovery reports whether a live binding has a durable reason
// to retry source discovery. Healthy active bindings are excluded.
func (s *Store) HasPendingUsageDiscovery(ctx context.Context) (bool, error) {
	pending, err := s.qr.HasPendingUsageDiscovery(ctx)
	if err != nil {
		return false, fmt.Errorf("check pending usage discovery: %w", err)
	}
	return pending != 0, nil
}

// ListUsageDiscoveryBindings returns live-session bindings that may need a
// main source, a relocated source, or newly-created subagent sources.
func (s *Store) ListUsageDiscoveryBindings(ctx context.Context, limit int64) ([]domain.UsageBindingRecord, error) {
	rows, err := s.qr.ListUsageDiscoveryBindings(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("list usage discovery bindings: %w", err)
	}
	out := make([]domain.UsageBindingRecord, 0, len(rows))
	for _, row := range rows {
		out = append(out, usageBindingFromGen(row))
	}
	return out, nil
}

// GetUsageSourceForIngestion returns a source plus immutable binding/session
// facts needed by the ingestor.
func (s *Store) GetUsageSourceForIngestion(ctx context.Context, id int64) (domain.UsageSourceContext, bool, error) {
	row, err := s.qr.GetUsageSourceWithBindingAndSession(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.UsageSourceContext{}, false, nil
	}
	if err != nil {
		return domain.UsageSourceContext{}, false, fmt.Errorf("get usage source %d: %w", id, err)
	}
	return usageSourceContextFromGen(row), true, nil
}

// MarkUsageSourceState updates only the source lifecycle/error state.
func (s *Store) MarkUsageSourceState(ctx context.Context, id int64, state domain.UsageSourceState, lastErrorCode string, nextRetryAt *time.Time, updatedAt time.Time) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.UpdateUsageSourceLifecycle(ctx, gen.UpdateUsageSourceLifecycleParams{
		ID:            id,
		State:         usageSourceStateOrDefault(state),
		FailureCount:  sql.NullInt64{},
		LastErrorCode: lastErrorCode,
		NextRetryAt:   ptrTimeToNullTime(nextRetryAt),
		UpdatedAt:     timeOrNow(updatedAt),
	})
	if err != nil {
		return false, fmt.Errorf("mark usage source %d: %w", id, err)
	}
	return n > 0, nil
}

// ReactivateUsageSource makes a completed or failed source eligible for
// immediate resumed-session ingestion and clears transient retry state.
func (s *Store) ReactivateUsageSource(ctx context.Context, id int64, updatedAt time.Time) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.UpdateUsageSourceLifecycle(ctx, gen.UpdateUsageSourceLifecycleParams{
		ID:            id,
		State:         domain.UsageSourceActive,
		FailureCount:  sql.NullInt64{Int64: 0, Valid: true},
		LastErrorCode: "",
		NextRetryAt:   sql.NullTime{},
		UpdatedAt:     timeOrNow(updatedAt),
	})
	if err != nil {
		return false, fmt.Errorf("reactivate usage source %d: %w", id, err)
	}
	return n > 0, nil
}

// MarkUsageSourceFailure records a failed ingestion attempt and its bounded
// retry schedule.
func (s *Store) MarkUsageSourceFailure(ctx context.Context, id, failureCount int64, lastErrorCode string, nextRetryAt, updatedAt time.Time) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	n, err := s.qw.UpdateUsageSourceLifecycle(ctx, gen.UpdateUsageSourceLifecycleParams{
		ID:            id,
		State:         domain.UsageSourceError,
		FailureCount:  sql.NullInt64{Int64: failureCount, Valid: true},
		LastErrorCode: lastErrorCode,
		NextRetryAt:   sql.NullTime{Time: nextRetryAt.UTC(), Valid: true},
		UpdatedAt:     timeOrNow(updatedAt),
	})
	if err != nil {
		return false, fmt.Errorf("mark usage source %d failure: %w", id, err)
	}
	return n > 0, nil
}

// ApplyUsageChunk atomically writes parsed usage events and advances the source
// cursor/baselines. The cursor never moves unless all event writes commit.
func (s *Store) ApplyUsageChunk(
	ctx context.Context,
	sourceID, expectedOffset int64,
	expectedRevision time.Time,
	nextState domain.SourceCursorState,
	events []domain.ModelUsageEvent,
) error {
	if nextState.ParserStateJSON != "" {
		if err := validateParserStateObject(nextState.ParserStateJSON); err != nil {
			return err
		}
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	err := s.inTx(ctx, "apply usage chunk", func(q *gen.Queries) error {
		source, err := q.GetUsageSourceWithBindingAndSession(ctx, sourceID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("usage source %d not found", sourceID)
		}
		if err != nil {
			return err
		}
		if source.ByteOffset != expectedOffset {
			return fmt.Errorf("%w: source %d has offset %d, expected %d", domain.ErrUsageSourceOffsetConflict, sourceID, source.ByteOffset, expectedOffset)
		}
		if !source.SourceUpdatedAt.Equal(expectedRevision) ||
			(source.SourceState == domain.UsageSourceComplete && source.SourceLastErrorCode == domain.UsageErrorArtifactReplaced) {
			return fmt.Errorf("%w: source %d changed while its chunk was being read", domain.ErrUsageSourceRevisionConflict, sourceID)
		}
		insertedEvent := false
		for _, ev := range events {
			ev.ModelID = strings.TrimSpace(ev.ModelID)
			ev.BillingProviderID = strings.TrimSpace(ev.BillingProviderID)
			if err := validateUsageEvent(source.Harness, ev); err != nil {
				return err
			}
			existing, err := q.GetModelUsageEventByKey(ctx, gen.GetModelUsageEventByKeyParams{
				BindingID:      source.BindingID,
				SourceEventKey: ev.SourceEventKey,
			})
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if err == nil {
				matches, promoteAttribution := usageEventReplayDisposition(existing, ev)
				if !matches {
					return fmt.Errorf("%w: binding %d event %q", domain.ErrUsageSourceEventConflict, source.BindingID, ev.SourceEventKey)
				}
				// A replaced transcript re-emits the same logical event under
				// the same stable key, so this dedup hit can be a row still
				// pointing at the retired generation. Repair skips that
				// generation, so leaving it there strands the attribution.
				if existing.UsageSourceID != sourceID {
					if _, err := q.RehomeOpenUsageEventToReplacementSource(
						ctx,
						gen.RehomeOpenUsageEventToReplacementSourceParams{
							UsageSourceID:         sourceID,
							ID:                    existing.ID,
							ExpectedUsageSourceID: existing.UsageSourceID,
						},
					); err != nil {
						return err
					}
				}
				if promoteAttribution {
					rows, err := q.PromoteInferredUsageEventToObserved(ctx, gen.PromoteInferredUsageEventToObservedParams{
						BillingProviderID:         stringOrNull(ev.BillingProviderID),
						InputCostNanos:            ptrInt64ToNull(ev.Costs.InputCostNanos),
						CachedInputCostNanos:      ptrInt64ToNull(ev.Costs.CachedInputCostNanos),
						OutputCostNanos:           ptrInt64ToNull(ev.Costs.OutputCostNanos),
						EstimatedCostNanos:        ptrInt64ToNull(ev.Costs.EstimatedCostNanos),
						PricingVersion:            ev.Costs.PricingVersion,
						ID:                        existing.ID,
						ExpectedUsageSourceID:     sourceID,
						ExpectedBillingProviderID: existing.BillingProviderID,
					})
					if err != nil {
						return err
					}
					if rows != 1 {
						return fmt.Errorf("%w: binding %d event %q attribution changed", domain.ErrUsageSourceEventConflict, source.BindingID, ev.SourceEventKey)
					}
					insertedEvent = true
				}
				if existing.ProviderUsageJson.Valid || ev.ProviderUsageJSON == "" {
					continue
				}
				rows, err := q.EnrichModelUsageEventProviderUsage(ctx, gen.EnrichModelUsageEventProviderUsageParams{
					ProviderUsageJson: stringOrNull(ev.ProviderUsageJSON),
					ID:                existing.ID,
				})
				if err != nil {
					return err
				}
				if rows > 0 {
					insertedEvent = true
				}
				continue
			}
			if _, err := q.InsertModelUsageEvent(ctx, usageEventInsertParams(source, ev)); err != nil {
				return err
			}
			insertedEvent = true
		}
		if err := q.UpdateUsageSourceCursor(ctx, gen.UpdateUsageSourceCursorParams{
			ID:              sourceID,
			ByteOffset:      nextState.ByteOffset,
			ParserStateJson: stringOrDefault(nextState.ParserStateJSON, source.ParserStateJson),
			State:           usageSourceStateOrDefault(nextState.State),
			FailureCount:    nextState.FailureCount,
			AnomalyCount:    nextState.AnomalyCount,
			NextRetryAt:     ptrTimeToNullTime(nextState.NextRetryAt),
			LastErrorCode:   nextState.LastErrorCode,
			UpdatedAt:       timeOrNow(nextState.UpdatedAt),
		}); err != nil {
			return err
		}
		if insertedEvent {
			return q.TouchUsageBinding(ctx, gen.TouchUsageBindingParams{
				UpdatedAt: timeOrNow(nextState.UpdatedAt),
				ID:        source.BindingID,
			})
		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

// ListUsageCostCandidates returns the next stable bounded page of total-null
// events whose exact canonical provider has not been attempted at version.
func (s *Store) ListUsageModelAggregates(ctx context.Context, sessionID domain.SessionID) ([]domain.UsageModelAggregate, error) {
	rows, err := s.qr.AggregateUsageBySessionHarnessModel(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("aggregate usage for session %s: %w", sessionID, err)
	}
	out := make([]domain.UsageModelAggregate, 0, len(rows))
	for _, row := range rows {
		out = append(out, usageAggregateFromGen(row))
	}
	return out, nil
}

// GetUsageSessionIncomplete reports whether durable collection facts indicate missing usage.
func (s *Store) GetUsageSessionIncomplete(ctx context.Context, sessionID domain.SessionID) (bool, error) {
	incomplete, err := s.qr.GetUsageSessionIncomplete(ctx, string(sessionID))
	if err != nil {
		return false, fmt.Errorf("get usage integrity for session %s: %w", sessionID, err)
	}
	return incomplete != 0, nil
}

// ListCompactSessionUsageAggregates returns sessions with observed usage in
// one grouped read. projectID optionally limits the rows to one dashboard board.
func (s *Store) ListCompactSessionUsageAggregates(ctx context.Context, projectID domain.ProjectID) ([]domain.CompactSessionUsageAggregate, error) {
	rows, err := s.qr.ListCompactSessionUsage(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list compact session usage for project %s: %w", projectID, err)
	}
	out := make([]domain.CompactSessionUsageAggregate, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.CompactSessionUsageAggregate{
			SessionID:       row.SessionID,
			ProcessedTokens: int64PtrWhen(row.ProcessedTokens, row.ProcessedTokensKnown != 0),
			Incomplete:      row.Incomplete != 0,
		})
	}
	return out, nil
}

func usageBindingFromGen(row gen.UsageBinding) domain.UsageBindingRecord {
	return domain.UsageBindingRecord{
		ID:             row.ID,
		SessionID:      row.SessionID,
		Harness:        row.Harness,
		NativeRootID:   row.NativeRootID,
		InitialModelID: row.InitialModelID,
		ProviderHint:   row.ProviderHint,
		State:          row.State,
		LastErrorCode:  row.LastErrorCode,
		UpdatedAt:      row.UpdatedAt,
	}
}

func usageSourceFromGen(row gen.UsageSource) domain.UsageSourceRecord {
	return domain.UsageSourceRecord{
		ID:              row.ID,
		BindingID:       row.BindingID,
		Kind:            row.Kind,
		NativeSessionID: row.NativeSessionID,
		SubagentID:      row.SubagentID,
		ArtifactPath:    row.ArtifactPath,
		FileIdentity:    row.FileIdentity,
		Generation:      row.Generation,
		ByteOffset:      row.ByteOffset,
		ParserStateJSON: row.ParserStateJson,
		State:           row.State,
		FailureCount:    row.FailureCount,
		AnomalyCount:    row.AnomalyCount,
		NextRetryAt:     nullTimePtr(row.NextRetryAt),
		LastErrorCode:   row.LastErrorCode,
		UpdatedAt:       row.UpdatedAt,
	}
}

func usageSourceContextFromGen(row gen.GetUsageSourceWithBindingAndSessionRow) domain.UsageSourceContext {
	return domain.UsageSourceContext{
		Source: domain.UsageSourceRecord{
			ID:              row.SourceID,
			BindingID:       row.BindingID,
			Kind:            row.Kind,
			NativeSessionID: row.NativeSessionID,
			SubagentID:      row.SubagentID,
			ArtifactPath:    row.ArtifactPath,
			FileIdentity:    row.FileIdentity,
			Generation:      row.Generation,
			ByteOffset:      row.ByteOffset,
			ParserStateJSON: row.ParserStateJson,
			State:           row.SourceState,
			FailureCount:    row.FailureCount,
			AnomalyCount:    row.AnomalyCount,
			NextRetryAt:     nullTimePtr(row.NextRetryAt),
			LastErrorCode:   row.SourceLastErrorCode,
			UpdatedAt:       row.SourceUpdatedAt,
		},
		SessionID:      row.SessionID,
		NativeRootID:   row.NativeRootID,
		InitialModelID: row.InitialModelID,
		ProviderHint:   row.ProviderHint,
		BindingState:   row.BindingState,
	}
}

func usageSourceInsertParams(rec domain.UsageSourceRecord) gen.InsertUsageSourceParams {
	return gen.InsertUsageSourceParams{
		BindingID:       rec.BindingID,
		Kind:            rec.Kind,
		NativeSessionID: rec.NativeSessionID,
		SubagentID:      rec.SubagentID,
		ArtifactPath:    rec.ArtifactPath,
		FileIdentity:    rec.FileIdentity,
		Generation:      rec.Generation,
		ByteOffset:      rec.ByteOffset,
		ParserStateJson: stringOrDefault(rec.ParserStateJSON, "{}"),
		State:           usageSourceStateOrDefault(rec.State),
		FailureCount:    rec.FailureCount,
		AnomalyCount:    rec.AnomalyCount,
		NextRetryAt:     ptrTimeToNullTime(rec.NextRetryAt),
		LastErrorCode:   rec.LastErrorCode,
		UpdatedAt:       timeOrNow(rec.UpdatedAt),
	}
}

func usageEventInsertParams(source gen.GetUsageSourceWithBindingAndSessionRow, ev domain.ModelUsageEvent) gen.InsertModelUsageEventParams {
	return gen.InsertModelUsageEventParams{
		BindingID:             source.BindingID,
		UsageSourceID:         source.SourceID,
		ProviderID:            string(ev.ProviderID),
		BillingProviderID:     stringOrNull(ev.BillingProviderID),
		BillingProviderSource: stringOrNull(string(ev.BillingProviderSource)),
		ModelID:               ev.ModelID,
		UsageMeasurementKind:  string(ev.MeasurementKind),
		InputTokens:           ptrInt64ToNull(ev.Tokens.InputTokens),
		CachedInputTokens:     ptrInt64ToNull(ev.Tokens.CachedInputTokens),
		UncachedInputTokens:   ptrInt64ToNull(ev.Tokens.UncachedInputTokens),
		OutputTokens:          ptrInt64ToNull(ev.Tokens.OutputTokens),
		ProviderUsageJson:     stringOrNull(ev.ProviderUsageJSON),
		InputCostNanos:        ptrInt64ToNull(ev.Costs.InputCostNanos),
		CachedInputCostNanos:  ptrInt64ToNull(ev.Costs.CachedInputCostNanos),
		OutputCostNanos:       ptrInt64ToNull(ev.Costs.OutputCostNanos),
		EstimatedCostNanos:    ptrInt64ToNull(ev.Costs.EstimatedCostNanos),
		PricingVersion:        ev.Costs.PricingVersion,
		SourceEventKey:        ev.SourceEventKey,
		CreatedAt:             sql.NullTime{Time: ev.CreatedAt.UTC(), Valid: !ev.CreatedAt.IsZero()},
	}
}

// stringOrNull maps the empty attribution sentinel to SQL NULL so unattributed
// events stay selectable by the legacy repairer.
func stringOrNull(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

// HasOpenUsageAttribution reports whether a source still owns an event whose
// billing attribution a repair pass could finish.
func (s *Store) HasOpenUsageAttribution(ctx context.Context, sourceID int64) (bool, error) {
	open, err := s.qr.HasOpenUsageAttribution(ctx, sourceID)
	if err != nil {
		return false, fmt.Errorf("check open usage attribution for source %d: %w", sourceID, err)
	}
	return open != 0, nil
}

func usageEventReplayDisposition(existing gen.GetModelUsageEventByKeyRow, event domain.ModelUsageEvent) (matches, promoteAttribution bool) {
	genericMatches := existing.ProviderID == string(event.ProviderID) && existing.ModelID == event.ModelID &&
		existing.UsageMeasurementKind == string(event.MeasurementKind) &&
		existing.InputTokens == ptrInt64ToNull(event.Tokens.InputTokens) &&
		existing.CachedInputTokens == ptrInt64ToNull(event.Tokens.CachedInputTokens) &&
		existing.UncachedInputTokens == ptrInt64ToNull(event.Tokens.UncachedInputTokens) &&
		existing.OutputTokens == ptrInt64ToNull(event.Tokens.OutputTokens)
	// A stored object is immutable; a stored NULL predates the capture and the
	// replay is allowed to enrich it.
	if genericMatches && existing.ProviderUsageJson.Valid && event.ProviderUsageJSON != "" {
		genericMatches = existing.ProviderUsageJson.String == event.ProviderUsageJSON
	}
	// A stored row with no billing attribution predates this feature. Keep it
	// token-only comparable so replay never rejects it; the legacy repairer owns
	// filling that column in.
	if !genericMatches || !existing.BillingProviderID.Valid {
		return genericMatches, false
	}
	// An inference is intentionally replaceable by the first observation. That
	// observation also replaces every cost derived from the inferred provider.
	if existing.BillingProviderSource.String == string(domain.UsageBillingProviderInferred) &&
		event.BillingProviderSource == domain.UsageBillingProviderObserved {
		return true, true
	}
	return existing.BillingProviderID.String == event.BillingProviderID, false
}

func usageAggregateFromGen(row gen.AggregateUsageBySessionHarnessModelRow) domain.UsageModelAggregate {
	return domain.UsageModelAggregate{
		Harness: row.Harness,
		ModelID: row.ModelID,
		// A summed metric is only meaningful when every event in the group
		// carried it; one uncollected counter makes the whole sum unknown.
		Tokens: domain.UsageTokenMetrics{
			InputTokens:         int64PtrWhen(row.InputTokens, row.KnownInputTokenCount == row.EventCount),
			CachedInputTokens:   int64PtrWhen(row.CachedInputTokens, row.KnownCachedInputTokenCount == row.EventCount),
			UncachedInputTokens: int64PtrWhen(row.UncachedInputTokens, row.KnownUncachedInputTokenCount == row.EventCount),
			OutputTokens:        int64PtrWhen(row.OutputTokens, row.KnownOutputTokenCount == row.EventCount),
		},
	}
}

func validateUsageEvent(harness domain.AgentHarness, event domain.ModelUsageEvent) error {
	if event.ProviderID == "" || event.ModelID == "" || event.SourceEventKey == "" {
		return fmt.Errorf("invalid usage event identity for %s", harness)
	}
	switch event.MeasurementKind {
	case domain.UsageMeasurementNativeReported, domain.UsageMeasurementAOEstimated,
		domain.UsageMeasurementMixed, domain.UsageMeasurementUnknown:
	default:
		return fmt.Errorf("invalid usage measurement kind %q", event.MeasurementKind)
	}
	metrics := event.Tokens
	for _, value := range []*int64{
		metrics.InputTokens, metrics.CachedInputTokens, metrics.UncachedInputTokens, metrics.OutputTokens,
	} {
		if value != nil && *value < 0 {
			return errors.New("usage event tokens must be nonnegative")
		}
	}
	if metrics.InputTokens != nil && metrics.CachedInputTokens != nil && metrics.UncachedInputTokens != nil &&
		*metrics.InputTokens != *metrics.CachedInputTokens+*metrics.UncachedInputTokens {
		return errors.New("usage input does not equal cached plus uncached input")
	}
	// ALTER TABLE cannot add a CHECK that reads another column, so the pairing
	// the schema comment describes is enforced here: a provider without a source
	// would be an attribution nobody can tell apart from an inference, and a
	// source without a provider names nothing.
	switch event.BillingProviderSource {
	case domain.UsageBillingProviderObserved, domain.UsageBillingProviderInferred:
		if event.BillingProviderID == "" {
			return errors.New("usage event billing provider source requires a billing provider")
		}
	case "":
		if event.BillingProviderID != "" {
			return errors.New("usage event billing provider requires a source")
		}
	default:
		return fmt.Errorf("invalid usage billing provider source %q", event.BillingProviderSource)
	}
	if event.ProviderUsageJSON != "" {
		var object map[string]json.RawMessage
		if err := json.Unmarshal([]byte(event.ProviderUsageJSON), &object); err != nil {
			return fmt.Errorf("provider usage must be a JSON object: %w", err)
		}
	}
	return nil
}

func usageBindingStateOrDefault(state domain.UsageBindingState) domain.UsageBindingState {
	if state == "" {
		return domain.UsageBindingDiscovering
	}
	return state
}

func usageSourceStateOrDefault(state domain.UsageSourceState) domain.UsageSourceState {
	if state == "" {
		return domain.UsageSourcePending
	}
	return state
}

func stringOrDefault(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}

func validateParserStateObject(raw string) error {
	if raw == "" {
		raw = "{}"
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &object); err != nil || object == nil {
		return errors.New("usage parser state must be a JSON object")
	}
	return nil
}

func timeOrNow(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}

func ptrTimeToNullTime(t *time.Time) sql.NullTime {
	if t == nil || t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t.UTC(), Valid: true}
}

func nullTimePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	out := t.Time
	return &out
}

func ptrInt64ToNull(v *int64) sql.NullInt64 {
	if v == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *v, Valid: true}
}

func nullInt64Ptr(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	value := v.Int64
	return &value
}

func int64PtrWhen(v int64, ok bool) *int64 {
	if !ok {
		return nil
	}
	return &v
}
