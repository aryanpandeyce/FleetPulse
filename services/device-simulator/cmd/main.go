// Package main is the entrypoint for the virtual UCS server fleet simulator.
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

	"github.com/fleetpulse/fleetpulse/pkg/logging"
	"github.com/fleetpulse/fleetpulse/services/device-simulator/internal/api"
	"github.com/fleetpulse/fleetpulse/services/device-simulator/internal/connector"
	"github.com/fleetpulse/fleetpulse/services/device-simulator/internal/engine"
	"github.com/fleetpulse/fleetpulse/services/device-simulator/internal/model"
	"github.com/nats-io/nats.go"
)

func main() {
	logger := logging.New("device-simulator", slog.LevelInfo)

	port := getEnv("PORT", "8081")
	deviceCount := getEnvInt("SIM_DEVICE_COUNT", 1000)
	seed := int64(getEnvInt("SIM_SEED", 42))
	intervalSec := getEnvInt("SIM_INTERVAL", 5)
	natsURL := getEnv("NATS_URL", "nats://localhost:4222")

	logger.Info("starting device-simulator",
		"device_count", deviceCount,
		"seed", seed,
		"interval_sec", intervalSec,
		"nats_url", natsURL,
		"port", port,
	)

	sensorEng := engine.NewSensorEngine(seed)
	registry := engine.NewDeviceRegistry()

	// Demo tenants matching seed.sql
	orgs := []string{
		"c827b56a-1234-4b55-a22b-8a89201f92e1", // Acme
		"d938c67b-5678-4c66-b33c-9b90312fa3f2", // Globex
	}
	models := []string{
		model.ModelUCSC220M7,
		model.ModelUCSC240M7,
		model.ModelUCSX210c,
	}

	logger.Info("generating simulated virtual server fleet...")
	for i := 0; i < deviceCount; i++ {
		orgID := orgs[i%len(orgs)]
		devModel := models[i%len(models)]
		serial := fmt.Sprintf("%s-%05d", devModel, i+1)

		d := &model.SimulatedDevice{
			ID:              fmt.Sprintf("dev-%05d", i+1),
			Serial:          serial,
			OrgID:           orgID,
			Model:           devModel,
			FirmwareVersion: "4.2(3a)",
		}
		sensorEng.InitializeDevice(d)
		registry.Register(d)
	}
	logger.Info("simulated virtual server fleet generated", "total_registered", registry.Count())

	// Connect to NATS
	var nc *nats.Conn
	var pub connector.Publisher
	conn, err := nats.Connect(natsURL, nats.Timeout(3*time.Second))
	if err != nil {
		logger.Warn("could not connect to NATS, proceeding in standalone REST mode", "error", err)
	} else {
		nc = conn
		defer nc.Close()
		pub = nc
		logger.Info("connected to NATS message bus", "url", natsURL)
	}

	// Create and start connector with consistent-hash worker sharding
	connConfig := connector.Config{
		NumWorkers: 8,
		Interval:   time.Duration(intervalSec) * time.Second,
		JitterPct:  0.1,
		Logger:     logger,
		Publisher:  pub,
		Registry:   registry,
		Engine:     sensorEng,
	}
	connPool := connector.NewConnector(connConfig)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	connPool.Start(ctx)

	// HTTP Redfish and simulator API
	httpHandler := api.NewHandler(registry, 0.05)
	server := &http.Server{
		Addr:         ":" + port,
		Handler:      httpHandler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("redfish REST API listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("redfish server failure", "error", err)
		}
	}()

	// Graceful shutdown handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("shutting down device simulator gracefully...")
	cancel()
	connPool.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
	logger.Info("device simulator terminated cleanly")
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if n, err := strconv.Atoi(val); err == nil {
			return n
		}
	}
	return defaultVal
}
