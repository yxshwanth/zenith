-- Migration 002: Enable system-versioned temporal tables for audit trail
-- This migration converts the relation_tuples table to a temporal table
-- enabling point-in-time queries and complete audit history

-- Note: CockroachDB doesn't support system-versioned temporal tables directly
-- Instead, we'll use AS OF SYSTEM TIME for point-in-time queries
-- and maintain a history table manually for audit purposes

-- Create history table for audit trail
CREATE TABLE IF NOT EXISTS relation_tuples_history (
    namespace STRING NOT NULL,
    object_id STRING NOT NULL,
    relation STRING NOT NULL,
    subject_namespace STRING NOT NULL,
    subject_id STRING NOT NULL,
    subject_relation STRING NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ,
    valid_from TIMESTAMPTZ NOT NULL DEFAULT now(),
    valid_to TIMESTAMPTZ,
    
    PRIMARY KEY (namespace, object_id, relation, subject_namespace, subject_id, subject_relation, valid_from),
    
    INDEX idx_history_object (namespace, object_id, relation, valid_from),
    INDEX idx_history_subject (subject_namespace, subject_id, subject_relation, valid_from),
    INDEX idx_history_time_range (valid_from, valid_to)
);

-- Add valid_from and valid_to columns to main table for temporal queries
-- These will be populated using AS OF SYSTEM TIME
ALTER TABLE relation_tuples 
ADD COLUMN IF NOT EXISTS valid_from TIMESTAMPTZ DEFAULT now(),
ADD COLUMN IF NOT EXISTS valid_to TIMESTAMPTZ;

-- Create index for temporal queries
CREATE INDEX IF NOT EXISTS idx_temporal_valid_from ON relation_tuples (valid_from);
CREATE INDEX IF NOT EXISTS idx_temporal_valid_to ON relation_tuples (valid_to);

-- Note: For point-in-time queries, use:
-- SELECT * FROM relation_tuples AS OF SYSTEM TIME <timestamp>
-- WHERE namespace = '...' AND object_id = '...' AND relation = '...'
--
-- For audit history, query relation_tuples_history:
-- SELECT * FROM relation_tuples_history
-- WHERE namespace = '...' AND object_id = '...' 
--   AND valid_from <= <timestamp> AND (valid_to IS NULL OR valid_to > <timestamp>)

