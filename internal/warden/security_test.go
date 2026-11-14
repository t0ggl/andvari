package warden

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWebLandlockNegativeSecurity(t *testing.T) {
	if os.Getenv("ANDVARI_TEST_WEB_LANDLOCK") == "1" {
		runWebLandlockChild(t)
		return
	}

	if runtime.GOOS != "linux" {
		t.Skip("Landlock is only supported on Linux")
	}

	tmpDir, err := os.MkdirTemp("", "andvari-web-landlock-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	workerDir := filepath.Join(tmpDir, "workers")
	configDir := filepath.Join(tmpDir, "config")
	dataDir := filepath.Join(tmpDir, "data")

	_ = os.MkdirAll(workerDir, 0700)
	_ = os.MkdirAll(configDir, 0700)
	_ = os.MkdirAll(dataDir, 0700)

	configFile := filepath.Join(configDir, "secrets.json")
	if err := os.WriteFile(configFile, []byte(`{"jwt_secret":"supersecret"}`), 0600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	dataFile := filepath.Join(dataDir, "user_data.txt")
	if err := os.WriteFile(dataFile, []byte("sensitive user documents"), 0600); err != nil {
		t.Fatalf("failed to write data file: %v", err)
	}

	// Spawn helper process to test Landlock sandbox
	cmd := exec.Command(os.Args[0], "-test.run=TestWebLandlockNegativeSecurity")
	cmd.Env = append(os.Environ(),
		"ANDVARI_TEST_WEB_LANDLOCK=1",
		"TEST_WORKER_DIR="+workerDir,
		"TEST_CONFIG_FILE="+configFile,
		"TEST_DATA_FILE="+dataFile,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Web Landlock security test helper failed: %v\nOutput: %s", err, string(out))
	}
}

func runWebLandlockChild(t *testing.T) {
	workerDir := os.Getenv("TEST_WORKER_DIR")
	configFile := os.Getenv("TEST_CONFIG_FILE")
	dataFile := os.Getenv("TEST_DATA_FILE")

	if workerDir == "" || configFile == "" || dataFile == "" {
		fmt.Fprintf(os.Stderr, "missing required env vars\n")
		os.Exit(1)
	}

	// Verify before Landlock, files are readable
	if _, err := os.ReadFile(configFile); err != nil {
		fmt.Fprintf(os.Stderr, "pre-check config read failed: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.ReadFile(dataFile); err != nil {
		fmt.Fprintf(os.Stderr, "pre-check data read failed: %v\n", err)
		os.Exit(1)
	}

	// Apply Web Server Landlock
	if err := ApplyWebLandlock(workerDir); err != nil {
		fmt.Fprintf(os.Stderr, "ApplyWebLandlock failed: %v\n", err)
		os.Exit(1)
	}

	// 1. Negative test: Attempt to read config file -> MUST BE DENIED
	_, err := os.ReadFile(configFile)
	if err == nil {
		fmt.Fprintf(os.Stderr, "SECURITY BREACH: Web Server was able to read config file %s after Landlock!\n", configFile)
		os.Exit(2)
	}
	if !os.IsPermission(err) && !strings.Contains(err.Error(), "permission denied") {
		fmt.Fprintf(os.Stderr, "Expected permission denied for config file, got: %v\n", err)
		os.Exit(2)
	}

	// 2. Negative test: Attempt to read data file -> MUST BE DENIED
	_, err = os.ReadFile(dataFile)
	if err == nil {
		fmt.Fprintf(os.Stderr, "SECURITY BREACH: Web Server was able to read user data file %s after Landlock!\n", dataFile)
		os.Exit(3)
	}
	if !os.IsPermission(err) && !strings.Contains(err.Error(), "permission denied") {
		fmt.Fprintf(os.Stderr, "Expected permission denied for data file, got: %v\n", err)
		os.Exit(3)
	}

	// 3. Positive test: WorkerDir must still be accessible
	allowedTestFile := filepath.Join(workerDir, "test.sock")
	if err := os.WriteFile(allowedTestFile, []byte("ok"), 0600); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write in workerDir: %v\n", err)
		os.Exit(4)
	}

	fmt.Fprintf(os.Stdout, "SUCCESS: Web Server Landlock correctly blocked config and data access!\n")
	os.Exit(0)
}
