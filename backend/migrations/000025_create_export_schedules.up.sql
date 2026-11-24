CREATE TABLE IF NOT EXISTS export_schedules (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    frequency VARCHAR(20) NOT NULL,
    format VARCHAR(10) NOT NULL,
    destination VARCHAR(20) NOT NULL,
    webhook_url VARCHAR(500),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    last_run_at TIMESTAMPTZ,
    next_run_at TIMESTAMPTZ NOT NULL,
    last_status VARCHAR(20),
    last_error TEXT
);

CREATE INDEX IF NOT EXISTS idx_export_schedules_user_id ON export_schedules(user_id);
CREATE INDEX IF NOT EXISTS idx_export_schedules_due ON export_schedules(is_active, next_run_at) WHERE deleted_at IS NULL;
