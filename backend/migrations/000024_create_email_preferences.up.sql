CREATE TABLE IF NOT EXISTS email_preferences (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    user_id BIGINT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    transaction_confirmations BOOLEAN NOT NULL DEFAULT TRUE,
    marketing BOOLEAN NOT NULL DEFAULT FALSE,
    security_alerts BOOLEAN NOT NULL DEFAULT TRUE CHECK (security_alerts = TRUE)
);

CREATE INDEX IF NOT EXISTS idx_email_preferences_user_id ON email_preferences(user_id);
