#!/usr/bin/env bash
# Stosuje migracje z backend/migrations w kolejnosci numerycznej.
# Zastosowane migracje sa zapisywane w tabeli schema_migrations, wiec skrypt jest idempotentny.
set -euo pipefail
cd "$(dirname "$0")/.."
PSQL="docker compose exec -T db psql -v ON_ERROR_STOP=1 -U doyousend -d doyousend"
$PSQL -c "CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());" >/dev/null
shopt -s nullglob
for file in backend/migrations/*.sql; do
  version="$(basename "$file")"
  applied="$($PSQL -tAc "SELECT 1 FROM schema_migrations WHERE version = '$version';")"
  if [ "$applied" = "1" ]; then
    echo "  pominieto  $version"
    continue
  fi
  echo "  stosuje    $version"
  $PSQL < "$file"
  $PSQL -c "INSERT INTO schema_migrations (version) VALUES ('$version');" >/dev/null
done
echo "Migracje aktualne."
