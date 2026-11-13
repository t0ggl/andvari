//go:build !linux

package worker

import "log"

// ApplyLandlock is a no-op on non-Linux platforms.
func ApplyLandlock(userDir string) error {
	log.Printf("[landlock] landlock is not supported on this platform; skipping kernel sandbox")
	return nil
}
