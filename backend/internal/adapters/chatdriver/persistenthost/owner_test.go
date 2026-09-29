package persistenthost

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/processalive"
)

// TestDeadOwnerHelperExit is a helper-process entry point, not a real test:
// it exits immediately so tests can observe a deterministically dead PID.
func TestDeadOwnerHelperExit(t *testing.T) {
	if os.Getenv("OPEN_AGENTS_DEAD_OWNER_HELPER") != "1" {
		t.Skip("helper process entry point")
	}
}

// deadOwnerPID returns a PID that is guaranteed to name no live process on
// any platform: a child that already exited. Zombies read as dead on Unix
// and signaled handles read as dead on Windows.
func deadOwnerPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestDeadOwnerHelperExit")
	cmd.Env = append(os.Environ(), "OPEN_AGENTS_DEAD_OWNER_HELPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start dead-owner helper: %v", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait dead-owner helper: %v", err)
	}
	if processalive.Alive(pid) {
		t.Fatalf("helper pid %d reads as alive after exit", pid)
	}
	return pid
}

func setOwnerSeams(t *testing.T, foreignWait, releaseGrace, exitGrace time.Duration) {
	t.Helper()
	oldForeign, oldRelease, oldExit := foreignOwnerWait, ownerReleaseGrace, ownerExitGrace
	foreignOwnerWait, ownerReleaseGrace, ownerExitGrace = foreignWait, releaseGrace, exitGrace
	t.Cleanup(func() {
		foreignOwnerWait, ownerReleaseGrace, ownerExitGrace = oldForeign, oldRelease, oldExit
	})
}

func TestParseHostArgs(t *testing.T) {
	raw := []string{"sess", "/data", "/work", "1234", "token-1", "--", "/usr/bin/opencode", "acp"}
	cfg, err := ParseHostArgs(raw)
	if err != nil {
		t.Fatalf("raw: %v", err)
	}
	if cfg.SessionID != "sess" || cfg.DataDir != "/data" || cfg.Workdir != "/work" ||
		cfg.Owner.PID != 1234 || cfg.Owner.Token != "token-1" || cfg.Protocol != ProtocolRaw {
		t.Fatalf("raw parsed = %+v", cfg)
	}
	if len(cfg.Argv) != 2 || cfg.Argv[0] != "/usr/bin/opencode" || cfg.Argv[1] != "acp" {
		t.Fatalf("raw argv = %q", cfg.Argv)
	}

	acp := []string{"sess", "/data", "/work", "99", "token-2", "acp", "fp-1", "--", "prov"}
	cfg, err = ParseHostArgs(acp)
	if err != nil {
		t.Fatalf("acp: %v", err)
	}
	if cfg.Protocol != ProtocolACP || cfg.OwnershipFingerprint != "fp-1" || len(cfg.Argv) != 1 {
		t.Fatalf("acp parsed = %+v", cfg)
	}

	for _, args := range [][]string{
		{},
		{"only-session"},
		{"sess", "/data", "/work", "1234", "token-1"},
		{"sess", "/data", "/work", "not-a-pid", "token-1", "--", "prov"},
		{"sess", "/data", "/work", "0", "token-1", "--", "prov"},
		{"sess", "/data", "/work", "1234", "", "--", "prov"},
		{"sess", "/data", "/work", "1234", "token-1", "acp", "--", "prov"},
		{"sess", "/data", "/work", "1234", "token-1", "no-separator", "prov"},
		{"", "/data", "/work", "1234", "token-1", "--", "prov"},
	} {
		if _, err := ParseHostArgs(args); err == nil {
			t.Fatalf("args %q parsed without error", args)
		}
	}
}

func TestHostArgsRoundTrip(t *testing.T) {
	want := Config{
		SessionID: "round-trip", DataDir: t.TempDir(), Workdir: t.TempDir(),
		Owner:    Owner{PID: 4242, Token: "round-trip-token"},
		Protocol: ProtocolACP, OwnershipFingerprint: "fp-round",
		Argv: []string{"/usr/bin/opencode", "acp"},
	}
	got, err := ParseHostArgs(hostArgs(want)[1:])
	if err != nil {
		t.Fatalf("round trip: %v", err)
	}
	if got.SessionID != want.SessionID || got.DataDir != want.DataDir || got.Workdir != want.Workdir ||
		got.Owner != want.Owner || got.Protocol != want.Protocol ||
		got.OwnershipFingerprint != want.OwnershipFingerprint ||
		len(got.Argv) != len(want.Argv) || got.Argv[0] != want.Argv[0] || got.Argv[1] != want.Argv[1] {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
}

func TestOwnerIdentity(t *testing.T) {
	if (Owner{}).Valid() || (Owner{PID: 1}).Valid() || (Owner{Token: "x"}).Valid() {
		t.Fatal("zero/partial owner reads as valid")
	}
	a := Owner{PID: 1, Token: "a"}
	if !a.Valid() || !a.Same(a) || !a.Same(Owner{PID: 1, Token: "a"}) {
		t.Fatal("identical owners do not match")
	}
	for _, other := range []Owner{{PID: 2, Token: "a"}, {PID: 1, Token: "b"}, {}} {
		if a.Same(other) {
			t.Fatalf("owner %+v matches %+v", a, other)
		}
	}
	if token := NewOwnerToken(os.Getpid()); token == "" {
		t.Fatal("empty owner token")
	} else if second := NewOwnerToken(os.Getpid()); second == token {
		t.Fatal("owner token is not unique per call")
	}
}

func TestConnectOrStartRefusesLiveForeignOwner(t *testing.T) {
	setOwnerSeams(t, 200*time.Millisecond, time.Second, time.Second)
	dataDir := t.TempDir()
	foreign := Owner{PID: os.Getpid(), Token: "foreign-live-token"}
	if err := writeDescriptor(dataDir, Descriptor{
		Version: ProtocolVersion, SessionID: "foreign-live", Protocol: ProtocolRaw,
		OwnerPID: foreign.PID, OwnerToken: foreign.Token,
		Address: "127.0.0.1:1", Token: "host-token", PID: os.Getpid(),
	}); err != nil {
		t.Fatal(err)
	}
	cfg := Config{SessionID: "foreign-live", DataDir: dataDir, Workdir: t.TempDir(),
		Owner: testOwner(), Argv: []string{"provider"}}
	start := time.Now()
	_, err := ConnectOrStart(context.Background(), cfg)
	if !errors.Is(err, ErrForeignOwner) {
		t.Fatalf("ConnectOrStart = %v, want ErrForeignOwner", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("foreign-owner refusal took %v", elapsed)
	}
}

func TestConnectOrStartReplacesDeadForeignOwner(t *testing.T) {
	setOwnerSeams(t, 5*time.Second, 2*time.Second, 2*time.Second)
	dataDir := t.TempDir()
	sessionID := "foreign-dead"
	deadPID := deadOwnerPID(t)
	if err := writeDescriptor(dataDir, Descriptor{
		Version: ProtocolVersion, SessionID: sessionID, Protocol: ProtocolRaw,
		OwnerPID: deadPID, OwnerToken: "foreign-dead-token",
		Address: "127.0.0.1:1", Token: "stale-host-token", PID: deadPID,
	}); err != nil {
		t.Fatal(err)
	}
	cfg := Config{SessionID: sessionID, DataDir: dataDir, Workdir: t.TempDir(),
		Owner: testOwner(),
		Env:   append(os.Environ(), "OPEN_AGENTS_CHAT_HOST_PROVIDER_HELPER=1"),
		Argv:  []string{os.Args[0], "-test.run=TestProviderHelper"}}
	transport, err := ConnectOrStart(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ConnectOrStart: %v", err)
	}
	defer func() { _ = transport.Stdin.Close() }()
	if transport.Reconnected {
		t.Fatal("fresh spawn after foreign release unexpectedly reconnected")
	}
	t.Cleanup(func() { _ = Shutdown(context.Background(), dataDir, sessionID) })
	current, err := readDescriptor(dataDir, sessionID)
	if err != nil {
		t.Fatalf("reread descriptor: %v", err)
	}
	if current.OwnerToken != cfg.Owner.Token || current.Token == "stale-host-token" {
		t.Fatalf("descriptor not re-owned: %+v", current)
	}
}

// TestHostExitsWhenOwnerDies is the core die-with-the-app guarantee: a host
// whose owner is already gone shuts its provider down and removes its
// descriptor after the exit grace instead of lingering.
func TestHostExitsWhenOwnerDies(t *testing.T) {
	setOwnerSeams(t, 5*time.Second, 2*time.Second, 2*time.Second)
	dataDir := t.TempDir()
	sessionID := "owner-dies"
	cfg := Config{SessionID: sessionID, DataDir: dataDir, Workdir: t.TempDir(),
		Owner: Owner{PID: deadOwnerPID(t), Token: "dead-owner-token"},
		Env:   append(os.Environ(), "OPEN_AGENTS_CHAT_HOST_PROVIDER_HELPER=1"),
		Argv:  []string{os.Args[0], "-test.run=TestProviderHelper"}}
	transport, err := ConnectOrStart(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ConnectOrStart: %v", err)
	}
	providerPID := requestProviderPID(t, transport, 1, "pid")
	_ = transport.Stdin.Close()

	deadline := time.Now().Add(15 * time.Second)
	for {
		_, readErr := readDescriptor(dataDir, sessionID)
		hostGone := readErr != nil
		providerGone := !processalive.Alive(providerPID)
		if hostGone && providerGone {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("ownerless host lingered: descriptorGone=%v providerGone=%v", hostGone, providerGone)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// TestHostSurvivesWhileOwnerLives guards the other side: a host with a live
// owner does not exit on its own, and an explicit Shutdown still ends it.
func TestHostSurvivesWhileOwnerLives(t *testing.T) {
	dataDir := t.TempDir()
	sessionID := "owner-lives"
	cfg := Config{SessionID: sessionID, DataDir: dataDir, Workdir: t.TempDir(),
		Owner: testOwner(),
		Env:   append(os.Environ(), "OPEN_AGENTS_CHAT_HOST_PROVIDER_HELPER=1"),
		Argv:  []string{os.Args[0], "-test.run=TestProviderHelper"}}
	transport, err := ConnectOrStart(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ConnectOrStart: %v", err)
	}
	_ = transport.Stdin.Close()
	time.Sleep(2500 * time.Millisecond)
	if _, err := readDescriptor(dataDir, sessionID); err != nil {
		t.Fatalf("live-owner host exited on its own: %v", err)
	}
	if err := Shutdown(context.Background(), dataDir, sessionID); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if _, err := readDescriptor(dataDir, sessionID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("descriptor after shutdown: %v", err)
	}
}

func TestForeignDescriptorRoundTrip(t *testing.T) {
	dataDir := t.TempDir()
	sessionID := "foreign-fields"
	cfg := Config{SessionID: sessionID, DataDir: dataDir, Workdir: t.TempDir(),
		Owner: testOwner(),
		Env:   append(os.Environ(), "OPEN_AGENTS_CHAT_HOST_PROVIDER_HELPER=1"),
		Argv:  []string{os.Args[0], "-test.run=TestProviderHelper"}}
	done := make(chan error, 1)
	go func() { done <- Run(context.Background(), cfg) }()
	t.Cleanup(func() {
		_ = Shutdown(context.Background(), dataDir, sessionID)
		<-done
	})
	d := awaitDescriptor(t, dataDir, sessionID)
	if d.OwnerPID != cfg.Owner.PID || d.OwnerToken != cfg.Owner.Token {
		t.Fatalf("descriptor owner = (%d,%q), want (%d,%q)",
			d.OwnerPID, d.OwnerToken, cfg.Owner.PID, cfg.Owner.Token)
	}
	raw, err := os.ReadFile(filepath.Join(dataDir, "chat-hosts", sessionID, "host.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"ownerPid"`, `"ownerToken"`} {
		if !strings.Contains(string(raw), field) {
			t.Fatalf("host.json missing %s: %s", field, raw)
		}
	}
}
