package db

import (
	"context"
	"fmt"
	"time"

	"github.com/zenith/zenith/internal/metrics"
	"github.com/zenith/zenith/internal/models"
	"github.com/zenith/zenith/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// TupleRepo provides methods for tuple operations
type TupleRepo struct {
	db *DB
}

// NewTupleRepo creates a new tuple repository
func NewTupleRepo(db *DB) *TupleRepo {
	return &TupleRepo{db: db}
}

// Insert inserts a new relation tuple
func (r *TupleRepo) Insert(ctx context.Context, tuple *models.Tuple) (int64, error) {
	if err := tuple.Validate(); err != nil {
		return 0, err
	}

	query := `
		INSERT INTO relation_tuples 
		(namespace, object_id, relation, subject_namespace, subject_id, subject_relation)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (namespace, object_id, relation, subject_namespace, subject_id, subject_relation) DO NOTHING
	`

	zookie, err := r.db.ExecWithZookie(ctx, query,
		tuple.Namespace,
		tuple.ObjectID,
		tuple.Relation,
		tuple.SubjectNamespace,
		tuple.SubjectID,
		tuple.SubjectRelation, // Empty string for direct users
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert tuple: %w", err)
	}

	return zookie, nil
}

// Delete removes a relation tuple
func (r *TupleRepo) Delete(ctx context.Context, tuple *models.Tuple) (int64, error) {
	if err := tuple.Validate(); err != nil {
		return 0, err
	}

	query := `
		DELETE FROM relation_tuples
		WHERE namespace = $1 
		  AND object_id = $2 
		  AND relation = $3
		  AND subject_namespace = $4
		  AND subject_id = $5
		  AND subject_relation = $6
	`

	zookie, err := r.db.ExecWithZookie(ctx, query,
		tuple.Namespace,
		tuple.ObjectID,
		tuple.Relation,
		tuple.SubjectNamespace,
		tuple.SubjectID,
		tuple.SubjectRelation, // Empty string for direct users
	)
	if err != nil {
		return 0, fmt.Errorf("failed to delete tuple: %w", err)
	}

	return zookie, nil
}

// CheckDirect performs a direct lookup to see if a tuple exists.
// Existence and the cluster logical timestamp are fetched in a single round trip.
func (r *TupleRepo) CheckDirect(ctx context.Context, tuple *models.Tuple, requiredZookie int64) (bool, int64, error) {
	// Create span for database lookup
	ctx, span := observability.StartSpan(ctx, "DBLookup",
		trace.WithAttributes(
			attribute.String("db.operation", "CheckDirect"),
			attribute.String("db.tuple", tuple.String()),
		),
	)
	defer span.End()

	if requiredZookie > 0 {
		observability.AddZookieToSpan(span, requiredZookie)
	}

	if err := tuple.Validate(); err != nil {
		span.RecordError(err)
		return false, 0, err
	}

	start := time.Now()

	// One round trip: existence + logical timestamp.
	// Previously this was SELECT 1 plus a second cluster_logical_timestamp() query.
	tuplesFrom := "relation_tuples"
	if requiredZookie > 0 {
		tuplesFrom = fmt.Sprintf("relation_tuples AS OF SYSTEM TIME %d", requiredZookie)
	}
	query := fmt.Sprintf(`
		SELECT EXISTS (
			SELECT 1 FROM %s
			WHERE namespace = $1
			  AND object_id = $2
			  AND relation = $3
			  AND subject_namespace = $4
			  AND subject_id = $5
			  AND subject_relation = $6
		), cluster_logical_timestamp()::STRING
	`, tuplesFrom)

	var found bool
	var zookieStr string
	err := r.db.conn.QueryRowContext(ctx, query,
		tuple.Namespace,
		tuple.ObjectID,
		tuple.Relation,
		tuple.SubjectNamespace,
		tuple.SubjectID,
		tuple.SubjectRelation,
	).Scan(&found, &zookieStr)
	if err != nil {
		span.RecordError(err)
		return false, 0, fmt.Errorf("failed to check tuple: %w", err)
	}

	zookie, err := ParseZookieString(zookieStr)
	if err != nil {
		span.RecordError(err)
		return false, 0, err
	}

	metrics.RecordDatabaseQuery("check_direct", time.Since(start))
	span.SetAttributes(attribute.Bool("db.found", found))
	observability.AddZookieToSpan(span, zookie)
	return found, zookie, nil
}

// FindUsersetDefinitions finds all tuples where the subject is a userset
// that grants the specified relation on the object.
// Returns tuples where object#relation is defined via usersets (e.g., doc_1#viewer -> folder_A#viewer)
func (r *TupleRepo) FindUsersetDefinitions(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*models.Tuple, error) {
	query := `
		SELECT namespace, object_id, relation, subject_namespace, subject_id, subject_relation
		FROM relation_tuples
		WHERE namespace = $1 
		  AND object_id = $2 
		  AND relation = $3
		  AND subject_relation != ''
	`

	rows, err := r.db.QueryWithZookie(ctx, requiredZookie, query, namespace, objectID, relation)
	if err != nil {
		return nil, fmt.Errorf("failed to find userset definitions: %w", err)
	}
	defer rows.Close()

	var tuples []*models.Tuple
	for rows.Next() {
		tuple := &models.Tuple{}
		err := rows.Scan(
			&tuple.Namespace,
			&tuple.ObjectID,
			&tuple.Relation,
			&tuple.SubjectNamespace,
			&tuple.SubjectID,
			&tuple.SubjectRelation,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan userset definition: %w", err)
		}
		tuples = append(tuples, tuple)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating userset definitions: %w", err)
	}

	return tuples, nil
}

// GetZookie returns the current logical timestamp (Zookie) from CockroachDB
func (r *TupleRepo) GetZookie(ctx context.Context) (int64, error) {
	return r.db.GetZookie(ctx)
}

// FindDirectSubjects finds all direct user subjects (not usersets) for an object and relation
// Used for reverse expansion: "Who are the direct users with this relation?"
// Optimized to use idx_reverse_expansion_covering for index-only scans
func (r *TupleRepo) FindDirectSubjects(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*models.Tuple, error) {
	start := time.Now()

	// Create span for database lookup
	ctx, span := observability.StartSpan(ctx, "DBLookup",
		trace.WithAttributes(
			attribute.String("db.operation", "FindDirectSubjects"),
			attribute.String("db.namespace", namespace),
			attribute.String("db.object_id", objectID),
			attribute.String("db.relation", relation),
		),
	)
	defer span.End()

	// The covering index idx_reverse_expansion_covering will be used automatically
	// for this query pattern, enabling index-only scans
	query := `
		SELECT namespace, object_id, relation, subject_namespace, subject_id, subject_relation
		FROM relation_tuples
		WHERE namespace = $1 
		  AND object_id = $2 
		  AND relation = $3
		  AND subject_relation = ''
	`

	rows, err := r.db.QueryWithZookie(ctx, requiredZookie, query, namespace, objectID, relation)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to find direct subjects: %w", err)
	}
	defer rows.Close()

	var tuples []*models.Tuple
	for rows.Next() {
		tuple := &models.Tuple{}
		err := rows.Scan(
			&tuple.Namespace,
			&tuple.ObjectID,
			&tuple.Relation,
			&tuple.SubjectNamespace,
			&tuple.SubjectID,
			&tuple.SubjectRelation,
		)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan direct subject: %w", err)
		}
		tuples = append(tuples, tuple)
	}

	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating direct subjects: %w", err)
	}

	duration := time.Since(start)
	metrics.RecordDatabaseQuery("reverse_expansion", duration)
	span.SetAttributes(
		attribute.Int("db.result_count", len(tuples)),
		attribute.Float64("db.duration_ms", float64(duration.Nanoseconds())/1e6),
	)

	return tuples, nil
}

// FindUsersetSubjects finds all userset subjects for an object and relation
// Used for reverse expansion: "What usersets have this relation?"
func (r *TupleRepo) FindUsersetSubjects(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*models.Tuple, error) {
	// This is the same as FindUsersetDefinitions, but kept separate for clarity
	return r.FindUsersetDefinitions(ctx, namespace, objectID, relation, requiredZookie)
}

// FindUsersetMembers finds all members of a userset (both direct users and nested usersets)
// Used for reverse expansion: "Who is in group:eng#member?"
// This queries tuples where the userset is the object, not the subject
// Returns both direct users (subject_relation = ”) and nested usersets (subject_relation != ”)
func (r *TupleRepo) FindUsersetMembers(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*models.Tuple, error) {
	query := `
		SELECT namespace, object_id, relation, subject_namespace, subject_id, subject_relation
		FROM relation_tuples
		WHERE namespace = $1 
		  AND object_id = $2 
		  AND relation = $3
	`

	rows, err := r.db.QueryWithZookie(ctx, requiredZookie, query, namespace, objectID, relation)
	if err != nil {
		return nil, fmt.Errorf("failed to find userset members: %w", err)
	}
	defer rows.Close()

	var tuples []*models.Tuple
	for rows.Next() {
		tuple := &models.Tuple{}
		err := rows.Scan(
			&tuple.Namespace,
			&tuple.ObjectID,
			&tuple.Relation,
			&tuple.SubjectNamespace,
			&tuple.SubjectID,
			&tuple.SubjectRelation,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan userset member: %w", err)
		}
		tuples = append(tuples, tuple)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating userset members: %w", err)
	}

	return tuples, nil
}

// ListAll lists all tuples in the database
// Used for graph visualization and tuple management
func (r *TupleRepo) ListAll(ctx context.Context) ([]*models.Tuple, error) {
	// Create span for database lookup
	ctx, span := observability.StartSpan(ctx, "DBLookup",
		trace.WithAttributes(
			attribute.String("db.operation", "ListAll"),
		),
	)
	defer span.End()

	query := `
		SELECT namespace, object_id, relation, subject_namespace, subject_id, subject_relation
		FROM relation_tuples
		ORDER BY namespace, object_id, relation, subject_namespace, subject_id
	`

	rows, err := r.db.conn.QueryContext(ctx, query)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to list tuples: %w", err)
	}
	defer rows.Close()

	var tuples []*models.Tuple
	for rows.Next() {
		tuple := &models.Tuple{}
		err := rows.Scan(
			&tuple.Namespace,
			&tuple.ObjectID,
			&tuple.Relation,
			&tuple.SubjectNamespace,
			&tuple.SubjectID,
			&tuple.SubjectRelation,
		)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan tuple: %w", err)
		}
		tuples = append(tuples, tuple)
	}

	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating tuples: %w", err)
	}

	span.SetAttributes(attribute.Int("db.result_count", len(tuples)))
	return tuples, nil
}

// QueryAtTime performs a point-in-time query using AS OF SYSTEM TIME
// This allows querying the state of tuples at a specific logical timestamp (zookie)
func (r *TupleRepo) QueryAtTime(ctx context.Context, timestamp int64, namespace, objectID, relation string) ([]*models.Tuple, error) {
	// Create span for temporal query
	ctx, span := observability.StartSpan(ctx, "DBLookup",
		trace.WithAttributes(
			attribute.String("db.operation", "QueryAtTime"),
			attribute.String("db.namespace", namespace),
			attribute.String("db.object_id", objectID),
			attribute.String("db.relation", relation),
		),
	)
	defer span.End()

	if timestamp > 0 {
		observability.AddZookieToSpan(span, timestamp)
	}

	// Use AS OF SYSTEM TIME for point-in-time query
	query := `
		SELECT namespace, object_id, relation, subject_namespace, subject_id, subject_relation
		FROM relation_tuples AS OF SYSTEM TIME $1
		WHERE namespace = $2 
		  AND object_id = $3 
		  AND relation = $4
	`

	rows, err := r.db.conn.QueryContext(ctx, query, timestamp, namespace, objectID, relation)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to query at time: %w", err)
	}
	defer rows.Close()

	var tuples []*models.Tuple
	for rows.Next() {
		tuple := &models.Tuple{}
		err := rows.Scan(
			&tuple.Namespace,
			&tuple.ObjectID,
			&tuple.Relation,
			&tuple.SubjectNamespace,
			&tuple.SubjectID,
			&tuple.SubjectRelation,
		)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan tuple: %w", err)
		}
		tuples = append(tuples, tuple)
	}

	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating tuples: %w", err)
	}

	span.SetAttributes(attribute.Int("db.result_count", len(tuples)))
	return tuples, nil
}

// QueryHistoryAtTime queries the history table for audit trail at a specific time
func (r *TupleRepo) QueryHistoryAtTime(ctx context.Context, timestamp time.Time, namespace, objectID, relation string) ([]*models.Tuple, error) {
	// Create span for history query
	ctx, span := observability.StartSpan(ctx, "DBLookup",
		trace.WithAttributes(
			attribute.String("db.operation", "QueryHistoryAtTime"),
			attribute.String("db.namespace", namespace),
			attribute.String("db.object_id", objectID),
			attribute.String("db.relation", relation),
		),
	)
	defer span.End()

	query := `
		SELECT namespace, object_id, relation, subject_namespace, subject_id, subject_relation
		FROM relation_tuples_history
		WHERE namespace = $1 
		  AND object_id = $2 
		  AND relation = $3
		  AND valid_from <= $4
		  AND (valid_to IS NULL OR valid_to > $4)
	`

	rows, err := r.db.conn.QueryContext(ctx, query, namespace, objectID, relation, timestamp)
	if err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("failed to query history: %w", err)
	}
	defer rows.Close()

	var tuples []*models.Tuple
	for rows.Next() {
		tuple := &models.Tuple{}
		err := rows.Scan(
			&tuple.Namespace,
			&tuple.ObjectID,
			&tuple.Relation,
			&tuple.SubjectNamespace,
			&tuple.SubjectID,
			&tuple.SubjectRelation,
		)
		if err != nil {
			span.RecordError(err)
			return nil, fmt.Errorf("failed to scan tuple: %w", err)
		}
		tuples = append(tuples, tuple)
	}

	if err := rows.Err(); err != nil {
		span.RecordError(err)
		return nil, fmt.Errorf("error iterating tuples: %w", err)
	}

	span.SetAttributes(attribute.Int("db.result_count", len(tuples)))
	return tuples, nil
}
