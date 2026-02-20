#!/usr/bin/env sh
set -eu

docker compose -f compose.yml up -d --build
