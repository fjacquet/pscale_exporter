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
	unsetForTest(t, "PSCALE_DEFINITELY_UNSET_VAR")
	if _, err := ExpandEnv("${PSCALE_DEFINITELY_UNSET_VAR}"); err == nil {
		t.Error("expected error for unset variable")
	}
}

// unsetForTest clears name for the duration of the test and restores whatever was there —
// value and set/unset state alike. Tests that assert on an *unset* variable are otherwise
// at the mercy of whatever the developer or CI runner happens to export.
func unsetForTest(t *testing.T, name string) {
	t.Helper()
	old, had := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("unset %s: %v", name, err)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(name, old)
			return
		}
		_ = os.Unsetenv(name)
	})
}

// TestExpandEnvSecretRejectsEmpty pins the credential-only strictness: a plain ExpandEnv
// lets an exported-but-empty variable through (matching os.Expand), but a credential that
// was written as a reference and resolves to nothing is a misconfiguration — without this
// the exporter would authenticate with an empty password and blame the appliance.
func TestExpandEnvSecretRejectsEmpty(t *testing.T) {
	t.Setenv("PSCALE_TEST_EMPTY_SECRET", "")

	if _, err := ExpandEnv("${PSCALE_TEST_EMPTY_SECRET}"); err != nil {
		t.Fatalf("plain ExpandEnv must stay lenient on an exported-empty variable: %v", err)
	}
	if _, err := ExpandEnvSecret("password", "${PSCALE_TEST_EMPTY_SECRET}"); err == nil {
		t.Error("a credential resolving to an empty value must be rejected")
	}
	// A literal credential is not a reference and must pass through untouched, as must an
	// omitted optional one — otherwise passwordFile setups would break.
	if got, err := ExpandEnvSecret("password", "literal-pw"); err != nil || got != "literal-pw" {
		t.Errorf("literal credential: got %q err=%v", got, err)
	}
	if got, err := ExpandEnvSecret("password", ""); err != nil || got != "" {
		t.Errorf("omitted credential: got %q err=%v", got, err)
	}
}

// TestExpandEnvDefault covers the ${VAR:-default} form, which is what lets config.yaml
// ship an env-driven value that still starts on a host where the variable is not exported.
// A reference without ":-" must keep failing loudly — that is what protects secrets.
func TestExpandEnvDefault(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
	}{
		{"unset falls back", "${PSCALE_DEFINITELY_UNSET_VAR:-false}", "false"},
		{"set wins over default", "${PSCALE_TEST_SKIP:-false}", "true"},
		{"empty default allowed", "${PSCALE_DEFINITELY_UNSET_VAR:-}", ""},
		// ":-" carries its shell / docker-compose meaning: an exported-but-empty variable
		// falls back too, so an empty .env line cannot silently mean "unset".
		{"exported empty falls back", "${PSCALE_TEST_EMPTY:-fallback}", "fallback"},
		{"mixed with literal text", "a${PSCALE_DEFINITELY_UNSET_VAR:-b}c", "abc"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unsetForTest(t, "PSCALE_DEFINITELY_UNSET_VAR")
			t.Setenv("PSCALE_TEST_SKIP", "true")
			t.Setenv("PSCALE_TEST_EMPTY", "")
			got, err := ExpandEnv(tc.in)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("ExpandEnv(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
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
