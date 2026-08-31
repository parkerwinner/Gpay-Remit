-- Migration: Enforce idempotency_key uniqueness at the database level
-- Description: Fixes a race condition (#195) where two concurrent requests
-- with the same Idempotency-Key header could both pass the application-level
-- "does a record exist?" check before either had committed its INSERT,
-- creating duplicate idempotency_records rows for the same key. A regular
-- (non-unique) index existed but never prevented this.
--
-- Partial index (WHERE deleted_at IS NULL) so a soft-deleted record doesn't
-- block reuse of its key, matching GORM's soft-delete behavior on this model.

CREATE UNIQUE INDEX IF NOT EXISTS idx_idempotency_records_key_unique
    ON idempotency_records(idempotency_key)
    WHERE deleted_at IS NULL;

-- The unique index above supersedes the plain lookup index from migration
-- 000005 (a unique index already serves equality lookups just as well).
DROP INDEX IF EXISTS idx_idempotency_records_key;
