//go:build linux

package worker

import (
	"log"

	"github.com/landlock-lsm/go-landlock/landlock"
)

// ApplyLandlock restricts the current process so it can only access files under userDir.
// Any attempt to open, stat, or modify paths outside userDir will be blocked with EACCES by the kernel.
func ApplyLandlock(userDir string) error {
	err := landlock.V5.BestEffort().RestrictPaths(
		landlock.RWDirs(userDir).WithRefer(),
	)
	if err != nil {
		log.Printf("[landlock] failed to apply Landlock sandbox for %s: %v", userDir, err)
		return err
	}
	return nil
}
