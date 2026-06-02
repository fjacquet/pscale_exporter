package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
)

// resetLogger restores the global logrus output to stdout after a test so that
// a test pointing the logger at a temp file (later removed) can't affect others.
func resetLogger(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { log.SetOutput(os.Stdout) })
}

func TestPrepareLogsStdoutOnly(t *testing.T) {
	resetLogger(t)
	if err := PrepareLogs(""); err != nil {
		t.Fatalf("PrepareLogs(\"\") returned error: %v", err)
	}
}

func TestPrepareLogsWritesFile(t *testing.T) {
	resetLogger(t)

	// Nested path exercises the MkdirAll branch on a writable parent.
	logPath := filepath.Join(t.TempDir(), "sub", "pflex-exporter.log")
	if err := PrepareLogs(logPath); err != nil {
		t.Fatalf("PrepareLogs(%q) returned error: %v", logPath, err)
	}

	const marker = "log-file-marker"
	LogInfo(marker)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected log file at %q: %v", logPath, err)
	}
	if !strings.Contains(string(data), marker) {
		t.Fatalf("log file does not contain %q; got: %s", marker, data)
	}
}

func TestPrepareLogsFallsBackWhenDirUnwritable(t *testing.T) {
	resetLogger(t)

	// Make the parent of the log dir a regular file so MkdirAll fails with
	// ENOTDIR — this fails regardless of uid (works even when tests run as root).
	parent := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(parent, []byte("x"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	logPath := filepath.Join(parent, "logs", "pflex-exporter.log")

	if err := PrepareLogs(logPath); err != nil {
		t.Fatalf("PrepareLogs should fall back to stdout, got error: %v", err)
	}
	// No log file should exist under the bad path (stat fails with ENOENT or
	// ENOTDIR depending on which component is the offending file).
	if _, err := os.Stat(logPath); err == nil {
		t.Fatalf("expected no log file at %q after fallback", logPath)
	}
}

func TestPrepareLogsFallsBackWhenFileUnwritable(t *testing.T) {
	resetLogger(t)

	// Point logName at an existing directory: MkdirAll on its parent succeeds,
	// but OpenFile on a directory fails with EISDIR — again independent of uid.
	dirAsLog := t.TempDir()

	if err := PrepareLogs(dirAsLog); err != nil {
		t.Fatalf("PrepareLogs should fall back to stdout, got error: %v", err)
	}
}
