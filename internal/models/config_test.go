package models

import (
	"testing"

	"gopkg.in/yaml.v2"
)

func TestClusterConfigBaseURLAndValidation(t *testing.T) {
	c := ClusterConfig{Name: "c1", Endpoint: "onefs.example.com", Port: 8080, Username: "u", Password: "p"}
	if got := c.BaseURL(); got != "https://onefs.example.com:8080" {
		t.Fatalf("BaseURL = %q", got)
	}

	cfg := &Config{Clusters: []ClusterConfig{c}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	if cfg.Clusters[0].Port != 8080 {
		t.Fatalf("port = %d", cfg.Clusters[0].Port)
	}
}

func TestConfigDefaultsPortAndRejectsMissing(t *testing.T) {
	cfg := &Config{Clusters: []ClusterConfig{{Name: "c1", Endpoint: "h", Username: "u", Password: "p"}}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if cfg.Clusters[0].Port != 8080 {
		t.Fatalf("expected default port 8080, got %d", cfg.Clusters[0].Port)
	}

	bad := &Config{Clusters: []ClusterConfig{{Name: "c1", Username: "u", Password: "p"}}}
	if err := bad.Validate(); err == nil {
		t.Fatal("expected error for missing endpoint")
	}
}

func TestConfigRejectsDuplicateClusterNames(t *testing.T) {
	cfg := &Config{Clusters: []ClusterConfig{
		{Name: "dup", Endpoint: "h1", Port: 8080, Username: "u", Password: "p"},
		{Name: "dup", Endpoint: "h2", Port: 8080, Username: "u", Password: "p"},
	}}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for duplicate cluster names")
	}
}

func TestConfigAcceptsPasswordFileOnly(t *testing.T) {
	cfg := &Config{Clusters: []ClusterConfig{
		{Name: "c1", Endpoint: "h", Port: 8080, Username: "u", PasswordFile: "/etc/secret"},
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("passwordFile-only config should validate, got: %v", err)
	}
}

func TestEnvBoolNativeAndEnvRef(t *testing.T) {
	// native YAML bool
	var native struct {
		Skip EnvBool `yaml:"skip"`
	}
	if err := yaml.Unmarshal([]byte("skip: true\n"), &native); err != nil {
		t.Fatalf("unmarshal native bool: %v", err)
	}
	if !native.Skip.Bool() {
		t.Fatal("native bool true not resolved to true")
	}

	// ${VAR} reference: unresolved until Resolve is called (defaults false)
	var ref struct {
		Skip EnvBool `yaml:"skip"`
	}
	if err := yaml.Unmarshal([]byte("skip: ${SKIP_TLS}\n"), &ref); err != nil {
		t.Fatalf("unmarshal env ref: %v", err)
	}
	if ref.Skip.Bool() {
		t.Fatal("env ref should be false before Resolve")
	}
	// resolve via a fake expander returning "true"
	if err := ref.Skip.Resolve(func(s string) (string, error) { return "true", nil }); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !ref.Skip.Bool() {
		t.Fatal("env ref true not resolved")
	}

	// absent field defaults to false and Resolve is a no-op
	var absent struct {
		Skip EnvBool `yaml:"skip"`
	}
	if err := yaml.Unmarshal([]byte("other: 1\n"), &absent); err != nil {
		t.Fatalf("unmarshal absent: %v", err)
	}
	if err := absent.Skip.Resolve(func(s string) (string, error) { return "", nil }); err != nil {
		t.Fatalf("resolve absent: %v", err)
	}
	if absent.Skip.Bool() {
		t.Fatal("absent field should be false")
	}
}
