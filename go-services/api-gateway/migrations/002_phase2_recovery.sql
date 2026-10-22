-- Aegis Migration: 002_phase2_recovery.sql
-- Production-grade Recovery Management, Policy Evaluation, Verification Tracking, and Escalation Records.

CREATE TABLE IF NOT EXISTS recovery_executions (
    id VARCHAR(64) PRIMARY KEY,
    plan_id VARCHAR(64) NOT NULL,
    action_id VARCHAR(64) NOT NULL,
    action_type VARCHAR(64) NOT NULL,
    target_service_id VARCHAR(64) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'Pending', -- Pending, Verifying, Verified, Ineffective, RolledBack
    risk_level VARCHAR(32) NOT NULL,
    risk_score DOUBLE PRECISION NOT NULL,
    requires_approval BOOLEAN NOT NULL DEFAULT FALSE,
    approver VARCHAR(255),
    dry_run BOOLEAN NOT NULL DEFAULT FALSE,
    idempotency_key VARCHAR(128),
    duration_ms BIGINT DEFAULT 0,
    duration_str VARCHAR(32) DEFAULT '',
    metrics_before JSONB,
    metrics_after JSONB,
    risk_before DOUBLE PRECISION DEFAULT 0.0,
    risk_after DOUBLE PRECISION DEFAULT 0.0,
    escalated_action VARCHAR(64),
    error_message TEXT,
    started_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_recovery_executions_target ON recovery_executions(target_service_id);
CREATE INDEX IF NOT EXISTS idx_recovery_executions_status ON recovery_executions(status);
CREATE INDEX IF NOT EXISTS idx_recovery_executions_idempotency ON recovery_executions(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_recovery_executions_started_at ON recovery_executions(started_at DESC);
