package main

import (
	"context"
	"encoding/json"
	"flag"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/core"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"github.com/signalsfoundry/constellation-simulator/internal/nbi"
	"github.com/signalsfoundry/constellation-simulator/internal/observability"
	sim "github.com/signalsfoundry/constellation-simulator/internal/sim/state"
	"github.com/signalsfoundry/constellation-simulator/kb"
	"google.golang.org/grpc"
)

func main() {
	grpcAddr := flag.String("grpc-addr", ":50051", "TCP address the NBI gRPC server listens on")
	metricsAddr := flag.String("metrics-addr", ":9090", "HTTP address for Prometheus /metrics")
	transceiverPath := flag.String("transceivers", "configs/transceivers.json", "Path to a JSON file containing transceiver models")
	flag.Parse()

	log := logging.NewFromEnv()
	ctx := context.Background()

	collector, err := observability.NewNBICollector(nil)
	if err != nil {
		log.Error(ctx, "failed to initialise metrics collector", logging.String("error", err.Error()))
		os.Exit(1)
	}

	metricsSrv := serveMetrics(*metricsAddr, collector, log)

	physKB := kb.NewKnowledgeBase()
	netKB := core.NewKnowledgeBase()
	loadTransceivers(log, netKB, *transceiverPath)

	state := sim.NewScenarioState(
		physKB,
		netKB,
		log,
		sim.WithMetricsRecorder(collector),
	)

	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			nbi.RequestIDUnaryServerInterceptor(log),
			collector.UnaryServerInterceptor(),
		),
	)

	v1alpha.RegisterPlatformServiceServer(server, nbi.NewPlatformService(state, nil, log))
	v1alpha.RegisterNetworkNodeServiceServer(server, nbi.NewNetworkNodeService(state, log))
	v1alpha.RegisterNetworkLinkServiceServer(server, nbi.NewNetworkLinkService(state, log))
	v1alpha.RegisterServiceRequestServiceServer(server, nbi.NewServiceRequestService(state, log))
	v1alpha.RegisterScenarioServiceServer(server, nbi.NewScenarioService(state, log))

	lis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		log.Error(ctx, "failed to listen for gRPC", logging.String("addr", *grpcAddr), logging.String("error", err.Error()))
		os.Exit(1)
	}

	log.Info(ctx, "starting NBI gRPC server", logging.String("addr", *grpcAddr))
	go func() {
		if err := server.Serve(lis); err != nil {
			log.Error(ctx, "gRPC server exited", logging.String("error", err.Error()))
		}
	}()

	stopCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	<-stopCtx.Done()

	log.Info(ctx, "shutting down NBI server")
	server.GracefulStop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if metricsSrv != nil {
		_ = metricsSrv.Shutdown(shutdownCtx)
	}
}

func serveMetrics(addr string, collector *observability.NBICollector, log logging.Logger) *http.Server {
	if collector == nil {
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
