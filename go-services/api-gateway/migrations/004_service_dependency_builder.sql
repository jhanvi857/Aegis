-- Aegis Migration: 004_service_dependency_builder.sql
-- Service & Dependency Builder: monitoring mode, telemetry source, recovery binding, edge lifecycle & validation

ALTER TABLE topology_services ADD COLUMN IF NOT EXISTS monitoring_mode VARCHAR(32) NOT NULL DEFAULT 'observe';
ALTER TABLE topology_services ADD COLUMN IF NOT EXISTS telemetry_source JSONB NULL;
ALTER TABLE topology_services ADD COLUMN IF NOT EXISTS recovery_binding JSONB NULL;
ALTER TABLE topology_services ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '';
ALTER TABLE topology_services ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '[]';
ALTER TABLE topology_services ADD COLUMN IF NOT EXISTS last_telemetry_time TIMESTAMP WITH TIME ZONE NULL;

CREATE INDEX IF NOT EXISTS idx_topology_services_mode ON topology_services(monitoring_mode);

ALTER TABLE topology_edges ADD COLUMN IF NOT EXISTS source VARCHAR(32) NOT NULL DEFAULT 'manual';
ALTER TABLE topology_edges ADD COLUMN IF NOT EXISTS state VARCHAR(32) NOT NULL DEFAULT 'active';
ALTER TABLE topology_edges ADD COLUMN IF NOT EXISTS edge_type VARCHAR(32) NOT NULL DEFAULT 'sync';
ALTER TABLE topology_edges ADD COLUMN IF NOT EXISTS created_by VARCHAR(255) NOT NULL DEFAULT 'system';

CREATE INDEX IF NOT EXISTS idx_topology_edges_state ON topology_edges(state);
CREATE INDEX IF NOT EXISTS idx_topology_edges_source ON topology_edges(source);
