package powerscale

import (
	"context"
	"encoding/json"
	"fmt"

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
	name string
	cli  api.Client
}

// NewClusterClient establishes one authenticated gopowerscale session for the cluster.
//
// gopowerscale builds request URLs by writing the hostname argument directly, so it must
// be a full base URL (scheme + host + port). It also has no Port field in ClientOptions;
// the port is therefore carried inside the base URL via models.ClusterConfig.BaseURL().
func NewClusterClient(ctx context.Context, cfg models.ClusterConfig) (*ClusterClient, error) {
	if cfg.InsecureSkipVerify {
		log.Warnf("cluster %q: TLS verification disabled (insecureSkipVerify=true)", cfg.Name)
	}
	opts := &api.ClientOptions{
		Insecure: cfg.InsecureSkipVerify,
	}
	cli, err := api.New(ctx, cfg.BaseURL(), cfg.Username, cfg.Password, "", 0, sessionAuthType, opts)
	if err != nil {
		return nil, fmt.Errorf("cluster %q: auth failed: %w", cfg.Name, err)
	}
	return &ClusterClient{name: cfg.Name, cli: cli}, nil
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
	var raw json.RawMessage
	if err := c.cli.Get(ctx, path, "", params, nil, &raw); err != nil {
		return fmt.Errorf("cluster %q: GET %s: %w", c.name, path, err)
	}
	*dst = raw
	return nil
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
		return nil, fmt.Errorf("cluster %q: parse cluster config: %w", c.name, err)
	}
	if err := c.getRaw(ctx, "platform/3/cluster/nodes", &nodesBytes); err != nil {
		return nil, err
	}
	nodes, err := models.ParseNodes(nodesBytes)
	if err != nil {
		return nil, fmt.Errorf("cluster %q: parse nodes: %w", c.name, err)
	}
	if err := c.getRaw(ctx, "platform/1/quota/quotas", &quotaBytes); err != nil {
		return nil, err
	}
	quotas, err := models.ParseQuotas(quotaBytes)
	if err != nil {
		return nil, fmt.Errorf("cluster %q: parse quotas: %w", c.name, err)
	}
	return &models.Inventory{
		Cluster:      info,
		Nodes:        nodes,
		Quotas:       quotas,
		Counts:       c.inventoryCounts(ctx),
		Snapshot:     c.snapshotSummary(ctx),
		SyncPolicies: c.syncPolicies(ctx),
		Events:       c.activeEvents(ctx),
	}, nil
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
		log.Debugf("cluster %q: parse snapshot summary failed: %v", c.name, err)
		return models.SnapshotSummary{}
	}
	return s
}

// syncPolicies fetches SyncIQ policies best-effort (clusters without SyncIQ yield none).
func (c *ClusterClient) syncPolicies(ctx context.Context) []models.SyncPolicy {
	var b []byte
	if err := c.getRaw(ctx, "platform/11/sync/policies", &b); err != nil {
		log.Debugf("cluster %q: sync policies failed: %v", c.name, err)
		return nil
	}
	p, err := models.ParseSyncPolicies(b)
	if err != nil {
		log.Debugf("cluster %q: parse sync policies failed: %v", c.name, err)
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
		log.Debugf("cluster %q: parse event occurrences failed: %v", c.name, err)
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
		return nil, fmt.Errorf("cluster %q: parse statistics: %w", c.name, err)
	}
	st := &models.Statistics{Current: current}

	var protoBytes []byte
	if err := c.getRaw(ctx, "platform/2/statistics/summary/protocol", &protoBytes); err != nil {
		log.Debugf("cluster %q: protocol summary failed: %v", c.name, err)
		return st, nil
	}
	if proto, perr := models.ParseProtocolSummary(protoBytes); perr == nil {
		st.Proto = proto
	} else {
		log.Debugf("cluster %q: parse protocol summary failed: %v", c.name, perr)
	}
	return st, nil
}

// Close releases resources. gopowerscale holds no long-lived resources beyond the
// default HTTP transport, so there is nothing to tear down.
func (c *ClusterClient) Close() error { return nil }
