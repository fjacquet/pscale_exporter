// Command extract-schemas regenerates internal/powerscale/testdata/onefs_schemas.json:
// for each endpoint the exporter consumes, it resolves the documented 200-response schema
// (flattening $ref/allOf and arrays) into a sorted set of dotted field paths. Run via
// `make schemas` whenever a new OneFS OpenAPI spec is adopted.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// targets maps each consumed endpoint to its fixture. Fixtures with no typed response
// schema (nfs_exports.json, smb_shares.json, snapshots.json, session.json, latest.json)
// are intentionally not guarded.
var targets = map[string]string{
	"/platform/3/cluster/config":               "cluster_config.json",
	"/platform/3/cluster/nodes":                "nodes.json",
	"/platform/8/quota/quotas":                 "quotas.json",
	"/platform/1/snapshot/snapshots-summary":   "snapshots_summary.json",
	"/platform/1/dedupe/dedupe-summary":        "dedupe_summary.json",
	"/platform/3/event/eventgroup-occurrences": "events.json",
	"/platform/1/statistics/current":           "stat_current.json",
	"/platform/3/statistics/summary/protocol":  "stat_protocol.json",
	"/platform/3/statistics/summary/drive":     "stat_drive.json",
	"/platform/3/statistics/summary/client":    "stat_client.json",
	"/platform/7/sync/policies":                "sync_policies.json",
}

const (
	// defaultSpec must be updated when adopting a new OneFS spec (see docs/swagger/README.md).
	defaultSpec = "docs/swagger/11035-9.14.0.json"
	outPath     = "internal/powerscale/testdata/onefs_schemas.json"
	maxDepth    = 8
)

func main() {
	specPath := defaultSpec
	if len(os.Args) > 1 {
		specPath = os.Args[1]
	}
	raw, err := os.ReadFile(specPath)
	if err != nil {
		fail("read spec %s: %v", specPath, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		fail("parse spec: %v", err)
	}
	comps := dig(doc, "components", "schemas")

	// Build output using an ordered slice of endpoint entries to avoid ranging over a map.
	type entry struct {
		fx     string
		fields []string
	}
	entries := make([]entry, 0, len(targets))
	for path, fx := range targets {
		sch := responseSchema(doc, path)
		if sch == nil {
			fail("no 200 schema for %s", path)
		}
		fields := schemaFields(sch, comps)
		sort.Strings(fields)
		entries = append(entries, entry{fx: fx, fields: fields})
	}

	// Assemble the JSON-serialisable map from the already-collected entries.
	out := make(map[string][]string, len(entries))
	for i := range entries {
		out[entries[i].fx] = entries[i].fields
	}
	b, err := json.MarshalIndent(out, "", " ")
	if err != nil {
		fail("marshal: %v", err)
	}
	if err := os.WriteFile(outPath, append(b, '\n'), 0o600); err != nil {
		fail("write %s: %v", outPath, err)
	}
	fmt.Printf("wrote %s (%d endpoints)\n", outPath, len(entries))
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "extract-schemas: "+format+"\n", a...)
	os.Exit(1)
}

func dig(m map[string]any, keys ...string) map[string]any {
	cur := m
	for _, k := range keys {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

func responseSchema(doc map[string]any, path string) map[string]any {
	content := dig(doc, "paths", path, "get", "responses", "200", "content")
	if m, ok := content["application/json"].(map[string]any); ok {
		if s, ok := m["schema"].(map[string]any); ok {
			return s
		}
	}
	for _, v := range content {
		if m, ok := v.(map[string]any); ok {
			if s, ok := m["schema"].(map[string]any); ok {
				return s
			}
		}
	}
	return nil
}

func resolveRef(ref string, comps map[string]any) map[string]any {
	parts := strings.Split(ref, "/")
	if s, ok := comps[parts[len(parts)-1]].(map[string]any); ok {
		return s
	}
	return nil
}

// fieldSet accumulates unique dotted field paths as an append-only slice.
// Uniqueness is tracked by a separate visited bool-map that is never ranged over.
type fieldSet struct {
	list    []string
	visited map[string]bool
}

func newFieldSet() *fieldSet {
	return &fieldSet{visited: make(map[string]bool)}
}

func (fs *fieldSet) add(path string) {
	if !fs.visited[path] {
		fs.visited[path] = true
		fs.list = append(fs.list, path)
	}
}

// schemaFields resolves all dotted property paths for a schema and returns them as a slice.
func schemaFields(schema map[string]any, comps map[string]any) []string {
	fs := newFieldSet()
	walk(schema, "", fs, comps, map[string]bool{}, 0)
	return fs.list
}

// walk collects dotted property paths, following $ref/allOf/anyOf/oneOf and descending
// into array items. seen guards against $ref cycles along a single path; a fresh copy is
// taken per property so the same schema may legitimately appear under different paths.
func walk(schema map[string]any, prefix string, fs *fieldSet, comps map[string]any, seen map[string]bool, depth int) {
	if schema == nil || depth > maxDepth {
		return
	}
	if ref, ok := schema["$ref"].(string); ok {
		if seen[ref] {
			return
		}
		next := copySeen(seen)
		next[ref] = true
		walk(resolveRef(ref, comps), prefix, fs, comps, next, depth)
		return
	}
	if items, ok := schema["items"].(map[string]any); ok {
		// Descend into the element schema, then fall through: an array node may also
		// carry allOf/properties (OpenAPI composition) that define additional fields.
		walk(items, prefix, fs, comps, seen, depth)
	}
	for _, kw := range []string{"allOf", "anyOf", "oneOf"} {
		if arr, ok := schema[kw].([]any); ok {
			for _, sub := range arr {
				if m, ok := sub.(map[string]any); ok {
					walk(m, prefix, fs, comps, seen, depth)
				}
			}
		}
	}
	if props, ok := schema["properties"].(map[string]any); ok {
		for name, sub := range props {
			p := name
			if prefix != "" {
				p = prefix + "." + name
			}
			fs.add(p)
			if m, ok := sub.(map[string]any); ok {
				walk(m, p, fs, comps, copySeen(seen), depth+1)
			}
		}
	}
}

func copySeen(s map[string]bool) map[string]bool {
	c := make(map[string]bool, len(s))
	for k := range s {
		c[k] = true
	}
	return c
}
