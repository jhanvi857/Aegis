-- Aegis Migration: 003_phase3_dynamic_topology.sql
-- Production Dynamic Service Registry, Topology Graph Edges, Trace Auto-Discovery, and Versioned Snapshots.

CREATE TABLE IF NOT EXISTS topology_services (
    id VARCHAR(128) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(64) NOT NULL DEFAULT 'service', -- gateway, service, database, cache, queue
    namespace VARCHAR(128) NOT NULL DEFAULT 'default',
    cluster_id VARCHAR(128) NOT NULL DEFAULT 'cluster-primary',
    tier VARCHAR(64) NOT NULL DEFAULT 'tier-2',   -- tier-1-ingress, tier-1-storage, tier-2, tier-3
    capabilities TEXT NOT NULL DEFAULT 'scale,restart', -- comma-delimited: scale,restart,pool,cache,reroute,isolate
    owner VARCHAR(255) NOT NULL DEFAULT 'platform-team',
    version VARCHAR(64) NOT NULL DEFAULT 'v1.0.0',
    replicas INT NOT NULL DEFAULT 1,
    max_replicas INT NOT NULL DEFAULT 10,
    status VARCHAR(64) NOT NULL DEFAULT 'healthy', -- healthy, degraded, critical, unknown
    cpu DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    memory DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    latency DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    rps DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    error_rate DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    queue_depth INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_topology_services_ns ON topology_services(namespace);
CREATE INDEX IF NOT EXISTS idx_topology_services_cluster ON topology_services(cluster_id);
CREATE INDEX IF NOT EXISTS idx_topology_services_tier ON topology_services(tier);
CREATE INDEX IF NOT EXISTS idx_topology_services_status ON topology_services(status);

CREATE TABLE IF NOT EXISTS topology_edges (
    id VARCHAR(128) PRIMARY KEY,
    source_service_id VARCHAR(128) NOT NULL,
    target_service_id VARCHAR(128) NOT NULL,
    relationship VARCHAR(64) NOT NULL DEFAULT 'calls',
    namespace VARCHAR(128) NOT NULL DEFAULT 'default',
    cluster_id VARCHAR(128) NOT NULL DEFAULT 'cluster-primary',
    confidence DOUBLE PRECISION NOT NULL DEFAULT 1.0, -- 1.0 = manual override, 0.0-0.99 = auto-discovered from traces
    is_manual_override BOOLEAN NOT NULL DEFAULT FALSE,
    calls_per_sec DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    latency_ms DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    error_rate DOUBLE PRECISION NOT NULL DEFAULT 0.0,
    sample_count BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_topology_edge UNIQUE (source_service_id, target_service_id, namespace, cluster_id)
);

CREATE INDEX IF NOT EXISTS idx_topology_edges_source ON topology_edges(source_service_id);
CREATE INDEX IF NOT EXISTS idx_topology_edges_target ON topology_edges(target_service_id);
CREATE INDEX IF NOT EXISTS idx_topology_edges_ns ON topology_edges(namespace);

CREATE TABLE IF NOT EXISTS topology_versions (
    id VARCHAR(128) PRIMARY KEY,
    version INT NOT NULL,
    description TEXT NOT NULL,
    snapshot JSONB NOT NULL,
    created_by VARCHAR(255) NOT NULL DEFAULT 'system',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_topology_versions_ver ON topology_versions(version DESC);
