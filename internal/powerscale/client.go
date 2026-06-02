package powerscale

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

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
	cfg  models.ClusterConfig
	cli  api.Client

	mu      sync.Mutex
	version int
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
	return &ClusterClient{name: cfg.Name, cfg: cfg, cli: cli}, nil
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

// APIVersion resolves platform/latest once and caches the result.
func (c *ClusterClient) APIVersion(ctx context.Context) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.version != 0 {
		return c.version, nil
	}
	var b []byte
	if err := c.getRaw(ctx, "platform/latest", &b); err != nil {
		return 0, err
	}
	var raw struct {
		Latest string `json:"latest"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return 0, err
	}
	v, err := strconv.Atoi(strings.TrimSpace(raw.Latest))
	if err != nil {
		return 0, fmt.Errorf("cluster %q: unparseable api version %q", c.name, raw.Latest)
	}
	c.version = v
	return v, nil
}

// GetInventory fetches cluster config, nodes, quotas, and best-effort resource counts.
func (c *ClusterClient) GetInventory(ctx context.Context) (*models.Inventory, error) {
	var cfgBytes, nodesBytes, quotaBytes []byte
	if err := c.getRaw(ctx, "platform/3/cluster/config", &cfgBytes); err != nil {
		return nil, err
	}
	info, err := models.ParseClusterConfig(cfgBytes)
	if err != nil {
		return nil, err
	}
	if err := c.getRaw(ctx, "platform/3/cluster/nodes", &nodesBytes); err != nil {
		return nil, err
	}
	nodes, err := models.ParseNodes(nodesBytes)
	if err != nil {
		return nil, err
	}
	if err := c.getRaw(ctx, "platform/1/quota/quotas", &quotaBytes); err != nil {
		return nil, err
	}
	quotas, err := models.ParseQuotas(quotaBytes)
	if err != nil {
		return nil, err
	}
	return &models.Inventory{Cluster: info, Nodes: nodes, Quotas: quotas, Counts: c.inventoryCounts(ctx)}, nil
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
		return nil, err
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
