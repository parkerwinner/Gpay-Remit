-- Remove query performance indexes (#241)

DROP INDEX IF EXISTS idx_payments_status;
DROP INDEX IF EXISTS idx_payments_created_at;
DROP INDEX IF EXISTS idx_payments_sender_id_created_at;
DROP INDEX IF EXISTS idx_payments_recipient_id_created_at;

DROP INDEX IF EXISTS idx_invoices_status;
DROP INDEX IF EXISTS idx_invoices_created_at;
DROP INDEX IF EXISTS idx_invoices_issuer_id;
DROP INDEX IF EXISTS idx_invoices_recipient_id;
