package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLandlockNegativeSecurity(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "andvari-landlock-sec-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	aliceDir := filepath.Join(tmpDir, "data", "users", "alice")
	bobDir := filepath.Join(tmpDir, "data", "users", "bob")
	configDir := filepath.Join(tmpDir, "config")
	workerDir := filepath.Join(tmpDir, "workers")

	_ = os.MkdirAll(aliceDir, 0700)
	_ = os.MkdirAll(bobDir, 0700)
	_ = os.MkdirAll(configDir, 0700)

	// Alice's legitimate file
	aliceFile := filepath.Join(aliceDir, "notes.txt")
	if err := os.WriteFile(aliceFile, []byte("alice secret notes"), 0600); err != nil {
		t.Fatalf("failed to create alice file: %v", err)
	}

	// Bob's file that Alice must NOT be able to read
	bobFile := filepath.Join(bobDir, "bob_private.key")
	if err := os.WriteFile(bobFile, []byte("BOB_PRIVATE_KEY"), 0600); err != nil {
		t.Fatalf("failed to create bob file: %v", err)
	}

	// Config file containing system secrets that Alice must NOT read
	configFile := filepath.Join(configDir, "andvari_system.json")
	if err := os.WriteFile(configFile, []byte(`{"jwt_secret":"root"}`), 0600); err != nil {
		t.Fatalf("failed to create config file: %v", err)
	}

	// Malicious symlink inside Alice's directory attempting to escape to Bob
	symlinkPath := filepath.Join(aliceDir, "symlink_to_bob")
	_ = os.Symlink(bobDir, symlinkPath)

	mgr := NewManager(workerDir, 1*time.Minute)
	defer mgr.Close()

	// 1. Legitimate access inside Alice's jail must succeed
	body, code, err := mgr.SecurityTestRead("alice", aliceDir, aliceFile)
	if err != nil || code != 200 {
		t.Fatalf("expected 200 for alice reading own file, got code=%d err=%v", code, err)
	}
	if string(body) != "alice secret notes" {
		t.Fatalf("unexpected content for alice file: %s", string(body))
	}

	// 2. Negative security test: attempt to read Bob's file directly from Alice's worker
	_, code, _ = mgr.SecurityTestRead("alice", aliceDir, bobFile)
	if code != 403 {
		t.Errorf("SECURITY FAILURE: Alice's worker was able to read Bob's file directly! Got code %d", code)
	}

	// 3. Negative security test: attempt to read config directory from Alice's worker
	_, code, _ = mgr.SecurityTestRead("alice", aliceDir, configFile)
	if code != 403 {
		t.Errorf("SECURITY FAILURE: Alice's worker was able to read config directory! Got code %d", code)
	}

	// 4. Negative security test: attempt to read host system files (/etc/passwd or /etc/os-release)
	for _, hostFile := range []string{"/etc/passwd", "/etc/os-release"} {
		if _, err := os.Stat(hostFile); err == nil {
			body, code, _ := mgr.SecurityTestRead("alice", aliceDir, hostFile)
			if code != 403 {
				t.Errorf("SECURITY FAILURE: Alice's worker was able to read host file %s! Code: %d, body: %s", hostFile, code, string(body))
			}
		}
	}

	// 5. Negative security test: attempt to escape sandbox via symlink inside jail pointing outside
	linkTarget := filepath.Join(symlinkPath, "bob_private.key")
	body, code, _ = mgr.SecurityTestRead("alice", aliceDir, linkTarget)
	if code != 403 {
		t.Errorf("SECURITY FAILURE: Alice's worker escaped sandbox via symlink! Got code %d, body: %s", code, string(body))
	}
}

func TestLandlockDeniedErrorReason(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "andvari-landlock-reason-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	userDir := filepath.Join(tmpDir, "data", "users", "tenant1")
	workerDir := filepath.Join(tmpDir, "workers")
	_ = os.MkdirAll(userDir, 0700)

	mgr := NewManager(workerDir, 1*time.Minute)
	defer mgr.Close()

	body, code, _ := mgr.SecurityTestRead("tenant1", userDir, "/etc/shadow")
	if code != 403 {
		t.Fatalf("expected 403 for /etc/shadow, got %d", code)
	}
	// Verify error string mentions permission denied
	if !strings.Contains(string(body), "permission denied") {
		t.Logf("Notice: denied response: %s", string(body))
	}
}
