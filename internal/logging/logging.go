// Package logging provides centralized structured logging using logrus.
package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	log "github.com/sirupsen/logrus"
)

var startTime = time.Now().Format("2006-01-02T15:04:05")
var programName = filepath.Base(os.Args[0]) + "-" + startTime

// LogInfo logs an informational message tagged with the program name.
func LogInfo(msg string) {
	log.WithFields(log.Fields{"job": programName}).Info(msg)
}

// LogError logs a recoverable error message tagged with the program name.
func LogError(msg string) {
	log.WithFields(log.Fields{"job": programName}).Error(msg)
}

// PrepareLogs configures logging to write JSON to both stdout and a log file.
// If logName is empty, logs go to stdout only.
//
// If the log file cannot be created (e.g. an unwritable path or a read-only
// root filesystem in a container), logging degrades to stdout-only with a
// warning rather than aborting the process: stdout already carries every line
// via the MultiWriter, so the only thing lost is the on-disk copy. This keeps
// the exporter running in restricted environments instead of crash-looping.
func PrepareLogs(logName string) error {
	log.SetFormatter(&log.JSONFormatter{})

	if logName == "" {
		log.SetOutput(os.Stdout)
		return nil
	}

	if dir := filepath.Dir(logName); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.SetOutput(os.Stdout)
			LogError(fmt.Sprintf("log directory %q is not writable, logging to stdout only: %v", dir, err))
			return nil
		}
	}

	logFile, err := os.OpenFile(logName, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o644)
	if err != nil {
		log.SetOutput(os.Stdout)
		LogError(fmt.Sprintf("log file %q is not writable, logging to stdout only: %v", logName, err))
		return nil
	}
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	return nil
}
