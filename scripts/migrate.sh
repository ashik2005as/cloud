#!/usr/bin/env sh
set -eu

DB_URL="${DATABASE_URL:-host=localhost port=5432 user=postgres dbname=cloud sslmode=disable}"

for file in /app/migrations/*.up.sql; do
  echo "Applying $file"
  psql "$DB_URL" -f "$file"
done
