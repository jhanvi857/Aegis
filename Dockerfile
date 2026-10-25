FROM node:20-alpine AS frontend-builder

WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26-bookworm AS gateway-builder

WORKDIR /src
COPY go-services/go.mod go-services/go.sum ./go-services/
RUN cd go-services && go mod download
COPY go-services/ ./go-services/
RUN cd go-services && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api-gateway ./api-gateway

FROM python:3.11-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PYTHONPATH=/opt/aegis/python-ml:/opt/aegis/python-ml/shared/pb \
    PORT=50051 \
    OMP_NUM_THREADS=2 \
    AEGIS_TORCH_NUM_THREADS=2 \
    API_GATEWAY_PORT=8000 \
    PYTHON_GRPC_ADDR=127.0.0.1:50051 \
    DATABASE_URL= \
    REDIS_URL= \
    CHAOS_ENGINE_URL=http://127.0.0.1:8091 \
    RECOVERY_ENGINE_URL=http://127.0.0.1:8092 \
    DEMO_MODE=true \
    AEGIS_AUTONOMOUS_RECOVERY=false \
    CHAOS_ENABLED=true \
    AEGIS_CORS_ALLOWED_ORIGINS=

RUN apt-get update \
    && apt-get install --no-install-recommends -y nginx supervisor \
    && rm -rf /var/lib/apt/lists/*

COPY python-ml/requirements-serve.txt /tmp/requirements-serve.txt
RUN pip install --no-cache-dir -r /tmp/requirements-serve.txt grpcio-tools==1.68.1

WORKDIR /opt/aegis
COPY --from=gateway-builder /out/api-gateway /usr/local/bin/aegis-api-gateway
COPY --from=frontend-builder /src/dist /usr/share/nginx/html
COPY proto ./proto
COPY configs ./configs
COPY python-ml ./python-ml
COPY --from=gateway-builder /src/go-services/shared/pb ./go-services/shared/pb
RUN python -m grpc_tools.protoc \
    -I /opt/aegis/proto \
    --python_out=/opt/aegis/python-ml/shared/pb \
    --grpc_python_out=/opt/aegis/python-ml/shared/pb \
    /opt/aegis/proto/*.proto \
    && touch /opt/aegis/python-ml/shared/pb/__init__.py \
    && mkdir -p /var/log/supervisor /var/lib/nginx

COPY deploy/hf/nginx.conf /etc/nginx/conf.d/default.conf
COPY deploy/hf/supervisord.conf /etc/supervisor/conf.d/aegis.conf
COPY deploy/hf/start.sh /opt/aegis/start.sh
RUN chmod 0755 /opt/aegis/start.sh \
    && useradd --create-home --uid 10001 aegis \
    && chown -R aegis:aegis /opt/aegis /var/log/supervisor /var/lib/nginx

EXPOSE 7860
ENTRYPOINT ["/opt/aegis/start.sh"]
