CREATE TABLE IF NOT EXISTS audit_logs (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    user_id BIGINT,
    action VARCHAR(100) NOT NULL,
    resource VARCHAR(255) NOT NULL,
    old_value JSONB,
    new_value JSONB,
    ip_address VARCHAR(64) NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource ON audit_logs(resource);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at);

ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS platform_fee DECIMAL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS forex_fee DECIMAL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS compliance_fee DECIMAL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS network_fee DECIMAL DEFAULT 0;
