-- Migration 003: Add covering index for reverse expansion optimization
-- This index optimizes queries that find all subjects with a relation on an object
-- The INCLUDE clause allows index-only scans without accessing the base table

CREATE INDEX IF NOT EXISTS idx_reverse_expansion_covering ON relation_tuples 
(namespace, object_id, relation, subject_namespace, subject_id) 
INCLUDE (subject_relation);

-- This covering index enables fast reverse lookups:
-- SELECT DISTINCT subject_id FROM relation_tuples 
-- WHERE namespace = 'doc' AND object_id = 'doc_1' AND relation = 'viewer';
-- 
-- The index includes all columns needed for the query, allowing index-only scans
-- which are 10-50x faster than table scans on large datasets

