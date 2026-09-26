package session

import (
	"errors"
	"strings"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/httpd/apierr"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

// MapChatDriverError maps a Chat driver's sentinels onto their stable API error
// codes, so a client can tell "this harness cannot do Chat" from "the agent is
// not authenticated" from "the stored conversation could not be resumed".
//
// It returns nil when err is not a Chat-driver sentinel, leaving the caller's
// own mapping in charge.
//
// Both the session routes (resume, restore, spawn) and the conversation routes
// can be handed the same driver failure, so they share this one table rather
// than keeping separate copies. They used to: the conversation routes mapped
// ErrChatResumeFailed to 409 CHAT_RESUME_FAILED while mapSessionError had no
// case for it, so the identical failure answered 409 on one route and an opaque
// 500 INTERNAL_ERROR on the other -- which is how a manager that could not be
// resumed reported itself as a server fault.
func MapChatDriverError(err error) error {
	switch {
	case err == nil:
		return nil

	case errors.Is(err, ports.ErrChatUnsupported):
		var capabilityErr *ports.ChatCapabilityError
		if errors.As(err, &capabilityErr) {
			missing := make([]string, 0, len(capabilityErr.Missing))
			for _, missingCapability := range capabilityErr.Missing {
				missing = append(missing, string(missingCapability))
			}
			allowed := make([]string, 0, len(capabilityErr.AllowedPermissionModes))
			for _, allowedMode := range capabilityErr.AllowedPermissionModes {
				allowed = append(allowed, string(allowedMode))
			}
			return apierr.Conflict("SESSION_MODE_UNSUPPORTED", err.Error(), map[string]any{
				"missingCapabilities":  missing,
				"allowedApprovalModes": allowed,
			})
		}
		return apierr.Conflict("SESSION_MODE_UNSUPPORTED", err.Error(), nil)

	case errors.Is(err, ports.ErrChatDriverUnavailable):
		return apierr.Conflict("CHAT_DRIVER_UNAVAILABLE", err.Error(), nil)

	case errors.Is(err, ports.ErrChatDriverIncompatible):
		return apierr.Conflict("CHAT_DRIVER_INCOMPATIBLE", err.Error(), nil)

	case errors.Is(err, ports.ErrChatAuthRequired):
		return apierr.Conflict("CHAT_AUTH_REQUIRED", "the agent is installed but not authenticated", nil)

	case errors.Is(err, ports.ErrChatResumeFailed):
		// Deliberately not a silent recovery: the client must offer the user a
		// choice rather than have Open Agents invent a fresh conversation.
		//
		// The driver's own explanation rides along in details. Without it this
		// error is unactionable -- the same code covers a missing stored id, an
		// agent that advertises no session/load, and a provider that rejected the
		// load -- and the user is left staring at a generic conflict.
		details := map[string]any(nil)
		if reason := chatDriverReason(err, ports.ErrChatResumeFailed); reason != "" {
			details = map[string]any{"reason": reason}
		}
		return apierr.Conflict("CHAT_RESUME_FAILED",
			"the stored provider conversation could not be resumed", details)

	case errors.Is(err, ports.ErrChatRecoveryInconclusive):
		// Not proof the agent died: a detached host may still own live work. The
		// durable session and worktree are intact, so this is a conflict the user
		// can retry, never a reason to discard anything.
		details := map[string]any(nil)
		if reason := chatDriverReason(err, ports.ErrChatRecoveryInconclusive); reason != "" {
			details = map[string]any{"reason": reason}
		}
		return apierr.Conflict("CHAT_RECOVERY_INCONCLUSIVE",
			"Open Agents could not confirm whether the previous agent is still running", details)

	default:
		return nil
	}
}

// chatDriverReason returns the driver's own explanation, stripped of the Open
// Agents wrapping that precedes it. The sentinel text is located rather than
// trimmed from the front because the chain reads
// "resume agent x: resume chat: <sentinel>: <driver reason>" and the useful part
// is the tail.
func chatDriverReason(err, sentinel error) string {
	text := err.Error()
	if i := strings.LastIndex(text, sentinel.Error()); i >= 0 {
		text = text[i+len(sentinel.Error()):]
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(text), ":"))
}
