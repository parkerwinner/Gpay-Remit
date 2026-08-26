-- Add indexes on frequently queried fields for better query performance (#241)

-- Payments table indexes
CREATE INDEX IF NOT EXISTS idx_payments_status ON payments(status);
CREATE INDEX IF NOT EXISTS idx_payments_created_at ON payments(created_at);
CREATE INDEX IF NOT EXISTS idx_payments_sender_id_created_at ON payments(sender_id, created_at);
CREATE INDEX IF NOT EXISTS idx_payments_recipient_id_created_at ON payments(recipient_id, created_at);

-- Invoices table indexes
CREATE INDEX IF NOT EXISTS idx_invoices_status ON invoices(status);
CREATE INDEX IF NOT EXISTS idx_invoices_created_at ON invoices(created_at);
CREATE INDEX IF NOT EXISTS idx_invoices_issuer_id ON invoices(issuer_id);
CREATE INDEX IF NOT EXISTS idx_invoices_recipient_id ON invoices(recipient_id);
