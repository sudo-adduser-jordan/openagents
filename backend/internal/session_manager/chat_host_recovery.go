package sessionmanager

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/adapters/chatdriver/persistenthost"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

const (
	// providerHostShutdownTimeout bounds the best-effort destruction of an
	// unreachable provider host before a restore retry. Shutdown itself waits
	// up to 5s for the host to exit; the margin covers the authenticated
	// handshake on a loaded loopback.
	providerHostShutdownTimeout = 15 * time.Second
)

// isProviderHostTransportFailure reports whether a Chat restore error looks
// like a dead control plane to the persistent provider host rather than a
// verdict from the provider or the recovery machinery. Provider refusals,
// auth failures, and inconclusive-recovery states must never destroy a host:
// the host may still own live work, and killing it would discard evidence.
//
// Transport signatures are matched as substrings because the ACP boundary
// delivers them embedded in JSON-RPC error payloads
// ({"code":-32603,...,"data":{"error":"context deadline exceeded"}}), where
// errors.Is cannot reach them.
func isProviderHostTransportFailure(err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, context.Canceled):
		// The caller is going away; a retry would only drag shutdown.
		return false
	case errors.Is(err, ports.ErrChatRecoveryInconclusive),
		errors.Is(err, ports.ErrChatResumeFailed),
		errors.Is(err, ports.ErrChatConfigOptionInvalid),
		errors.Is(err, ports.ErrChatAuthRequired),
		errors.Is(err, ports.ErrChatDriverIncompatible),
		errors.Is(err, ports.ErrChatDriverUnavailable):
		return false
	}
	text := strings.ToLower(err.Error())
	for _, signature := range []string{
		"context deadline exceeded",
		"use of closed network connection",
		"connection reset by peer",
		"connection refused",
		"broken pipe",
		"unexpected eof",
	} {
		if strings.Contains(text, signature) {
			return true
		}
	}
	return false
}

// resumeChatWithHostRecovery relaunches a Chat controller, and when the
// relaunch fails against an unreachable provider host it destroys that host
// and retries once with a fresh recovery budget. A host that survived its
// daemon with a broken provider conversation fails every resume the same
// way; without the retry the session wedges until someone manually kills the
// provider processes. The retry resumes natively from durable state, so no
// turn is duplicated: StartChat sends no prompts, and staged (deferred)
// sessions are excluded because their first prompt may already have been
// delivered post-commit.
func (m *Manager) resumeChatWithHostRecovery(
	ctx context.Context,
	operation string,
	rec domain.SessionRecord,
	project domain.ProjectRecord,
	ws ports.WorkspaceInfo,
	requireNativeHistory bool,
	historyPolicy domain.SessionInterfaceTransitionHistoryPolicy,
) (RestoreResult, error) {
	result, err := m.resumeChatController(ctx, operation, rec, project, ws, requireNativeHistory, historyPolicy)
	if err == nil || rec.AgentDeferred || !isProviderHostTransportFailure(err) {
		return result, err
	}
	shutCtx, shutCancel := context.WithTimeout(context.WithoutCancel(ctx), providerHostShutdownTimeout)
	shutErr := persistenthost.Shutdown(shutCtx, m.dataDir, string(rec.ID))
	shutCancel()
	if shutErr != nil {
		m.logger.Warn("restore: unreachable provider host survived shutdown; preserving without retry",
			"sessionID", rec.ID, "restoreError", err, "shutdownError", shutErr)
		return result, err
	}
	m.logger.Info("restore: replaced unreachable provider host; retrying resume",
		"sessionID", rec.ID)
	retryCtx, retryCancel := context.WithTimeout(context.WithoutCancel(ctx), m.statusVerificationLimit)
	defer retryCancel()
	return m.resumeChatController(retryCtx, operation, rec, project, ws, requireNativeHistory, historyPolicy)
}
