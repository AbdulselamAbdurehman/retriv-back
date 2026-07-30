.PHONY: db down up-full models run tidy test fmt vet

COMPOSE := docker compose -f deploy/docker-compose.yml

# --- Workflow 1: dev (only the DB in Docker; Go + Ollama run on the host) ---

# Start just Postgres (PostGIS+pgvector).
db:
	$(COMPOSE) up -d --build

# Pull the embedding model into the HOST Ollama (once). llama3.2:1b already local.
models:
	ollama pull nomic-embed-text

# Run the API on the host (migrations apply on boot).
run:
	set -a; . ./.env; set +a; go run ./cmd/main.go

# --- Workflow 2: everything in Docker (Ollama still on the host) ---

up-full:
	$(COMPOSE) --profile full up -d --build

# --- Common ---

down:
	$(COMPOSE) --profile full down

tidy:
	go mod tidy

test:
	go test ./...

fmt:
	go fmt ./...

vet:
	go vet ./...
