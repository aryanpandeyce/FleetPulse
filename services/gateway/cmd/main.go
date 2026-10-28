// Main entrypoint for API Gateway.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/fleetpulse/fleetpulse/api/proto/inventoryv1"
	"github.com/fleetpulse/fleetpulse/api/proto/telemetryv1"
	"github.com/fleetpulse/fleetpulse/api/proto/workflowv1"
	"github.com/fleetpulse/fleetpulse/pkg/auth"
	"github.com/fleetpulse/fleetpulse/pkg/grpcx"
	"github.com/fleetpulse/fleetpulse/pkg/logging"
	"github.com/fleetpulse/fleetpulse/services/gateway/internal/api"
	gwAuth "github.com/fleetpulse/fleetpulse/services/gateway/internal/auth"
	"github.com/fleetpulse/fleetpulse/services/gateway/internal/ratelimit"
	"github.com/fleetpulse/fleetpulse/pkg/redis"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	logger := logging.New("gateway", slog.LevelInfo)
	slog.SetDefault(logger)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "fleetpulse-super-secret-jwt-signing-key-32-chars-long"
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
	}

	invAddr := os.Getenv("INVENTORY_GRPC_ADDR")
	if invAddr == "" {
		invAddr = "localhost:50051"
	}

	telAddr := os.Getenv("TELEMETRY_GRPC_ADDR")
	if telAddr == "" {
		telAddr = "localhost:50052"
	}

	wfAddr := os.Getenv("WORKFLOW_GRPC_ADDR")
	if wfAddr == "" {
		wfAddr = "localhost:50053"
	}

	burstCap := 100.0
	if bStr := os.Getenv("RATE_LIMIT_BURST"); bStr != "" {
		if b, err := strconv.ParseFloat(bStr, 64); err == nil {
			burstCap = b
		}
	}

	refillRate := 20.0
	if rStr := os.Getenv("RATE_LIMIT_REFILL"); rStr != "" {
		if r, err := strconv.ParseFloat(rStr, 64); err == nil {
			refillRate = r
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Token Manager & User Store
	tokenManager := auth.NewTokenManager(jwtSecret, "fleetpulse-gateway", 15*time.Minute, 7*24*time.Hour)
	userStore := gwAuth.NewSeededUserStore()

	// Redis connection
	rdb := redis.NewClient(redisURL)
	pingCtx, pingCancel := context.WithTimeout(ctx, 2*time.Second)
	if err := rdb.Ping(pingCtx); err != nil {
		logger.Warn("could not connect to redis, running with in-memory rate limiting fallback", slog.Any("error", err))
		rdb = nil
	} else {
		logger.Info("connected to redis successfully")
	}
	pingCancel()

	rateLimiter := ratelimit.New(rdb, burstCap, refillRate)

	// gRPC Client Helper
	dialGRPC := func(addr string) *grpc.ClientConn {
		conn, err := grpc.DialContext(
			ctx,
			addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithUnaryInterceptor(grpcx.UnaryClientTenantInterceptor()),
		)
		if err != nil {
			logger.Warn("gRPC dial failed", slog.String("addr", addr), slog.Any("error", err))
		}
		return conn
	}

	invConn := dialGRPC(invAddr)
	if invConn != nil {
		defer invConn.Close()
	}
	telConn := dialGRPC(telAddr)
	if telConn != nil {
		defer telConn.Close()
	}
	wfConn := dialGRPC(wfAddr)
	if wfConn != nil {
		defer wfConn.Close()
	}

	router := api.NewRouter(api.Config{
		TokenManager:    tokenManager,
		UserStore:       userStore,
		RateLimiter:     rateLimiter,
		Logger:          logger,
		InventoryClient: inventoryv1.NewInventoryServiceClient(invConn),
		TelemetryClient: telemetryv1.NewTelemetryServiceClient(telConn),
		WorkflowClient:  workflowv1.NewWorkflowServiceClient(wfConn),
	})

	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("gateway HTTP server listening", slog.String("port", port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("gateway HTTP server failed", slog.Any("error", err))
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down gateway gracefully...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)

	logger.Info("gateway shutdown complete")
}
