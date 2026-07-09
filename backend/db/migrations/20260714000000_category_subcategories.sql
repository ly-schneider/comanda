ALTER TABLE category
    ADD COLUMN parent_id VARCHAR(12) REFERENCES category (id) ON DELETE RESTRICT;

CREATE INDEX category_parent_id_idx ON category (parent_id);
