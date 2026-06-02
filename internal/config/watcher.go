// Package config provides hot-reload helpers: SIGHUP handling and file watching.
package config

import (
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/fsnotify/fsnotify"
	log "github.com/sirupsen/logrus"
)

// ReloadFunc is invoked when a config reload is triggered. Errors are logged but
// do not stop the watcher/handler.
type ReloadFunc func(configPath string) error

// SetupSIGHUPHandler installs a SIGHUP handler that triggers a config reload.
// Runs in a goroutine and returns immediately.
func SetupSIGHUPHandler(configPath string, reloadFn ReloadFunc) {
	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)

	go func() {
		for range sighup {
			log.Info("SIGHUP received, reloading configuration...")
			if err := reloadFn(configPath); err != nil {
				log.Errorf("Configuration reload failed: %v", err)
			}
		}
	}()

	log.Info("SIGHUP handler configured for config reload")
}

// WatchConfigFile watches the directory containing configPath and triggers a reload
// when the file is written or replaced. Watching the directory (not the file) handles
// editors that save via atomic rename. Returns the watcher for cleanup.
func WatchConfigFile(configPath string, reloadFn ReloadFunc) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	configDir := filepath.Dir(configPath)
	configName := filepath.Base(configPath)

	if err := watcher.Add(configDir); err != nil {
		_ = watcher.Close()
		return nil, err
	}

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Base(event.Name) == configName &&
					(event.Has(fsnotify.Write) || event.Has(fsnotify.Create)) {
					log.Info("Config file changed, reloading...")
					if err := reloadFn(configPath); err != nil {
						log.Errorf("Configuration reload failed: %v", err)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Errorf("File watcher error: %v", err)
			}
		}
	}()

	log.Infof("Watching config file: %s", configPath)
	return watcher, nil
}
