//go:build !windows

package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func persistentHostPID(t *testing.T, dataDir, sessionID string) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dataDir, "chat-hosts", sessionID, "host.json"))
	if err != nil {
		t.Fatalf("read persistent host descriptor: %v", err)
	}
	var d struct {
		PID int `json:"pid"`
	}
	if err := json.Unmarshal(b, &d); err != nil || d.PID <= 0 {
		t.Fatalf("decode persistent host descriptor: pid=%d err=%v", d.PID, err)
	}
	return d.PID
}

func processAlive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func TestChatTurnSurvivesGracefulUpdaterStyleRestart(t *testing.T) {
	requireE2E(t)
	dataDir := t.TempDir()
	d := startDaemon(t, dataDir)
	project := seedProject(t, d, "graceful-midturn")
	session := chatSession(t, d, project, "Reply with exactly: READY")

	send(t, d, session, "Run the shell command `sleep 20`, then reply with exactly: SURVIVED-GRACEFUL", "graceful-long")
	d.awaitConversation(session, 90*time.Second, "the graceful-restart turn to run", func(s snapshot) bool {
		return s.Turns[len(s.Turns)-1].State == "running"
	})
	hostBefore := persistentHostPID(t, dataDir, session)
	d.stop()
	// Quitting the app takes its provider processes with it: the ownerless
	// host must exit after its grace instead of lingering for adoption.
	deadline := time.Now().Add(30 * time.Second)
	for processAlive(hostBefore) {
		if time.Now().After(deadline) {
			t.Fatalf("detached host %d outlived its daemon", hostBefore)
		}
		time.Sleep(200 * time.Millisecond)
	}

	restarted := startDaemon(t, dataDir)
	restarted.awaitLiveController(session, 90*time.Second)
	// The replacement daemon spawns a fresh host and resumes natively from
	// durable state. The turn the old provider was killed mid-flight settles
	// as interrupted rather than completing.
	settled := restarted.awaitConversation(session, 2*time.Minute, "the interrupted turn to settle", func(s snapshot) bool {
		return terminal(s.Turns[len(s.Turns)-1].State)
	})
	lost := settled.Turns[len(settled.Turns)-1]
	if lost.State == "completed" || contains(settled.assistantText(), "SURVIVED-GRACEFUL") {
		t.Fatalf("killed provider falsely completed the interrupted turn:\n%s", describe(settled))
	}
	send(t, restarted, session, "Reply with exactly: RECOVERED-AFTER-RESTART", "graceful-after")
	recovered := restarted.awaitConversation(session, 3*time.Minute, "a turn after restart", func(s snapshot) bool {
		return contains(s.assistantText(), "RECOVERED-AFTER-RESTART")
	})
	hostAfter := persistentHostPID(t, dataDir, session)
	t.Logf("graceful restart: old_host_pid=%d new_host_pid=%d interrupted_state=%s", hostBefore, hostAfter, lost.State)
	if hostAfter == hostBefore || !contains(recovered.assistantText(), "RECOVERED-AFTER-RESTART") {
		t.Fatalf("session did not recover through a fresh host:\n%s", describe(recovered))
	}
}

func TestChatSessionRecoversAfterMachineStyleHostLoss(t *testing.T) {
	requireE2E(t)
	dataDir := t.TempDir()
	d := startDaemon(t, dataDir)
	project := seedProject(t, d, "machine-loss")
	session := chatSession(t, d, project, "Reply with exactly: READY")

	send(t, d, session, "Run the shell command `sleep 20`, then reply with exactly: MUST-NOT-COMPLETE", "machine-loss-long")
	d.awaitConversation(session, 90*time.Second, "the machine-loss turn to run", func(s snapshot) bool {
		return s.Turns[len(s.Turns)-1].State == "running"
	})
	hostBefore := persistentHostPID(t, dataDir, session)
	// The detached host is a session leader; killing its process group removes
	// both host and provider, matching the process loss caused by a machine reboot.
	if err := syscall.Kill(-hostBefore, syscall.SIGKILL); err != nil {
		t.Fatalf("kill detached host process group %d: %v", hostBefore, err)
	}
	d.kill()

	restarted := startDaemon(t, dataDir)
	restarted.awaitLiveController(session, 90*time.Second)
	settled := restarted.awaitConversation(session, 2*time.Minute, "the lost in-flight turn to settle", func(s snapshot) bool {
		return terminal(s.Turns[len(s.Turns)-1].State)
	})
	lost := settled.Turns[len(settled.Turns)-1]
	if lost.State == "completed" || contains(settled.assistantText(), "MUST-NOT-COMPLETE") {
		t.Fatalf("provider loss falsely completed the interrupted turn:\n%s", describe(settled))
	}
	send(t, restarted, session, "Reply with exactly: RECOVERED-AFTER-HOST-LOSS", "machine-loss-after")
	recovered := restarted.awaitConversation(session, 3*time.Minute, "a turn after host loss", func(s snapshot) bool {
		return contains(s.assistantText(), "RECOVERED-AFTER-HOST-LOSS")
	})
	hostAfter := persistentHostPID(t, dataDir, session)
	t.Logf("machine-style host loss: old_host_pid=%d new_host_pid=%d interrupted_state=%s", hostBefore, hostAfter, lost.State)
	if hostAfter == hostBefore || !contains(recovered.assistantText(), "RECOVERED-AFTER-HOST-LOSS") {
		t.Fatalf("session did not recover through native resume:\n%s", describe(recovered))
	}
}
