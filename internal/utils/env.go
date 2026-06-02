package utils

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/fjacquet/pscale_exporter/internal/models"
)

var envRefPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// ExpandEnv replaces ${VAR} references with the value of the environment variable VAR.
// It returns an error if a referenced variable is not set, so misconfiguration fails
// loudly at startup rather than silently authenticating with an empty secret.
func ExpandEnv(s string) (string, error) {
	var missing []string
	out := envRefPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := envRefPattern.FindStringSubmatch(match)[1]
		val, ok := os.LookupEnv(name)
		if !ok {
			missing = append(missing, name)
			return ""
		}
		return val
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("environment variable(s) referenced in config but not set: %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// ResolveSecrets expands ${ENV} references in cluster gateway/password fields and
// loads passwords from passwordFile when set. Mutates cfg in place.
func ResolveSecrets(cfg *models.Config) error {
	for i := range cfg.Clusters {
		cl := &cfg.Clusters[i]

		gateway, err := ExpandEnv(cl.Gateway)
		if err != nil {
			return fmt.Errorf("cluster %q gateway: %w", cl.Name, err)
		}
		cl.Gateway = gateway

		if cl.Password == "" && cl.PasswordFile != "" {
			data, err := os.ReadFile(cl.PasswordFile)
			if err != nil {
				return fmt.Errorf("cluster %q passwordFile: %w", cl.Name, err)
			}
			cl.Password = strings.TrimSpace(string(data))
			continue
		}

		password, err := ExpandEnv(cl.Password)
		if err != nil {
			return fmt.Errorf("cluster %q password: %w", cl.Name, err)
		}
		cl.Password = password
	}
	return nil
}
