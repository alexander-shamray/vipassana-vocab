#!/bin/sh
# Restore the database from R2 on a fresh volume, refresh the dictionary,
# then serve while Litestream streams every change back to R2.
set -eu
: "${DATABASE_PATH:=/data/app.sqlite}"
export DATABASE_PATH

if [ -n "${LITESTREAM_BUCKET:-}" ]; then
  litestream restore -config /etc/litestream.yml -if-db-not-exists -if-replica-exists "$DATABASE_PATH"
fi

if [ -f /srv/dictionary/examples.jsonl ]; then
  server import --words /srv/dictionary/words.jsonl --examples /srv/dictionary/examples.jsonl
else
  server import --words /srv/dictionary/words.jsonl
fi

if [ -n "${LITESTREAM_BUCKET:-}" ]; then
  exec litestream replicate -config /etc/litestream.yml -exec "server serve"
fi
echo "LITESTREAM_BUCKET is not set: running without backups" >&2
exec server serve
