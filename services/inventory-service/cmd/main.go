// Main entrypoint for Inventory Service.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fleetpulse/fleetpulse/api/proto/inventoryv1"
	"github.com/fleetpulse/fleetpulse/pkg/grpcx"
	"github.com/fleetpulse/fleetpulse/pkg/logging"
	"github.com/fleetpulse/fleetpulse/services/inventory-service/internal/api"
	"github.com/fleetpulse/fleetpulse/services/inventory-service/internal/repo"
	"github.com/fleetpulse/fleetpulse/services/inventory-service/internal/service"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
)

type natsPublisher struct {
	nc *nats.Conn
}

func (p *natsPublisher) Publish(subject string, data []byte) error {
	if p.nc != nil && !p.nc.IsClosed() {
		return p.nc.Publish(subject, data)
	}
	return nil
}

func main() {
	logger := logging.New("inventory-service", slog.LevelInfo)
	slog.SetDefault(logger)

	port := os.Getenv("PORT")
	if port == "" {
		port = "50051"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/fleetpulse?sslmode=disable"
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Connect PostgreSQL
	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		logger.Error("failed to parse db config", slog.Any("error", err))
		os.Exit(1)
	}
	poolConfig.MaxConns = 25
	poolConfig.MinConns = 2

	dbPool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		logger.Warn("could not connect to postgres, running in degraded state", slog.Any("error", err))
	} else {
		defer dbPool.Close()
		logger.Info("connected to postgresql successfully")
	}

	// Connect NATS
	var nc *nats.Conn
	nc, err = nats.Connect(natsURL, nats.Timeout(3*time.Second))
	if err != nil {
		logger.Warn("could not connect to NATS, event publishing disabled", slog.Any("error", err))
	} else {
		defer nc.Close()
		logger.Info("connected to NATS bus successfully")
	}

	inventoryRepo := repo.NewPostgresRepo(dbPool)
	inventorySvc := service.New(inventoryRepo, &natsPublisher{nc: nc}, 5000)
	grpcServerImpl := api.NewServer(inventorySvc)

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpcx.UnaryServerTenantInterceptor(),
			grpcx.UnaryServerLoggingInterceptor(logger),
		),
	)
	inventoryv1.RegisterInventoryServiceServer(grpcServer, grpcServerImpl)

	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		logger.Error("failed to bind tcp listener", slog.Any("error", err))
		os.Exit(1)
	}

	go func() {
		logger.Info("inventory-service gRPC server listening", slog.String("port", port))
		if err := grpcServer.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			logger.Error("gRPC server failed", slog.Any("error", err))
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down inventory-service gracefully...")
	grpcServer.GracefulStop()
	logger.Info("inventory-service shutdown complete")
}
