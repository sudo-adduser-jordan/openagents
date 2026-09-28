// Package head implements the commit-observing observer that starts delivery
// for work an agent committed on its own.
//
// It exists because a git commit is not a SQLite change. The daemon's change
// stream is built from database triggers, so a commit made inside a session
// worktree is invisible to it: there is no row to update, and therefore no event
// for the board to react to. The daemon instead polls each live session's
// worktree, compares the HEAD commit against the durable delivered-head fact,
// and hands a genuinely new commit to the existing pull-request delivery path.
//
// What this observer deliberately does not do is advance a card. It has no
// column to choose and no opinion about review. It only starts delivery, and
// the card moves afterwards because the daemon observes a pull request -- the
// same path a user-initiated commit takes. That keeps the board invariant
// honest: lanes remain a function of observed PR facts, never of a local commit.
package head

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/observe"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

const (
	// DefaultTickInterval is faster than the SCM observer because a commit is
	// the trigger users are waiting on: it is the moment their work becomes
	// reviewable. It is still a poll, so the cost is one cheap local git
	// rev-parse per live session per tick.
	DefaultTickInterval = 30 * time.Second
	// DefaultFailureBackoff suppresses retries for a session after a failed
	// delivery. The attempt is not abandoned -- the durable delivered-head fact
	// stays empty, so the same commit is retried -- but a session that cannot
	// reach its remote must not be retried on every tick for the life of the
	// daemon.
	DefaultFailureBackoff = 5 * time.Minute
)

// Store is the durable read surface the observer needs.
type Store interface {
	ListAllSessions(ctx context.Context) ([]domain.SessionRecord, error)
}

// Outcome reports what one delivery attempt did, so the observer can log and
// back off without re-deriving the decision.
type Outcome struct {
	// Delivered is true only when this attempt pushed and opened (or found)
	// the pull request. A skip leaves the durable delivered-head fact untouched
	// so a later commit can still be delivered.
	Delivered bool
	// Reason names why the session was skipped. Empty when Delivered is true.
	Reason string
	// URL is the delivered pull request, when there is one.
	URL string
}

// Deliverer starts delivery for one observed commit. The session service
// implements it; the observer never talks to a git adapter or a provider
// directly, so both the automatic and the user-initiated paths share one
// delivery implementation and its exactly-one-PR de-duplication.
type Deliverer interface {
	DeliverSessionHead(ctx context.Context, id domain.SessionID, headSHA string) (Outcome, error)
}

// Config holds optional observer knobs. Zero values use production defaults.
type Config struct {
	Tick           time.Duration
	FailureBackoff time.Duration
	Clock          func() time.Time
	Logger         *slog.Logger
}

// Observer polls live session worktrees for commits the daemon has not
// delivered yet.
type Observer struct {
	store          Store
	deliverer      Deliverer
	workspaces     ports.WorkspaceObserver
	tick           time.Duration
	failureBackoff time.Duration
	clock          func() time.Time
	logger         *slog.Logger
	backoffUntil   map[string]time.Time
}

// New constructs an Observer with safe defaults.
func New(store Store, deliverer Deliverer, workspaces ports.WorkspaceObserver, cfg Config) *Observer {
	o := &Observer{
		store:          store,
		deliverer:      deliverer,
		workspaces:     workspaces,
		tick:           cfg.Tick,
		failureBackoff: cfg.FailureBackoff,
		clock:          cfg.Clock,
		logger:         cfg.Logger,
		backoffUntil:   map[string]time.Time{},
	}
	if o.tick <= 0 {
		o.tick = DefaultTickInterval
	}
	if o.failureBackoff <= 0 {
		o.failureBackoff = DefaultFailureBackoff
	}
	if o.clock == nil {
		o.clock = time.Now
	}
	if o.logger == nil {
		o.logger = slog.Default()
	}
	return o
}

// Start launches the observer loop. The first poll runs immediately inside the
// goroutine, keeping daemon startup non-blocking.
func (o *Observer) Start(ctx context.Context) <-chan struct{} {
	return observe.StartPollLoop(ctx, o.tick, o.Poll, o.logger, "head observer")
}

// Poll runs one synchronous pass over live sessions.
//
// A workspace that cannot be read is skipped without penalty: the observer must
// never treat an unreadable worktree as evidence that anything is wrong, and the
// next tick re-reads it. Only a delivery failure arms the backoff, because that
// is the one outcome the observer should stop retrying quickly.
func (o *Observer) Poll(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if o.store == nil || o.deliverer == nil || o.workspaces == nil {
		return nil
	}
	now := o.clock().UTC()
	sessions, err := o.store.ListAllSessions(ctx)
	if err != nil {
		return err
	}
	for _, rec := range sessions {
		if err := ctx.Err(); err != nil {
			return err
		}
		o.pollSession(ctx, rec, now)
	}
	return nil
}

func (o *Observer) pollSession(ctx context.Context, rec domain.SessionRecord, now time.Time) {
	if until, ok := o.backoffUntil[string(rec.ID)]; ok && now.Before(until) {
		o.logger.Debug("head observer: session in failure backoff", "session", rec.ID, "until", until)
		return
	}
	// Cheap local pre-filter. The authoritative eligibility gate lives in the
	// delivery policy, next to the commit itself; this only avoids a git call
	// for sessions that could not be delivered anyway.
	if rec.IsTerminated || rec.Kind != domain.KindWorker || rec.WorkflowMode != domain.WorkflowModeBuilding {
		return
	}
	worktree := strings.TrimSpace(rec.Metadata.WorkspacePath)
	branch := strings.TrimSpace(rec.Metadata.Branch)
	if worktree == "" || branch == "" {
		return
	}
	obs, err := o.workspaces.ObserveWorkspace(ctx, ports.WorkspaceInfo{Path: worktree, Branch: branch})
	if err != nil {
		// A worktree that cannot be observed tells us nothing. Not a failure.
		o.logger.Debug("head observer: workspace observation failed", "session", rec.ID, "err", err)
		return
	}
	headSHA := strings.TrimSpace(obs.HeadSHA)
	if headSHA == "" || headSHA == strings.TrimSpace(rec.DeliveredHeadSHA) {
		return
	}
	outcome, err := o.deliverer.DeliverSessionHead(ctx, rec.ID, headSHA)
	if err != nil {
		// The delivered-head fact is deliberately left empty on failure, so the
		// same commit is retried. The backoff only keeps that from happening on
		// every tick.
		if !errors.Is(err, context.Canceled) {
			o.logger.Warn("head observer: automatic delivery failed; will retry", "session", rec.ID, "head", headSHA, "err", err)
		}
		o.backoffUntil[string(rec.ID)] = now.Add(o.failureBackoff)
		return
	}
	delete(o.backoffUntil, string(rec.ID))
	if outcome.Delivered {
		o.logger.Info("head observer: delivered session commit", "session", rec.ID, "head", headSHA, "pr", outcome.URL)
		return
	}
	o.logger.Debug("head observer: no automatic delivery for commit", "session", rec.ID, "head", headSHA, "reason", outcome.Reason)
}
