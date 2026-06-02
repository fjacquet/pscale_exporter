package models

import "testing"

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
