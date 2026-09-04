# Aegis Live Demo Script

## 1. Environment Setup
```bash
# Start cluster
docker-compose up --build -d

# Verify all services UP
curl http://localhost:8080/health  # Gateway
curl http://localhost:8081/health  # Node A
curl http://localhost:8082/health  # Node B
curl http://localhost:8083/health  # Node C
curl http://localhost:8084/health  # Node D
```

## 2. Generate Traffic
```bash
# Route request through gateway
curl -X POST http://localhost:8080/route/node-a
```

## 3. Inject Chaos Fault
```bash
# Inject latency on downstream node-b
bash scripts/run_episode.sh latency node-b 15
```

## 4. Observe Recovery
- Prediction head flags risk on `node-b` propagating upstream to `node-a`.
- Decision engine schedules recovery restart.
- Mesh self-heals back to nominal health.
