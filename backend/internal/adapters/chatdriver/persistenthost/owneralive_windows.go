//go:build windows

package persistenthost

import (
	"github.com/sudo-adduser-jordan/open-agents/backend/internal/processalive"
)

// controllerAlive reports whether the recorded owner is still a live process.
// Windows has no parent-death reparenting signal, so this is a plain PID
// liveness probe: a recycled PID can briefly read as alive, in which case the
// host lingers until the next successful owner check. Lingering is fail-safe
// (the foreign-owner gate still refuses adoption), never fail-deadly.
func controllerAlive(pid int) bool {
	return processalive.Alive(pid)
}
