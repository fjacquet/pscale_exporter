package powerscale

import (
	"sync"
	"time"
)

// ClusterSnapshot is the collected state for a single cluster at one collection cycle.
type ClusterSnapshot struct {
	Cluster     string
	Up          bool
	APIVersion  int // OneFS platform API version detected for this cluster (0 if unknown)
	ScrapeError string
	LastScrape  time.Time
	Samples     []Sample
}

// Snapshot is an immutable view of all clusters' collected metrics, plus an index of
// samples by metric name (shared by the Prometheus and OTLP export paths).
type Snapshot struct {
	PerCluster map[string]*ClusterSnapshot
	byName     map[string][]Sample
	names      []string
}

// BuildSnapshot assembles an immutable Snapshot from per-cluster results.
func BuildSnapshot(clusters []*ClusterSnapshot) *Snapshot {
	snap := &Snapshot{
		PerCluster: make(map[string]*ClusterSnapshot, len(clusters)),
		byName:     make(map[string][]Sample),
	}
	for _, cs := range clusters {
		if cs == nil {
			continue
		}
		snap.PerCluster[cs.Cluster] = cs
		for _, s := range cs.Samples {
			snap.byName[s.Name] = append(snap.byName[s.Name], s)
		}
	}
	snap.names = make([]string, 0, len(snap.byName))
	for name := range snap.byName {
		snap.names = append(snap.names, name)
	}
	return snap
}

// SamplesByName returns all samples (across clusters) for a metric name.
func (s *Snapshot) SamplesByName(name string) []Sample { return s.byName[name] }

// MetricNames returns the distinct metric names present in the snapshot.
func (s *Snapshot) MetricNames() []string { return s.names }

// SnapshotStore holds the latest published Snapshot with a pointer-swap under RWMutex,
// so the collection loop can publish while exporters read concurrently.
type SnapshotStore struct {
	mu      sync.RWMutex
	current *Snapshot
}

// NewSnapshotStore returns a store seeded with an empty snapshot.
func NewSnapshotStore() *SnapshotStore {
	return &SnapshotStore{current: BuildSnapshot(nil)}
}

// Load returns the current snapshot (safe for concurrent readers).
func (st *SnapshotStore) Load() *Snapshot {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.current
}

// Store publishes a new snapshot.
func (st *SnapshotStore) Store(s *Snapshot) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.current = s
}
