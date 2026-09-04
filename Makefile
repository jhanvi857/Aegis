.PHONY: help up down build proto test lint clean logs status

help:
	@echo "Aegis Development Commands:"
	@echo "  make up          - Start playground cluster with docker-compose"
	@echo "  make down        - Tear down playground cluster"
	@echo "  make build       - Build all Go services and Docker images"
	@echo "  make proto       - Regenerate Protobuf stubs for Go and Python"
	@echo "  make test        - Run test suite"
	@echo "  make logs        - Tail logs from all cluster containers"
	@echo "  make status      - Check health endpoints of all running nodes"

up:
	docker-compose up -d

down:
	docker-compose down

build:
	cd go-services && go build ./...
	docker-compose build

proto:
	bash scripts/gen_proto.sh

test:
	cd go-services && go test ./...

logs:
	docker-compose logs -f

status:
	@echo "Checking cluster health..."
	@curl -s http://localhost:8080/health || echo "Gateway unreachable"
	@echo ""
	@curl -s http://localhost:8081/health || echo "Node A unreachable"
	@echo ""
	@curl -s http://localhost:8082/health || echo "Node B unreachable"
	@echo ""
	@curl -s http://localhost:8083/health || echo "Node C unreachable"
	@echo ""
	@curl -s http://localhost:8084/health || echo "Node D unreachable"
	@echo ""
