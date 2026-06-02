package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fjacquet/pscale_exporter/internal/models"
)

func TestExpandEnvSuccess(t *testing.T) {
	t.Setenv("PFLEX_TEST_SECRET", "hunter2")
	got, err := ExpandEnv("pre-${PFLEX_TEST_SECRET}-post")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "pre-hunter2-post" {
		t.Errorf("ExpandEnv = %q", got)
	}
}

func TestExpandEnvMissing(t *testing.T) {
	if _, err := ExpandEnv("${PFLEX_DEFINITELY_UNSET_VAR}"); err == nil {
		t.Error("expected error for unset variable")
	}
}

func TestResolveSecretsInterpolatesAndLoadsFile(t *testing.T) {
	t.Setenv("PFLEX_PW1", "envpass")

	pwFile := filepath.Join(t.TempDir(), "pw.txt")
	if err := os.WriteFile(pwFile, []byte("  filepass\n"), 0o600); err != nil {
		t.Fatalf("write pw file: %v", err)
	}

	cfg := &models.Config{Clusters: []models.ClusterConfig{
		{Name: "a", Gateway: "gw-a", Username: "u", Password: "${PFLEX_PW1}"},
		{Name: "b", Gateway: "gw-b", Username: "u", PasswordFile: pwFile},
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
