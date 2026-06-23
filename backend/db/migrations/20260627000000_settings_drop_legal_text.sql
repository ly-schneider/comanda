-- The free-form legal text field is no longer collected or displayed; legal name
-- and address remain.
ALTER TABLE settings
  DROP COLUMN legal_text;
