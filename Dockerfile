FROM golang:1.24-bookworm AS go-build
WORKDIR /src
COPY go-services/go.mod go-services/go.sum ./go-services/
COPY go-services ./go-services
RUN cd go-services && CGO_ENABLED=0 go build -o /out/aegis-gateway ./api-gateway

FROM python:3.12-slim
WORKDIR /app
COPY --from=go-build /out/aegis-gateway /usr/local/bin/aegis-gateway
COPY python-ml /app/python-ml
RUN pip install --no-cache-dir -r /app/python-ml/requirements-serve.txt
COPY deploy/render-entrypoint.sh /usr/local/bin/render-entrypoint.sh
RUN chmod +x /usr/local/bin/render-entrypoint.sh
ENV PORT=8000 AEGIS_GRPC_PORT=50051 PYTHON_GRPC_ADDR=127.0.0.1:50051 USE_KAFKA=false
EXPOSE 8000 4318 50051
ENTRYPOINT ["/usr/local/bin/render-entrypoint.sh"]
