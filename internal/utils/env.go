package utils

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/fjacquet/pscale_exporter/internal/models"
)

var envRefPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(:-[^}]*)?\}`)

// ExpandEnv replaces ${VAR} references with the value of the environment variable VAR.
// It returns an error if a referenced variable is not set (exported-but-empty is not the
// same thing: that expands to the empty string, matching os.Expand and long-standing
// behaviour here). Credential fields get the stricter treatment — see ExpandEnvSecret.
//
// A reference may carry a fallback as ${VAR:-default}, borrowing the shell / docker-compose
// syntax and its meaning: unset OR empty falls back, and the reference never errors. That
// is what lets a shipped config.yaml drive a non-secret setting from the environment
// (insecureSkipVerify: "${PSCALE1_SKIP_CERTIFICATE:-false}") while still starting on a host
// that never exported it. Use it only where a safe default exists.
func ExpandEnv(s string) (string, error) {
	var missing []string
	out := envRefPattern.ReplaceAllStringFunc(s, func(match string) string {
		m := envRefPattern.FindStringSubmatch(match)
		name, fallback := m[1], m[2]
		val, ok := os.LookupEnv(name)
		if ok && val != "" {
			return val
		}
		if fallback != "" {
			return fallback[len(":-"):] // group 2 keeps its ":-" prefix, so "" means absent
		}
		if !ok {
			missing = append(missing, name)
		}
		return ""
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("environment variable(s) referenced in config but not set: %s", strings.Join(missing, ", "))
	}
	return out, nil
}

// ExpandEnvSecret expands like ExpandEnv, but additionally rejects a credential that was
// written as an env reference yet resolves to nothing. `PSCALE1_PASSWORD=` in a .env file
// is a plausible typo, and without this the exporter would authenticate with an empty
// password and report an auth failure that names the wrong cause.
//
// It fires only when the field actually contains a ${...} reference: a literal value is
// passed through untouched, and an omitted optional credential stays omitted, so this
// cannot break a config that never referenced the environment in the first place.
func ExpandEnvSecret(field, s string) (string, error) {
	out, err := ExpandEnv(s)
	if err != nil {
		return "", err
	}
	if out == "" && envRefPattern.MatchString(s) {
		return "", fmt.Errorf("%s references %s, which resolved to an empty value", field, s)
	}
	return out, nil
}

// ResolveSecrets expands ${ENV} references in cluster endpoint/password fields and
// loads passwords from passwordFile when set. Mutates cfg in place.
func ResolveSecrets(cfg *models.Config) error {
	for i := range cfg.Clusters {
		cl := &cfg.Clusters[i]

		if err := cl.InsecureSkipVerify.Resolve(ExpandEnv); err != nil {
			return fmt.Errorf("cluster %q insecureSkipVerify: %w", cl.Name, err)
		}

		endpoint, err := ExpandEnvSecret("endpoint", cl.Endpoint)
		if err != nil {
			return fmt.Errorf("cluster %q endpoint: %w", cl.Name, err)
		}
		cl.Endpoint = endpoint

		username, err := ExpandEnvSecret("username", cl.Username)
		if err != nil {
			return fmt.Errorf("cluster %q username: %w", cl.Name, err)
		}
		cl.Username = username

		if cl.Password == "" && cl.PasswordFile != "" {
			data, err := os.ReadFile(cl.PasswordFile)
			if err != nil {
				return fmt.Errorf("cluster %q passwordFile: %w", cl.Name, err)
			}
			cl.Password = strings.TrimSpace(string(data))
			continue
		}

		password, err := ExpandEnvSecret("password", cl.Password)
		if err != nil {
			return fmt.Errorf("cluster %q password: %w", cl.Name, err)
		}
		cl.Password = password
	}
	return nil
}
