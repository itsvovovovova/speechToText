#!/usr/bin/env bash
set -euo pipefail

source_db="stt_drill_source_$$"
target_db="stt_drill_target_$$"
archive="$(mktemp)"
pg() {
  docker compose exec -T postgres "$@" -U postgres
}
cleanup() {
  pg dropdb --if-exists "$target_db" >/dev/null
  pg dropdb --if-exists "$source_db" >/dev/null
  rm -f "$archive"
}
trap cleanup EXIT

pg createdb "$source_db"
pg createdb "$target_db"
pg psql -v ON_ERROR_STOP=1 -d "$source_db" <<'SQL'
CREATE TABLE restore_probe (id integer PRIMARY KEY, payload text NOT NULL);
INSERT INTO restore_probe VALUES (1, 'speech-to-text restore verified');
SQL

SECONDS=0
pg pg_dump -Fc "$source_db" > "$archive"
pg pg_restore --exit-on-error -d "$target_db" < "$archive"
elapsed="$SECONDS"

restored="$(pg psql -At -v ON_ERROR_STOP=1 -d "$target_db" -c 'SELECT payload FROM restore_probe WHERE id = 1')"
[[ "$restored" == 'speech-to-text restore verified' ]]
printf 'Restore verified in %ss (%s bytes)\n' "$elapsed" "$(wc -c < "$archive")"
