# syntax=docker/dockerfile:1.7
FROM golang:1.24-alpine AS builder

WORKDIR /app
ENV GOTOOLCHAIN=auto

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG SERVICE_DIR=playground/service-template
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /app/bin/service ./${SERVICE_DIR}

# Final minimal runtime image
FROM alpine:3.21

ARG INSTALL_DOCKER_CLI=false
ARG INSTALL_WGET=false
RUN packages="ca-certificates"; \
    if [ "$INSTALL_DOCKER_CLI" = "true" ]; then packages="$packages docker-cli"; fi; \
    if [ "$INSTALL_WGET" = "true" ]; then packages="$packages wget"; fi; \
    apk --no-cache add $packages && \
    addgroup -S aegis && adduser -S -G aegis aegis

WORKDIR /app
COPY --from=builder /app/bin/service /app/service
USER aegis

EXPOSE 8080 8081 8082 8083 8084

ENTRYPOINT ["/app/service"]
