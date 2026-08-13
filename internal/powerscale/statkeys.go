package powerscale

// NOTE: Stat-key names (e.g. the node.ifs.cache.* keys in statisticsKeys.json) are NOT in the
// OneFS OpenAPI spec — they are runtime values served by /platform/1/statistics/keys.
// They cannot be validated by the schema-drift guard (schema_guard_test.go); they need
// live-cluster validation. See the provisional-onefs-keys memory note.

import (
	_ "embed"
	"encoding/json"
	"log"
)

//go:embed statisticsKeys.json
var statKeysRaw []byte

// StatKeySpec maps a OneFS statistics key to an exported metric name and scope.
type StatKeySpec struct {
	Key    string `json:"key"`
	Metric string `json:"metric"`
	Scope  string `json:"scope"` // "cluster" or "node"
	// Divisor divides the raw OneFS value to reach the unit named by Metric. Omit it (or
	// use 0) when the key is already in the target unit; init normalizes that to 1. The
	// cpu.*.avg keys report tenths of a percent (idle+user+sys sums to 1000), so they
	// carry 10 to satisfy the _percent suffix.
	//
	// It divides rather than multiplies so decimal rescaling stays exact: 41/10 is the
	// double nearest 4.1, whereas 41*0.1 yields 4.1000000000000005, which would litter
	// the exposition output. A future key needing multiplication by N takes 1/N.
	Divisor float64 `json:"divisor,omitempty"`
}

var (
	statKeySpecs []StatKeySpec
	statKeyByKey = map[string]StatKeySpec{}
)

func init() {
	if err := json.Unmarshal(statKeysRaw, &statKeySpecs); err != nil {
		log.Fatalf("powerscale: invalid statisticsKeys.json: %v", err)
	}
	for i := range statKeySpecs {
		if statKeySpecs[i].Divisor == 0 {
			statKeySpecs[i].Divisor = 1
		}
		statKeyByKey[statKeySpecs[i].Key] = statKeySpecs[i]
	}
}

// QueryKeys returns the distinct statistics keys to request from /statistics/current.
func QueryKeys() []string {
	keys := make([]string, 0, len(statKeySpecs))
	seen := map[string]struct{}{}
	for _, s := range statKeySpecs {
		if _, ok := seen[s.Key]; ok {
			continue
		}
		seen[s.Key] = struct{}{}
		keys = append(keys, s.Key)
	}
	return keys
}
