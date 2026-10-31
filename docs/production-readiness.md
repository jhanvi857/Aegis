# Operational readiness

This document describes the current control-plane boundaries; it is not a certification that a deployment is production ready.

- Use `.env.example` to configure database credentials, auth secrets, ingestion token, and service endpoints. Do not commit secrets.
- OTLP HTTP intake uses the configured gateway listener. Set an ingestion token for machine exporters in shared or untrusted networks.
- Service identity comes from `service.name`; use stable names and optional namespace/environment resource attributes.
- Configure Prometheus pull targets only for endpoints Aegis is authorized to scrape. Treat failed scrape as unknown/offline according to the returned status, not as zero-valued telemetry.
- API reads require an authenticated role. Recovery execution requires configured capabilities, policy validation, and the required approval.
- The Python gRPC process performs service-level statistical detection and graph analysis. It does not execute actions.
- Recovery success must be confirmed by post-action health observations. A successful command response alone is not verification.

Before production use, validate authentication, TLS/network boundaries, database durability, backups, rate limits, ingestion backpressure, retention, audit integrity, orchestrator permissions, approval policy, and recovery verification in the target environment. These checks have not been universally verified across deployments.
