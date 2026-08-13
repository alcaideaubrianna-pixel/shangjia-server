#!/usr/bin/env bash
set -o pipefail
src=$(sed -n 's/^PGCOPYDB_SOURCE_PGURI=//p' /tmp/xiaohuiji-pg.env | sed 's#10.5.4.5:5432#127.0.0.1:15432#')
dst=$(sed -n 's/^PGCOPYDB_TARGET_PGURI=//p' /tmp/xiaohuiji-pg.env)
echo "START=$(date -u)" > /tmp/xiaohuiji-pg-migrate.status
echo "PHASE=SCHEMA" >> /tmp/xiaohuiji-pg-migrate.status
pg_dump "$src" --format=plain --section=pre-data --no-owner --no-acl --no-tablespaces --no-comments | pv -f -i 10 -brt | psql "$dst" -v ON_ERROR_STOP=1
echo "PHASE=DATA" >> /tmp/xiaohuiji-pg-migrate.status
pg_dump "$src" --format=plain --section=data --no-owner --no-acl --no-tablespaces --no-comments | pv -f -i 10 -brt | psql "$dst" -v ON_ERROR_STOP=1
echo "PHASE=POSTDATA" >> /tmp/xiaohuiji-pg-migrate.status
pg_dump "$src" --format=plain --section=post-data --no-owner --no-acl --no-tablespaces --no-comments | pv -f -i 10 -brt | psql "$dst" -v ON_ERROR_STOP=1
echo "EXIT=0" >> /tmp/xiaohuiji-pg-migrate.status
echo "DONE=$(date -u)" >> /tmp/xiaohuiji-pg-migrate.status
