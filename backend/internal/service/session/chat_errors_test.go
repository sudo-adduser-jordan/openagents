package session

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/httpd/apierr"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

// A chat resume failure covers several distinct causes -- no stored id, an agent
// that advertises no session/load, a provider that rejected the load -- behind
// one code. Without the driver's own explanation the response is unactionable, so
// the reason has to survive the mapping.
func TestMapChatDriverErrorCarriesTheDriverReason(t *testing.T) {
	err := fmt.Errorf(
		"resume agent openagents-1: resume chat: %w: ACP session/load: {\"code\":-32603,\"message\":\"Internal error\"}",
		ports.ErrChatResumeFailed,
	)

	var e *apierr.Error
	if !errors.As(MapChatDriverError(err), &e) {
		t.Fatalf("mapped = %v, want an *apierr.Error", MapChatDriverError(err))
	}
	if e.Code != "CHAT_RESUME_FAILED" || e.Kind != apierr.KindConflict {
		t.Fatalf("mapped = %v %s, want conflict CHAT_RESUME_FAILED", e.Kind, e.Code)
	}
	reason, _ := e.Details["reason"].(string)
	// The Open Agents wrapping is noise; the driver's own words are the part
	// that identifies the failure.
	if !strings.Contains(reason, "ACP session/load") {
		t.Fatalf("reason = %q, want the driver's explanation", reason)
	}
	if strings.Contains(reason, "resume agent") || strings.Contains(reason, ports.ErrChatResumeFailed.Error()) {
		t.Fatalf("reason = %q, want the sentinel and Open Agents wrapping stripped", reason)
	}
}

// A bare sentinel carries no reason; the details must be omitted rather than
// present-and-empty, so a client can tell "no explanation offered" from one.
func TestMapChatDriverErrorOmitsAnEmptyReason(t *testing.T) {
	var e *apierr.Error
	if !errors.As(MapChatDriverError(ports.ErrChatResumeFailed), &e) {
		t.Fatal("bare sentinel was not mapped")
	}
	if e.Details != nil {
		t.Fatalf("details = %#v, want nil for a bare sentinel", e.Details)
	}
}

// Returning nil is what lets a caller fall through to its own table, so an
// unrelated error must not be claimed here.
func TestMapChatDriverErrorIgnoresUnrelatedErrors(t *testing.T) {
	if got := MapChatDriverError(errors.New("some other failure")); got != nil {
		t.Fatalf("mapped = %v, want nil", got)
	}
	if got := MapChatDriverError(nil); got != nil {
		t.Fatalf("mapped = %v, want nil for a nil error", got)
	}
}

// Every driver sentinel the two route families share must map, and none may
// collide on a code. Two different failures sharing a code is how a client ends
// up offering the wrong recovery.
func TestMapChatDriverErrorCodesAreDistinct(t *testing.T) {
	sentinels := map[string]error{
		"SESSION_MODE_UNSUPPORTED":   ports.ErrChatUnsupported,
		"CHAT_DRIVER_UNAVAILABLE":    ports.ErrChatDriverUnavailable,
		"CHAT_DRIVER_INCOMPATIBLE":   ports.ErrChatDriverIncompatible,
		"CHAT_AUTH_REQUIRED":         ports.ErrChatAuthRequired,
		"CHAT_RESUME_FAILED":         ports.ErrChatResumeFailed,
		"CHAT_RECOVERY_INCONCLUSIVE": ports.ErrChatRecoveryInconclusive,
	}
	for wantCode, sentinel := range sentinels {
		var e *apierr.Error
		if !errors.As(MapChatDriverError(sentinel), &e) {
			t.Fatalf("%s: not mapped", wantCode)
		}
		if e.Code != wantCode {
			t.Fatalf("code = %s, want %s", e.Code, wantCode)
		}
		if e.Message == "" {
			t.Fatalf("%s: mapped with an empty message", wantCode)
		}
	}
}

// The session routes and the conversation routes are handed the same driver
// failure, so they must not answer differently. This is the regression that let
// a failed manager resume answer 500 on one route while the other said 409.
func TestMapChatDriverErrorIsTheOnlyChatTableOnSessionRoutes(t *testing.T) {
	// Anything mapSessionError claims on its own must be a session-lifecycle
	// sentinel, never a chat-driver one: a driver failure has to reach
	// MapChatDriverError first.
	for _, sentinel := range []error{
		ports.ErrChatUnsupported,
		ports.ErrChatDriverUnavailable,
		ports.ErrChatDriverIncompatible,
		ports.ErrChatAuthRequired,
		ports.ErrChatResumeFailed,
		ports.ErrChatRecoveryInconclusive,
	} {
		mapped := toAPIError(fmt.Errorf("resume agent mer-1: %w", sentinel))
		if MapChatDriverError(sentinel) == nil {
			t.Fatalf("%v: sentinel is not in the shared table", sentinel)
		}
		if strings.Contains(mapped.Error(), "INTERNAL") {
			t.Fatalf("%v: fell through to the 500 catch-all", sentinel)
		}
	}
}
