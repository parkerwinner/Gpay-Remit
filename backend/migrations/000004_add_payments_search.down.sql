-- Remove tsvector column, trigger, index and function
DROP TRIGGER IF EXISTS payments_search_vector_update ON payments;
DROP INDEX IF EXISTS payments_search_idx;
ALTER TABLE payments DROP COLUMN IF EXISTS search_vector;
DROP FUNCTION IF EXISTS payments_search_vector_update();
