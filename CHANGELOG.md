# Changelog

All notable changes to the Aegis platform will be documented in this file.

## [Unreleased]

### Added
- **Lean CPU Development Stack**:
  - Split Python serving and training requirements, pinned a CPU-only PyTorch wheel, and removed CUDA packages, training libraries, and `grpcio-tools` from the serving runtime image.
  - Added cached multi-stage Python, Go, and frontend images, non-root runtimes, Compose `core`/`full` profiles, resource limits, model volume mounts, and explicit `make up-lean`/`make up-full` workflows.
- **Service & Dependency Builder Platform (Phases 1-3)**:
  - **Data Model & Schema Migration (`004_service_dependency_builder.sql`)**: Extended `topology_services` with `monitoring_mode` (`ignore`, `observe`, `recommend`, `auto_remediate`), `telemetry_source` (Prometheus, Kubernetes, Agent Push), `recovery_binding` (Kubernetes, Docker), `status`, `owner`, `tier`, and `tags`. Extended `topology_edges` with `source` (`manual`, `suggested`, `discovered`), `state` (`active`, `suggested`, `rejected`), `edge_type` (`sync`, `async`, `db`, `cache`), `confidence`, and `created_by`.
  - **Cryptographic Protection & SSRF Shield (`crypto.go`, `ssrf.go`)**: Encrypted sensitive telemetry tokens and passwords at rest with AES-256-GCM. Implemented SSRF blocklist prohibiting private CIDRs, link-local, and cloud metadata (`169.254.169.254`), with strict timeouts and size caps. Redacted all credentials from API responses, export manifests, and audit logs.
  - **Graph Health & Tarjan SCC Validation (`validate.go`)**: Added `POST /api/registry/validate` endpoint analyzing graphs for duplicate IDs, self-loops, orphan nodes, cycles with SCC component diagnostics, and unmonitored dependencies.
  - **Trace Auto-Discovery Lifecycle & Immutability**: Auto-discovered traces emit `suggested` edges with confidence scores (`/api/registry/suggestions`). Supported operator accept/reject actions; rejected suggestions stick permanently and manual edges are strictly immutable against discovery overwrites.
  - **Bulk Import / Export with Dry-Run Diff Preview**: Added `dryRun=true` preview returning structured additions, modifications, and deletions diff before committing changes. Added YAML, JSON, and CSV exports with credentials redacted.
  - **Interactive Graph Workbench & Add Service Wizard (React + `@xyflow/react`)**: Implemented 5-step Add/Edit Service wizard with mandatory real-time connection test and copyable agent-push snippets. Built interactive graph builder with drag-and-drop connections, edge type selectors, auto-layout, undo/redo, neighborhood focus for 200+ services, and click-to-focus validation drawer.
  - **Live Registry System Wiring & Action Gates**: Prediction and recovery engines evaluate live registry state. Recovery execution strictly gates against observe-only modes and unconfigured bindings, verifying requested actions against `allowed_actions`. Added topology version tracking across predictions and audit trails.

### Fixed
- **Recovery Fault Isolation & Genuine Telemetry Verification (Issue 1)**:
  - Removed auto-clearing of `State.ActiveFaults` and forwarding of stop commands to Chaos Engine during remediation in both manual and autonomous recovery paths.
  - Active chaos faults terminate exclusively through duration expiration or explicit admin stop via chaos API.
  - Telemetry verification strictly checks live service health; if an ongoing fault leaves metrics degraded/critical, recovery status transitions to `Ineffective` and executes the escalation ladder.
  - Populated finalized `DurationMs` and `DurationStr` from the real verification window instead of initial zero placeholders.
  - Added comprehensive tests for active fault retention under `SCALE_UP`, ineffective escalation under active faults, natural fault expiration verification, and finalized duration recording.
- **Sparse TGNN & Two-Stage Pipeline Serving Path Integration (Issue 2)**:
  - Loaded Sparse TGNN and `TwoStageInferencePipeline` in Python gRPC server, dynamically routing between Dense TGNN ($N \le \text{AEGIS\_SPARSE\_THRESHOLD}$, default 20) and Two-Stage Sparse TGNN ($N > 20$) on $k$-hop neighborhoods.
  - Passed dynamic registry topology (arbitrary $N$ services) and built `edge_index` from topology edges, removing fixed node count assumptions.
  - Extended Protobuf `PredictResponse` and Go control plane to emit inference `engine` (`dense_tgnn`, `sparse_two_stage`, or `heuristic_fallback`), `model_version`, and `calibrated_confidence`.
  - Updated `/readyz` probe to report active engine in component status.
  - Updated React frontend PredictionPanel to render active engine badge and calibrated confidence, surfacing degraded state during fallback.
  - Added 200-node topology import and sparse inference integration test in Go control plane.
  - Formally documented real measured benchmarks across sizes 10, 50, 100, 200, and 500 nodes, bounding arbitrary topology generalization claims to $N \le 500$.
- **Secrets Hardening & Credential Elimination (Issue 3)**:
  - Replaced hardcoded default credentials in `docker-compose.yml` (`POSTGRES_PASSWORD`, `DATABASE_URL`, `AEGIS_BOOTSTRAP_ADMIN_PASSWORD`) with required `${VAR:?error_message}` variable interpolation.
  - Provided `.env.example` containing non-secret placeholders only and verified `.env` is ignored by `.gitignore`.
  - Removed all occurrences of default/compromised credentials (`aegissecretpassword`, `AdminSecretPassword#2026`) from configurations, scripts, documentation, and tests.
  - Implemented strict password complexity validation (minimum 12 characters, requiring uppercase, lowercase, digit, and special symbol) across bootstrap configuration and user update operations.
  - Enforced `mustChangePassword` flag on bootstrap administrator account, requiring immediate password rotation upon initial login via `POST /api/auth/change-password`.
  - Refactored Helm chart (`deploy/helm/aegis-agent`) to reference existing secrets via `existingSecret` with zero default secret values in `values.yaml`.
- **Environment & Simulation Scope Clarifications (Small Cleanups)**:
  - Formally documented RL recovery agent as trained and evaluated strictly within a simulated Gym environment (`MicroserviceChaosEnv`), removing any implication of live cluster or production validation.
  - Labeled `deploy/online-boutique` artifacts as declarative manifests and execution runbooks not yet validated on a live production cluster.

## [Phase 7: Production Readiness & Enterprise Operational Hardening] - 2026-10-22

### Added
- **Sparse-TGNN & Two-Stage Anomaly Detection Pipeline (`python-ml/models/two_stage_pipeline.py`, `python-ml/models/tgnn/sparse_model.py`)**:
  - Replaced dense $O(N^2)$ GAT adjacency with $O(|V| + |E|)$ Sparse GATv2 message passing on `edge_index`.
  - Two-Stage Inference: Stage 1 evaluates a vectorized $O(N)$ anomaly detector across all microservices; Stage 2 dynamically extracts the induced 2-hop subgraph around anomalous seeds and runs Sparse-TGNN on the localized neighborhood only.
  - Multi-task heads: failure prediction, root-cause probability distribution, and per-edge propagation forecast with calibrated confidence.
- **500-Microservice End-to-End ML Scaling Benchmark (`scripts/benchmark_sparse_tgnn_500.py`)**:
  - Benchmarks the two-stage pipeline from 10 to 500 microservices (1,667 edges).
  - Measured 500-node inference latency: p50 = 20.71ms, p95 = 25.83ms, p99 = 25.83ms (Target SLA: < 50ms).
  - Peak RAM overhead at 500 services: 0.08MB (Target SLA: < 100MB).
- **Google Online Boutique Automated End-to-End Cluster Validation (`deploy/online-boutique/test_online_boutique_e2e.py`)**:
  - Automated verification of the 11-service Online Boutique topology (`topology.yaml`) and Kubernetes manifests (`online-boutique.yaml`).
  - Simulates abrupt failure on `cartservice`, verifies root cause isolation (`cartservice`), confirms rulebook action selection (`RESTART`), orders recovery actions on DAG, and validates status transition (`Pending` -> `VERIFIED`).
- **Chaos Injection Safety Guards & Staging-Only Restriction (`config.go`, `routes/chaos.go`)**:
  - Added `CHAOS_ENABLED` flag (default true in staging/dev, strictly false in production).
  - In production mode, chaos injection is blocked with HTTP 403 unless `CHAOS_ALLOW_PRODUCTION=true` is explicitly set.
- **Single Source of Truth Alignment (`rulebook.yaml`, `policy.go`, `README.md`)**:
  - Aligned fault-to-action table in `README.md` with `rulebook.yaml` (including explicit `slow_query` -> `INCREASE_POOL` and `thread_exhaustion` -> `INCREASE_POOL` / `SCALE_UP`).
- **Eliminated Hardcoded Database Credentials in Production (`config.go`)**:
  - Config validation rejects startup in production if `DATABASE_URL` or `POSTGRES_PASSWORD` is omitted or left as insecure default development credentials.
- **Centralized Environment Configuration (`go-services/api-gateway/config/config.go`)**:
  - Typed `config.Config` loading all parameters from environment variables with defaults.
  - Strict production validation rejecting wildcard `*` CORS origins and missing TLS certificates/keys.
- **Enterprise Health & Readiness Probes (`routes/health.go`, `/healthz`, `/readyz`)**:
  - Kubernetes-compatible `/healthz` liveness probe.
  - Multi-dependency `/readyz` readiness probe concurrently checking storage persistence, in-memory state engine, and downstream Python TGNN gRPC inference.
- **Structured JSON Logging & Middleware (`telemetry/logging.go`)**:
  - Standardized JSON access logs with UTC RFC3339 timestamps, trace IDs, caller identities, HTTP status codes, and execution durations in milliseconds.
- **Native Prometheus Metrics Registry (`telemetry/metrics.go`, `/metrics`)**:
  - Native Prometheus exposition format exporting `aegis_http_requests_total`, `aegis_http_request_duration_seconds`, `aegis_remediations_total`, `aegis_remediation_verifications_total`, `aegis_monitored_services_total`, and `aegis_active_dispatches`.
- **Graceful Lifecycle Management (`main.go`)**:
  - Traps `SIGINT` and `SIGTERM` signals for clean shutdown within a configurable timeout (`AEGIS_SHUTDOWN_TIMEOUT_SEC`).
  - Supports native TLS termination with `ListenAndServeTLS`.
- **CORS Allowlist Security Enforcement (`routes/router.go`)**:
  - Dynamic evaluation against configured origins list with `Vary: Origin` response header.
- **Automated CI/CD Workflow (`.github/workflows/ci.yml`)**:
  - End-to-end GitHub Actions pipeline executing Go tests, Python tests, frontend build, 500-service benchmark, and repository-wide zero-emoji validation.
- **Comprehensive Production Documentation (`docs/production-readiness.md`)**:
  - Operational specifications for deployment, monitoring, configuration, probes, and scaling targets.

## [Phase 6: Reinforcement Learning Recovery Agent & Extended Hardening] - 2026-10-22

### Added
- **Reinforcement Learning Microservice Gym Environment (`python-ml/rl_agent/env.py`)**:
  - Gym-compatible `MicroserviceChaosEnv` modeling microservice topologies, cascading degradation, and remediation dynamics.
  - Normalized 5-dimensional feature representation per node (status code, CPU, memory, latency, error rate).
  - Discrete action space mapping indices to `(Node, ActionType)`.
  - Shaped reward function incentivizing fast SLA restoration while penalizing disruptive restarts or reroutes.
- **Deep Q-Network Recovery Policy Network (`python-ml/rl_agent/policy.py`)**:
  - `RecoveryPolicyNetwork` parameterizing action-value Q-scores across discrete remediations.
  - Epsilon-greedy action selection and checkpoint serialization.
- **RL Training Loop & Experience Replay (`python-ml/rl_agent/train.py`)**:
  - Deep Q-Learning training loop with 5000-transition experience replay buffer, target network synchronization, and Smooth L1 Huber loss.
- **10 Full Chaos Injector Suite (`go-services/chaos-engine/injectors/`, `routes/chaos.go`)**:
  - Complete support for all 10 chaos injectors: `cpu_stress`, `latency`, `kill_service`, `memory_leak`, `packet_loss`, `mq_lag`, `cache_down`, `db_lock`, `slow_query`, and `thread_exhaustion`.
- **Authority Boundary Human-in-the-Loop Approval Gate (`go-services/recovery-engine/approval_gate.go`)**:
  - Dual-mode deterministic gating: auto-approves low-risk mitigations while holding high-risk actions pending operator authorization.
- **Architecture Documentation (`docs/rl-agent-architecture.md`)**:
  - Full technical specification of the RL agent, state representations, reward shaping, and chaos fault mechanics.

## [Phase 5: Dashboard + Demo Polish] - 2026-10-22

### Added
- **Unified Mission Control Dashboard (`pages/Dashboard/Dashboard.tsx`)**:
  - Unifies real-time topological visualization, TGNN anomaly prediction, autonomous self-healing, and chaos controls into a single presentation-ready screen.
  - Interactive **1-Click Demo Scenarios Toolbar**:
    - `CPU Cascade`: CPU core saturation on `node-c` triggering autonomous `SCALE_UP`.
    - `Latency Gate`: 950ms latency deadlock on `node-b` triggering high-risk human approval gate.
    - `Memory SCC`: Memory leak on `node-d` demonstrating bottom-up SCC DAG recovery ordering.
    - `Reset`: Instant cluster fault halt and baseline recovery.
  - Live metric status cards including Autonomous Recovery Engine mode (`ACTIVE` / `PAUSED`).
- **Modular Topology Graph Component (`components/TopologyGraph/`)**:
  - Live `@xyflow/react` hierarchical graph visualization auto-deriving level depth from service dependencies.
  - Animated edge telemetry pulses with color transitions (green $\to$ amber $\to$ crimson $\to$ purple).
  - Pulsing root-cause halos and cascade blast-radius badges with hop distances.
  - Integrated topology selector (`topology.yaml`, `topology.example.yaml`), minimap, and zoom/fit controls.
- **Chaos Injection Controls Component (`components/ChaosControls/`)**:
  - Catalog of 10 generic fault injectors with target service selection and duration presets.
  - Quick 1-click preset triggers and active fault monitors with live countdowns and emergency halt.
- **Predictive Analytics & Root Cause Panel (`components/PredictionPanel/`)**:
  - Live failure probability gauge, classification confidence score, and time-to-failure horizon countdown.
  - Root-cause entity card with breakdown of contributing telemetry anomalies (CPU, Memory, Latency, Error Rate).
  - Cascade propagation sequence breadcrumbs.
  - Direct 1-click remediation action dispatch with verification state feedback.
- **Remediation & Recovery Log Component (`components/RecoveryLog/`)**:
  - Autonomous self-healing indicator pill and pause/resume toggle switch.
  - In-flight 10-second verification countdown progress bar.
  - Chronological audit cards detailing risk deltas ($92\% \to 12\%$), elapsed verification duration, and approval status (`AUTONOMOUS` vs operator).
  - 1-click Rollback trigger for reversible mitigations.
- **Autonomous Recovery Store & API Client (`store/useTelemetryStore.ts`, `api/recovery.ts`)**:
  - Wired `getAutonomousStatus` and `setAutonomousStatus` to the control plane.
  - Real-time synchronization of recovery history and autonomous state on 1-second telemetry ticks.
- **Comprehensive Presentation Runbook (`docs/demo-script.md`)**:
  - Updated presentation walkthrough detailing the end-to-end demo flow, credentials, architecture ports, and scenarios.

## [Phase 4: Decision Engine & DAG Recovery Planning (Rule-Based + Closed-Loop)] - 2026-10-22

### Added
- **Declarative Rulebook (`python-ml/decision_engine/rulebook.yaml`)**:
  - Declarative conditions mapping anomaly signals (memory, CPU, latency, error rate, queue depth, cache miss, DB wait) to primary remediation actions and secondary escalations.
  - Base severities, blast-radius weightings, reversibility flags, and human-approval gates for 6 fault families.
- **Decision Engine & Risk Assessment (`python-ml/decision_engine/risk_assessment.py`)**:
  - Evaluates live telemetry against rulebook conditions to produce deterministic risk assessments.
  - Dynamic blast radius and node criticality tier penalties.
- **SCC-Condensed DAG Recovery Planner (`python-ml/recovery_planner/dependency_order.py`)**:
  - Strongly Connected Components (SCC) condensation (Tarjan's algorithm) handling cyclic dependencies without infinite loops or crashes.
  - Bottom-up topological ordering ensuring dependencies (callees) are prioritized for remediation before callers (dependents).
- **Protobuf-Compliant Plan Builder (`python-ml/recovery_planner/plan_builder.py`)**:
  - Generates structured `RecoveryPlan` objects adhering to `proto/recovery.proto`.
  - Supports dry-run generation, risk explanation metadata, and rollback hints.
- **Python ML gRPC Serving Layer (`python-ml/serving/grpc_server.py`)**:
  - Integrated `RecoveryServiceServicer` implementing `GeneratePlan` and `ExecutePlan` RPCs.
- **Closed-Loop Autonomous Self-Healing Engine (`go-services/api-gateway/routes/state.go`, `recovery.go`)**:
  - Automated background monitoring cycle in `State.runTicker()`.
  - Auto-evaluates degraded services and initiates `ExecuteAutonomousRecovery` when no other dispatches are active.
  - Strict human approval gating: Actions with risk $\ge 0.40$ or high-risk actions on Tier-1 nodes hold at `PENDING_APPROVAL` for operator authorization.
  - Auto-halts active chaos faults on remediation, initiates real asynchronous verification windows, and logs audit events.
- **Autonomous Control & Status Endpoints (`routes/recovery.go`, `routes/router.go`)**:
  - `GET /api/recovery/autonomous`: Returns active autonomous recovery status, safety dispatch counts, and cooldowns.
  - `POST /api/recovery/autonomous`: Enables or disables autonomous recovery mode (requires Operator/Admin role).
- **Phase 4 Test Suites**:
  - `tests/python-ml/test_phase4_recovery.py`: 9 comprehensive tests for rule matching, risk assessment, cyclic/acyclic SCC condensation, and plan building (all 65 Python tests pass).
  - `go-services/api-gateway/routes/phase4_autonomous_recovery_test.go`: End-to-end tests for autonomous self-healing loop, approval gate holding for high-risk plans, and autonomous control toggling.
- **Architecture Documentation (`docs/decision-engine-recovery-planning.md`)**: Full documentation of Phase 4 components, SCC condensation, and closed-loop self-healing workflows.

## [Phase 3: Real Telemetry + Dynamic Topology (Any N Services)] - 2026-10-22

### Added
- **Dynamic PostgreSQL Service Registry (`migrations/003_phase3_dynamic_topology.sql`)**:
  - `topology_services`: Stores any number of services across clusters, namespaces, tiers (`tier-0` to `tier-3`), capabilities (`scale,restart,reroute,flush_cache`), and ownership.
  - `topology_edges`: Stores directed dependencies with auto-discovery confidence metrics and manual override protection.
  - `topology_versions`: Complete audit snapshots of topology revisions with rollback and diff capabilities.
- **Service Registry & Bulk Import REST API (`routes/registry.go`)**:
  - `GET /api/registry/services`, `POST /api/registry/services`, `PUT /api/registry/services/{id}`, `DELETE /api/registry/services/{id}`.
  - `GET /api/registry/edges`, `POST /api/registry/edges`, `DELETE /api/registry/edges/{source}/{target}`.
  - `POST /api/registry/import`: Ingests and snapshots dynamic topologies of 20+ to 200+ microservices in a single call.
  - `GET /api/registry/versions`: Lists historical topology snapshots.
- **OpenTelemetry Trace Span Ingestion & Auto-Discovery (`routes/ingestion.go`)**:
  - `POST /api/telemetry/spans`: Ingests distributed trace spans to auto-discover topology edges.
  - Asymptotic confidence growth: Starts at $0.75$ and scales toward $0.99$ as sample counts increment.
  - **Manual Override Protection**: If an operator creates or marks an edge with `isManualOverride = true`, auto-discovery will never degrade confidence below $1.0$ or overwrite the relationship type.
- **Prometheus & Agent Metric Ingestion (`POST /api/telemetry/metrics`)**:
  - Ingests real metric points directly from Prometheus or outbound telemetry agents.
- **Outbound-Only Telemetry Agent Packaging (`go-services/telemetry-agent`)**:
  - Multi-stage unprivileged Docker container (`USER 10001:10001`, minimal Alpine base).
  - Production Helm chart (`deploy/helm/aegis-agent`) with ConfigMaps, Secrets, resource quotas, and hardened PodSecurityContext (`readOnlyRootFilesystem: true`, `drop: [ALL]`).
  - Machine API Key Authentication: Authenticated via `X-API-Key: aegis_ak_*`.
  - Kafka Partition Key Isolation: Strictly formatted as `cluster_id + "/" + service_id` for multi-tenant isolation.
- **`DEMO_MODE` Gating vs Real Mode (`routes/state.go`)**:
  - Elimination of `math/rand` simulation from the real monitoring path. `DEMO_MODE` defaults strictly to `false`.
  - Random metric jitter is strictly gated behind `if s.DemoMode`.
  - Toggle API: `GET /api/system/mode` and `POST /api/system/mode`.
- **Frontend Mode Pill & Registry Client (`frontend/src/api/registry.ts`, `Navbar.tsx`)**:
  - Visual indicator pill in Navbar displaying `LIVE CLUSTER (cluster-primary)` (emerald) vs `DEMO SIMULATION` (amber).
  - Instant toggle switch for operators and administrators.
- **Phase 3 Test Suites**:
  - `routes/phase3_dynamic_topology_test.go`: Tests service registry CRUD, manual edge override protection, trace span auto-discovery, bulk topology import (25 nodes), system mode toggle, and machine API key ingestion.
  - `storage/storage_test.go`: Added `TestMemoryStore_Phase3_TopologyAndAutoDiscovery`.
- **Architecture Documentation (`docs/dynamic-topology-telemetry.md`)**: Full documentation of Phase 3 components and workflows.

## [Phase 2: Risk Calculation, Approval Policy, Verification Window & Safety Limits] - 2026-10-22

### Added
- **Canonical Policy Engine (`routes/policy.go`)**: Evaluates $Risk = f(\text{action type}, \text{target criticality/tier}, \text{blast radius})$ with deterministic score in $[0.0, 1.0]$ and levels (`LOW`, `MEDIUM`, `HIGH`).
- **Policy Check Endpoint (`POST /api/recovery/policy-check`)**: Unifies manual UI triggers and autonomous actions under the exact same authoritative backend policy evaluation.
- **BFS Blast Radius Computation (`ComputeBlastRadius`)**: Traverses reachable downstream dependent services on live topology graph.
- **Safety Limits Manager (`SafetyLimitsManager`)**:
  - **Service Cooldown**: Enforces 30s pause between remediations on the same service (`HTTP 429`).
  - **Max Concurrent Limit**: Caps active in-flight verification dispatches at 2 (`HTTP 429`).
  - **Global Blast Radius Budget**: Cluster-wide cap of 5 affected nodes across active dispatches (`HTTP 429`).
- **Idempotency Key Enforcement**: Dispatches support `Idempotency-Key` header; replays return previously recorded execution instantly with `idempotentReplay: true`.
- **Dry-Run Mode**: Supports `dryRun: true` in payload or `X-Dry-Run: true` header to preview policy evaluation without side-effects or cooldown.
- **Real Asynchronous Verification Window**:
  - Replaced fake `1.2s` static timer with a real 10s telemetry verification window.
  - State machine transitions: `Verifying` $\to$ `Verified` $\mid$ `Ineffective` $\mid$ `RolledBack`.
  - Captures real pre- and post-remediation metric snapshots and elapsed duration.
- **Fault Escalation Ladder**: When verification evaluates to `Ineffective`, automatically escalates to secondary remediation (e.g. `SCALE_UP` $\to$ `RESTART`) from `FaultFamilyEscalations` and creates an escalation alert.
- **Rollback API (`POST /api/recovery/rollback/{id}`)**: Reverts reversible actions (`SCALE_UP`, `REROUTE_TRAFFIC`, `INCREASE_POOL`), resets parameters, marks record as `RolledBack`, and logs audit trail.
- **Persistent Recovery Execution Storage (`migrations/002_phase2_recovery.sql`)**: PostgreSQL storage for `recovery_executions` with auto-migration.
- **Comprehensive Unit & Integration Test Suite (`routes/phase2_recovery_test.go`)**: 8 comprehensive tests covering risk formula, policy-check endpoint, cooldown, concurrency, blast budget, idempotency replay, dry-run, verification/escalation, and rollback.

### Changed
- **Removed Hardcoded Fallback Probabilities**: Replaced artificial fallback numbers (`93.5`, `68`, `75`) in `recovery.go` with live pre-remediation prediction and policy calculation.
- **Frontend Remediation Controls**: Updated `ServiceDetailModal.tsx` and `Recovery.tsx` to query `/api/recovery/policy-check` before dispatching, handle cooldown warnings, render real verification badges, and support one-click rollback.

## [Phase 1: Production Security Hardening] - 2026-10-21

### Added
- **Global Auth & RBAC Middleware (`routes/middleware.go`)**: Applied across all `/api/*` routes with explicit public whitelist (`/health`, `/api/auth/login`, `/api/auth/signup`).
- **Hierarchical RBAC (`storage/models.go`)**: Enforced server-side roles: `viewer` (read-only), `operator` (approvals, low/med recovery, topology selection), and `admin` (chaos injection/stop, high-risk recovery execution, user management, API keys).
- **Argon2id Password Hashing (`storage/argon2.go`)**: RFC 9106 compliant memory-hard password hashing with per-user CSPRNG salt and constant-time verification.
- **PostgreSQL Storage Layer (`storage/postgres.go`)**: Durable persistence for users, sessions, machine API keys, and audit logs with automatic idempotent schema migration.
- **In-Memory Storage (`storage/memory.go`)**: Clean abstraction implementing `storage.Store` for rapid offline testing and fallback.
- **Machine API Keys**: Rotatable, scoped, hashed machine credentials (`aegis_ak_*`) for telemetry agents and automated workflows.
- **Append-Only Audit Log (`audit_logs`)**: Permanent audit trail tracking actor identity, role, action, target, risk, approval status, payload, result, and IP address.
- **Brute-Force Protection**: 5 consecutive failed login attempts trigger an automatic 15-minute account lockout (`HTTP 429`).
- **Comprehensive Security Test Suite (`routes/security_matrix_test.go`)**: Verifies 401 Unauthorized on unauthenticated requests and 403 Forbidden on under-privileged calls across all routes.
- **Database Migration (`migrations/001_phase1_security.sql`)**: PostgreSQL DDL schema with indexes.

### Changed
- **Registration Security**: Removed client-chosen role vulnerability. Registration now assigns `admin` to the bootstrap user and strictly defaults all subsequent users to `viewer`.
- **High-Risk Recovery Execution**: Restricted high-risk remediation actions to `admin` role.
- **Docker Compose Integration**: Configured PostgreSQL persistence and bootstrap administrator credentials in `docker-compose.yml`.
