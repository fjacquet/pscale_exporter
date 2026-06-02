package models

import (
	"fmt"
	"os"
	"reflect"
	"sync"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// SecretResolver expands ${ENV} references and passwordFile entries in a freshly
// decoded config. It is injected to avoid an import cycle with the utils package.
type SecretResolver func(*Config) error

// SafeConfig provides thread-safe access to configuration with hot reload.
// Reads are concurrent (RLock); reloads validate before swapping the pointer.
type SafeConfig struct {
	mu       sync.RWMutex
	C        *Config
	resolver SecretResolver
}

// NewSafeConfig wraps cfg for thread-safe access. The resolver is applied to newly
// loaded configs during ReloadConfig; pass nil to skip secret resolution.
func NewSafeConfig(cfg *Config, resolver SecretResolver) *SafeConfig {
	return &SafeConfig{C: cfg, resolver: resolver}
}

// Get returns the current configuration (read-locked).
func (sc *SafeConfig) Get() *Config {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.C
}

// ReloadConfig loads, resolves and validates a new config from disk, then swaps it in.
// Validation happens before the write lock (fail-fast): an invalid file never affects
// the running exporter. Returns clustersChanged=true when the cluster set differs,
// signalling that the client pool must be rebuilt.
func (sc *SafeConfig) ReloadConfig(configPath string) (clustersChanged bool, err error) {
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return false, fmt.Errorf("config file not found: %s", configPath)
	}

	f, err := os.Open(configPath)
	if err != nil {
		return false, fmt.Errorf("failed to open config: %w", err)
	}
	defer func() { _ = f.Close() }()

	var newCfg Config
	if err := yaml.NewDecoder(f).Decode(&newCfg); err != nil {
		return false, fmt.Errorf("failed to decode config: %w", err)
	}

	if sc.resolver != nil {
		if err := sc.resolver(&newCfg); err != nil {
			return false, fmt.Errorf("failed to resolve secrets: %w", err)
		}
	}

	if err := newCfg.Validate(); err != nil {
		return false, fmt.Errorf("config validation failed: %w", err)
	}

	sc.mu.Lock()
	clustersChanged = !reflect.DeepEqual(sc.C.Clusters, newCfg.Clusters)
	sc.C = &newCfg
	sc.mu.Unlock()

	log.Info("Configuration reloaded successfully")
	if clustersChanged {
		log.Info("Cluster set changed, client pool will be rebuilt")
	}
	return clustersChanged, nil
}
