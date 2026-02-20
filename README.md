# WebGuard Instance v2 (Go)

This repository is a Go implementation of the WebGuard worker instance.

It keeps compatibility with the existing PHP worker contract by preserving:

- Core API endpoint paths:
  - `GET /api/v1/internal/monitorings`
  - `POST /api/v1/internal/monitoring-responses`
  - `POST /api/v1/internal/ssl-results`
- Header name: `X-API-KEY`
- Query/body field names and monitoring type/status values
- Command name:
  - `monitoring`
- The combined run executes response and SSL phases in parallel.
- 5-minute scheduling semantics in `serve` mode

## Run commands directly

```bash
go run ./cmd/webguard-instance serve
go run ./cmd/webguard-instance monitoring
```

## Docker

Before starting containers, create your runtime env file:

```bash
cp .env.example .env
```

Production compose:

```bash
./start-prod.sh
# or:
docker compose -f compose.yml up -d --build
```

Local development compose (source mounted, runs development stage):

```bash
./start-dev.sh
# or:
docker compose -f compose.yml -f docker-compose.override.yml up -d --build
```

Stop services:

```bash
docker compose -f compose.yml down
docker compose -f compose.yml -f docker-compose.override.yml down
```

Run one-off compatible commands in container:

```bash
docker compose -f compose.yml run --rm webguard-instance monitoring
```

## Environment variables

See `.env.example`. Compatibility-critical variables:

- `WEBGUARD_LOCATION`
- `WEBGUARD_CORE_API_KEY`
- `WEBGUARD_CORE_API_URL`

Worker tuning:

- `QUEUE_DEFAULT_WORKERS`

HTTP health endpoint:

- `PORT` (default: `8080`) exposes `GET /`

## GitHub Actions

Workflows included:

- `.github/workflows/ci.yml`
  - `gofmt` check
  - `go vet`
  - `go test`
  - `go build`
  - production Docker build validation
- `.github/workflows/docker-image.yml`
  - builds and publishes multi-arch images to GHCR on `main` and `v*` tags
