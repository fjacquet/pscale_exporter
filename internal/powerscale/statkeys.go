package powerscale

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
