package sessionmanager

import (
	"context"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
)

// The store fakes in this package implement the full Store surface but only the
// methods their own tests exercise. RetireSession is on that surface, so each
// fake needs a stub; the fakes that do test retiring override these.
func (f *fakeStore) RetireSession(ctx context.Context, id domain.SessionID, now time.Time) (bool, error) {
	if _, ok := f.sessions[id]; !ok {
		return false, nil
	}
	delete(f.sessions, id)
	return true, nil
}

func (h *historicalChatRestoreStore) RetireSession(ctx context.Context, id domain.SessionID, now time.Time) (bool, error) {
	return h.transitionStore.RetireSession(ctx, id, now)
}
