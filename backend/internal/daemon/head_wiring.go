package daemon

import (
	"context"
	"log/slog"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/observe/head"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
	sessionsvc "github.com/sudo-adduser-jordan/open-agents/backend/internal/service/session"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/storage/sqlite"
)

// sessionDeliveryAdapter bridges the session service's delivery outcome to the
// head observer's local interface. The observer declares narrow interfaces of
// its own (as scm.Lifecycle and trackerintake.Spawner do) so it can be tested
// without the session service; this adapter is the only place the two meet.
type sessionDeliveryAdapter struct {
	sessions *sessionsvc.Service
}

func (a sessionDeliveryAdapter) DeliverSessionHead(ctx context.Context, id domain.SessionID, headSHA string) (head.Outcome, error) {
	outcome, err := a.sessions.DeliverSessionHead(ctx, id, headSHA)
	return head.Outcome{Delivered: outcome.Delivered, Reason: outcome.Reason, URL: outcome.URL}, err
}

// startHeadObservation wires the commit observer that starts delivery for work
// an agent committed on its own.
//
// The loop always runs. Its own eligibility gate re-reads each session on every
// tick and skips anything that is not a live building-mode worker with a branch
// and a workspace, so no configuration is needed and a session switched into
// building mode after boot is picked up without a restart.
//
// workspaces is the same routed git/scratch adapter the sessions themselves use,
// so a commit is read from exactly the worktree that produced it.
func startHeadObservation(ctx context.Context, store *sqlite.Store, sessions *sessionsvc.Service, workspaces ports.WorkspaceObserver, logger *slog.Logger) <-chan struct{} {
	observer := head.New(store, sessionDeliveryAdapter{sessions: sessions}, workspaces, head.Config{Logger: logger})
	return observer.Start(ctx)
}
