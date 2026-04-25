ALTER TABLE payments
    DROP COLUMN IF EXISTS platform_fee,
    DROP COLUMN IF EXISTS forex_fee,
    DROP COLUMN IF EXISTS compliance_fee,
    DROP COLUMN IF EXISTS network_fee;

DROP TABLE IF EXISTS audit_logs;
