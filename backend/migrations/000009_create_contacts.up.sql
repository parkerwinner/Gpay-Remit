CREATE TABLE IF NOT EXISTS contacts (
    id SERIAL PRIMARY KEY,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    nickname VARCHAR(255) NOT NULL,
    stellar_address VARCHAR(56) NOT NULL,
    currency VARCHAR(12) DEFAULT 'USDC',
    email VARCHAR(255),
    notes VARCHAR(500),
    verification_status VARCHAR(20) DEFAULT 'pending',
    is_verified BOOLEAN DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_contacts_user_id ON contacts(user_id);
CREATE INDEX IF NOT EXISTS idx_contacts_deleted_at ON contacts(deleted_at);
