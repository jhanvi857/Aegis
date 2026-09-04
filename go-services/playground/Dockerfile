# Multi-stage Dockerfile for Aegis Playground Services
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG SERVICE_DIR=playground/service-template
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/bin/service ./${SERVICE_DIR}

# Final minimal runtime image
FROM alpine:3.19

RUN apk --no-cache add ca-certificates curl

WORKDIR /app
COPY --from=builder /app/bin/service /app/service

EXPOSE 8080 8081 8082 8083 8084

ENTRYPOINT ["/app/service"]
