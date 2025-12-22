-- Note: Create the database manually before running this migration:
-- CREATE DATABASE zenith;
-- Or ensure your connection string includes the database name

-- Relation tuples table
-- Stores all permission relationships as a DAG
-- Note: subject_relation uses empty string "" for direct user subjects (not NULL)
-- because CockroachDB doesn't allow NULL values in primary keys
CREATE TABLE IF NOT EXISTS relation_tuples (
    namespace STRING NOT NULL,
    object_id STRING NOT NULL,
    relation STRING NOT NULL,
    subject_namespace STRING NOT NULL,
    subject_id STRING NOT NULL,
    subject_relation STRING NOT NULL DEFAULT '',  -- Empty string for direct users, set for usersets
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    
    PRIMARY KEY (namespace, object_id, relation, subject_namespace, subject_id, subject_relation),
    
    -- Index for fast lookups by object and relation
    INDEX idx_object_relation (namespace, object_id, relation),
    
    -- Index for reverse lookups (finding what objects a subject has access to)
    INDEX idx_subject (subject_namespace, subject_id, subject_relation)
);

-- Enable AS OF SYSTEM TIME for Zookie-based queries
-- CockroachDB automatically supports this feature

