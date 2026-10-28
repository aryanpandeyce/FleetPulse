-- FleetPulse Database Schema Migration 001
-- PostgreSQL 16 compatible

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Organizations (Tenants)
CREATE TABLE IF NOT EXISTS organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL UNIQUE,
    slug VARCHAR(100) NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Users
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(32) NOT NULL CHECK (role IN ('admin', 'operator', 'viewer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_users_org ON users(org_id);

-- Devices
CREATE TABLE IF NOT EXISTS devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    serial VARCHAR(64) NOT NULL UNIQUE,
    model VARCHAR(64) NOT NULL,
    firmware_version VARCHAR(32) NOT NULL,
    power_state VARCHAR(16) NOT NULL DEFAULT 'on',
    health_status VARCHAR(16) NOT NULL DEFAULT 'ok' CHECK (health_status IN ('ok', 'warning', 'critical')),
    connection_state VARCHAR(16) NOT NULL DEFAULT 'connected' CHECK (connection_state IN ('connected', 'disconnected')),
    tags JSONB NOT NULL DEFAULT '{}'::jsonb,
    version INT NOT NULL DEFAULT 1,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- High-performance Keyset (cursor) pagination index:
CREATE INDEX IF NOT EXISTS idx_devices_keyset ON devices(org_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_devices_org_model ON devices(org_id, model);
CREATE INDEX IF NOT EXISTS idx_devices_org_health ON devices(org_id, health_status);
CREATE INDEX IF NOT EXISTS idx_devices_tags ON devices USING GIN (tags);

-- Device Claims (Single-use, expiring)
CREATE TABLE IF NOT EXISTS device_claims (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    claim_code VARCHAR(64) NOT NULL UNIQUE,
    serial VARCHAR(64),
    status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'claimed', 'expired')),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    claimed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_device_claims_lookup ON device_claims(claim_code) WHERE status = 'pending';

-- Server Profiles
CREATE TABLE IF NOT EXISTS server_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(128) NOT NULL,
    description TEXT,
    target_firmware VARCHAR(32) NOT NULL,
    power_policy VARCHAR(32) NOT NULL DEFAULT 'performance',
    boot_order JSONB NOT NULL DEFAULT '["nvme", "pxe"]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(org_id, name)
);

-- Device to Profile Assignment
CREATE TABLE IF NOT EXISTS device_profile_assignments (
    device_id UUID PRIMARY KEY REFERENCES devices(id) ON DELETE CASCADE,
    profile_id UUID NOT NULL REFERENCES server_profiles(id) ON DELETE CASCADE,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Compliance Results
CREATE TABLE IF NOT EXISTS compliance_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    profile_id UUID NOT NULL REFERENCES server_profiles(id) ON DELETE CASCADE,
    status VARCHAR(16) NOT NULL CHECK (status IN ('compliant', 'drifted')),
    drift_details JSONB NOT NULL DEFAULT '[]'::jsonb,
    evaluated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_compliance_device ON compliance_results(device_id, evaluated_at DESC);

-- Alarms
CREATE TABLE IF NOT EXISTS alarms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    rule_name VARCHAR(128) NOT NULL,
    severity VARCHAR(16) NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    status VARCHAR(16) NOT NULL DEFAULT 'raised' CHECK (status IN ('raised', 'acknowledged', 'cleared')),
    message TEXT NOT NULL,
    metric_name VARCHAR(64),
    current_value DOUBLE PRECISION,
    threshold_value DOUBLE PRECISION,
    raised_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    acknowledged_at TIMESTAMPTZ,
    acknowledged_by UUID REFERENCES users(id),
    cleared_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_alarms_org_status ON alarms(org_id, status, raised_at DESC);
CREATE INDEX IF NOT EXISTS idx_alarms_device_rule ON alarms(device_id, rule_name) WHERE status != 'cleared';

-- Telemetry Rollups
CREATE TABLE IF NOT EXISTS telemetry_rollups (
    id BIGSERIAL PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    device_id UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    bucket_interval VARCHAR(8) NOT NULL CHECK (bucket_interval IN ('1m', '5m')),
    window_start TIMESTAMPTZ NOT NULL,
    avg_inlet_temp DOUBLE PRECISION NOT NULL,
    max_inlet_temp DOUBLE PRECISION NOT NULL,
    avg_cpu_temp DOUBLE PRECISION NOT NULL,
    max_cpu_temp DOUBLE PRECISION NOT NULL,
    avg_power_watts DOUBLE PRECISION NOT NULL,
    max_power_watts DOUBLE PRECISION NOT NULL,
    avg_fan_rpm DOUBLE PRECISION NOT NULL,
    min_fan_rpm DOUBLE PRECISION NOT NULL,
    sample_count INT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_rollups_query ON telemetry_rollups(device_id, bucket_interval, window_start DESC);

-- Workflow Jobs
CREATE TABLE IF NOT EXISTS workflow_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    type VARCHAR(32) NOT NULL CHECK (type IN ('firmware_upgrade', 'drift_remediation', 'power_cycle')),
    priority INT NOT NULL DEFAULT 2,
    state VARCHAR(16) NOT NULL DEFAULT 'queued' CHECK (state IN ('queued', 'running', 'succeeded', 'failed', 'cancelled')),
    targets JSONB NOT NULL,
    parameters JSONB NOT NULL DEFAULT '{}'::jsonb,
    idempotency_key VARCHAR(128) NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    step_history JSONB NOT NULL DEFAULT '[]'::jsonb,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    UNIQUE(org_id, idempotency_key)
);
CREATE INDEX IF NOT EXISTS idx_workflow_scheduler ON workflow_jobs(state, priority, created_at ASC);
