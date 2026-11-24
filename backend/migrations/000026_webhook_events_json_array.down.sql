ALTER TABLE webhooks
    ALTER COLUMN events TYPE TEXT
    USING array_to_string(ARRAY(SELECT jsonb_array_elements_text(events)), ',');
