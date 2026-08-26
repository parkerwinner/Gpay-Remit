#!/usr/bin/env bash
# Dumps the gpay_remit Postgres database and uploads it to S3.
# Intended to run on a daily schedule (cron / k8s CronJob).
#
# Required env vars:
#   DATABASE_URL      - Postgres connection string
#   BACKUP_S3_BUCKET  - target S3 bucket, e.g. s3://gpay-remit-backups
# Requires: pg_dump, aws CLI (or an S3-compatible equivalent) on PATH.
set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL is required}"
: "${BACKUP_S3_BUCKET:?BACKUP_S3_BUCKET is required}"

TIMESTAMP="$(date -u +%Y%m%dT%H%M%SZ)"
FILENAME="gpay_remit_${TIMESTAMP}.sql.gz"
TMP_PATH="/tmp/${FILENAME}"

echo "Dumping database to ${TMP_PATH}..."
pg_dump "${DATABASE_URL}" | gzip > "${TMP_PATH}"

echo "Uploading to ${BACKUP_S3_BUCKET}/${FILENAME}..."
aws s3 cp "${TMP_PATH}" "${BACKUP_S3_BUCKET}/${FILENAME}"

rm -f "${TMP_PATH}"
echo "Backup complete: ${BACKUP_S3_BUCKET}/${FILENAME}"
