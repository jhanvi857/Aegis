.PHONY: help up up-lean up-full down build proto test lint clean logs status notebooks-smoke

help:
	@echo "Aegis Development Commands:"
	@echo "  make build       - Build all images once (run this explicitly after code changes)"
	@echo "  make up-lean     - Start the lightweight core profile without rebuilding"
	@echo "  make up-full     - Start the full playground and infrastructure without rebuilding"
	@echo "  make down        - Tear down the Compose stack"
	@echo "  make proto       - Regenerate Protobuf stubs for Go and Python"
	@echo "  make test        - Run test suite"
	@echo "  make logs        - Tail logs from all cluster containers"
	@echo "  make status      - Check health endpoints of all running nodes"

up: up-lean

up-lean:
	docker compose --profile core up -d

down:
	docker compose down

build:
	cd go-services && go build ./...
	docker compose --profile core --profile full build

up-full:
	docker compose --profile full up -d

proto:
	bash scripts/gen_proto.sh

test:
	cd go-services && go test ./...

logs:
	docker compose logs -f

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

notebooks-smoke:
	python notebooks/scripts/smoke_notebooks.py --reduced
