#!/usr/bin/env bash
set -e

echo "=== Deploying Aegis Self-Healing Platform ==="
docker-compose down
docker-compose up --build -d

echo "Waiting for services to become healthy..."
sleep 5
docker-compose ps
