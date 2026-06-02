// Package models defines the core data structures for the PowerScale exporter:
// application configuration and the PowerScale REST API response types.
package models

import (
	"errors"
	"fmt"
	"strconv"
	"time"
)

// ClusterConfig holds the connection details for a single PowerScale (OneFS) cluster.
// One exporter process monitors many clusters; Name becomes the `cluster` label.
type ClusterConfig struct {
	Name               string `yaml:"name"`
	Endpoint           string `yaml:"endpoint"` // hostname or IP of any node / SmartConnect name
	Port               int    `yaml:"port"`     // OneFS platform API port (default 8080)
	Username           string `yaml:"username"`
	Password           string `yaml:"password"`
	PasswordFile       string `yaml:"passwordFile"`
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify"`
}

// BaseURL returns the HTTPS base URL for the cluster's OneFS platform API.
func (c ClusterConfig) BaseURL() string {
	return fmt.Sprintf("https://%s:%d", c.Endpoint, c.Port)
}

// MaskPassword returns a masked password suitable for logging.
func (c ClusterConfig) MaskPassword() string {
	if len(c.Password) <= 8 {
		return "****"
	}
	return c.Password[:2] + "****" + c.Password[len(c.Password)-2:]
}

// OTelExportConfig holds the settings shared by the metrics-push and tracing exporters.
type OTelExportConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Endpoint string `yaml:"endpoint"`
	Insecure bool   `yaml:"insecure"`
	// Interval is the OTLP metric push period (metrics only). Ignored for tracing.
	Interval string `yaml:"interval"`
	// SamplingRate is the trace sampling ratio 0.0-1.0 (tracing only). Ignored for metrics.
	SamplingRate float64 `yaml:"samplingRate"`
}

// Config represents the complete application configuration for the PowerScale exporter.
type Config struct {
	Server struct {
		Host    string `yaml:"host"`
		Port    string `yaml:"port"`
		URI     string `yaml:"uri"`
		LogName string `yaml:"logName"`
	} `yaml:"server"`

	Collection struct {
		Interval string `yaml:"interval"` // background collection loop period (e.g. "10s")
		Timeout  string `yaml:"timeout"`  // per-cluster collection timeout (e.g. "8s")
	} `yaml:"collection"`

	OpenTelemetry struct {
		Metrics OTelExportConfig `yaml:"metrics"`
		Tracing OTelExportConfig `yaml:"tracing"`
	} `yaml:"opentelemetry"`

	Clusters []ClusterConfig `yaml:"clusters"`
}

// SetDefaults sets default values for optional configuration fields.
func (c *Config) SetDefaults() {
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.Server.Port == "" {
		c.Server.Port = "2112"
	}
	if c.Server.URI == "" {
		c.Server.URI = "/metrics"
	}
	if c.Collection.Interval == "" {
		c.Collection.Interval = "10s"
	}
	if c.Collection.Timeout == "" {
		c.Collection.Timeout = "8s"
	}
	if c.OpenTelemetry.Metrics.Interval == "" {
		c.OpenTelemetry.Metrics.Interval = c.Collection.Interval
	}
	for i := range c.Clusters {
		if c.Clusters[i].Port == 0 {
			c.Clusters[i].Port = 8080
		}
	}
}

// Validate checks the configuration and returns an error on the first problem found.
// SetDefaults is applied first so optional fields have sensible values.
func (c *Config) Validate() error {
	c.SetDefaults()

	if err := c.validateServer(); err != nil {
		return err
	}
	if err := c.validateCollection(); err != nil {
		return err
	}
	if err := c.validateClusters(); err != nil {
		return err
	}
	if err := c.validateOTel("metrics", c.OpenTelemetry.Metrics); err != nil {
		return err
	}
	return c.validateOTel("tracing", c.OpenTelemetry.Tracing)
}

func (c *Config) validateServer() error {
	if c.Server.Host == "" {
		return errors.New("server host is required")
	}
	if err := validatePort(c.Server.Port); err != nil {
		return fmt.Errorf("invalid server port: %s", c.Server.Port)
	}
	if c.Server.URI == "" {
		return errors.New("server URI is required")
	}
	return nil
}

func (c *Config) validateCollection() error {
	if _, err := time.ParseDuration(c.Collection.Interval); err != nil {
		return fmt.Errorf("invalid collection interval '%s': %w (expected format: 10s, 1m)", c.Collection.Interval, err)
	}
	if _, err := time.ParseDuration(c.Collection.Timeout); err != nil {
		return fmt.Errorf("invalid collection timeout '%s': %w (expected format: 8s, 30s)", c.Collection.Timeout, err)
	}
	return nil
}

func (c *Config) validateClusters() error {
	if len(c.Clusters) == 0 {
		return errors.New("at least one cluster must be configured")
	}
	seen := make(map[string]struct{}, len(c.Clusters))
	for i, cl := range c.Clusters {
		if cl.Name == "" {
			return fmt.Errorf("cluster[%d]: name is required", i)
		}
		if _, dup := seen[cl.Name]; dup {
			return fmt.Errorf("duplicate cluster name: %s", cl.Name)
		}
		seen[cl.Name] = struct{}{}
		if cl.Endpoint == "" {
			return fmt.Errorf("cluster %q: endpoint is required", cl.Name)
		}
		if cl.Port < 1 || cl.Port > 65535 {
			return fmt.Errorf("cluster %q: port must be 1-65535, got %d", cl.Name, cl.Port)
		}
		if cl.Username == "" {
			return fmt.Errorf("cluster %q: username is required", cl.Name)
		}
		if cl.Password == "" && cl.PasswordFile == "" {
			return fmt.Errorf("cluster %q: password is required (set password or passwordFile)", cl.Name)
		}
	}
	return nil
}

func (c *Config) validateOTel(name string, o OTelExportConfig) error {
	if !o.Enabled {
		return nil
	}
	if o.Endpoint == "" {
		return fmt.Errorf("opentelemetry.%s endpoint is required when enabled", name)
	}
	host, port, err := splitHostPort(o.Endpoint)
	if err != nil || host == "" {
		return fmt.Errorf("invalid opentelemetry.%s endpoint: %s (expected host:port)", name, o.Endpoint)
	}
	if err := validatePort(port); err != nil {
		return fmt.Errorf("invalid opentelemetry.%s endpoint port: %s", name, port)
	}
	if name == "metrics" {
		if _, err := time.ParseDuration(o.Interval); err != nil {
			return fmt.Errorf("invalid opentelemetry.metrics interval '%s': %w", o.Interval, err)
		}
	}
	if name == "tracing" && (o.SamplingRate < 0.0 || o.SamplingRate > 1.0) {
		return fmt.Errorf("opentelemetry.tracing samplingRate must be between 0.0 and 1.0, got %f", o.SamplingRate)
	}
	return nil
}

// GetServerAddress returns the host:port the HTTP server binds to.
func (c *Config) GetServerAddress() string {
	return fmt.Sprintf("%s:%s", c.Server.Host, c.Server.Port)
}

// GetCollectionInterval returns the background collection loop period.
func (c *Config) GetCollectionInterval() time.Duration {
	return mustDuration(c.Collection.Interval, 10*time.Second)
}

// GetCollectionTimeout returns the per-cluster collection timeout.
func (c *Config) GetCollectionTimeout() time.Duration {
	return mustDuration(c.Collection.Timeout, 8*time.Second)
}

// GetMetricsPushInterval returns the OTLP metric push period.
func (c *Config) GetMetricsPushInterval() time.Duration {
	return mustDuration(c.OpenTelemetry.Metrics.Interval, c.GetCollectionInterval())
}

// IsOTelMetricsEnabled reports whether OTLP metric push is enabled.
func (c *Config) IsOTelMetricsEnabled() bool { return c.OpenTelemetry.Metrics.Enabled }

// IsOTelTracingEnabled reports whether OTLP tracing is enabled.
func (c *Config) IsOTelTracingEnabled() bool { return c.OpenTelemetry.Tracing.Enabled }

func mustDuration(s string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return fallback
	}
	return d
}

func validatePort(portStr string) error {
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	return nil
}

// splitHostPort splits "host:port" handling IPv6 forms like [::1]:4317.
func splitHostPort(endpoint string) (host, port string, err error) {
	lastColon := -1
	for i := len(endpoint) - 1; i >= 0; i-- {
		if endpoint[i] == ':' {
			lastColon = i
			break
		}
	}
	if lastColon == -1 {
		return "", "", errors.New("missing port in endpoint")
	}
	host = endpoint[:lastColon]
	port = endpoint[lastColon+1:]
	if host == "" || port == "" {
		return "", "", errors.New("invalid host:port format")
	}
	return host, port, nil
}
