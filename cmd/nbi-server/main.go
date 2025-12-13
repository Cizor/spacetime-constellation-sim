package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	schedulingpb "aalyria.com/spacetime/api/scheduling/v1alpha"
	telemetrypb "aalyria.com/spacetime/api/telemetry/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/nbi"
	"github.com/signalsfoundry/constellation-simulator/internal/observability"
	"github.com/signalsfoundry/constellation-simulator/internal/sbi"
	sbicontroller "github.com/signalsfoundry/constellation-simulator/internal/sbi/controller"
	sbiruntime "github.com/signalsfoundry/constellation-simulator/internal/sbi/runtime"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"github.com/signalsfoundry/constellation-simulator/timectrl"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type Config struct {
	ListenAddress       string
	MetricsAddress      string
	EnableTLS           bool
	TLSCertPath         string
	TLSKeyPath          string
	LogLevel            string
	LogFormat           string
	TransceiversPath    string
	NetworkScenarioPath string
	TickInterval        time.Duration
	Accelerated         bool
}

func main() {
	cfg := loadConfig()
	log := logging.New(logging.Config{
		Level:     cfg.LogLevel,
		Format:    cfg.LogFormat,
		AddSource: true,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, log, nil); err != nil {
		log.Error(context.Background(), "nbi-server exited with error", logging.String("error", err.Error()))
		os.Exit(1)
	}
}

func loadConfig() Config {
	defaultListen := envOrDefault("NBI_LISTEN_ADDRESS", "0.0.0.0:50051")
	defaultMetrics := envOrDefault("NBI_METRICS_ADDRESS", ":9090")
	defaultTLS := envBool("NBI_ENABLE_TLS", false)
	defaultTick := envDuration("NBI_TICK_INTERVAL", time.Second)
	defaultTransceivers := envOrDefault("NBI_TRANSCEIVERS_PATH", "configs/transceivers.json")
	defaultScenario := envOrDefault("NBI_NETWORK_SCENARIO", "")
	defaultLogLevel := envOrDefault("LOG_LEVEL", "info")
	defaultLogFormat := envOrDefault("LOG_FORMAT", "text")
	defaultAccelerated := envBool("NBI_ACCELERATED", true)

	listenAddr := flag.String("listen-address", defaultListen, "TCP address the NBI gRPC server listens on")
	metricsAddr := flag.String("metrics-address", defaultMetrics, "HTTP address for Prometheus /metrics (empty to disable)")
	enableTLS := flag.Bool("enable-tls", defaultTLS, "Enable TLS for the gRPC server")
	tlsCert := flag.String("tls-cert", envOrDefault("NBI_TLS_CERT", ""), "Path to TLS certificate (requires --enable-tls)")
	tlsKey := flag.String("tls-key", envOrDefault("NBI_TLS_KEY", ""), "Path to TLS private key (requires --enable-tls)")
	logLevel := flag.String("log-level", defaultLogLevel, "Log level: debug, info, warn")
	logFormat := flag.String("log-format", defaultLogFormat, "Log format: text or json")
	transceiverPath := flag.String("transceivers", defaultTransceivers, "Path to a JSON file containing transceiver models")
	networkScenario := flag.String("network-scenario", defaultScenario, "Optional JSON file with interfaces/links/positions to pre-load")
	tick := flag.Duration("tick", defaultTick, "Simulation tick interval")
	accelerated := flag.Bool("accelerated", defaultAccelerated, "Run simulation ticks in accelerated mode")

	flag.Parse()

	if *tick <= 0 {
		*tick = time.Second
	}

	return Config{
		ListenAddress:       *listenAddr,
		MetricsAddress:      *metricsAddr,
		EnableTLS:           *enableTLS,
		TLSCertPath:         *tlsCert,
		TLSKeyPath:          *tlsKey,
		LogLevel:            *logLevel,
		LogFormat:           *logFormat,
		TransceiversPath:    *transceiverPath,
		NetworkScenarioPath: *networkScenario,
		TickInterval:        *tick,
		Accelerated:         *accelerated,
	}
}

func run(ctx context.Context, cfg Config, log logging.Logger, lis net.Listener) error {
	if log == nil {
		log = logging.Noop()
	}
	if cfg.EnableTLS && (cfg.TLSCertPath == "" || cfg.TLSKeyPath == "") {
		return fmt.Errorf("enable-tls set but TLS cert or key path is empty")
	}

	traceShutdown := func(context.Context) error { return nil }
	if shutdown, err := observability.InitTracing(ctx, observability.TracingConfigFromEnv(), log); err != nil {
		log.Warn(ctx, "failed to initialise tracing", logging.String("error", err.Error()))
	} else {
		traceShutdown = shutdown
	}
	defer observability.ShutdownWithTimeout(context.Background(), traceShutdown, log)

	collector, err := observability.NewNBICollector(nil)
	if err != nil {
		return fmt.Errorf("init metrics collector: %w", err)
	}

	var metricsSrv *http.Server
	if cfg.MetricsAddress != "" {
		metricsSrv = serveMetrics(cfg.MetricsAddress, collector, log)
	}

	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()

	motion := core.NewMotionModel(core.WithPositionUpdater(physKB))
	connectivity := core.NewConnectivityService(netKB)

	loadTransceivers(log, netKB, cfg.TransceiversPath)
	loadNetworkScenario(log, netKB, cfg.NetworkScenarioPath)

	state := sim.NewScenarioState(
		physKB,
		netKB,
		log,
		sim.WithMotionModel(motion),
		sim.WithConnectivityService(connectivity),
		sim.WithMetricsRecorder(collector),
	)

	tc := timectrl.NewTimeController(time.Now().UTC(), cfg.TickInterval, timeMode(cfg))
	eventScheduler := sbi.NewEventScheduler(tc)

	telemetryState := sim.NewTelemetryState()
	telemetryServer := sbicontroller.NewTelemetryServer(telemetryState, log)
	cdpiServer := sbicontroller.NewCDPIServer(state, eventScheduler, log)

	server, err := buildGRPCServer(cfg, state, motion, collector, log, cdpiServer, telemetryServer)
	if err != nil {
		return err
	}

	if lis == nil {
		lis, err = net.Listen("tcp", cfg.ListenAddress)
		if err != nil {
			return fmt.Errorf("listen on %s: %w", cfg.ListenAddress, err)
		}
	}

	log.Info(ctx, "starting NBI gRPC server", logging.String("addr", lis.Addr().String()))
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(lis)
	}()

	conn, err := dialLocalGRPC(ctx, lis.Addr().String(), cfg)
	if err != nil {
		return fmt.Errorf("dial local gRPC server: %w", err)
	}
	defer conn.Close()

	sbiRuntime, err := sbiruntime.NewSBIRuntimeWithServers(state, eventScheduler, telemetryState, telemetryServer, cdpiServer, conn, log)
	if err != nil {
		return fmt.Errorf("initialise SBI runtime: %w", err)
	}
	defer sbiRuntime.Close()

	if err := sbiRuntime.StartAgents(ctx); err != nil {
		return fmt.Errorf("start SBI agents: %w", err)
	}

	if err := sbiRuntime.Scheduler.RunInitialSchedule(ctx); err != nil {
		return fmt.Errorf("run initial SBI schedule: %w", err)
	}

	simCtx, simCancel := context.WithCancel(ctx)
	defer simCancel()
	go runSimLoop(simCtx, tc, state, motion, connectivity, eventScheduler, sbiRuntime.Scheduler, log)

	var retErr error
	var serveResult error
	select {
	case err := <-serveErr:
		serveResult = err
		if err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			retErr = err
		}
	case <-ctx.Done():
		log.Info(ctx, "shutdown requested", logging.String("reason", ctx.Err().Error()))
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	server.GracefulStop()
	if serveResult == nil {
		serveResult = <-serveErr
	}
	if serveResult != nil && !errors.Is(serveResult, grpc.ErrServerStopped) {
		log.Error(ctx, "gRPC server exited", logging.String("error", serveResult.Error()))
		if retErr == nil {
			retErr = serveResult
		}
	}

	if metricsSrv != nil {
		_ = metricsSrv.Shutdown(shutdownCtx)
	}

	return retErr
}

func buildGRPCServer(
	cfg Config,
	state *sim.ScenarioState,
	motion *core.MotionModel,
	collector *observability.NBICollector,
	log logging.Logger,
	cdpiServer *sbicontroller.CDPIServer,
	telemetryServer *sbicontroller.TelemetryServer,
) (*grpc.Server, error) {
	interceptors := []grpc.UnaryServerInterceptor{
		nbi.RequestIDUnaryServerInterceptor(log),
		otelgrpc.UnaryServerInterceptor(
			otelgrpc.WithTracerProvider(otel.GetTracerProvider()),
			otelgrpc.WithPropagators(otel.GetTextMapPropagator()),
		),
		nbi.TracingUnaryServerInterceptor(),
	}
	if collector != nil {
		interceptors = append(interceptors, collector.UnaryServerInterceptor())
	}

	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(interceptors...),
	}

	if cfg.EnableTLS {
		creds, err := credentials.NewServerTLSFromFile(cfg.TLSCertPath, cfg.TLSKeyPath)
		if err != nil {
			return nil, fmt.Errorf("load TLS credentials: %w", err)
		}
		opts = append(opts, grpc.Creds(creds))
	}

	server := grpc.NewServer(opts...)

	v1alpha.RegisterPlatformServiceServer(server, nbi.NewPlatformService(state, motion, log))
	v1alpha.RegisterNetworkNodeServiceServer(server, nbi.NewNetworkNodeService(state, log))
	v1alpha.RegisterNetworkLinkServiceServer(server, nbi.NewNetworkLinkService(state, log))
	v1alpha.RegisterServiceRequestServiceServer(server, nbi.NewServiceRequestService(state, log))
	v1alpha.RegisterScenarioServiceServer(server, nbi.NewScenarioService(state, log))

	if cdpiServer != nil {
		schedulingpb.RegisterSchedulingServer(server, cdpiServer)
	}
	if telemetryServer != nil {
		telemetrypb.RegisterTelemetryServer(server, telemetryServer)
	}

	return server, nil
}

var replanInterval = 5 * time.Minute

type runLoopScheduler interface {
	RecomputeContactWindows(context.Context, time.Time, time.Time)
	ScheduleServiceRequests(context.Context) error
}

func runSimLoop(
	ctx context.Context,
	tc *timectrl.TimeController,
	state *sim.ScenarioState,
	motion *core.MotionModel,
	connectivity *core.ConnectivityService,
	eventScheduler sbi.EventScheduler,
	controllerScheduler runLoopScheduler,
	log logging.Logger,
) {
	if tc == nil || state == nil {
		return
	}

	ticker := time.NewTicker(tc.Tick)
	defer ticker.Stop()

	simTime := tc.StartTime
	tc.SetTime(simTime)
	if err := state.RunSimTick(simTime, motion, connectivity, func() {
		syncNodePositions(state.PhysicalKB(), state.NetworkKB())
	}); err != nil {
		log.Warn(ctx, "initial simulation tick failed", logging.String("error", err.Error()))
	}
	if eventScheduler != nil {
		eventScheduler.RunDue()
	}

	lastReplanTime := simTime

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			simTime = simTime.Add(tc.Tick)
			tc.SetTime(simTime)

			if err := state.RunSimTick(simTime, motion, connectivity, func() {
				syncNodePositions(state.PhysicalKB(), state.NetworkKB())
			}); err != nil {
				log.Warn(ctx, "simulation tick failed", logging.String("error", err.Error()))
			}

			if eventScheduler != nil {
				eventScheduler.RunDue()
			}

			if controllerScheduler != nil && simTime.Sub(lastReplanTime) >= replanInterval {
				log.Debug(ctx, "Running periodic re-planning",
					logging.String("sim_time", simTime.Format(time.RFC3339)),
				)
				horizon := simTime.Add(sbicontroller.ContactHorizon)
				controllerScheduler.RecomputeContactWindows(ctx, simTime, horizon)
				if err := controllerScheduler.ScheduleServiceRequests(ctx); err != nil {
					log.Warn(ctx, "Periodic re-planning failed",
						logging.String("error", err.Error()),
					)
				}
				lastReplanTime = simTime
			}
		}
	}
}

func syncNodePositions(phys *kb.KnowledgeBase, netKB *core.KnowledgeBase) {
	if phys == nil || netKB == nil {
		return
	}

	platforms := phys.ListPlatforms()
	platformByID := make(map[string]core.Vec3, len(platforms))
	for _, p := range platforms {
		if p == nil {
			continue
		}
		platformByID[p.ID] = core.Vec3{
			X: p.Coordinates.X / 1000.0,
			Y: p.Coordinates.Y / 1000.0,
			Z: p.Coordinates.Z / 1000.0,
		}
	}

	for _, node := range phys.ListNetworkNodes() {
		if node == nil {
			continue
		}
		if pos, ok := platformByID[node.PlatformID]; ok {
			netKB.SetNodeECEFPosition(node.ID, pos)
		}
	}
}

func serveMetrics(addr string, collector *observability.NBICollector, log logging.Logger) *http.Server {
	if collector == nil || addr == "" {
		return nil
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", collector.Handler())

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Warn(context.Background(), "metrics server exited", logging.String("error", err.Error()))
		}
	}()

	log.Info(context.Background(), "serving Prometheus metrics", logging.String("addr", addr))
	return srv
}

func loadTransceivers(log logging.Logger, netKB *core.KnowledgeBase, path string) {
	if path == "" || netKB == nil {
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Warn(context.Background(), "skipping transceiver load", logging.String("path", path), logging.String("error", err.Error()))
		return
	}

	var trxs []*core.TransceiverModel
	if err := json.Unmarshal(data, &trxs); err != nil {
		log.Warn(context.Background(), "failed to parse transceiver models", logging.String("path", path), logging.String("error", err.Error()))
		return
	}

	added := 0
	for _, trx := range trxs {
		if trx == nil {
			continue
		}
		if err := netKB.AddTransceiverModel(trx); err == nil {
			added++
		} else {
			log.Warn(context.Background(), "skipping transceiver", logging.String("id", trx.ID), logging.String("error", err.Error()))
		}
	}

	log.Info(context.Background(), "loaded transceiver models",
		logging.String("path", path),
		logging.Int("count", added),
	)
}

func loadNetworkScenario(log logging.Logger, netKB *core.KnowledgeBase, path string) {
	if netKB == nil || path == "" {
		return
	}

	f, err := os.Open(path)
	if err != nil {
		log.Warn(context.Background(), "failed to open network scenario", logging.String("path", path), logging.String("error", err.Error()))
		return
	}
	defer f.Close()

	summary, err := core.LoadNetworkScenario(netKB, f)
	if err != nil {
		log.Warn(context.Background(), "failed to load network scenario", logging.String("path", path), logging.String("error", err.Error()))
		return
	}

	log.Info(context.Background(), "loaded network scenario",
		logging.String("path", path),
		logging.Int("interfaces", len(summary.InterfaceIDs)),
		logging.Int("links", len(summary.LinkIDs)),
		logging.Int("positions", len(summary.NodeIDs)),
	)
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			return parsed
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil {
			return parsed
		}
	}
	return fallback
}

func timeMode(cfg Config) timectrl.Mode {
	if cfg.Accelerated {
		return timectrl.Accelerated
	}
	return timectrl.RealTime
}

func dialLocalGRPC(ctx context.Context, target string, cfg Config) (*grpc.ClientConn, error) {
	var dialCred credentials.TransportCredentials
	if cfg.EnableTLS {
		cred, err := credentials.NewClientTLSFromFile(cfg.TLSCertPath, "")
		if err != nil {
			return nil, fmt.Errorf("load TLS client credentials: %w", err)
		}
		dialCred = cred
	} else {
		dialCred = insecure.NewCredentials()
	}

	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return grpc.DialContext(dialCtx, target,
		grpc.WithTransportCredentials(dialCred),
		grpc.WithBlock(),
	)
}
