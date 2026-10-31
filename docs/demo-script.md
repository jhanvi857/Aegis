# Integration walkthrough

1. Start the core Aegis services with `docker compose --profile core up --build` after configuring `.env`.
2. Open the Connect page and follow [connect.md](connect.md) to export OTLP from an application using a stable `service.name`.
3. Confirm the service appears after telemetry arrives. Check last-seen time, signal availability, and integration status.
4. Generate a controlled test request or use an optional fixture. Observe discovered trace edges and review their provenance.
5. When a configured signal crosses a threshold, inspect the anomaly evidence and the RCA candidate uncertainty. Potential blast radius is a graph reachability result, not a claim that each reachable service failed.
6. Review any recovery recommendation, policy result, approval requirement, execution status, and post-action health verification.

Do not present empty, disconnected, or stale telemetry states as healthy. Do not present a recommendation as an executed or verified action.
