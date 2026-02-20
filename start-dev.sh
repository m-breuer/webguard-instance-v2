#!/usr/bin/env sh
set -eu

docker compose -f compose.yml -f docker-compose.override.yml up -d --build
