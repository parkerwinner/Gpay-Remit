-- Rollback: restore the plain (non-unique) index and drop the unique one.

CREATE INDEX IF NOT EXISTS idx_idempotency_records_key ON idempotency_records(idempotency_key);

DROP INDEX IF EXISTS idx_idempotency_records_key_unique;
