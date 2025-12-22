package db

import (
	"context"
	"database/sql"
	"fmt"

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

// CheckDirect performs a direct lookup to see if a tuple exists
// This is Phase 1 implementation - no recursive expansion
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

	query := `
		SELECT 1 FROM relation_tuples
		WHERE namespace = $1 
		  AND object_id = $2 
		  AND relation = $3
		  AND subject_namespace = $4
		  AND subject_id = $5
		  AND subject_relation = $6
		LIMIT 1
	`

	var exists int
	row := r.db.QueryRowWithZookie(ctx, requiredZookie, query,
		tuple.Namespace,
		tuple.ObjectID,
		tuple.Relation,
		tuple.SubjectNamespace,
		tuple.SubjectID,
		tuple.SubjectRelation, // Empty string for direct users
	)

	err := row.Scan(&exists)
	if err == sql.ErrNoRows {
		// Tuple doesn't exist, get zookie and return false
		zookie, err := r.db.GetZookie(ctx)
		if err != nil {
			span.RecordError(err)
			return false, 0, fmt.Errorf("failed to get zookie: %w", err)
		}
		span.SetAttributes(attribute.Bool("db.found", false))
		observability.AddZookieToSpan(span, zookie)
		return false, zookie, nil
	}
	if err != nil {
		span.RecordError(err)
		return false, 0, fmt.Errorf("failed to check tuple: %w", err)
	}

	// Tuple exists, get zookie and return true
	zookie, err := r.db.GetZookie(ctx)
	if err != nil {
		span.RecordError(err)
		return false, 0, fmt.Errorf("failed to get zookie: %w", err)
	}

	span.SetAttributes(attribute.Bool("db.found", true))
	observability.AddZookieToSpan(span, zookie)
	return true, zookie, nil
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
func (r *TupleRepo) FindDirectSubjects(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*models.Tuple, error) {
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
			return nil, fmt.Errorf("failed to scan direct subject: %w", err)
		}
		tuples = append(tuples, tuple)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating direct subjects: %w", err)
	}

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
// Returns both direct users (subject_relation = '') and nested usersets (subject_relation != '')
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

