ALTER TABLE webhooks
    ALTER COLUMN events TYPE JSONB
    USING CASE
        WHEN events IS NULL OR btrim(events) = '' THEN '[]'::jsonb
        WHEN left(btrim(events), 1) = '[' THEN events::jsonb
        ELSE to_jsonb(regexp_split_to_array(btrim(events), '\s*,\s*'))
    END;
