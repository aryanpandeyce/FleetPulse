// Package main is the entrypoint for the FleetPulse Workflow Orchestration Service.
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/fleetpulse/fleetpulse/api/proto/workflowv1"
	"github.com/fleetpulse/fleetpulse/pkg/grpcx"
	"github.com/fleetpulse/fleetpulse/pkg/logging"
	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/api"
	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/executor"
	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/repo"
	"github.com/fleetpulse/fleetpulse/services/workflow-service/internal/scheduler"
	"google.golang.org/grpc"
)

func main() {
	logger := logging.New("workflow-service", slog.LevelInfo)

	grpcPort := getEnv("GRPC_PORT", "50053")
	simURL := getEnv("SIMULATOR_URL", "http://localhost:8081")

	logger.Info("starting workflow-service", "grpc_port", grpcPort, "simulator_url", simURL)

	jobRepo := repo.NewRepository()
	jobScheduler := scheduler.NewPriorityScheduler()

	runner := executor.NewRunner(executor.Config{
		Scheduler:    jobScheduler,
		Repo:         jobRepo,
		Concurrency:  2,
		SimulatorURL: simURL,
		Logger:       logger,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner.Start(ctx)

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

	wfServer := api.NewServer(jobRepo, jobScheduler)
	workflowv1.RegisterWorkflowServiceServer(grpcServer, wfServer)

	go func() {
		logger.Info("gRPC server listening", "addr", lis.Addr().String())
		if err := grpcServer.Serve(lis); err != nil {
			logger.Error("gRPC server error", "error", err)
		}
	}()

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down workflow-service...")
	cancel()
	runner.Stop()
	grpcServer.GracefulStop()
	logger.Info("workflow-service terminated cleanly")
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
