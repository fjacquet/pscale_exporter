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
	// Divisor divides the raw OneFS value to reach the unit named by Metric; omit it when
	// the key is already in that unit. The cpu.*.avg keys report tenths of a percent
	// (idle+user+sys sums to 1000), so they carry 10 to satisfy the _percent suffix.
	//
	// It divides rather than multiplies so decimal rescaling stays exact: 41/10 is the
	// double nearest 4.1, whereas 41*0.1 yields 4.1000000000000005, which would litter
	// the exposition output. A future key needing multiplication by N takes 1/N.
	Divisor float64 `json:"divisor,omitempty"`
}

// scale converts a raw OneFS value into the unit Metric names. An unset Divisor passes the
// value through, so the zero StatKeySpec is safe rather than a divide-by-zero trap — these
// keys are ultimately runtime values from /platform/1/statistics/keys, so a spec need not
// come from the embedded table.
func (s StatKeySpec) scale(v float64) float64 {
	if s.Divisor == 0 {
		return v
	}
	return v / s.Divisor
}

var (
	statKeySpecs []StatKeySpec
	statKeyByKey = map[string]StatKeySpec{}
)

func init() {
	if err := json.Unmarshal(statKeysRaw, &statKeySpecs); err != nil {
		log.Fatalf("powerscale: invalid statisticsKeys.json: %v", err)
	}
	for _, s := range statKeySpecs {
		statKeyByKey[s.Key] = s
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
