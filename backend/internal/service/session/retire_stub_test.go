package session

import (
	"context"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
)

// fakeCommander implements the whole commander surface but only the methods its
// own tests exercise. RetireSession is on that surface, so the stub exists to
// satisfy it; a test that retires for real records the id in retiredSessions.
func (c *fakeCommander) RetireSession(ctx context.Context, id domain.SessionID) (bool, error) {
	c.retiredSessions = append(c.retiredSessions, id)
	return true, nil
}
