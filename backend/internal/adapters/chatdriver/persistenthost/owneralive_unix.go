//go:build !windows

package persistenthost

import "os"

// controllerAlive reports whether the recorded owner is still this host's
// parent. A Unix child is reparented the moment its parent dies, so a changed
// parent PID proves the owner is gone even if the OS has already recycled its
// numeric PID for an unrelated process.
func controllerAlive(pid int) bool {
	return pid > 0 && os.Getppid() == pid
}
