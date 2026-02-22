#!/usr/bin/env sh
set -eu

SERVICE="webguard-instance"

compose() {
  docker compose -f compose.yml -f docker-compose.override.yml "$@"
}

compose up -d --build

echo "Connecting to shell in container ${SERVICE} ..."
if compose exec -T "${SERVICE}" sh -lc "command -v bash >/dev/null 2>&1"; then
  compose exec "${SERVICE}" bash
  exit 0
fi

compose exec "${SERVICE}" sh
