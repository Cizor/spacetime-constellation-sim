package main

import (
	"context"
	"net"
	"testing"
	"time"

	v1alpha "aalyria.com/spacetime/api/nbi/v1alpha"
	"github.com/signalsfoundry/constellation-simulator/internal/logging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestNBIServerStartupSmoke(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}

	cfg := Config{
		ListenAddress:       lis.Addr().String(),
		MetricsAddress:      "",
		EnableTLS:           false,
		TLSCertPath:         "",
		TLSKeyPath:          "",
		LogLevel:            "warn",
		LogFormat:           "text",
		TransceiversPath:    "configs/transceivers.json",
		NetworkScenarioPath: "",
		TickInterval:        20 * time.Millisecond,
		Accelerated:         true,
	}

	log := logging.New(logging.Config{Level: cfg.LogLevel, Format: cfg.LogFormat})

	errCh := make(chan error, 1)
	go func() {
		errCh <- run(ctx, cfg, log, lis)
	}()

	conn, err := grpc.DialContext(ctx, cfg.ListenAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.DialContext: %v", err)
	}
	defer conn.Close()

	client := v1alpha.NewScenarioServiceClient(conn)
	resp, err := client.GetScenario(ctx, &v1alpha.GetScenarioRequest{})
	if err != nil {
		t.Fatalf("GetScenario: %v", err)
	}
	if resp == nil {
		t.Fatalf("GetScenario response is nil")
	}

	cancel()

	if err := <-errCh; err != nil {
		t.Fatalf("server returned error: %v", err)
	}
}