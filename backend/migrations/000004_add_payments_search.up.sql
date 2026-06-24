-- Add tsvector column and GIN index for full-text search on payments
CREATE FUNCTION payments_search_vector_update() RETURNS trigger AS $$
BEGIN
  NEW.search_vector := to_tsvector('english', coalesce(NEW.notes,'') || ' ' || coalesce(NEW.currency,'') || ' ' || coalesce(NEW.status,'') || ' ' || coalesce(NEW.amount::text,''));
  RETURN NEW;
END
$$ LANGUAGE plpgsql;

ALTER TABLE payments ADD COLUMN IF NOT EXISTS search_vector tsvector;

-- Backfill existing rows
UPDATE payments SET search_vector = to_tsvector('english', coalesce(notes,'') || ' ' || coalesce(currency,'') || ' ' || coalesce(status,'') || ' ' || coalesce(amount::text,''));

CREATE INDEX IF NOT EXISTS payments_search_idx ON payments USING GIN (search_vector);

CREATE TRIGGER payments_search_vector_update BEFORE INSERT OR UPDATE
    ON payments FOR EACH ROW EXECUTE FUNCTION payments_search_vector_update();
