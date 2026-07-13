package powerscale

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dell/gopowerscale/api"
	"github.com/fjacquet/pscale_exporter/internal/models"
	log "github.com/sirupsen/logrus"
)

// sessionAuthType selects gopowerscale's session-cookie authentication.
// The SDK uses an unexported constant authTypeSessionBased = 1 (see
// github.com/dell/gopowerscale/api/api.go); it exposes no public named
// constant, so the literal is documented here.
const sessionAuthType = uint8(1)

// ClusterClient satisfies the Client interface.
var _ Client = (*ClusterClient)(nil)

// ClusterClient is the OneFS API client for a single cluster. It owns one authenticated
// gopowerscale session used for both typed resources and the raw statistics API.
type ClusterClient struct {
	name    string
	cli     api.Client
	baseURL string
	dumpDir string // when non-empty, every raw response body is written under it
	trace   bool   // when true, every API response body is logged (see traceResponse)
}

// NewClusterClient establishes one authenticated gopowerscale session for the cluster.
// A non-empty dumpDir enables raw-response dumping (see dump) for offline diagnosis;
// trace enables per-request response-body logging (see traceResponse).
//
// gopowerscale builds request URLs by writing the hostname argument directly, so it must
// be a full base URL (scheme + host + port). It also has no Port field in ClientOptions;
// the port is therefore carried inside the base URL via models.ClusterConfig.BaseURL().
func NewClusterClient(ctx context.Context, cfg models.ClusterConfig, dumpDir string, trace bool) (*ClusterClient, error) {
	if cfg.InsecureSkipVerify.Bool() {
		log.Warnf("cluster %q: TLS verification disabled (insecureSkipVerify=true)", cfg.Name)
	}
	opts := &api.ClientOptions{
		Insecure: cfg.InsecureSkipVerify.Bool(),
	}
	cli, err := api.New(ctx, cfg.BaseURL(), cfg.Username, cfg.Password, "", 0, sessionAuthType, opts)
	if err != nil {
		return nil, fmt.Errorf("cluster %q: auth failed: %w", cfg.Name, err)
	}
	return &ClusterClient{name: cfg.Name, cli: cli, baseURL: cfg.BaseURL(), dumpDir: dumpDir, trace: trace}, nil
}

// Name returns the configured cluster name.
func (c *ClusterClient) Name() string { return c.name }

func (c *ClusterClient) getRaw(ctx context.Context, path string, dst *[]byte) error {
	return c.getRawParams(ctx, path, nil, dst)
}

// getRawParams performs an authenticated GET and captures the raw JSON body. A
// *json.RawMessage destination works because gopowerscale decodes the response via
// json.NewDecoder(...).Decode(resp); RawMessage copies the bytes verbatim, which keeps
// all parsing centralized in the unit-tested models.Parse* functions.
func (c *ClusterClient) getRawParams(ctx context.Context, path string, params api.OrderedValues, dst *[]byte) error {
	start := time.Now()
	var raw json.RawMessage
	if err := c.cli.Get(ctx, path, "", params, nil, &raw); err != nil {
		c.traceResponse(path, nil, err)
		return fmt.Errorf("cluster %q: GET %s/%s: %w", c.name, c.baseURL, path, err)
	}
	log.Debugf("cluster %q: GET %s/%s (%d params): %d bytes in %s",
		c.name, c.baseURL, path, len(params), len(raw), time.Since(start).Round(time.Millisecond))
	c.traceResponse(path, raw, nil)
	c.dump(path, raw)
	*dst = raw
	return nil
}

// traceResponse logs one management API exchange for --trace: method, URL, status,
// and the response BODY — never headers, so OneFS session credentials (the isisessid
// cookie and CSRF token live exclusively in headers) cannot leak. The
// /session/1/session login exchange happens inside gopowerscale (api.New and its 401
// re-authentication) and never flows through this wrapper, so credential endpoints
// are structurally excluded from tracing. gopowerscale discards the *http.Response
// after decoding and only yields bodies for 2xx statuses, so the exact code is
// available only on failures (via its typed errors).
func (c *ClusterClient) traceResponse(path string, body []byte, err error) {
	if !c.trace {
		return
	}
	fields := log.Fields{
		"cluster": c.name,
		"method":  http.MethodGet,
		"url":     c.baseURL + "/" + path,
	}
	if err != nil {
		if status := statusFromError(err); status != 0 {
			fields["status"] = status
		}
		log.WithFields(fields).Infof("API trace: request failed: %v", err)
		return
	}
	fields["status"] = "2xx"
	log.WithFields(fields).Infof("API trace:\n%s", body)
}

// statusFromError extracts the HTTP status code from gopowerscale's typed errors;
// 0 when the error carries none (e.g. transport or decode failures).
func statusFromError(err error) int {
	var jsonErr *api.JSONError
	if errors.As(err, &jsonErr) {
		return jsonErr.StatusCode
	}
	var htmlErr *api.HTMLError
	if errors.As(err, &htmlErr) {
		return htmlErr.StatusCode
	}
	return 0
}

// dump writes a raw response body to <dumpDir>/<cluster>/<endpoint>.json so operators on
// an unreachable-to-us site can ship the exact payloads back for diagnosis; the files are
// drop-in testdata fixtures. Each cycle overwrites the previous one. Payloads carry no
// credentials (session cookies live in headers, never in bodies).
func (c *ClusterClient) dump(path string, body []byte) {
	if c.dumpDir == "" {
		return
	}
	dir := filepath.Join(c.dumpDir, sanitizeFilename(c.name))
	if err := os.MkdirAll(dir, 0o750); err != nil {
		log.Warnf("cluster %q: dump dir %s: %v", c.name, dir, err)
		return
	}
	file := filepath.Join(dir, sanitizeFilename(path)+".json")
	if err := os.WriteFile(file, body, 0o600); err != nil {
		log.Warnf("cluster %q: dump %s: %v", c.name, file, err)
	}
}

// sanitizeFilename maps an endpoint path or cluster name to a single safe filename
// component (e.g. "platform/1/statistics/current" -> "platform_1_statistics_current").
// Separators are replaced, so the result can never traverse out of the dump directory;
// the lone remaining hazards ("", ".", "..") collapse to "_".
func sanitizeFilename(s string) string {
	out := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
			return r
		default:
			return '_'
		}
	}, s)
	if out == "" || strings.Trim(out, ".") == "" {
		return "_"
	}
	return out
}

// snippet returns a bounded prefix of a payload for debug logs, so a parse failure on a
// system we can't reach still shows what the API actually returned.
func snippet(b []byte) string {
	const max = 256
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + fmt.Sprintf("... (%d bytes total)", len(b))
}

// APIVersion returns the platform API version negotiated by the SDK at construction.
func (c *ClusterClient) APIVersion(_ context.Context) (int, error) {
	return int(c.cli.APIVersion()), nil
}

// GetInventory fetches cluster config, nodes, quotas, and best-effort resource counts.
func (c *ClusterClient) GetInventory(ctx context.Context) (*models.Inventory, error) {
	var cfgBytes, nodesBytes, quotaBytes []byte
	if err := c.getRaw(ctx, "platform/3/cluster/config", &cfgBytes); err != nil {
		return nil, err
	}
	info, err := models.ParseClusterConfig(cfgBytes)
	if err != nil {
		log.Debugf("cluster %q: cluster config payload: %s", c.name, snippet(cfgBytes))
		return nil, fmt.Errorf("cluster %q: parse cluster config: %w", c.name, err)
	}
	if err := c.getRaw(ctx, "platform/3/cluster/nodes", &nodesBytes); err != nil {
		return nil, err
	}
	nodes, err := models.ParseNodes(nodesBytes)
	if err != nil {
		log.Debugf("cluster %q: nodes payload: %s", c.name, snippet(nodesBytes))
		return nil, fmt.Errorf("cluster %q: parse nodes: %w", c.name, err)
	}
	if err := c.getRaw(ctx, "platform/8/quota/quotas", &quotaBytes); err != nil {
		return nil, err
	}
	quotas, err := models.ParseQuotas(quotaBytes)
	if err != nil {
		log.Debugf("cluster %q: quotas payload: %s", c.name, snippet(quotaBytes))
		return nil, fmt.Errorf("cluster %q: parse quotas: %w", c.name, err)
	}
	inv := &models.Inventory{
		Cluster:      info,
		Nodes:        nodes,
		Quotas:       quotas,
		Counts:       c.inventoryCounts(ctx),
		Snapshot:     c.snapshotSummary(ctx),
		SyncPolicies: c.syncPolicies(ctx),
		Events:       c.activeEvents(ctx),
		Dedupe:       c.dedupeSummary(ctx),
		Licenses:     c.licenses(ctx),
		StoragePools: c.storagePools(ctx),
	}
	if log.IsLevelEnabled(log.DebugLevel) {
		var sensors int
		for _, n := range inv.Nodes {
			sensors += len(n.Temperatures) + len(n.Fans)
		}
		log.Debugf("cluster %q: inventory parsed: release=%s nodes=%d (sensor values=%d) quotas=%d "+
			"nfs_exports=%d smb_shares=%d snapshots=%d sync_policies=%d events=%v licenses=%d storage_pools=%d",
			c.name, inv.Cluster.Release, len(inv.Nodes), sensors, len(inv.Quotas),
			inv.Counts.NFSExports, inv.Counts.SMBShares, inv.Counts.Snapshots,
			len(inv.SyncPolicies), inv.Events, len(inv.Licenses), len(inv.StoragePools))
	}
	return inv, nil
}

// dedupeSummary fetches cluster-wide deduplication efficiency best-effort.
func (c *ClusterClient) dedupeSummary(ctx context.Context) models.DedupeSummary {
	var b []byte
	if err := c.getRaw(ctx, "platform/1/dedupe/dedupe-summary", &b); err != nil {
		log.Debugf("cluster %q: dedupe summary failed: %v", c.name, err)
		return models.DedupeSummary{}
	}
	s, err := models.ParseDedupeSummary(b)
	if err != nil {
		log.Debugf("cluster %q: parse dedupe summary failed: %v; payload: %s", c.name, err, snippet(b))
		return models.DedupeSummary{}
	}
	return s
}

// snapshotSummary fetches aggregate snapshot usage best-effort: a failure logs at debug
// and yields a zero summary rather than failing the whole inventory.
func (c *ClusterClient) snapshotSummary(ctx context.Context) models.SnapshotSummary {
	var b []byte
	if err := c.getRaw(ctx, "platform/1/snapshot/snapshots-summary", &b); err != nil {
		log.Debugf("cluster %q: snapshot summary failed: %v", c.name, err)
		return models.SnapshotSummary{}
	}
	s, err := models.ParseSnapshotSummary(b)
	if err != nil {
		log.Debugf("cluster %q: parse snapshot summary failed: %v; payload: %s", c.name, err, snippet(b))
		return models.SnapshotSummary{}
	}
	return s
}

// syncPolicies fetches SyncIQ policies best-effort (clusters without SyncIQ yield none).
func (c *ClusterClient) syncPolicies(ctx context.Context) []models.SyncPolicy {
	var b []byte
	if err := c.getRaw(ctx, "platform/7/sync/policies", &b); err != nil {
		log.Debugf("cluster %q: sync policies failed: %v", c.name, err)
		return nil
	}
	p, err := models.ParseSyncPolicies(b)
	if err != nil {
		log.Debugf("cluster %q: parse sync policies failed: %v; payload: %s", c.name, err, snippet(b))
		return nil
	}
	return p
}

// licenses fetches OneFS license status best-effort (a missing ISI_PRIV_LICENSE privilege
// or an older release simply yields no license metrics).
func (c *ClusterClient) licenses(ctx context.Context) []models.License {
	var b []byte
	if err := c.getRaw(ctx, "platform/5/license/licenses", &b); err != nil {
		log.Debugf("cluster %q: licenses failed: %v", c.name, err)
		return nil
	}
	l, err := models.ParseLicenses(b)
	if err != nil {
		log.Debugf("cluster %q: parse licenses failed: %v; payload: %s", c.name, err, snippet(b))
		return nil
	}
	return l
}

// storagePools fetches per-pool/per-tier capacity best-effort (a missing ISI_PRIV_SMARTPOOLS
// privilege or an older release simply yields no storage-pool metrics).
func (c *ClusterClient) storagePools(ctx context.Context) []models.StoragePool {
	var b []byte
	if err := c.getRaw(ctx, "platform/1/storagepool/storagepools", &b); err != nil {
		log.Debugf("cluster %q: storagepools failed: %v", c.name, err)
		return nil
	}
	p, err := models.ParseStoragePools(b)
	if err != nil {
		log.Debugf("cluster %q: parse storagepools failed: %v; payload: %s", c.name, err, snippet(b))
		return nil
	}
	return p
}

// activeEvents fetches unresolved event-group occurrences best-effort, counted by severity.
func (c *ClusterClient) activeEvents(ctx context.Context) map[string]int {
	var b []byte
	if err := c.getRaw(ctx, "platform/3/event/eventgroup-occurrences", &b); err != nil {
		log.Debugf("cluster %q: event occurrences failed: %v", c.name, err)
		return nil
	}
	ev, err := models.ParseEventOccurrences(b)
	if err != nil {
		log.Debugf("cluster %q: parse event occurrences failed: %v; payload: %s", c.name, err, snippet(b))
		return nil
	}
	return ev
}

// inventoryCounts fetches resource totals best-effort: a failure logs at debug and
// yields 0 rather than failing the whole inventory.
func (c *ClusterClient) inventoryCounts(ctx context.Context) models.Counts {
	count := func(path string) int {
		var b []byte
		if err := c.getRaw(ctx, path, &b); err != nil {
			log.Debugf("cluster %q: count %s failed: %v", c.name, path, err)
			return 0
		}
		n, err := models.ParseTotal(b)
		if err != nil {
			log.Debugf("cluster %q: parse count %s failed: %v", c.name, path, err)
			return 0
		}
		return n
	}
	return models.Counts{
		NFSExports: count("platform/4/protocols/nfs/exports"),
		SMBShares:  count("platform/1/protocols/smb/shares"),
		Snapshots:  count("platform/1/snapshot/snapshots"),
	}
}

// GetStatistics requests the curated statistics keys plus a best-effort protocol summary.
func (c *ClusterClient) GetStatistics(ctx context.Context) (*models.Statistics, error) {
	keys := QueryKeys()
	params := make(api.OrderedValues, 0, len(keys)+1)
	params = append(params, [][]byte{[]byte("devid"), []byte("all")})
	for _, k := range keys {
		params = append(params, [][]byte{[]byte("key"), []byte(k)})
	}

	var curBytes []byte
	if err := c.getRawParams(ctx, "platform/1/statistics/current", params, &curBytes); err != nil {
		return nil, err
	}
	current, err := models.ParseStatCurrent(curBytes)
	if err != nil {
		log.Debugf("cluster %q: statistics payload: %s", c.name, snippet(curBytes))
		return nil, fmt.Errorf("cluster %q: parse statistics: %w", c.name, err)
	}
	st := &models.Statistics{Current: current}

	var protoBytes []byte
	if err := c.getRaw(ctx, "platform/3/statistics/summary/protocol", &protoBytes); err != nil {
		log.Debugf("cluster %q: protocol summary failed: %v", c.name, err)
	} else if proto, perr := models.ParseProtocolSummary(protoBytes); perr == nil {
		st.Proto = proto
	} else {
		log.Debugf("cluster %q: parse protocol summary failed: %v; payload: %s", c.name, perr, snippet(protoBytes))
	}

	st.Drives = c.driveSummary(ctx)
	st.Clients = c.clientSummary(ctx)
	st.Workloads = c.workloadSummary(ctx)

	if log.IsLevelEnabled(log.DebugLevel) {
		returned := make(map[string]bool, len(st.Current))
		for _, p := range st.Current {
			returned[p.Key] = true
		}
		var missing []string
		for _, k := range keys {
			if !returned[k] {
				missing = append(missing, k)
			}
		}
		log.Debugf("cluster %q: statistics parsed: keys=%d/%d requested (missing: %v) "+
			"proto_rows=%d drive_rows=%d client_rows=%d workload_rows=%d",
			c.name, len(returned), len(keys), missing, len(st.Proto), len(st.Drives), len(st.Clients), len(st.Workloads))
	}
	return st, nil
}

// driveSummary fetches per-drive performance best-effort.
func (c *ClusterClient) driveSummary(ctx context.Context) []models.DriveStat {
	var b []byte
	if err := c.getRaw(ctx, "platform/3/statistics/summary/drive", &b); err != nil {
		log.Debugf("cluster %q: drive summary failed: %v", c.name, err)
		return nil
	}
	d, err := models.ParseDriveSummary(b)
	if err != nil {
		log.Debugf("cluster %q: parse drive summary failed: %v; payload: %s", c.name, err, snippet(b))
		return nil
	}
	return d
}

// clientSummary fetches per-client-class performance best-effort.
func (c *ClusterClient) clientSummary(ctx context.Context) []models.ClientStat {
	var b []byte
	if err := c.getRaw(ctx, "platform/3/statistics/summary/client", &b); err != nil {
		log.Debugf("cluster %q: client summary failed: %v", c.name, err)
		return nil
	}
	cl, err := models.ParseClientSummary(b)
	if err != nil {
		log.Debugf("cluster %q: parse client summary failed: %v; payload: %s", c.name, err, snippet(b))
		return nil
	}
	return cl
}

// workloadSummary fetches per-workload performance best-effort. Rows require OneFS
// performance datasets (isi performance datasets) to be configured; without one this yields
// few or no rows.
func (c *ClusterClient) workloadSummary(ctx context.Context) []models.Workload {
	var b []byte
	if err := c.getRaw(ctx, "platform/4/statistics/summary/workload", &b); err != nil {
		log.Debugf("cluster %q: workload summary failed: %v", c.name, err)
		return nil
	}
	w, err := models.ParseWorkloadSummary(b)
	if err != nil {
		log.Debugf("cluster %q: parse workload summary failed: %v; payload: %s", c.name, err, snippet(b))
		return nil
	}
	return w
}

// Close releases resources. gopowerscale holds no long-lived resources beyond the
// default HTTP transport, so there is nothing to tear down.
func (c *ClusterClient) Close() error { return nil }
