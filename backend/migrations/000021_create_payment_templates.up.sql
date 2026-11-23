CREATE TABLE IF NOT EXISTS payment_templates (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ,
    user_id BIGINT NOT NULL,
    template_name VARCHAR(255) NOT NULL,
    recipient_account VARCHAR(56) NOT NULL,
    amount DOUBLE PRECISION NOT NULL,
    asset_code VARCHAR(12) NOT NULL,
    asset_issuer VARCHAR(56),
    notes VARCHAR(500)
);
CREATE INDEX IF NOT EXISTS idx_payment_templates_deleted_at ON payment_templates(deleted_at);
CREATE INDEX IF NOT EXISTS idx_payment_templates_user_id ON payment_templates(user_id);
