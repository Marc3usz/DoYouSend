#!/usr/bin/env bash
# Wgrywa dane demonstracyjne. WYLACZNIE dane fikcyjne (example.test, numery +48 500 100 1xx).
set -euo pipefail
cd "$(dirname "$0")/.."
docker compose exec -T db psql -v ON_ERROR_STOP=1 -U doyousend -d doyousend < backend/testdata/seed.sql
echo "Dane demonstracyjne wgrane."
