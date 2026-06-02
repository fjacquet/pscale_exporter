// PowerScale Exporter collects metrics from Dell PowerScale clusters (OneFS) and
// exposes them via a Prometheus /metrics endpoint and an optional OTLP metric push.
//
// Usage:
//
//	pscale_exporter --config config.yaml [--debug] [--once]
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/fjacquet/pscale_exporter/internal/config"
	"github.com/fjacquet/pscale_exporter/internal/logging"
	"github.com/fjacquet/pscale_exporter/internal/models"
	"github.com/fjacquet/pscale_exporter/internal/powerscale"
	"github.com/fjacquet/pscale_exporter/internal/telemetry"
	"github.com/fjacquet/pscale_exporter/internal/utils"
	"github.com/fsnotify/fsnotify"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/trace"
)

const (
	programName       = "pscale_exporter"
	shutdownTimeout   = 15 * time.Second
	readHeaderTimeout = 5 * time.Second
)

// version is the build version, injected via -ldflags "-X main.version=...".
var version = "dev"

var (
	configFile string
	debug      bool
	once       bool
)

// Server owns the HTTP server, the snapshot store, the collection loop, the per-cluster
// clients, and the dual export paths.
type Server struct {
	cfg        *models.SafeConfig
	configPath string

	httpSrv  *http.Server
	registry *prometheus.Registry
	store    *powerscale.SnapshotStore

	telemetry      *telemetry.Manager
	tracerProvider trace.TracerProvider

	mu            sync.Mutex // guards clients/collector/collectCancel across reloads
	clients       []powerscale.Client
	collector     *powerscale.Collector
	collectCancel context.CancelFunc
	otlp          *powerscale.OTLPExporter

	configWatcher *fsnotify.Watcher
	serverErrChan chan error
}

// NewServer creates a server bound to the given thread-safe config.
func NewServer(safeCfg *models.SafeConfig, configPath string) *Server {
	return &Server{
		cfg:           safeCfg,
		configPath:    configPath,
		registry:      prometheus.NewRegistry(),
		store:         powerscale.NewSnapshotStore(),
		serverErrChan: make(chan error, 1),
	}
}

// Start initializes telemetry, builds the client pool, runs the first collection cycle,
// wires both export paths, and starts the HTTP server.
func (s *Server) Start() error {
	cfg := s.cfg.Get()

	s.initTracing(cfg)

	if err := s.startCollection(context.Background(), cfg); err != nil {
		return err
	}

	if err := s.initOTLP(cfg); err != nil {
		log.Warnf("OTLP metrics export disabled: %v", err)
	}

	if err := s.registry.Register(powerscale.NewPromCollector(s.store)); err != nil {
		return fmt.Errorf("failed to register collector: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle(cfg.Server.URI, promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", s.healthHandler)

	s.httpSrv = &http.Server{
		Addr:              cfg.GetServerAddress(),
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	go func() {
		log.Infof("Starting %s on %s%s", programName, cfg.GetServerAddress(), cfg.Server.URI)
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.serverErrChan <- fmt.Errorf("HTTP server error: %w", err)
		}
	}()

	return nil
}

// initTracing sets up the optional OpenTelemetry tracer provider.
func (s *Server) initTracing(cfg *models.Config) {
	if !cfg.IsOTelTracingEnabled() {
		return
	}
	mgr := telemetry.NewManager(telemetry.Config{
		Endpoint:       cfg.OpenTelemetry.Tracing.Endpoint,
		Insecure:       cfg.OpenTelemetry.Tracing.Insecure,
		SamplingRate:   cfg.OpenTelemetry.Tracing.SamplingRate,
		ServiceName:    "pscale-exporter",
		ServiceVersion: version,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mgr.Initialize(ctx); err != nil {
		log.Warnf("Failed to initialize tracing: %v. Continuing without tracing.", err)
		return
	}
	s.telemetry = mgr
	s.tracerProvider = mgr.TracerProvider()
}

// startCollection builds the client pool and collector, runs an initial synchronous
// cycle, and starts the background loop. Caller must hold no locks.
func (s *Server) startCollection(ctx context.Context, cfg *models.Config) error {
	clients := buildClients(cfg, s.tracerProvider)
	collector := powerscale.NewCollector(clients, s.store, cfg.GetCollectionInterval(), cfg.GetCollectionTimeout(), s.tracerProvider)

	// Initial synchronous cycle so the first scrape isn't empty.
	initCtx, cancel := context.WithTimeout(ctx, cfg.GetCollectionTimeout()+5*time.Second)
	collector.CollectOnce(initCtx)
	cancel()

	if once {
		s.mu.Lock()
		s.clients = clients
		s.collector = collector
		s.mu.Unlock()
		return nil
	}

	loopCtx, loopCancel := context.WithCancel(context.Background())
	go collector.Run(loopCtx)

	s.mu.Lock()
	s.clients = clients
	s.collector = collector
	s.collectCancel = loopCancel
	s.mu.Unlock()
	return nil
}

// initOTLP sets up the OTLP metric push path if enabled.
func (s *Server) initOTLP(cfg *models.Config) error {
	if !cfg.IsOTelMetricsEnabled() {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	exp, err := powerscale.NewOTLPExporter(ctx, cfg.OpenTelemetry.Metrics, s.store, version)
	if err != nil {
		return err
	}
	if err := exp.EnsureInstruments(); err != nil {
		_ = exp.Shutdown(ctx)
		return err
	}
	s.otlp = exp
	log.Infof("OTLP metrics push enabled to %s", cfg.OpenTelemetry.Metrics.Endpoint)
	return nil
}

func buildClients(cfg *models.Config, _ trace.TracerProvider) []powerscale.Client {
	clients := make([]powerscale.Client, 0, len(cfg.Clusters))
	for _, cl := range cfg.Clusters {
		clients = append(clients, powerscale.NewStubClient(cl.Name))
	}
	return clients
}

// ErrorChan returns the channel that receives fatal HTTP server errors.
func (s *Server) ErrorChan() <-chan error { return s.serverErrChan }

// ReloadConfig reloads configuration; when the cluster set changes it rebuilds the
// client pool and collection loop.
func (s *Server) ReloadConfig(configPath string) error {
	clustersChanged, err := s.cfg.ReloadConfig(configPath)
	if err != nil {
		return err
	}
	if !clustersChanged {
		return nil
	}

	log.Info("Cluster set changed; rebuilding client pool and collection loop")
	s.stopCollection()

	if err := s.startCollection(context.Background(), s.cfg.Get()); err != nil {
		return fmt.Errorf("failed to restart collection after reload: %w", err)
	}
	// New metric names (if any) need OTLP instruments registered.
	if s.otlp != nil {
		if err := s.otlp.EnsureInstruments(); err != nil {
			log.Warnf("Failed to register OTLP instruments after reload: %v", err)
		}
	}
	return nil
}

// stopCollection cancels the loop and closes the current client pool.
func (s *Server) stopCollection() {
	s.mu.Lock()
	cancel := s.collectCancel
	clients := s.clients
	s.collectCancel = nil
	s.clients = nil
	s.collector = nil
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	for _, c := range clients {
		if err := c.Close(); err != nil {
			log.Debugf("client close during reload: %v", err)
		}
	}
}

// SetConfigWatcher stores the file watcher for cleanup at shutdown.
func (s *Server) SetConfigWatcher(w *fsnotify.Watcher) { s.configWatcher = w }

// Shutdown stops the watcher, HTTP server, collection loop, exporters, and clients.
func (s *Server) Shutdown() error {
	if s.configWatcher != nil {
		if err := s.configWatcher.Close(); err != nil {
			log.Warnf("Config watcher close warning: %v", err)
		}
	}

	if s.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		log.Info("Shutting down HTTP server...")
		if err := s.httpSrv.Shutdown(ctx); err != nil {
			log.Warnf("HTTP server shutdown warning: %v", err)
		}
		cancel()
	}

	s.stopCollection()

	if s.otlp != nil {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		if err := s.otlp.Shutdown(ctx); err != nil {
			log.Warnf("OTLP shutdown warning: %v", err)
		}
		cancel()
	}

	if s.telemetry != nil {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		if err := s.telemetry.Shutdown(ctx); err != nil {
			log.Warnf("Telemetry shutdown warning: %v", err)
		}
		cancel()
	}

	close(s.serverErrChan)
	log.Info("Server stopped gracefully")
	return nil
}

// healthHandler reports 200 if any cluster was scraped successfully, 503 if all are
// down, and 200 "starting" before the first cycle populates the store.
func (s *Server) healthHandler(w http.ResponseWriter, _ *http.Request) {
	snap := s.store.Load()
	if len(snap.PerCluster) == 0 {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "OK (starting)")
		return
	}
	for _, cs := range snap.PerCluster {
		if cs.Up {
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintln(w, "OK")
			return
		}
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = fmt.Fprintln(w, "UNHEALTHY: all clusters unreachable")
}

func validateConfig(configPath string) (*models.Config, error) {
	if !utils.FileExists(configPath) {
		return nil, fmt.Errorf("config file not found: %s", configPath)
	}
	var cfg models.Config
	if err := utils.ReadFile(&cfg, configPath); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	return &cfg, nil
}

func setupLogging(cfg models.Config, debugMode bool) error {
	if err := logging.PrepareLogs(cfg.Server.LogName); err != nil {
		return fmt.Errorf("failed to initialize logging: %w", err)
	}
	if debugMode {
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug mode enabled")
	}
	return nil
}

func waitForShutdown(serverErr <-chan error) error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-stop:
		log.Infof("Received signal %v, initiating graceful shutdown...", sig)
		return nil
	case err := <-serverErr:
		return err
	}
}

func main() {
	rootCmd := &cobra.Command{
		Use:     programName,
		Version: version,
		Short:   "Prometheus/OTLP exporter for Dell PowerScale (OneFS)",
		Long:    "PowerScale Exporter collects metrics from PowerScale (OneFS) clusters and exposes them via Prometheus and OTLP.",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := validateConfig(configFile)
			if err != nil {
				return err
			}

			safeCfg := models.NewSafeConfig(cfg, utils.ResolveSecrets)
			if err := setupLogging(*safeCfg.Get(), debug); err != nil {
				return err
			}

			log.Infof("Starting %s...", programName)
			log.Infof("Monitoring %d cluster(s)", len(safeCfg.Get().Clusters))

			server := NewServer(safeCfg, configFile)
			if err := server.Start(); err != nil {
				return err
			}

			if once {
				log.Info("--once: single collection cycle complete, exiting")
				return server.Shutdown()
			}

			config.SetupSIGHUPHandler(configFile, server.ReloadConfig)
			if watcher, err := config.WatchConfigFile(configFile, server.ReloadConfig); err != nil {
				log.Warnf("File watcher setup failed: %v. SIGHUP reload still available.", err)
			} else {
				server.SetConfigWatcher(watcher)
			}

			if err := waitForShutdown(server.ErrorChan()); err != nil {
				log.Errorf("Server error: %v", err)
			}
			return server.Shutdown()
		},
	}

	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Path to configuration file (required)")
	rootCmd.PersistentFlags().BoolVarP(&debug, "debug", "d", false, "Enable debug mode")
	rootCmd.PersistentFlags().BoolVar(&once, "once", false, "Run a single collection cycle and exit")
	_ = rootCmd.MarkPersistentFlagRequired("config")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
