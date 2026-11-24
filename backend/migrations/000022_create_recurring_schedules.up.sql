CREATE TABLE IF NOT EXISTS recurring_schedules (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    user_id BIGINT NOT NULL,
    sender_account VARCHAR(56) NOT NULL,
    recipient_account VARCHAR(56) NOT NULL,
    amount BIGINT NOT NULL,
    currency VARCHAR(12) NOT NULL,
    asset_issuer VARCHAR(56),
    frequency VARCHAR(20) NOT NULL,
    status VARCHAR(20) DEFAULT 'active',
    next_run_at TIMESTAMPTZ NOT NULL,
    last_run_at TIMESTAMPTZ,
    consecutive_failures INTEGER DEFAULT 0,
    last_failure_reason VARCHAR(500),
    notes VARCHAR(500)
);
CREATE INDEX IF NOT EXISTS idx_recurring_schedules_deleted_at ON recurring_schedules(deleted_at);
CREATE INDEX IF NOT EXISTS idx_recurring_schedules_user_id ON recurring_schedules(user_id);
CREATE INDEX IF NOT EXISTS idx_recurring_schedules_due ON recurring_schedules(status, next_run_at);

CREATE TABLE IF NOT EXISTS recurring_schedule_executions (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    schedule_id BIGINT NOT NULL REFERENCES recurring_schedules(id) ON DELETE CASCADE,
    scheduled_for TIMESTAMPTZ NOT NULL,
    payment_id BIGINT,
    status VARCHAR(20) NOT NULL,
    error_message VARCHAR(500),
    CONSTRAINT idx_schedule_execution UNIQUE (schedule_id, scheduled_for)
);
