package engine

import (
	"context"

	"github.com/zenith/zenith/internal/cache"
	"github.com/zenith/zenith/internal/db"
	"github.com/zenith/zenith/internal/models"
)

// CheckRequest represents a permission check request for the expansion engine
type CheckRequest struct {
	SubjectNamespace string
	SubjectID        string
	SubjectRelation  string
	ObjectNamespace string
	ObjectID         string
	Relation         string
	RequiredZookie   int64
}

// TupleRepository defines the interface for tuple operations needed by the expansion engine
type TupleRepository interface {
	// Forward expansion methods
	CheckDirect(ctx context.Context, tuple *models.Tuple, requiredZookie int64) (bool, int64, error)
	FindUsersetDefinitions(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*models.Tuple, error)
	GetZookie(ctx context.Context) (int64, error)
	
	// Reverse expansion methods
	FindDirectSubjects(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*models.Tuple, error)
	FindUsersetSubjects(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*models.Tuple, error)
	FindUsersetMembers(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*models.Tuple, error)
}

// ExpansionEngine handles recursive permission checks with userset expansion
type ExpansionEngine struct {
	repo     TupleRepository
	cache    *cache.Cache // Optional cache for performance
	maxDepth int
	timeout  int64 // timeout in milliseconds
}

// NewExpansionEngine creates a new expansion engine
func NewExpansionEngine(repo TupleRepository, maxDepth int, timeoutMs int64) *ExpansionEngine {
	if maxDepth <= 0 {
		maxDepth = 10 // default max depth
	}
	if timeoutMs <= 0 {
		timeoutMs = 10 // default 10ms timeout
	}
	return &ExpansionEngine{
		repo:     repo,
		maxDepth: maxDepth,
		timeout:  timeoutMs,
	}
}

// NewExpansionEngineWithCache creates a new expansion engine with caching
func NewExpansionEngineWithCache(repo TupleRepository, cache *cache.Cache, maxDepth int, timeoutMs int64) *ExpansionEngine {
	engine := NewExpansionEngine(repo, maxDepth, timeoutMs)
	engine.cache = cache
	return engine
}

// NewExpansionEngineFromRepo creates a new expansion engine from a db.TupleRepo
func NewExpansionEngineFromRepo(repo *db.TupleRepo, maxDepth int, timeoutMs int64) *ExpansionEngine {
	return NewExpansionEngine(repo, maxDepth, timeoutMs)
}

// NewExpansionEngineFromRepoWithCache creates a new expansion engine from a db.TupleRepo with caching
func NewExpansionEngineFromRepoWithCache(repo *db.TupleRepo, cache *cache.Cache, maxDepth int, timeoutMs int64) *ExpansionEngine {
	return NewExpansionEngineWithCache(repo, cache, maxDepth, timeoutMs)
}

