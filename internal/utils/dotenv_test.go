package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fjacquet/pscale_exporter/internal/models"
)

func TestLoadDotEnvSetsUnsetVars(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DOTENV_TEST_HOST=h1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOTENV_TEST_HOST", "") // register cleanup, then unset for real
	_ = os.Unsetenv("DOTENV_TEST_HOST")

	LoadDotEnv(cfg)
	if got := os.Getenv("DOTENV_TEST_HOST"); got != "h1" {
		t.Errorf("DOTENV_TEST_HOST = %q, want h1", got)
	}
}

func TestLoadDotEnvNeverOverridesRealEnv(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("DOTENV_TEST_PW=from-file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOTENV_TEST_PW", "from-env")

	LoadDotEnv(cfg)
	if got := os.Getenv("DOTENV_TEST_PW"); got != "from-env" {
		t.Errorf("DOTENV_TEST_PW = %q, want from-env (real env must win)", got)
	}
}

func TestLoadDotEnvMissingFileIsNoop(t *testing.T) {
	LoadDotEnv(filepath.Join(t.TempDir(), "config.yaml")) // must not panic or log fatal
}

func TestLoadDotEnvFeedsResolveSecrets(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(filepath.Join(dir, ".env"),
		[]byte("PSCALE_DOTENV_HOST=onefs.example.com\nPSCALE_DOTENV_USER=mon\nPSCALE_DOTENV_PW=s3cret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(`
clusters:
  - name: c1
    endpoint: "${PSCALE_DOTENV_HOST}"
    port: 8080
    username: "${PSCALE_DOTENV_USER}"
    password: "${PSCALE_DOTENV_PW}"
`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"PSCALE_DOTENV_HOST", "PSCALE_DOTENV_USER", "PSCALE_DOTENV_PW"} {
		t.Setenv(v, "")
		_ = os.Unsetenv(v)
	}

	LoadDotEnv(cfgPath)

	cfg := &models.Config{Clusters: []models.ClusterConfig{
		{
			Name:     "c1",
			Endpoint: "${PSCALE_DOTENV_HOST}",
			Port:     8080,
			Username: "${PSCALE_DOTENV_USER}",
			Password: "${PSCALE_DOTENV_PW}",
		},
	}}
	if err := ResolveSecrets(cfg); err != nil {
		t.Fatal(err)
	}
	cl := cfg.Clusters[0]
	if cl.Endpoint != "onefs.example.com" || cl.Username != "mon" || cl.Password != "s3cret" {
		t.Errorf("interpolated cluster = %+v", cl)
	}
}
