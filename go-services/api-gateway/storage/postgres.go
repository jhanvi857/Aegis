package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(dsn string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to ping postgres database: %w", err)
	}

	store := &PostgresStore{db: db}
	if err := store.autoMigrate(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to auto-migrate database schema: %w", err)
	}

	return store, nil
}

func (p *PostgresStore) autoMigrate(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id VARCHAR(64) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		email VARCHAR(255) NOT NULL UNIQUE,
		password_hash VARCHAR(255) NOT NULL,
		salt VARCHAR(64) NOT NULL,
		role VARCHAR(32) NOT NULL DEFAULT 'viewer',
		must_change_password BOOLEAN NOT NULL DEFAULT FALSE,
		failed_login_attempts INT NOT NULL DEFAULT 0,
		locked_until TIMESTAMP WITH TIME ZONE NULL,
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
	ALTER TABLE users ADD COLUMN IF NOT EXISTS must_change_password BOOLEAN NOT NULL DEFAULT FALSE;

	CREATE TABLE IF NOT EXISTS sessions (
		id VARCHAR(128) PRIMARY KEY,
		user_id VARCHAR(64) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
		expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
		revoked_at TIMESTAMP WITH TIME ZONE NULL,
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
	CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

	CREATE TABLE IF NOT EXISTS api_keys (
		id VARCHAR(64) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		key_prefix VARCHAR(16) NOT NULL,
		key_hash VARCHAR(255) NOT NULL,
		scopes TEXT NOT NULL,
		role VARCHAR(32) NOT NULL DEFAULT 'operator',
		created_by VARCHAR(64) NOT NULL,
		expires_at TIMESTAMP WITH TIME ZONE NULL,
		revoked_at TIMESTAMP WITH TIME ZONE NULL,
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
	);
	CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(key_prefix);

	CREATE TABLE IF NOT EXISTS audit_logs (
		id BIGSERIAL PRIMARY KEY,
		timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
		actor_id VARCHAR(64) NOT NULL,
		actor_email VARCHAR(255) NOT NULL,
		actor_role VARCHAR(32) NOT NULL,
		action VARCHAR(64) NOT NULL,
		target_service_id VARCHAR(64),
		risk_level VARCHAR(32),
		approval_status VARCHAR(32),
		payload TEXT,
		result_status VARCHAR(32) NOT NULL,
		error_message TEXT,
		ip_address VARCHAR(64)
	);
	CREATE INDEX IF NOT EXISTS idx_audit_logs_timestamp ON audit_logs(timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_audit_logs_actor ON audit_logs(actor_email);
	CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);

	CREATE TABLE IF NOT EXISTS recovery_executions (
		id VARCHAR(64) PRIMARY KEY,
		plan_id VARCHAR(64) NOT NULL,
		action_id VARCHAR(64) NOT NULL,
		action_type VARCHAR(64) NOT NULL,
		target_service_id VARCHAR(64) NOT NULL,
		status VARCHAR(32) NOT NULL DEFAULT 'Pending',
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

	CREATE TABLE IF NOT EXISTS topology_services (
		id VARCHAR(128) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		type VARCHAR(64) NOT NULL DEFAULT 'service',
		namespace VARCHAR(128) NOT NULL DEFAULT 'default',
		cluster_id VARCHAR(128) NOT NULL DEFAULT 'cluster-primary',
		tier VARCHAR(64) NOT NULL DEFAULT 'tier-2',
		capabilities TEXT NOT NULL DEFAULT 'scale,restart',
		owner VARCHAR(255) NOT NULL DEFAULT 'platform-team',
		version VARCHAR(64) NOT NULL DEFAULT 'v1.0.0',
		replicas INT NOT NULL DEFAULT 1,
		max_replicas INT NOT NULL DEFAULT 10,
		status VARCHAR(64) NOT NULL DEFAULT 'healthy',
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
		confidence DOUBLE PRECISION NOT NULL DEFAULT 1.0,
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
	`
	_, err := p.db.ExecContext(ctx, schema)
	return err
}

func (p *PostgresStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT id, name, email, password_hash, salt, role, must_change_password, failed_login_attempts, locked_until, created_at, updated_at
		FROM users WHERE LOWER(email) = LOWER($1)
	`, email)

	var u User
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.Salt, &u.Role, &u.MustChangePassword, &u.FailedLoginAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (p *PostgresStore) GetUserByID(ctx context.Context, id string) (*User, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT id, name, email, password_hash, salt, role, must_change_password, failed_login_attempts, locked_until, created_at, updated_at
		FROM users WHERE id = $1
	`, id)

	var u User
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.Salt, &u.Role, &u.MustChangePassword, &u.FailedLoginAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (p *PostgresStore) CreateUser(ctx context.Context, user *User) error {
	now := time.Now().UTC()
	if user.CreatedAt.IsZero() {
		user.CreatedAt = now
	}
	user.UpdatedAt = now

	_, err := p.db.ExecContext(ctx, `
		INSERT INTO users (id, name, email, password_hash, salt, role, must_change_password, failed_login_attempts, locked_until, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`, user.ID, user.Name, user.Email, user.PasswordHash, user.Salt, user.Role, user.MustChangePassword, user.FailedLoginAttempts, user.LockedUntil, user.CreatedAt, user.UpdatedAt)
	return err
}

func (p *PostgresStore) UpdateUserPassword(ctx context.Context, id, passwordHash, salt string, mustChangePassword bool) error {
	res, err := p.db.ExecContext(ctx, `
		UPDATE users SET password_hash = $1, salt = $2, must_change_password = $3, updated_at = NOW()
		WHERE id = $4
	`, passwordHash, salt, mustChangePassword, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *PostgresStore) UpdateUserRole(ctx context.Context, id string, role string) error {
	res, err := p.db.ExecContext(ctx, `UPDATE users SET role = $1, updated_at = NOW() WHERE id = $2`, role, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *PostgresStore) UpdateUserLoginAttempt(ctx context.Context, email string, failedAttempts int, lockedUntil *time.Time) error {
	res, err := p.db.ExecContext(ctx, `
		UPDATE users SET failed_login_attempts = $1, locked_until = $2, updated_at = NOW()
		WHERE LOWER(email) = LOWER($3)
	`, failedAttempts, lockedUntil, email)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *PostgresStore) ListUsers(ctx context.Context) ([]*User, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, name, email, role, failed_login_attempts, locked_until, created_at, updated_at
		FROM users ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]*User, 0)
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.FailedLoginAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, &u)
	}
	return users, rows.Err()
}

func (p *PostgresStore) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

func (p *PostgresStore) CreateSession(ctx context.Context, session *Session) error {
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now().UTC()
	}
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, expires_at, revoked_at, created_at)
		VALUES ($1, $2, $3, $4, $5)
	`, session.ID, session.UserID, session.ExpiresAt, session.RevokedAt, session.CreatedAt)
	return err
}

func (p *PostgresStore) GetSession(ctx context.Context, token string) (*Session, *User, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT s.id, s.user_id, s.expires_at, s.revoked_at, s.created_at,
		       u.id, u.name, u.email, u.password_hash, u.salt, u.role, u.failed_login_attempts, u.locked_until, u.created_at, u.updated_at
		FROM sessions s
		JOIN users u ON s.user_id = u.id
		WHERE s.id = $1
	`, token)

	var s Session
	var u User
	err := row.Scan(
		&s.ID, &s.UserID, &s.ExpiresAt, &s.RevokedAt, &s.CreatedAt,
		&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.Salt, &u.Role, &u.FailedLoginAttempts, &u.LockedUntil, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	if s.RevokedAt != nil {
		return nil, nil, errors.New("session has been revoked")
	}
	if time.Now().After(s.ExpiresAt) {
		return nil, nil, errors.New("session has expired")
	}
	return &s, &u, nil
}

func (p *PostgresStore) RevokeSession(ctx context.Context, token string) error {
	res, err := p.db.ExecContext(ctx, `UPDATE sessions SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL`, token)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *PostgresStore) RevokeUserSessions(ctx context.Context, userID string) error {
	_, err := p.db.ExecContext(ctx, `UPDATE sessions SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return err
}

func (p *PostgresStore) CreateAPIKey(ctx context.Context, key *APIKey) error {
	if key.CreatedAt.IsZero() {
		key.CreatedAt = time.Now().UTC()
	}
	_, err := p.db.ExecContext(ctx, `
		INSERT INTO api_keys (id, name, key_prefix, key_hash, scopes, role, created_by, expires_at, revoked_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, key.ID, key.Name, key.KeyPrefix, key.KeyHash, key.Scopes, key.Role, key.CreatedBy, key.ExpiresAt, key.RevokedAt, key.CreatedAt)
	return err
}

func (p *PostgresStore) GetAPIKeyByHash(ctx context.Context, hash string) (*APIKey, error) {
	row := p.db.QueryRowContext(ctx, `
		SELECT id, name, key_prefix, key_hash, scopes, role, created_by, expires_at, revoked_at, created_at
		FROM api_keys WHERE key_hash = $1
	`, hash)

	var k APIKey
	err := row.Scan(&k.ID, &k.Name, &k.KeyPrefix, &k.KeyHash, &k.Scopes, &k.Role, &k.CreatedBy, &k.ExpiresAt, &k.RevokedAt, &k.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if k.RevokedAt != nil {
		return nil, errors.New("API key has been revoked")
	}
	if k.ExpiresAt != nil && time.Now().After(*k.ExpiresAt) {
		return nil, errors.New("API key has expired")
	}
	return &k, nil
}

func (p *PostgresStore) ListAPIKeys(ctx context.Context) ([]*APIKey, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, name, key_prefix, scopes, role, created_by, expires_at, revoked_at, created_at
		FROM api_keys ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := make([]*APIKey, 0)
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.KeyPrefix, &k.Scopes, &k.Role, &k.CreatedBy, &k.ExpiresAt, &k.RevokedAt, &k.CreatedAt); err != nil {
			return nil, err
		}
		keys = append(keys, &k)
	}
	return keys, rows.Err()
}

func (p *PostgresStore) RevokeAPIKey(ctx context.Context, id string) error {
	res, err := p.db.ExecContext(ctx, `UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND revoked_at IS NULL`, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *PostgresStore) AppendAuditLog(ctx context.Context, entry *AuditLogEntry) error {
	if entry.Timestamp.IsZero() {
		entry.Timestamp = time.Now().UTC()
	}
	err := p.db.QueryRowContext(ctx, `
		INSERT INTO audit_logs (timestamp, actor_id, actor_email, actor_role, action, target_service_id, risk_level, approval_status, payload, result_status, error_message, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`, entry.Timestamp, entry.ActorID, entry.ActorEmail, entry.ActorRole, entry.Action, entry.TargetServiceID, entry.RiskLevel, entry.ApprovalStatus, entry.Payload, entry.ResultStatus, entry.ErrorMessage, entry.IPAddress).Scan(&entry.ID)
	return err
}

func (p *PostgresStore) ListAuditLogs(ctx context.Context, limit int) ([]*AuditLogEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := p.db.QueryContext(ctx, `
		SELECT id, timestamp, actor_id, actor_email, actor_role, action, COALESCE(target_service_id, ''), COALESCE(risk_level, ''), COALESCE(approval_status, ''), COALESCE(payload, ''), result_status, COALESCE(error_message, ''), COALESCE(ip_address, '')
		FROM audit_logs ORDER BY id DESC LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]*AuditLogEntry, 0)
	for rows.Next() {
		var e AuditLogEntry
		if err := rows.Scan(&e.ID, &e.Timestamp, &e.ActorID, &e.ActorEmail, &e.ActorRole, &e.Action, &e.TargetServiceID, &e.RiskLevel, &e.ApprovalStatus, &e.Payload, &e.ResultStatus, &e.ErrorMessage, &e.IPAddress); err != nil {
			return nil, err
		}
		entries = append(entries, &e)
	}
	return entries, rows.Err()
}

func (p *PostgresStore) CreateRecoveryExecution(ctx context.Context, exec *RecoveryExecution) error {
	if exec.StartedAt.IsZero() {
		exec.StartedAt = time.Now().UTC()
	}

	beforeJSON, _ := json.Marshal(exec.MetricsBefore)
	afterJSON, _ := json.Marshal(exec.MetricsAfter)

	query := `
		INSERT INTO recovery_executions (
			id, plan_id, action_id, action_type, target_service_id, status, risk_level, risk_score,
			requires_approval, approver, dry_run, idempotency_key, duration_ms, duration_str,
			metrics_before, metrics_after, risk_before, risk_after, escalated_action, error_message,
			started_at, completed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22
		)
	`
	_, err := p.db.ExecContext(ctx, query,
		exec.ID, exec.PlanID, exec.ActionID, exec.ActionType, exec.TargetServiceID, exec.Status,
		exec.RiskLevel, exec.RiskScore, exec.RequiresApproval, exec.Approver, exec.DryRun,
		exec.IdempotencyKey, exec.DurationMs, exec.DurationStr, string(beforeJSON), string(afterJSON),
		exec.RiskBefore, exec.RiskAfter, exec.EscalatedAction, exec.ErrorMessage,
		exec.StartedAt, exec.CompletedAt,
	)
	return err
}

func (p *PostgresStore) UpdateRecoveryExecution(ctx context.Context, exec *RecoveryExecution) error {
	afterJSON, _ := json.Marshal(exec.MetricsAfter)
	query := `
		UPDATE recovery_executions SET
			status = $1, duration_ms = $2, duration_str = $3, metrics_after = $4,
			risk_after = $5, escalated_action = $6, error_message = $7, completed_at = $8
		WHERE id = $9
	`
	res, err := p.db.ExecContext(ctx, query,
		exec.Status, exec.DurationMs, exec.DurationStr, string(afterJSON),
		exec.RiskAfter, exec.EscalatedAction, exec.ErrorMessage, exec.CompletedAt, exec.ID,
	)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *PostgresStore) GetRecoveryExecution(ctx context.Context, id string) (*RecoveryExecution, error) {
	query := `
		SELECT id, plan_id, action_id, action_type, target_service_id, status, risk_level, risk_score,
		       requires_approval, COALESCE(approver, ''), dry_run, COALESCE(idempotency_key, ''),
		       duration_ms, duration_str, metrics_before, metrics_after, risk_before, risk_after,
		       COALESCE(escalated_action, ''), COALESCE(error_message, ''), started_at, completed_at
		FROM recovery_executions WHERE id = $1
	`
	row := p.db.QueryRowContext(ctx, query, id)
	return scanRecoveryExecution(row)
}

func (p *PostgresStore) GetRecoveryExecutionByIdempotencyKey(ctx context.Context, key string) (*RecoveryExecution, error) {
	if key == "" {
		return nil, ErrNotFound
	}
	query := `
		SELECT id, plan_id, action_id, action_type, target_service_id, status, risk_level, risk_score,
		       requires_approval, COALESCE(approver, ''), dry_run, COALESCE(idempotency_key, ''),
		       duration_ms, duration_str, metrics_before, metrics_after, risk_before, risk_after,
		       COALESCE(escalated_action, ''), COALESCE(error_message, ''), started_at, completed_at
		FROM recovery_executions WHERE idempotency_key = $1
	`
	row := p.db.QueryRowContext(ctx, query, key)
	return scanRecoveryExecution(row)
}

func (p *PostgresStore) ListRecoveryExecutions(ctx context.Context, limit int) ([]*RecoveryExecution, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, plan_id, action_id, action_type, target_service_id, status, risk_level, risk_score,
		       requires_approval, COALESCE(approver, ''), dry_run, COALESCE(idempotency_key, ''),
		       duration_ms, duration_str, metrics_before, metrics_after, risk_before, risk_after,
		       COALESCE(escalated_action, ''), COALESCE(error_message, ''), started_at, completed_at
		FROM recovery_executions ORDER BY started_at DESC LIMIT $1
	`
	rows, err := p.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*RecoveryExecution, 0)
	for rows.Next() {
		exec, err := scanRecoveryExecutionRow(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, exec)
	}
	return list, rows.Err()
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanRecoveryExecution(row *sql.Row) (*RecoveryExecution, error) {
	return scanRowHelper(row)
}

func scanRecoveryExecutionRow(rows *sql.Rows) (*RecoveryExecution, error) {
	return scanRowHelper(rows)
}

func scanRowHelper(scanner rowScanner) (*RecoveryExecution, error) {
	var exec RecoveryExecution
	var beforeStr, afterStr sql.NullString
	err := scanner.Scan(
		&exec.ID, &exec.PlanID, &exec.ActionID, &exec.ActionType, &exec.TargetServiceID, &exec.Status,
		&exec.RiskLevel, &exec.RiskScore, &exec.RequiresApproval, &exec.Approver, &exec.DryRun,
		&exec.IdempotencyKey, &exec.DurationMs, &exec.DurationStr, &beforeStr, &afterStr,
		&exec.RiskBefore, &exec.RiskAfter, &exec.EscalatedAction, &exec.ErrorMessage,
		&exec.StartedAt, &exec.CompletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if beforeStr.Valid && beforeStr.String != "" {
		_ = json.Unmarshal([]byte(beforeStr.String), &exec.MetricsBefore)
	}
	if afterStr.Valid && afterStr.String != "" {
		_ = json.Unmarshal([]byte(afterStr.String), &exec.MetricsAfter)
	}
	return &exec, nil
}

func (p *PostgresStore) UpsertService(ctx context.Context, svc *TopologyService) error {
	query := `
		INSERT INTO topology_services (
			id, name, type, namespace, cluster_id, tier, capabilities, owner, version,
			replicas, max_replicas, status, cpu, memory, latency, rps, error_rate, queue_depth,
			monitoring_mode, telemetry_source, recovery_binding, description, tags, last_telemetry_time,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			type = EXCLUDED.type,
			namespace = EXCLUDED.namespace,
			cluster_id = EXCLUDED.cluster_id,
			tier = EXCLUDED.tier,
			capabilities = EXCLUDED.capabilities,
			owner = EXCLUDED.owner,
			version = EXCLUDED.version,
			replicas = EXCLUDED.replicas,
			max_replicas = EXCLUDED.max_replicas,
			status = EXCLUDED.status,
			cpu = EXCLUDED.cpu,
			memory = EXCLUDED.memory,
			latency = EXCLUDED.latency,
			rps = EXCLUDED.rps,
			error_rate = EXCLUDED.error_rate,
			queue_depth = EXCLUDED.queue_depth,
			monitoring_mode = EXCLUDED.monitoring_mode,
			telemetry_source = EXCLUDED.telemetry_source,
			recovery_binding = EXCLUDED.recovery_binding,
			description = EXCLUDED.description,
			tags = EXCLUDED.tags,
			last_telemetry_time = EXCLUDED.last_telemetry_time,
			updated_at = EXCLUDED.updated_at
	`
	now := time.Now().UTC()
	if svc.CreatedAt.IsZero() {
		svc.CreatedAt = now
	}
	svc.UpdatedAt = now
	if svc.Namespace == "" {
		svc.Namespace = "default"
	}
	if svc.ClusterID == "" {
		svc.ClusterID = "cluster-primary"
	}
	if svc.Tier == "" {
		svc.Tier = "tier-2"
	}
	if svc.Capabilities == "" {
		svc.Capabilities = "scale,restart"
	}
	if svc.MonitoringMode == "" {
		svc.MonitoringMode = "observe"
	}
	if svc.Status == "" {
		svc.Status = svc.ComputeStatus()
	}

	var telemetryJSON []byte
	if svc.TelemetrySource != nil {
		srcCopy := *svc.TelemetrySource
		if srcCopy.AuthToken != "" && !strings.HasPrefix(srcCopy.AuthToken, "enc::") {
			srcCopy.AuthToken, _ = EncryptCredential(srcCopy.AuthToken)
		}
		if srcCopy.AuthPassword != "" && !strings.HasPrefix(srcCopy.AuthPassword, "enc::") {
			srcCopy.AuthPassword, _ = EncryptCredential(srcCopy.AuthPassword)
		}
		telemetryJSON, _ = json.Marshal(srcCopy)
	}

	var recoveryJSON []byte
	if svc.RecoveryBinding != nil {
		recoveryJSON, _ = json.Marshal(svc.RecoveryBinding)
	}

	tagsJSON, _ := json.Marshal(svc.Tags)

	_, err := p.db.ExecContext(ctx, query,
		svc.ID, svc.Name, svc.Type, svc.Namespace, svc.ClusterID, svc.Tier, svc.Capabilities,
		svc.Owner, svc.Version, svc.Replicas, svc.MaxReplicas, svc.Status, svc.CPU, svc.Memory,
		svc.Latency, svc.RPS, svc.ErrorRate, svc.QueueDepth,
		svc.MonitoringMode, telemetryJSON, recoveryJSON, svc.Description, tagsJSON, svc.LastTelemetryTime,
		svc.CreatedAt, svc.UpdatedAt,
	)
	return err
}

func (p *PostgresStore) GetService(ctx context.Context, id string) (*TopologyService, error) {
	query := `
		SELECT id, name, type, namespace, cluster_id, tier, capabilities, owner, version,
		       replicas, max_replicas, status, cpu, memory, latency, rps, error_rate, queue_depth,
		       COALESCE(monitoring_mode, 'observe'), telemetry_source, recovery_binding,
		       COALESCE(description, ''), tags, last_telemetry_time,
		       created_at, updated_at
		FROM topology_services WHERE id = $1
	`
	var s TopologyService
	var telemJSON, recovJSON, tagsJSON []byte
	err := p.db.QueryRowContext(ctx, query, id).Scan(
		&s.ID, &s.Name, &s.Type, &s.Namespace, &s.ClusterID, &s.Tier, &s.Capabilities,
		&s.Owner, &s.Version, &s.Replicas, &s.MaxReplicas, &s.Status, &s.CPU, &s.Memory,
		&s.Latency, &s.RPS, &s.ErrorRate, &s.QueueDepth,
		&s.MonitoringMode, &telemJSON, &recovJSON,
		&s.Description, &tagsJSON, &s.LastTelemetryTime,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(telemJSON) > 0 {
		var src TelemetrySource
		if err := json.Unmarshal(telemJSON, &src); err == nil {
			s.TelemetrySource = &src
		}
	}
	if len(recovJSON) > 0 {
		var recov RecoveryBinding
		if err := json.Unmarshal(recovJSON, &recov); err == nil {
			s.RecoveryBinding = &recov
		}
	}
	if len(tagsJSON) > 0 {
		_ = json.Unmarshal(tagsJSON, &s.Tags)
	}
	return &s, nil
}

func (p *PostgresStore) DeleteService(ctx context.Context, id string) error {
	// Cascade delete edges connected to this service
	_, _ = p.db.ExecContext(ctx, "DELETE FROM topology_edges WHERE source_service_id = $1 OR target_service_id = $1", id)
	_, err := p.db.ExecContext(ctx, "DELETE FROM topology_services WHERE id = $1", id)
	return err
}

func (p *PostgresStore) ListServices(ctx context.Context, namespace, clusterID string) ([]*TopologyService, error) {
	query := `
		SELECT id, name, type, namespace, cluster_id, tier, capabilities, owner, version,
		       replicas, max_replicas, status, cpu, memory, latency, rps, error_rate, queue_depth,
		       COALESCE(monitoring_mode, 'observe'), telemetry_source, recovery_binding,
		       COALESCE(description, ''), tags, last_telemetry_time,
		       created_at, updated_at
		FROM topology_services
		WHERE ($1 = '' OR namespace = $1)
		  AND ($2 = '' OR cluster_id = $2)
		ORDER BY id ASC
	`
	rows, err := p.db.QueryContext(ctx, query, namespace, clusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*TopologyService, 0)
	for rows.Next() {
		var s TopologyService
		var telemJSON, recovJSON, tagsJSON []byte
		err := rows.Scan(
			&s.ID, &s.Name, &s.Type, &s.Namespace, &s.ClusterID, &s.Tier, &s.Capabilities,
			&s.Owner, &s.Version, &s.Replicas, &s.MaxReplicas, &s.Status, &s.CPU, &s.Memory,
			&s.Latency, &s.RPS, &s.ErrorRate, &s.QueueDepth,
			&s.MonitoringMode, &telemJSON, &recovJSON,
			&s.Description, &tagsJSON, &s.LastTelemetryTime,
			&s.CreatedAt, &s.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		if len(telemJSON) > 0 {
			var src TelemetrySource
			if err := json.Unmarshal(telemJSON, &src); err == nil {
				s.TelemetrySource = &src
			}
		}
		if len(recovJSON) > 0 {
			var recov RecoveryBinding
			if err := json.Unmarshal(recovJSON, &recov); err == nil {
				s.RecoveryBinding = &recov
			}
		}
		if len(tagsJSON) > 0 {
			_ = json.Unmarshal(tagsJSON, &s.Tags)
		}
		list = append(list, &s)
	}
	return list, rows.Err()
}

func (p *PostgresStore) UpsertEdge(ctx context.Context, edge *TopologyEdge) error {
	query := `
		INSERT INTO topology_edges (
			id, source_service_id, target_service_id, relationship, namespace, cluster_id,
			confidence, is_manual_override, calls_per_sec, latency_ms, error_rate, sample_count,
			source, state, edge_type, created_by,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (source_service_id, target_service_id, namespace, cluster_id) DO UPDATE SET
			relationship = EXCLUDED.relationship,
			confidence = EXCLUDED.confidence,
			is_manual_override = EXCLUDED.is_manual_override,
			calls_per_sec = EXCLUDED.calls_per_sec,
			latency_ms = EXCLUDED.latency_ms,
			error_rate = EXCLUDED.error_rate,
			sample_count = EXCLUDED.sample_count,
			source = EXCLUDED.source,
			state = EXCLUDED.state,
			edge_type = EXCLUDED.edge_type,
			created_by = EXCLUDED.created_by,
			updated_at = EXCLUDED.updated_at
	`
	now := time.Now().UTC()
	if edge.CreatedAt.IsZero() {
		edge.CreatedAt = now
	}
	edge.UpdatedAt = now
	if edge.Namespace == "" {
		edge.Namespace = "default"
	}
	if edge.ClusterID == "" {
		edge.ClusterID = "cluster-primary"
	}
	if edge.ID == "" {
		edge.ID = fmt.Sprintf("edge-%s-%s", edge.SourceServiceID, edge.TargetServiceID)
	}
	if edge.Source == "" {
		edge.Source = "manual"
	}
	if edge.State == "" {
		edge.State = "active"
	}
	if edge.EdgeType == "" {
		edge.EdgeType = "sync"
	}
	if edge.CreatedBy == "" {
		edge.CreatedBy = "system"
	}
	if edge.Source == "manual" {
		edge.IsManualOverride = true
		edge.Confidence = 1.0
	}

	_, err := p.db.ExecContext(ctx, query,
		edge.ID, edge.SourceServiceID, edge.TargetServiceID, edge.Relationship,
		edge.Namespace, edge.ClusterID, edge.Confidence, edge.IsManualOverride,
		edge.CallsPerSec, edge.LatencyMs, edge.ErrorRate, edge.SampleCount,
		edge.Source, edge.State, edge.EdgeType, edge.CreatedBy,
		edge.CreatedAt, edge.UpdatedAt,
	)
	return err
}

func (p *PostgresStore) GetEdgeByID(ctx context.Context, id string) (*TopologyEdge, error) {
	query := `
		SELECT id, source_service_id, target_service_id, relationship, namespace, cluster_id,
		       confidence, is_manual_override, calls_per_sec, latency_ms, error_rate, sample_count,
		       COALESCE(source, 'manual'), COALESCE(state, 'active'), COALESCE(edge_type, 'sync'), COALESCE(created_by, 'system'),
		       created_at, updated_at
		FROM topology_edges
		WHERE id = $1
	`
	var e TopologyEdge
	err := p.db.QueryRowContext(ctx, query, id).Scan(
		&e.ID, &e.SourceServiceID, &e.TargetServiceID, &e.Relationship, &e.Namespace, &e.ClusterID,
		&e.Confidence, &e.IsManualOverride, &e.CallsPerSec, &e.LatencyMs, &e.ErrorRate, &e.SampleCount,
		&e.Source, &e.State, &e.EdgeType, &e.CreatedBy,
		&e.CreatedAt, &e.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (p *PostgresStore) DeleteEdge(ctx context.Context, sourceID, targetID, namespace, clusterID string) error {
	if namespace == "" {
		namespace = "default"
	}
	if clusterID == "" {
		clusterID = "cluster-primary"
	}
	query := `
		DELETE FROM topology_edges
		WHERE source_service_id = $1 AND target_service_id = $2
		  AND namespace = $3 AND cluster_id = $4
	`
	_, err := p.db.ExecContext(ctx, query, sourceID, targetID, namespace, clusterID)
	return err
}

func (p *PostgresStore) ListEdges(ctx context.Context, namespace, clusterID string) ([]*TopologyEdge, error) {
	query := `
		SELECT id, source_service_id, target_service_id, relationship, namespace, cluster_id,
		       confidence, is_manual_override, calls_per_sec, latency_ms, error_rate, sample_count,
		       COALESCE(source, 'manual'), COALESCE(state, 'active'), COALESCE(edge_type, 'sync'), COALESCE(created_by, 'system'),
		       created_at, updated_at
		FROM topology_edges
		WHERE ($1 = '' OR namespace = $1)
		  AND ($2 = '' OR cluster_id = $2)
		ORDER BY source_service_id ASC, target_service_id ASC
	`
	rows, err := p.db.QueryContext(ctx, query, namespace, clusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*TopologyEdge, 0)
	for rows.Next() {
		var e TopologyEdge
		err := rows.Scan(
			&e.ID, &e.SourceServiceID, &e.TargetServiceID, &e.Relationship, &e.Namespace, &e.ClusterID,
			&e.Confidence, &e.IsManualOverride, &e.CallsPerSec, &e.LatencyMs, &e.ErrorRate, &e.SampleCount,
			&e.Source, &e.State, &e.EdgeType, &e.CreatedBy,
			&e.CreatedAt, &e.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		list = append(list, &e)
	}
	return list, rows.Err()
}

func (p *PostgresStore) ListSuggestions(ctx context.Context, namespace, clusterID string) ([]*TopologyEdge, error) {
	query := `
		SELECT id, source_service_id, target_service_id, relationship, namespace, cluster_id,
		       confidence, is_manual_override, calls_per_sec, latency_ms, error_rate, sample_count,
		       COALESCE(source, 'suggested'), COALESCE(state, 'suggested'), COALESCE(edge_type, 'sync'), COALESCE(created_by, 'discovery'),
		       created_at, updated_at
		FROM topology_edges
		WHERE state = 'suggested'
		  AND ($1 = '' OR namespace = $1)
		  AND ($2 = '' OR cluster_id = $2)
		ORDER BY created_at DESC
	`
	rows, err := p.db.QueryContext(ctx, query, namespace, clusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*TopologyEdge, 0)
	for rows.Next() {
		var e TopologyEdge
		err := rows.Scan(
			&e.ID, &e.SourceServiceID, &e.TargetServiceID, &e.Relationship, &e.Namespace, &e.ClusterID,
			&e.Confidence, &e.IsManualOverride, &e.CallsPerSec, &e.LatencyMs, &e.ErrorRate, &e.SampleCount,
			&e.Source, &e.State, &e.EdgeType, &e.CreatedBy,
			&e.CreatedAt, &e.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		list = append(list, &e)
	}
	return list, rows.Err()
}

func (p *PostgresStore) AutoDiscoverSpanEdge(ctx context.Context, sourceID, targetID, namespace, clusterID string, latencyMs float64, isError bool) (*TopologyEdge, error) {
	if namespace == "" {
		namespace = "default"
	}
	if clusterID == "" {
		clusterID = "cluster-primary"
	}

	// Auto-provision services in topology_services if not already present
	now := time.Now().UTC()
	provQuery := `
		INSERT INTO topology_services (id, name, type, namespace, cluster_id, tier, capabilities, status, monitoring_mode, created_at, updated_at)
		VALUES ($1, $1, 'service', $2, $3, 'tier-2', 'scale,restart', 'unconnected', 'observe', $4, $4)
		ON CONFLICT (id) DO NOTHING
	`
	_, _ = p.db.ExecContext(ctx, provQuery, sourceID, namespace, clusterID, now)
	_, _ = p.db.ExecContext(ctx, provQuery, targetID, namespace, clusterID, now)

	// Fetch existing edge if any to respect manual override and compute confidence
	checkQuery := `
		SELECT id, source_service_id, target_service_id, relationship, namespace, cluster_id,
		       confidence, is_manual_override, calls_per_sec, latency_ms, error_rate, sample_count,
		       COALESCE(source, 'manual'), COALESCE(state, 'active'), COALESCE(edge_type, 'sync'), COALESCE(created_by, 'system'),
		       created_at, updated_at
		FROM topology_edges
		WHERE source_service_id = $1 AND target_service_id = $2 AND namespace = $3 AND cluster_id = $4
	`
	var existing TopologyEdge
	err := p.db.QueryRowContext(ctx, checkQuery, sourceID, targetID, namespace, clusterID).Scan(
		&existing.ID, &existing.SourceServiceID, &existing.TargetServiceID, &existing.Relationship,
		&existing.Namespace, &existing.ClusterID, &existing.Confidence, &existing.IsManualOverride,
		&existing.CallsPerSec, &existing.LatencyMs, &existing.ErrorRate, &existing.SampleCount,
		&existing.Source, &existing.State, &existing.EdgeType, &existing.CreatedBy,
		&existing.CreatedAt, &existing.UpdatedAt,
	)

	errVal := 0.0
	if isError {
		errVal = 1.0
	}

	if err == nil {
		// Principle: Rejected suggestions must not reappear unless the user resets them!
		if existing.State == "rejected" {
			return &existing, nil
		}

		// Existing edge: update statistics
		existing.SampleCount++
		alpha := 0.2
		existing.LatencyMs = (1-alpha)*existing.LatencyMs + alpha*latencyMs
		existing.ErrorRate = (1-alpha)*existing.ErrorRate + alpha*errVal
		existing.UpdatedAt = now

		if !existing.IsManualOverride && existing.Source != "manual" {
			existing.Confidence = math.Min(0.99, 0.70+float64(existing.SampleCount)*0.02)
		}

		updateQuery := `
			UPDATE topology_edges
			SET sample_count = $1, latency_ms = $2, error_rate = $3, confidence = $4, updated_at = $5
			WHERE id = $6
		`
		_, _ = p.db.ExecContext(ctx, updateQuery, existing.SampleCount, existing.LatencyMs, existing.ErrorRate, existing.Confidence, existing.UpdatedAt, existing.ID)
		return &existing, nil
	}

	// Insert new discovered edge as suggested
	newEdge := TopologyEdge{
		ID:               fmt.Sprintf("edge-%s-%s", sourceID, targetID),
		SourceServiceID:  sourceID,
		TargetServiceID:  targetID,
		Relationship:     "calls",
		Namespace:        namespace,
		ClusterID:        clusterID,
		Confidence:       0.75,
		IsManualOverride: false,
		CallsPerSec:      1.0,
		LatencyMs:        latencyMs,
		ErrorRate:        errVal,
		SampleCount:      1,
		Source:           "suggested",
		State:            "suggested",
		EdgeType:         "sync",
		CreatedBy:        "discovery",
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	insertQuery := `
		INSERT INTO topology_edges (
			id, source_service_id, target_service_id, relationship, namespace, cluster_id,
			confidence, is_manual_override, calls_per_sec, latency_ms, error_rate, sample_count,
			source, state, edge_type, created_by,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (source_service_id, target_service_id, namespace, cluster_id) DO UPDATE SET
			sample_count = topology_edges.sample_count + 1,
			latency_ms = EXCLUDED.latency_ms,
			error_rate = EXCLUDED.error_rate,
			updated_at = EXCLUDED.updated_at
	`
	_, insertErr := p.db.ExecContext(ctx, insertQuery,
		newEdge.ID, newEdge.SourceServiceID, newEdge.TargetServiceID, newEdge.Relationship,
		newEdge.Namespace, newEdge.ClusterID, newEdge.Confidence, newEdge.IsManualOverride,
		newEdge.CallsPerSec, newEdge.LatencyMs, newEdge.ErrorRate, newEdge.SampleCount,
		newEdge.Source, newEdge.State, newEdge.EdgeType, newEdge.CreatedBy,
		newEdge.CreatedAt, newEdge.UpdatedAt,
	)
	if insertErr != nil {
		return nil, insertErr
	}
	return &newEdge, nil
}

func (p *PostgresStore) CreateTopologyVersion(ctx context.Context, version *TopologyVersion) error {
	query := `
		INSERT INTO topology_versions (id, version, description, snapshot, created_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	now := time.Now().UTC()
	if version.CreatedAt.IsZero() {
		version.CreatedAt = now
	}
	snapBytes, err := json.Marshal(version.Snapshot)
	if err != nil {
		return err
	}
	_, err = p.db.ExecContext(ctx, query,
		version.ID, version.Version, version.Description, snapBytes, version.CreatedBy, version.CreatedAt,
	)
	return err
}

func (p *PostgresStore) GetLatestTopologyVersion(ctx context.Context) (*TopologyVersion, error) {
	query := `
		SELECT id, version, description, snapshot, created_by, created_at
		FROM topology_versions
		ORDER BY version DESC LIMIT 1
	`
	var ver TopologyVersion
	var snapBytes []byte
	err := p.db.QueryRowContext(ctx, query).Scan(
		&ver.ID, &ver.Version, &ver.Description, &snapBytes, &ver.CreatedBy, &ver.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(snapBytes, &ver.Snapshot)
	return &ver, nil
}

func (p *PostgresStore) ListTopologyVersions(ctx context.Context, limit int) ([]*TopologyVersion, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `
		SELECT id, version, description, snapshot, created_by, created_at
		FROM topology_versions
		ORDER BY version DESC LIMIT $1
	`
	rows, err := p.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]*TopologyVersion, 0)
	for rows.Next() {
		var ver TopologyVersion
		var snapBytes []byte
		err := rows.Scan(
			&ver.ID, &ver.Version, &ver.Description, &snapBytes, &ver.CreatedBy, &ver.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		_ = json.Unmarshal(snapBytes, &ver.Snapshot)
		list = append(list, &ver)
	}
	return list, rows.Err()
}

func (p *PostgresStore) Close() error {
	return p.db.Close()
}
