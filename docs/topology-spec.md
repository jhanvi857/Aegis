# Aegis Topology Specification (`topology.yaml`)

## Philosophy
To ensure **zero industry/domain lock-in**, Aegis discovers and models any distributed system strictly through a generic graph specification.

## Schema Reference

```yaml
version: "1.0"
system_name: "string (e.g. aegis-playground)"
description: "string"

nodes:
  - id: "string (unique node identifier)"
    name: "string (human readable label)"
    type: "string (e.g. gateway, service, worker, database, cache)"
    host: "string (network resolvable hostname/ip)"
    port: integer
    health_endpoint: "string (e.g. /health)"
    metrics_endpoint: "string (e.g. /metrics)"
    dependencies:
      - "string (list of target node IDs this node sends traffic to)"
    metadata:
      key: "value (optional operational labels, e.g. tier, role)"
```

## Guarantees
1. **Dynamic Reconfiguration**: Pointing Aegis to a new `topology.yaml` allows instant monitoring and prediction for completely different microservice topologies without altering core pipeline code.
2. **Cycle Tolerance**: Cycles specified in `dependencies` are supported via SCC condensation.
