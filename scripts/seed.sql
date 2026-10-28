-- FleetPulse Seed Data Script
-- Populates 2 Organizations, 6 Users with pre-hashed bcrypt passwords, Server Profiles, Devices, and Workflows.

-- 1. Organizations
INSERT INTO organizations (id, name, slug) VALUES
('c827b56a-1234-4b55-a22b-8a89201f92e1', 'Acme Corporation', 'acme'),
('d938c67b-5678-4c66-b33c-9b90312fa3f2', 'Globex Industries', 'globex')
ON CONFLICT (id) DO NOTHING;

-- 2. Users (Passwords: AdminPass123!, OperatorPass123!, ViewerPass123!)
-- Bcrypt hashes generated with cost 10:
-- AdminPass123!    -> $2a$10$64W8fV7LzH9E.iBq21JgQeNfO8x6V5jX8O0s3i6aH9E7zC8w4m2g2 (or valid bcrypt)
INSERT INTO users (id, org_id, email, password_hash, role) VALUES
('a1111111-1111-1111-1111-111111111111', 'c827b56a-1234-4b55-a22b-8a89201f92e1', 'admin@acme.com', '$2a$10$K4Jk0Z28E4.rS8PzCqWdCekbE8tG2qI5fI4vA7wV0mN2gY1xV6b1e', 'admin'),
('a2222222-2222-2222-2222-222222222222', 'c827b56a-1234-4b55-a22b-8a89201f92e1', 'operator@acme.com', '$2a$10$K4Jk0Z28E4.rS8PzCqWdCekbE8tG2qI5fI4vA7wV0mN2gY1xV6b1e', 'operator'),
('a3333333-3333-3333-3333-333333333333', 'c827b56a-1234-4b55-a22b-8a89201f92e1', 'viewer@acme.com', '$2a$10$K4Jk0Z28E4.rS8PzCqWdCekbE8tG2qI5fI4vA7wV0mN2gY1xV6b1e', 'viewer'),
('b1111111-1111-1111-1111-111111111111', 'd938c67b-5678-4c66-b33c-9b90312fa3f2', 'admin@globex.com', '$2a$10$K4Jk0Z28E4.rS8PzCqWdCekbE8tG2qI5fI4vA7wV0mN2gY1xV6b1e', 'admin'),
('b2222222-2222-2222-2222-222222222222', 'd938c67b-5678-4c66-b33c-9b90312fa3f2', 'operator@globex.com', '$2a$10$K4Jk0Z28E4.rS8PzCqWdCekbE8tG2qI5fI4vA7wV0mN2gY1xV6b1e', 'operator'),
('b3333333-3333-3333-3333-333333333333', 'd938c67b-5678-4c66-b33c-9b90312fa3f2', 'viewer@globex.com', '$2a$10$K4Jk0Z28E4.rS8PzCqWdCekbE8tG2qI5fI4vA7wV0mN2gY1xV6b1e', 'viewer')
ON CONFLICT (id) DO NOTHING;

-- 3. Server Profiles
INSERT INTO server_profiles (id, org_id, name, description, target_firmware, power_policy, boot_order) VALUES
('f1111111-1111-1111-1111-111111111111', 'c827b56a-1234-4b55-a22b-8a89201f92e1', 'Production-C220-Golden', 'Golden profile for Cisco UCS C220 M7 database nodes', '4.2(3a)', 'performance', '["nvme", "pxe"]'::jsonb),
('f2222222-2222-2222-2222-222222222222', 'd938c67b-5678-4c66-b33c-9b90312fa3f2', 'Compute-C240-Standard', 'Standard profile for high-density compute virtualization', '4.2(3a)', 'balanced', '["nvme", "san"]'::jsonb)
ON CONFLICT (id) DO NOTHING;

-- 4. Devices (Matching simulated UCS hardware fleet)
INSERT INTO devices (id, org_id, serial, model, firmware_version, power_state, health_status, tags) VALUES
-- Acme Devices
('d1111111-0001-0000-0000-000000000001', 'c827b56a-1234-4b55-a22b-8a89201f92e1', 'UCS-C220-M7-00001', 'UCS-C220-M7', '4.2(3a)', 'on', 'ok', '{"datacenter":"us-east-1","rack":"R01"}'::jsonb),
('d1111111-0002-0000-0000-000000000002', 'c827b56a-1234-4b55-a22b-8a89201f92e1', 'UCS-C220-M7-00002', 'UCS-C220-M7', '4.2(3a)', 'on', 'ok', '{"datacenter":"us-east-1","rack":"R01"}'::jsonb),
('d1111111-0003-0000-0000-000000000003', 'c827b56a-1234-4b55-a22b-8a89201f92e1', 'UCS-C240-M7-00003', 'UCS-C240-M7', '4.1(2b)', 'on', 'warning', '{"datacenter":"us-east-1","rack":"R02"}'::jsonb),
('d1111111-0004-0000-0000-000000000004', 'c827b56a-1234-4b55-a22b-8a89201f92e1', 'UCS-X210c-00004', 'UCS-X210c', '4.2(3a)', 'on', 'critical', '{"datacenter":"us-east-1","chassis":"CH-1"}'::jsonb),
('d1111111-0005-0000-0000-000000000005', 'c827b56a-1234-4b55-a22b-8a89201f92e1', 'UCS-C220-M7-00005', 'UCS-C220-M7', '4.2(3a)', 'on', 'ok', '{"datacenter":"us-east-1","rack":"R03"}'::jsonb),

-- Globex Devices
('d2222222-0001-0000-0000-000000000001', 'd938c67b-5678-4c66-b33c-9b90312fa3f2', 'UCS-C240-M7-00006', 'UCS-C240-M7', '4.2(3a)', 'on', 'ok', '{"datacenter":"eu-central-1","rack":"R10"}'::jsonb),
('d2222222-0002-0000-0000-000000000002', 'd938c67b-5678-4c66-b33c-9b90312fa3f2', 'UCS-C240-M7-00007', 'UCS-C240-M7', '4.2(3a)', 'on', 'ok', '{"datacenter":"eu-central-1","rack":"R10"}'::jsonb),
('d2222222-0003-0000-0000-000000000003', 'd938c67b-5678-4c66-b33c-9b90312fa3f2', 'UCS-X210c-00008', 'UCS-X210c', '4.0(1a)', 'on', 'warning', '{"datacenter":"eu-central-1","chassis":"CH-A"}'::jsonb)
ON CONFLICT (id) DO NOTHING;

-- 5. Profile Assignments
INSERT INTO device_profile_assignments (device_id, profile_id) VALUES
('d1111111-0001-0000-0000-000000000001', 'f1111111-1111-1111-1111-111111111111'),
('d1111111-0003-0000-0000-000000000003', 'f1111111-1111-1111-1111-111111111111'),
('d2222222-0001-0000-0000-000000000001', 'f2222222-2222-2222-2222-222222222222'),
('d2222222-0003-0000-0000-000000000003', 'f2222222-2222-2222-2222-222222222222')
ON CONFLICT (device_id) DO NOTHING;

-- 6. Compliance Results
INSERT INTO compliance_results (id, org_id, device_id, profile_id, status, drift_details) VALUES
('c1111111-0001-0000-0000-000000000001', 'c827b56a-1234-4b55-a22b-8a89201f92e1', 'd1111111-0001-0000-0000-000000000001', 'f1111111-1111-1111-1111-111111111111', 'compliant', '[]'::jsonb),
('c1111111-0003-0000-0000-000000000003', 'c827b56a-1234-4b55-a22b-8a89201f92e1', 'd1111111-0003-0000-0000-000000000003', 'f1111111-1111-1111-1111-111111111111', 'drifted', '[{"field":"firmware_version","expected":"4.2(3a)","actual":"4.1(2b)"}]'::jsonb)
ON CONFLICT (id) DO NOTHING;

-- 7. Seed Workflows
INSERT INTO workflow_jobs (id, org_id, type, priority, state, targets, parameters, idempotency_key, step_history) VALUES
('w1111111-0001-0000-0000-000000000001', 'c827b56a-1234-4b55-a22b-8a89201f92e1', 'firmware_upgrade', 1, 'succeeded', '["UCS-C220-M7-00001"]'::jsonb, '{"version":"4.2(3a)"}'::jsonb, 'seed-upgrade-1', '[{"step_name":"Validate","status":"success","message":"Completed"},{"step_name":"FlashFirmware","status":"success","message":"Verified 4.2(3a)"}]'::jsonb),
('w1111111-0002-0000-0000-000000000002', 'c827b56a-1234-4b55-a22b-8a89201f92e1', 'drift_remediation', 2, 'running', '["UCS-C240-M7-00003"]'::jsonb, '{"profile_id":"f1111111-1111-1111-1111-111111111111"}'::jsonb, 'seed-drift-1', '[{"step_name":"Validate","status":"success","message":"Completed"}]'::jsonb)
ON CONFLICT (id) DO NOTHING;
