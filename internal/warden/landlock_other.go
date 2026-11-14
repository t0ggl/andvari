//go:build !linux

package warden

import "log"

// ApplyWebLandlock is a no-op on non-Linux platforms.
func ApplyWebLandlock(workerDir string) error {
	log.Printf("[landlock:web] landlock is not supported on this platform; skipping web server sandbox")
	return nil
}
