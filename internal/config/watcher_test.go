package config

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

const waitFor = 3 * time.Second

func TestWatchConfigFileReloadsOnWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("a: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	reloaded := make(chan string, 4)
	w, err := WatchConfigFile(path, func(p string) error { reloaded <- p; return nil })
	if err != nil {
		t.Fatalf("WatchConfigFile: %v", err)
	}
	t.Cleanup(func() { _ = w.Close() })

	if err := os.WriteFile(path, []byte("a: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-reloaded:
		if got != path {
			t.Fatalf("reload called with %q, want %q", got, path)
		}
	case <-time.After(waitFor):
		t.Fatal("no reload after the config file was written")
	}
}

func TestWatchConfigFileReloadsOnAtomicReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("a: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	reloaded := make(chan struct{}, 4)
	w, err := WatchConfigFile(path, func(string) error { reloaded <- struct{}{}; return nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })

	// Editors save through a temp file renamed over the target.
	tmp := filepath.Join(dir, "config.yaml.tmp")
	if err := os.WriteFile(tmp, []byte("a: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reloaded:
	case <-time.After(waitFor):
		t.Fatal("no reload after the config file was replaced by rename")
	}
}

func TestWatchConfigFileIgnoresOtherFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("a: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	reloaded := make(chan struct{}, 4)
	w, err := WatchConfigFile(path, func(string) error { reloaded <- struct{}{}; return nil })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })

	if err := os.WriteFile(filepath.Join(dir, "other.yaml"), []byte("x: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reloaded:
		t.Fatal("reload triggered by a file that is not the config")
	case <-time.After(300 * time.Millisecond):
	}
}

func TestWatchConfigFileKeepsWatchingAfterReloadError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("a: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	calls := make(chan struct{}, 8)
	w, err := WatchConfigFile(path, func(string) error { calls <- struct{}{}; return errors.New("invalid config") })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Close() })

	for i := 0; i < 2; i++ {
		if err := os.WriteFile(path, []byte("a: 2\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		select {
		case <-calls:
		case <-time.After(waitFor):
			t.Fatalf("write %d: no reload after a previous reload error", i+1)
		}
	}
}

func TestWatchConfigFileMissingDirFails(t *testing.T) {
	w, err := WatchConfigFile(filepath.Join(t.TempDir(), "nope", "config.yaml"), func(string) error { return nil })
	if err == nil {
		_ = w.Close()
		t.Fatal("expected an error for a config in a directory that does not exist")
	}
}

func TestSIGHUPHandlerReloadsAndSurvivesErrors(t *testing.T) {
	calls := make(chan string, 4)
	n := 0
	SetupSIGHUPHandler("/etc/pscale/config.yaml", func(p string) error {
		calls <- p
		n++
		if n == 1 {
			return errors.New("first reload fails")
		}
		return nil
	})

	for i := 0; i < 2; i++ {
		if err := syscall.Kill(os.Getpid(), syscall.SIGHUP); err != nil {
			t.Fatal(err)
		}
		select {
		case got := <-calls:
			if got != "/etc/pscale/config.yaml" {
				t.Fatalf("reload called with %q", got)
			}
		case <-time.After(waitFor):
			t.Fatalf("SIGHUP %d did not trigger a reload", i+1)
		}
	}
}
