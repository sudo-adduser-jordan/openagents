// Command backend is a compatibility wrapper for the Open Agents daemon.
// The user-facing CLI lives at cmd/open-agents; keep this wrapper so existing `go run .`
// development workflows continue to start the daemon while scripts migrate.
package main

import (
	"fmt"
	"os"

	"github.com/sudo-adduser-jordan/open-agents/backend/internal/daemon"
)

func main() {
	if err := daemon.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "open-agents backend daemon: "+err.Error())
		os.Exit(1)
	}
}
