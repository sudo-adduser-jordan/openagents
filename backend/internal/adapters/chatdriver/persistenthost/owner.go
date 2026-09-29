package persistenthost

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/processalive"
)

const (
	// foreignOwnerPoll is the re-check interval while waiting on an owner.
	foreignOwnerPoll = 500 * time.Millisecond
)

var (
	// foreignOwnerWait bounds how long ConnectOrStart waits for another
	// daemon's host to become replaceable before failing closed. A
	// desktop-updater handoff overlaps only briefly; a genuinely separate
	// live instance never releases, and waiting longer would just burn the
	// caller's own recovery budget.
	foreignOwnerWait = 20 * time.Second
	// ownerReleaseGrace is the host owner-watchdog grace plus margin: once
	// the owner is gone, a well-formed host exits by itself within this long.
	ownerReleaseGrace = startupTimeout + 5*time.Second
	// ownerExitGrace is how long a host waits for a replacement daemon to
	// attach after its owner dies before shutting its provider down.
	ownerExitGrace = startupTimeout
)

// Owner identifies the single daemon process that owns a persistent provider
// host. The zero value disables ownership: no tracking, no matching, exactly
// the legacy behavior. Every daemon-spawned host carries a real owner; only
// tests and pre-ownership descriptors are ever zero.
type Owner struct {
	// PID is the owning daemon's process id.
	PID int
	// Token is a per-boot random identity that tells daemons apart when PIDs
	// are recycled.
	Token string
}

// Valid reports whether the owner identifies a real daemon instance.
func (o Owner) Valid() bool {
	return o.PID > 0 && o.Token != ""
}

// Same reports whether two owners name the same daemon instance.
func (o Owner) Same(other Owner) bool {
	return o.Valid() && other.Valid() && o.PID == other.PID && o.Token == other.Token
}

// NewOwnerToken mints one per-boot daemon identity. Callers fall back to a
// PID-scoped value only if the OS entropy source is unavailable; the token
// still separates this boot from any other process with a different PID.
func NewOwnerToken(pid int) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("pid-%d-fallback", pid)
	}
	return hex.EncodeToString(b[:])
}

// ParseHostArgs decodes the single detached-host wire format shared by the
// Unix and Windows spawn paths:
//
//	chat-host <session> <data-dir> <workdir> <owner-pid> <owner-token> [acp <fingerprint>] -- <provider> [args...]
//
// Env is left unset for the caller (the CLI fills os.Environ()). An error
// always describes CLI misuse; the CLI surfaces it as a usage error.
func ParseHostArgs(args []string) (Config, error) {
	if len(args) < 6 {
		return Config{}, errors.New("chat-host requires <session> <data-dir> <workdir> <owner-pid> <owner-token> [acp <fingerprint>] -- <provider> [args...]")
	}
	pid, err := strconv.Atoi(strings.TrimSpace(args[3]))
	if err != nil || pid <= 0 {
		return Config{}, fmt.Errorf("chat-host requires a positive <owner-pid>, got %q", args[3])
	}
	token := strings.TrimSpace(args[4])
	if token == "" {
		return Config{}, errors.New("chat-host requires a non-empty <owner-token>")
	}
	cfg := Config{
		SessionID: strings.TrimSpace(args[0]),
		DataDir:   args[1],
		Workdir:   args[2],
		Owner:     Owner{PID: pid, Token: token},
	}
	rest := args[5:]
	if len(rest) > 0 && rest[0] == string(ProtocolACP) {
		cfg.Protocol = ProtocolACP
		rest = rest[1:]
		if len(rest) == 0 || rest[0] == "--" {
			return Config{}, errors.New("chat-host acp requires a <fingerprint> before --")
		}
		cfg.OwnershipFingerprint = strings.TrimSpace(rest[0])
		rest = rest[1:]
	}
	if len(rest) < 2 || rest[0] != "--" {
		return Config{}, errors.New("chat-host requires <session> <data-dir> <workdir> <owner-pid> <owner-token> [acp <fingerprint>] -- <provider> [args...]")
	}
	cfg.Argv = rest[1:]
	if cfg.SessionID == "" {
		return Config{}, errors.New("chat-host requires a non-empty <session>")
	}
	return cfg, nil
}

// isForeignOwner reports whether a descriptor names a different daemon
// instance than the caller. Descriptors or callers that predate ownership
// (empty token on either side) are never foreign: they keep the legacy
// adopt-if-compatible behavior.
func isForeignOwner(caller Owner, d Descriptor) bool {
	return d.OwnerToken != "" && caller.Valid() && d.OwnerToken != caller.Token
}

// ownerProcessAlive reports whether a descriptor's recorded owner is still a
// live process. A non-positive PID never counts as alive.
func ownerProcessAlive(d Descriptor) bool {
	return d.OwnerPID > 0 && processalive.Alive(d.OwnerPID)
}

// watchOwner binds the host's lifetime to its owning daemon. While the owner
// lives the watchdog is inert. Once the owner is gone an ownerExitGrace
// countdown runs, so a replacement daemon booting in the same moment (desktop
// updater handoff) still finds the provider alive; any attach cancels the
// countdown via the client generation. If nobody attaches, the watchdog
// triggers the ordinary shutdown path, which reaps the provider process group
// and removes the descriptor. An attached controller also holds the exit: it
// proves a live daemon is driving this host.
func (h *host) watchOwner(owner Owner) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var graceStart time.Time
	var graceGeneration uint64
	for {
		select {
		case <-h.ctx.Done():
			return
		case <-h.shutdown:
			return
		case now := <-ticker.C:
			h.mu.Lock()
			attached := h.client != nil
			generation := h.clientGeneration
			h.mu.Unlock()
			if controllerAlive(owner.PID) {
				graceStart = time.Time{}
				continue
			}
			if attached || (!graceStart.IsZero() && generation != graceGeneration) {
				graceStart = time.Time{}
				continue
			}
			if graceStart.IsZero() {
				graceStart = now
				graceGeneration = generation
				continue
			}
			if now.Sub(graceStart) >= ownerExitGrace {
				h.shutdownOnce.Do(func() { close(h.shutdown) })
				return
			}
		}
	}
}

// awaitForeignHostRelease waits for a host owned by another daemon instance to
// become safe to replace, then returns nil so the caller spawns a fresh host.
// The owner is given foreignOwnerWait to go away (a desktop-updater handoff
// overlaps only briefly); once it is gone, the host's own owner watchdog
// exits it within ownerReleaseGrace. Any failure preserves the durable
// session: the host may still own live work, so callers must treat this like
// any other inconclusive recovery, never like provider death.
func awaitForeignHostRelease(ctx context.Context, cfg Config, d Descriptor) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ownerDeadline := time.Now().Add(foreignOwnerWait)
	for ownerProcessAlive(d) {
		if time.Now().After(ownerDeadline) {
			return fmt.Errorf("%w: session %q owned by live process %d", ErrForeignOwner, cfg.SessionID, d.OwnerPID)
		}
		timer := time.NewTimer(foreignOwnerPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	releaseDeadline := time.Now().Add(ownerReleaseGrace)
	for {
		current, readErr := readDescriptor(cfg.DataDir, cfg.SessionID)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("%w: reread foreign host descriptor: %w", ErrOwnershipInconclusive, readErr)
		}
		if current.Token != d.Token {
			// Whoever owns the session now published a replacement descriptor;
			// this daemon must not race it.
			return fmt.Errorf("%w: session %q changed hands while waiting", ErrForeignOwner, cfg.SessionID)
		}
		if !processalive.Alive(current.PID) {
			// The host process is gone; its descriptor removal is imminent.
			// Fall through to a fresh spawn, whose launch lock reclaims the
			// dead owner's state.
			return nil
		}
		if time.Now().After(releaseDeadline) {
			return fmt.Errorf("%w: ownerless host for session %q did not exit", ErrOwnershipInconclusive, cfg.SessionID)
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
