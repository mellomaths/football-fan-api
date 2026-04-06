# Local development via Docker Compose only (from repo root).
# Usage: `just` lists recipes; `just up`, `just restart api`, etc.

compose := "docker compose -f docker-compose.yaml"

# Start all services in the background (build images if missing).
up *args:
    {{compose}} up -d {{args}}

# Stop and remove containers (keeps named volumes e.g. postgres data).
down *args:
    {{compose}} down {{args}}

# Stop and remove containers, networks, and volumes (wipes DB — confirm first).
down-volumes:
    {{compose}} down -v

# Build (or rebuild) images without starting.
build *args:
    {{compose}} build {{args}}

# Rebuild images (no cache) and start the stack detached.
rebuild:
    {{compose}} build --no-cache
    {{compose}} up -d

# Start existing stopped containers (after `down` without `-v`, or `stop`).
start *args:
    {{compose}} start {{args}}

# Stop running containers without removing them.
stop *args:
    {{compose}} stop {{args}}

# Docker Compose restart only (no image rebuild). Optional service names.
restart-services *args:
    {{compose}} restart {{args}}

# Drop app schema footballfan (all data + migration history there), rebuild api + scraper images, recreate those containers.
restart:
    #!/usr/bin/env bash
    set -euo pipefail
    {{compose}} up -d
    {{compose}} exec -T postgres sh -c 'until pg_isready -U football -d football; do sleep 1; done'
    {{compose}} exec -T postgres psql -U football -d football -v ON_ERROR_STOP=1 -c 'DROP SCHEMA IF EXISTS footballfan CASCADE;'
    {{compose}} build --no-cache api scraper
    {{compose}} up -d --force-recreate api scraper

# Follow logs (all services). Pass names to scope: `just logs api scraper`.
logs *args:
    {{compose}} logs -f {{args}}

# One-shot log tail without follow.
logs-tail *args:
    {{compose}} logs --tail 200 {{args}}

# Container status.
ps:
    {{compose}} ps -a

# Open psql in the Postgres container.
db-shell:
    {{compose}} exec postgres psql -U football -d football

# Run a shell inside a service container (default: api). Example: `just shell scraper`.
shell service="api":
    {{compose}} exec -it {{service}} /bin/sh
