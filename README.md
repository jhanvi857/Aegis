# Aegis

Aegis is a self-hostable control plane for distributed services. It accepts service
telemetry, builds a dependency graph, requests failure/root-cause/propagation
predictions from a Python inference service, and presents recovery recommendations
for operator approval.

The system is industry-agnostic: service identifiers and dependencies come from
`topology.yaml` or the registry API. Domain-specific services must not be embedded
in Aegis core logic.

## Current architecture

```text
Telemetry -> graph representation -> Python TGNN -> prediction
                                      |
                                      v
                         recommendation + approval gate
```

- **Go API gateway**: authentication, topology, telemetry, prediction, chaos, and
  recovery APIs. It uses PostgreSQL when configured and otherwise uses an in-memory
  store.
- **Python serving process**: CPU PyTorch gRPC inference server.
- **React frontend**: dashboard and operator controls.
- **Protobuf/gRPC**: typed Go/Python prediction and recovery contracts.
- **Kafka and OpenTelemetry ingestion**: available in the broader deployment
  configuration; not required for the single-container demo.

## Hugging Face Space deployment

The root [Dockerfile](./Dockerfile) packages nginx, the Go API gateway, and the
Python inference server in one container listening on port `7860`.

Create these Hugging Face Space secrets/configuration values:

- `AEGIS_BOOTSTRAP_ADMIN_PASSWORD`: required secret. Use a password meeting the
  gateway complexity rules. Never commit it.
- `AEGIS_BOOTSTRAP_ADMIN_EMAIL`: optional; defaults to `admin@aegis.local`.

The GitHub workflow at
[.github/workflows/deploy-hf.yml](./.github/workflows/deploy-hf.yml) expects
repository secrets `HF_TOKEN` and `HF_SPACE_ID`. The Space itself must receive
`AEGIS_BOOTSTRAP_ADMIN_PASSWORD` as a Space secret. The workflow does not copy
that password from GitHub.

The packaged demo sets `DEMO_MODE=true`, uses the in-memory store, serves CPU
inference, and leaves autonomous recovery disabled. Set
`AEGIS_AUTONOMOUS_RECOVERY=true` only in an explicitly controlled deployment.

## Local quickstart

### Frontend and gateway development

```powershell
Set-Location frontend
npm ci
npm run dev
```

```powershell
Set-Location go-services
$env:DEMO_MODE = "true"
$env:AEGIS_BOOTSTRAP_ADMIN_PASSWORD = "<strong-local-secret>"
$env:AEGIS_AUTONOMOUS_RECOVERY = "false"
go run ./api-gateway
```

The frontend uses `VITE_API_BASE_URL` when supplied and otherwise calls the
relative `/api` path. This makes the nginx-proxied Space deployment work without
source changes. For a separate local frontend, set
`VITE_API_BASE_URL=http://localhost:8000/api`.

### Compose deployment

The existing [docker-compose.yml](./docker-compose.yml) is the multi-service
deployment for Kafka, PostgreSQL, Redis, playground services, and the control
plane. Provide the required secrets in a local environment file and review the
profiles before starting it.

## Validation run for this Phase 1 change

Commands run in this workspace:

- `Set-Location frontend; npm run build` — passed.
- `Set-Location frontend; npm test` — passed; 6 tests passed.
- `Set-Location go-services; go test ./...` — passed.
- `Set-Location go-services; go build ./api-gateway` — passed.
- `python -m compileall -q python-ml` — passed.
- `git diff --check` — passed.
- `docker build --progress=plain -t aegis:hf .` — passed.
- Disposable container on port `7860` — passed.
- Dashboard shell and proxied `/health` — passed.
- Bootstrap admin login through `/api/auth/login` — passed.
- Authenticated `/api/predict` — passed; engine reported `dense_tgnn`.
- Demo latency injection through `/api/chaos` — passed for `node-b`.
- Authenticated prediction after chaos injection — passed.

The container was verified locally only. No Hugging Face Space live URL is
claimed until a deployment is actually published and exercised.

## Honest limitations

- The single-container demo is intentionally in-memory; data is lost on restart.
- `DEMO_MODE` creates demo topology state for local/UI evaluation. It is not a
  substitute for real telemetry validation.
- A prediction response is not evidence of production accuracy. Real-data
  validation, strict pre-injection windowing, episode-level splits, and baseline
  comparison are Phase 2 work.
- Recovery defaults to recommendation and human approval. Autonomous execution is
  an explicit opt-in configuration.
- TLS, durable storage, secret rotation, rate limits, and external identity
  integration require deployment-specific configuration.

## Repository layout

- [proto/](./proto): cross-language contracts.
- [go-services/](./go-services): gateway, telemetry, chaos, and recovery services.
- [python-ml/](./python-ml): graph processing, models, tasks, and gRPC serving.
- [frontend/](./frontend): React operator console.
- [configs/](./configs): example topology and model/rule configuration.
- [docs/](./docs): architecture and operational documentation.
