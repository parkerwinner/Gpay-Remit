#!/usr/bin/env bash
# Restores the gpay_remit Postgres database from an S3 backup produced by
# backup.sh. DESTRUCTIVE: overwrites the target database's contents.
#
# Usage: ./restore.sh s3://gpay-remit-backups/gpay_remit_20260101T000000Z.sql.gz
#
# Required env vars:
#   DATABASE_URL - Postgres connection string to restore into
set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL is required}"

BACKUP_PATH="${1:?Usage: restore.sh <s3-path-to-backup>}"
TMP_PATH="/tmp/$(basename "${BACKUP_PATH}")"

echo "Downloading ${BACKUP_PATH}..."
aws s3 cp "${BACKUP_PATH}" "${TMP_PATH}"

echo "Restoring into database from ${TMP_PATH}..."
gunzip -c "${TMP_PATH}" | psql "${DATABASE_URL}"

rm -f "${TMP_PATH}"
echo "Restore complete."
