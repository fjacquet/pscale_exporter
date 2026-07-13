package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fjacquet/pscale_exporter/internal/models"
	"gopkg.in/yaml.v2"
)

func TestExpandEnvSuccess(t *testing.T) {
	t.Setenv("PSCALE_TEST_SECRET", "hunter2")
	got, err := ExpandEnv("pre-${PSCALE_TEST_SECRET}-post")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "pre-hunter2-post" {
		t.Errorf("ExpandEnv = %q", got)
	}
}

func TestExpandEnvMissing(t *testing.T) {
	if _, err := ExpandEnv("${PSCALE_DEFINITELY_UNSET_VAR}"); err == nil {
		t.Error("expected error for unset variable")
	}
}

func TestResolveSecretsInterpolatesAndLoadsFile(t *testing.T) {
	t.Setenv("PSCALE_PW1", "envpass")

	pwFile := filepath.Join(t.TempDir(), "pw.txt")
	if err := os.WriteFile(pwFile, []byte("  filepass\n"), 0o600); err != nil {
		t.Fatalf("write pw file: %v", err)
	}

	cfg := &models.Config{Clusters: []models.ClusterConfig{
		{Name: "a", Endpoint: "onefs-a", Port: 8080, Username: "u", Password: "${PSCALE_PW1}"},
		{Name: "b", Endpoint: "onefs-b", Port: 8080, Username: "u", PasswordFile: pwFile},
	}}

	if err := ResolveSecrets(cfg); err != nil {
		t.Fatalf("ResolveSecrets: %v", err)
	}
	if cfg.Clusters[0].Password != "envpass" {
		t.Errorf("env password = %q", cfg.Clusters[0].Password)
	}
	if cfg.Clusters[1].Password != "filepass" {
		t.Errorf("file password = %q (want trimmed 'filepass')", cfg.Clusters[1].Password)
	}
}

func TestResolveSecretsExpandsUsername(t *testing.T) {
	t.Setenv("PSCALE_USER1", "monitor-user")
	t.Setenv("PSCALE_PW1", "secret")

	cfg := &models.Config{Clusters: []models.ClusterConfig{
		{Name: "a", Endpoint: "onefs-a", Port: 8080, Username: "${PSCALE_USER1}", Password: "${PSCALE_PW1}"},
	}}

	if err := ResolveSecrets(cfg); err != nil {
		t.Fatalf("ResolveSecrets: %v", err)
	}
	if cfg.Clusters[0].Username != "monitor-user" {
		t.Errorf("username = %q, want %q", cfg.Clusters[0].Username, "monitor-user")
	}
}

func TestResolveSecretsUnsetUsernameVarFails(t *testing.T) {
	t.Setenv("PSCALE_PW1", "secret")

	cfg := &models.Config{Clusters: []models.ClusterConfig{
		{Name: "a", Endpoint: "onefs-a", Port: 8080, Username: "${PSCALE_DEFINITELY_UNSET_USER}", Password: "${PSCALE_PW1}"},
	}}

	if err := ResolveSecrets(cfg); err == nil {
		t.Error("expected error for unset username variable, got nil")
	}
}

func TestResolveSecretsSkipCertificate(t *testing.T) {
	t.Setenv("PSCALE1_SKIP_CERTIFICATE", "true")
	cfg := &models.Config{Clusters: []models.ClusterConfig{{
		Name: "c1", Endpoint: "h", Username: "u", Password: "p",
	}}}
	// Simulate YAML having set a ${VAR} reference on the field.
	if err := yaml.Unmarshal([]byte("insecureSkipVerify: ${PSCALE1_SKIP_CERTIFICATE}\n"), &cfg.Clusters[0]); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	cfg.Clusters[0].Name, cfg.Clusters[0].Endpoint = "c1", "h"
	cfg.Clusters[0].Username, cfg.Clusters[0].Password = "u", "p"
	if err := ResolveSecrets(cfg); err != nil {
		t.Fatalf("ResolveSecrets: %v", err)
	}
	if !cfg.Clusters[0].InsecureSkipVerify.Bool() {
		t.Fatal("PSCALE1_SKIP_CERTIFICATE=true did not resolve to skip-verify")
	}
}
