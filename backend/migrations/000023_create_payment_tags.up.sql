CREATE TABLE IF NOT EXISTS payment_tags (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    payment_id BIGINT NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL,
    tag VARCHAR(32) NOT NULL,
    CONSTRAINT uq_payment_tag UNIQUE (payment_id, tag)
);

CREATE INDEX IF NOT EXISTS idx_payment_tags_user_tag ON payment_tags(user_id, tag);
CREATE INDEX IF NOT EXISTS idx_payment_tags_payment_id ON payment_tags(payment_id);
