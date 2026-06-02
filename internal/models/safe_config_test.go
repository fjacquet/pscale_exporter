package models

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfigFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

const oneCluster = `
server: {host: "0.0.0.0", port: "2112", uri: "/metrics"}
clusters:
  - {name: a, endpoint: onefs-a, port: 8080, username: u, password: p}
`

const twoClusters = `
server: {host: "0.0.0.0", port: "2112", uri: "/metrics"}
clusters:
  - {name: a, endpoint: onefs-a, port: 8080, username: u, password: p}
  - {name: b, endpoint: onefs-b, port: 8080, username: u, password: p}
`

func TestReloadDetectsClusterChange(t *testing.T) {
	sc := NewSafeConfig(&Config{Clusters: []ClusterConfig{{Name: "a", Endpoint: "onefs-a", Port: 8080, Username: "u", Password: "p"}}}, nil)

	path := writeConfigFile(t, oneCluster)
	changed, err := sc.ReloadConfig(path)
	if err != nil {
		t.Fatalf("reload same clusters: %v", err)
	}
	if changed {
		t.Error("expected clustersChanged=false for identical cluster set")
	}

	path2 := writeConfigFile(t, twoClusters)
	changed, err = sc.ReloadConfig(path2)
	if err != nil {
		t.Fatalf("reload new clusters: %v", err)
	}
	if !changed {
		t.Error("expected clustersChanged=true when a cluster is added")
	}
	if len(sc.Get().Clusters) != 2 {
		t.Errorf("expected 2 clusters after reload, got %d", len(sc.Get().Clusters))
	}
}

func TestReloadRejectsInvalidConfigWithoutMutating(t *testing.T) {
	sc := NewSafeConfig(&Config{Clusters: []ClusterConfig{{Name: "a", Endpoint: "onefs-a", Port: 8080, Username: "u", Password: "p"}}}, nil)

	badPath := writeConfigFile(t, "server: {port: \"2112\"}\nclusters: []\n")
	if _, err := sc.ReloadConfig(badPath); err == nil {
		t.Fatal("expected validation error for config with no clusters")
	}
	if len(sc.Get().Clusters) != 1 {
		t.Errorf("running config should be unchanged after failed reload, got %d clusters", len(sc.Get().Clusters))
	}
}

func TestReloadAppliesResolver(t *testing.T) {
	resolverCalled := false
	resolver := func(c *Config) error {
		resolverCalled = true
		for i := range c.Clusters {
			c.Clusters[i].Password = "resolved"
		}
		return nil
	}
	sc := NewSafeConfig(&Config{Clusters: []ClusterConfig{{Name: "a", Endpoint: "onefs-a", Port: 8080, Username: "u", Password: "p"}}}, resolver)

	path := writeConfigFile(t, oneCluster)
	if _, err := sc.ReloadConfig(path); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !resolverCalled {
		t.Error("expected resolver to be invoked during reload")
	}
	if sc.Get().Clusters[0].Password != "resolved" {
		t.Errorf("expected resolver to set password, got %q", sc.Get().Clusters[0].Password)
	}
}
