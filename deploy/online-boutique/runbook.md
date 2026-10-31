# Aegis Runbook: End-to-End Validation on Google Online Boutique (Kubernetes)

> [!NOTE]
> **Validation Status:** This directory contains manifests and a runbook; it is **not yet validated on a live cluster** until a real run is recorded.

This runbook guides operators through validating the complete Aegis self-healing pipeline on a real local Kubernetes cluster (`kind` or `minikube`) running Google's 11-service Online Boutique.

---

## 1. Prerequisites

- Kubernetes CLI (`kubectl`)
- Local Kubernetes runtime (`kind` or `minikube`)
- Helm v3
- Go 1.21+ (for Aegis control plane)

Verify cluster availability:
```bash
kubectl cluster-info
```

---

## 2. Deploy Google Online Boutique

Deploy the target microservices into the `online-boutique` namespace:

```bash
kubectl apply -f deploy/online-boutique/online-boutique.yaml
```

Wait until all pods are running and ready:
```bash
kubectl wait --namespace online-boutique --for=condition=ready pod --all --timeout=180s
```

Verify service listing:
```bash
kubectl get pods,svc -n online-boutique
```

---

## 3. Register Topology in Aegis

Import the Online Boutique topology into Aegis:

```bash
# Obtain Admin JWT token
export TOKEN=$(curl -s -X POST http://localhost:8000/api/auth/login \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${AEGIS_BOOTSTRAP_ADMIN_EMAIL:-admin@enterprise.io}\",\"password\":\"${AEGIS_BOOTSTRAP_ADMIN_PASSWORD}\"}" | jq -r .token)

# Import the 11-node Online Boutique topology
curl -s -X POST http://localhost:8000/api/registry/import \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d @deploy/online-boutique/topology.yaml
```

Verify registered services:
```bash
curl -s -X GET http://localhost:8000/api/registry/services \
  -H "Authorization: Bearer $TOKEN" | jq .
```

---

## 4. Install Aegis Telemetry Agent via Helm

Install the outbound Aegis agent into the cluster:

```bash
helm install aegis-agent deploy/helm/aegis-agent \
  --namespace aegis-system \
  --create-namespace \
  --set aegis.gatewayUrl="http://host.minikube.internal:8000" \
  --set aegis.apiKey="aegis_sec_live_key_001" \
  --set cluster.name="kind-online-boutique"
```

Verify agent daemonset:
```bash
kubectl get pods -n aegis-system
```

---

## 5. Fault Injection Scenarios

### Scenario A: Cart Service Outage (Kill Service)

Simulate a crash loop on `cartservice`:

```bash
# Inject fault: Scale cartservice to 0 or crash pod
kubectl scale deployment/cartservice -n online-boutique --replicas=0
```

#### Observation in Aegis:
1. Telemetry Agent detects 100% error rate and connection refusal on `cartservice`.
2. The baseline detection and evidence-based RCA pipeline evaluates:
   - Root Cause: `cartservice` (Confidence >= 0.94)
   - Blast Radius: `frontend`, `checkoutservice`
   - Failure Probability: 0.96
3. Decision Engine selects primary action `restart` for `cartservice`.
4. Recovery Engine dispatches orchestrated restart through `k8s_adapter.go`.
5. Post-Remediation Verification polls healthz endpoint for 15s window, updating status to `VERIFIED`.

### Scenario B: Database Latency Injection (Redis Cache Starvation)

Inject synthetic network latency into `redis-cart`:

```bash
# Inject 500ms packet delay inside redis-cart
kubectl exec -n online-boutique deployment/redis-cart -- tc qdisc add dev eth0 root netem delay 500ms
```

#### Observation in Aegis:
1. `checkoutservice` and `cartservice` experience latency spikes above 650ms.
2. Aegis Rulebook identifies `redis-cart` degraded status and triggers `flush_cache` and `scale`.
3. Operator approval is enforced if risk evaluation exceeds medium threshold.

---

## 6. Cleanup

```bash
helm uninstall aegis-agent -n aegis-system
kubectl delete namespace aegis-system
kubectl delete -f deploy/online-boutique/online-boutique.yaml
```
