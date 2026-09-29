package acp

import (
	"context"
	"io"
	"testing"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/adapters/chatdriver/persistenthost"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/domain"
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/ports"
)

// TestConnectProcessRecordsHostOwner pins the ownership handoff: the daemon
// identity the chat service attaches to a start/resume must reach the host
// descriptor, or hosts outlive their daemon and daemons adopt foreign hosts.
func TestConnectProcessRecordsHostOwner(t *testing.T) {
	var got persistenthost.Config
	driver := New(Config{
		Harness: domain.HarnessOpenCode,
		Launch: func(context.Context, LaunchConfig) (Launch, error) {
			return Launch{Command: "fake"}, nil
		},
	}, nil)
	pr, pw := io.Pipe()
	defer func() { _ = pr.Close(); _ = pw.Close() }()
	driver.connectHost = func(_ context.Context, hostCfg persistenthost.Config) (*persistenthost.Transport, error) {
		got = hostCfg
		return &persistenthost.Transport{Stdin: pw, Stdout: pr}, nil
	}
	want := ports.ChatHostOwner{PID: 4242, Token: "owner-token"}
	if _, err := driver.connectProcess(context.Background(), LaunchConfig{
		SessionID: "owner-map", DataDir: t.TempDir(), WorkspacePath: t.TempDir(),
		HostOwner: want,
	}, nil); err != nil {
		t.Fatalf("connectProcess: %v", err)
	}
	if got.Owner.PID != want.PID || got.Owner.Token != want.Token {
		t.Fatalf("host owner = %+v, want %+v", got.Owner, want)
	}
}
