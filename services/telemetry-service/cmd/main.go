// Package main is the entrypoint for the FleetPulse Telemetry Service.
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fleetpulse/fleetpulse/api/proto/telemetryv1"
	"github.com/fleetpulse/fleetpulse/pkg/grpcx"
	"github.com/fleetpulse/fleetpulse/pkg/logging"
	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/api"
	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/consumer"
	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/engine"
	"github.com/fleetpulse/fleetpulse/services/telemetry-service/internal/repo"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
)

func main() {
	logger := logging.New("telemetry-service", slog.LevelInfo)

	grpcPort := getEnv("GRPC_PORT", "50052")
	natsURL := getEnv("NATS_URL", "nats://localhost:4222")

	logger.Info("starting telemetry-service", "grpc_port", grpcPort, "nats_url", natsURL)

	ring := repo.NewRingBuffer(120)
	evaluator := engine.NewRuleEvaluator()
	anomalies := engine.NewAnomalyEngine(15, 3.0, 0.15)

	// Connect to NATS
	var nc *nats.Conn
	conn, err := nats.Connect(natsURL, nats.Timeout(3*time.Second))
	if err != nil {
		logger.Warn("could not connect to NATS message bus, running consumer in passive mode", "error", err)
	} else {
		nc = conn
		defer nc.Close()
		logger.Info("connected to NATS message bus", "url", natsURL)
	}

	cons := consumer.NewConsumer(consumer.Config{
		Conn:       nc,
		Ring:       ring,
		Evaluator:  evaluator,
		Anomalies:  anomalies,
		Logger:     logger,
		Subject:    "telemetry.>",
		QueueGroup: "telemetry-workers",
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := cons.Start(ctx); err != nil {
		logger.Error("failed to start telemetry consumer", "error", err)
	}

	// Setup gRPC Server
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		logger.Error("failed to listen on gRPC port", "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpcx.UnaryServerTenantInterceptor(),
			grpcx.UnaryServerLoggingInterceptor(logger),
		),
	)

	telemetryServer := api.NewServer(ring, evaluator, anomalies)
	telemetryv1.RegisterTelemetryServiceServer(grpcServer, telemetryServer)

	go func() {
		logger.Info("gRPC server listening", "addr", lis.Addr().String())
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("gRPC server encountered error", "error", err)
		}
	}()

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down telemetry-service...")
	cancel()
	cons.Stop()
	grpcServer.GracefulStop()
	logger.Info("telemetry-service terminated cleanly")
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
