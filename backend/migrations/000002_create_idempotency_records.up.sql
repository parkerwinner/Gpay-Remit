-- Migration: Create idempotency_records table
-- Description: Stores idempotency key information for request deduplication

CREATE TABLE IF NOT EXISTS idempotency_records (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    idempotency_key VARCHAR(256) NOT NULL,
    request_hash VARCHAR(64) NOT NULL,
    request_method VARCHAR(10) NOT NULL,
    request_path VARCHAR(512) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'processing',
    response_status INTEGER DEFAULT 0,
    response_body TEXT,
    created_at_unix BIGINT NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ,
    request_body TEXT,
    user_id BIGINT,
    ip_address VARCHAR(45)
);

CREATE INDEX IF NOT EXISTS idx_idempotency_records_key ON idempotency_records(idempotency_key);
CREATE INDEX IF NOT EXISTS idx_idempotency_records_expires_at ON idempotency_records(expires_at);
CREATE INDEX IF NOT EXISTS idx_idempotency_records_user_id ON idempotency_records(user_id);
CREATE INDEX IF NOT EXISTS idx_idempotency_records_deleted_at ON idempotency_records(deleted_at);