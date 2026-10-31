# Service onboarding

Use the Connect page or [connect.md](connect.md) to export OpenTelemetry signals from an application. A stable `service.name` discovers the service automatically. Use trace instrumentation to provide caller/dependency evidence and explicit topology configuration when traces cannot establish a relationship.

A service record should show source, status, last-seen time, available signals, and freshness. Missing measurements remain missing. Recovery bindings are optional and must match capabilities actually supported by the connected runtime.
