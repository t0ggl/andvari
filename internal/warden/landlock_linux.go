//go:build linux

package warden

import (
	"log"
	"os"

	"github.com/landlock-lsm/go-landlock/landlock"
)

// ApplyWebLandlock restricts the Web Server process so that:
// 1. It can access the worker directory containing Unix domain sockets (warden.sock, tenant workers).
// 2. It can read network resolution and root CA certificate files (/etc/ssl, /etc/resolv.conf, etc.).
// 3. Any attempt to read or modify /data (tenant files) or /config (secrets/hashes) is BLOCKED by the kernel with EACCES.
func ApplyWebLandlock(workerDir string) error {
	rules := []landlock.Rule{
		landlock.RWDirs(workerDir),
	}

	for _, p := range []string{
		"/etc/ssl",
		"/etc/pki",
		"/usr/share/ca-certificates",
		"/etc/ca-certificates",
	} {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			rules = append(rules, landlock.RODirs(p))
		}
	}

	for _, p := range []string{
		"/etc/resolv.conf",
		"/etc/hosts",
	} {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			rules = append(rules, landlock.ROFiles(p))
		}
	}

	err := landlock.V5.BestEffort().RestrictPaths(rules...)
	if err != nil {
		log.Printf("[landlock:web] failed to apply Landlock sandbox for web server: %v", err)
		return err
	}
	log.Printf("[landlock:web] Landlock sandbox successfully applied to web server")
	return nil
}
