# Dynamic topology and telemetry

Aegis discovers service identities from OTLP resource attributes and can infer caller-to-dependency observations from trace spans. Explicit topology configuration remains available when telemetry cannot show a relationship.

Edges retain whether they were configured or observed, when they were last seen, and their confidence/freshness where available. A single trace observation is not a permanent dependency. Stale services and edges should age out according to configured TTLs.

The edge convention is `caller -> dependency`. Use outgoing traversal to inspect what a service depends on and incoming traversal to identify callers that may be affected when that dependency fails. Describe BFS output as potential impact. Cycles are surfaced with Tarjan SCC and handled through SCC condensation during recovery ordering.

OTLP intake does not require a demo topology or demo service names. See [connect.md](connect.md) for onboarding.
