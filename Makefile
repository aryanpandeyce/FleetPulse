.PHONY: up down build test test-race bench lint proto seed demo clean

# Default shell
SHELL := /bin/bash

# Binaries destination
BIN_DIR := bin

up:
	@echo "Starting FleetPulse full stack with Docker Compose..."
	docker compose -f deploy/docker-compose.yml up -d

down:
	@echo "Stopping FleetPulse full stack..."
	docker compose -f deploy/docker-compose.yml down -v

build:
	@echo "Building all service binaries into $(BIN_DIR)/..."
	@mkdir -p $(BIN_DIR)
	go build -ldflags="-w -s" -o $(BIN_DIR)/gateway ./services/gateway/cmd
	go build -ldflags="-w -s" -o $(BIN_DIR)/inventory-service ./services/inventory-service/cmd
	go build -ldflags="-w -s" -o $(BIN_DIR)/telemetry-service ./services/telemetry-service/cmd
	go build -ldflags="-w -s" -o $(BIN_DIR)/workflow-service ./services/workflow-service/cmd
	go build -ldflags="-w -s" -o $(BIN_DIR)/device-simulator ./services/device-simulator/cmd

test:
	@echo "Running unit and package tests with statement coverage..."
	go test -p 1 -cover ./pkg/... ./services/...

test-race:
	@echo "Running tests with race detector enabled..."
	go test -race -cover ./pkg/... ./services/...

bench:
	@echo "Running data structures and anomaly detector benchmarks..."
	go test -p 1 -bench=. -benchmem ./pkg/ds/... ./services/telemetry-service/internal/engine/...

lint:
	@echo "Running static analysis..."
	go vet ./...

proto:
	@echo "Compiling protobuf contracts..."
	protoc -I api/proto --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative api/proto/*.proto

seed:
	@echo "Applying seed data to PostgreSQL..."
	docker exec -i fleetpulse-postgres psql -U fleetpulse -d fleetpulse < scripts/seed.sql

demo:
	@echo "Running FleetPulse automated demonstration scenario..."
	@bash scripts/demo.sh

clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BIN_DIR)
