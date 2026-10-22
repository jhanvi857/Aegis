# ─── Build Stage ──────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /workspace

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Cache go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source trees
COPY shared/ ./shared/
COPY telemetry-agent/ ./telemetry-agent/

# Build statically compiled binary
WORKDIR /workspace/telemetry-agent
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -extldflags '-static'" \
    -o /bin/telemetry-agent .

# ─── Final Minimal Non-Root Image ───────────────────────────────────────────
FROM alpine:3.19

RUN addgroup -g 10001 -S aegis && \
    adduser -u 10001 -S aegis -G aegis --no-create-home

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /bin/telemetry-agent /bin/telemetry-agent

USER 10001:10001

EXPOSE 8090

ENTRYPOINT ["/bin/telemetry-agent"]
