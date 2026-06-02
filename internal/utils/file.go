// Package utils provides file and environment helpers for the PowerScale exporter.
package utils

import (
	"fmt"
	"os"

	"github.com/fjacquet/pscale_exporter/internal/models"
	"gopkg.in/yaml.v2"
)

// FileExists reports whether the given path exists and is accessible.
func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// ReadFile parses a YAML config file into cfg, then resolves secrets
// (${ENV} interpolation and passwordFile references).
func ReadFile(cfg *models.Config, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open config file %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(cfg); err != nil {
		return fmt.Errorf("failed to decode config file %s: %w", path, err)
	}

	if err := ResolveSecrets(cfg); err != nil {
		return err
	}
	return nil
}
