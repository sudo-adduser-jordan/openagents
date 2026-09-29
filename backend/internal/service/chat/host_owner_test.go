package chat

import (
	"os"
	"testing"
)

// TestNewHostOwner pins the daemon identity handed to persistent provider
// hosts: this process's PID plus a unique per-service token. Two services
// (two daemon boots, or daemon plus test) must never share an identity, or
// hosts outlive their owner and daemons adopt foreign hosts.
func TestNewHostOwner(t *testing.T) {
	t.Parallel()
	first := newHostOwner()
	if first.PID != os.Getpid() || first.Token == "" {
		t.Fatalf("host owner = %+v, want this pid plus a token", first)
	}
	second := newHostOwner()
	if second.Token == first.Token {
		t.Fatal("host owner token is not unique per service")
	}
	for _, svc := range []*Service{New(Options{}), New(Options{})} {
		if svc.hostOwner.PID != os.Getpid() || svc.hostOwner.Token == "" {
			t.Fatalf("service host owner = %+v", svc.hostOwner)
		}
	}
	firstSvc := New(Options{}).hostOwner
	secondSvc := New(Options{}).hostOwner
	if firstSvc.Token == secondSvc.Token {
		t.Fatal("services share a host owner token")
	}
}
