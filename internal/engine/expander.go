package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zenith/zenith/internal/cache"
	"github.com/zenith/zenith/internal/models"
	"github.com/zenith/zenith/internal/observability"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// visitedSet is a thread-safe set for tracking visited paths
type visitedSet struct {
	mu   sync.RWMutex
	seen map[string]bool
}

func newVisitedSet() *visitedSet {
	return &visitedSet{
		seen: make(map[string]bool),
	}
}

func (vs *visitedSet) contains(key string) bool {
	vs.mu.RLock()
	defer vs.mu.RUnlock()
	return vs.seen[key]
}

func (vs *visitedSet) add(key string) {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.seen[key] = true
}

// Check performs a recursive permission check with userset expansion
func (e *ExpansionEngine) Check(ctx context.Context, req *CheckRequest) (bool, int64, error) {
	// Create span for expansion
	ctx, span := observability.StartSpan(ctx, "RecursiveExpand",
		trace.WithAttributes(
			attribute.String("expand.object", fmt.Sprintf("%s:%s#%s", req.ObjectNamespace, req.ObjectID, req.Relation)),
			attribute.String("expand.subject", fmt.Sprintf("%s:%s", req.SubjectNamespace, req.SubjectID)),
		),
	)
	defer span.End()

	// Create context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, time.Duration(e.timeout)*time.Millisecond)
	defer cancel()

	// Track visited paths to prevent cycles (thread-safe)
	visited := newVisitedSet()

	// Perform recursive expansion
	allowed, zookie, err := e.expand(checkCtx, req, visited, 0)
	if err != nil {
		// If timeout, return false (permission denied)
		if checkCtx.Err() == context.DeadlineExceeded {
			span.SetAttributes(attribute.Bool("expand.timeout", true))
			return false, 0, fmt.Errorf("check timeout exceeded")
		}
		span.RecordError(err)
		return false, 0, err
	}

	span.SetAttributes(
		attribute.Bool("expand.allowed", allowed),
	)
	if zookie > 0 {
		observability.AddZookieToSpan(span, zookie)
	}

	return allowed, zookie, nil
}

// expand performs recursive expansion of usersets
func (e *ExpansionEngine) expand(ctx context.Context, req *CheckRequest, visited *visitedSet, depth int) (bool, int64, error) {
	// Create nested span for this expansion level
	ctx, span := observability.StartSpan(ctx, "RecursiveExpand.Level",
		trace.WithAttributes(
			attribute.Int("expand.depth", depth),
			attribute.String("expand.path", fmt.Sprintf("%s:%s#%s", req.ObjectNamespace, req.ObjectID, req.Relation)),
		),
	)
	defer span.End()

	// Check depth limit
	if depth > e.maxDepth {
		span.SetAttributes(attribute.Bool("expand.max_depth_exceeded", true))
		return false, 0, nil
	}

	// Check for timeout
	select {
	case <-ctx.Done():
		span.SetAttributes(attribute.Bool("expand.timeout", true))
		return false, 0, ctx.Err()
	default:
	}

	// Create path key for cycle detection: object#relation
	pathKey := fmt.Sprintf("%s:%s#%s", req.ObjectNamespace, req.ObjectID, req.Relation)

	// Check for cycles
	if visited.contains(pathKey) {
		// Cycle detected, skip this path
		span.SetAttributes(attribute.Bool("expand.cycle_detected", true))
		return false, 0, nil
	}

	// Mark as visited
	visited.add(pathKey)

	// Fast path: Try direct check first
	tuple := &models.Tuple{
		Namespace:        req.ObjectNamespace,
		ObjectID:         req.ObjectID,
		Relation:         req.Relation,
		SubjectNamespace: req.SubjectNamespace,
		SubjectID:        req.SubjectID,
		SubjectRelation:  req.SubjectRelation,
	}

	// Check cache first (if enabled) with tracing
	if e.cache != nil {
		cacheKey := cache.CheckKey(
			req.SubjectNamespace, req.SubjectID, req.SubjectRelation,
			req.ObjectNamespace, req.ObjectID, req.Relation,
		)
		if entry, ok := e.cache.GetCheckWithContext(ctx, cacheKey); ok {
			// Cache hit - return cached result
			span.AddEvent("cache_hit", trace.WithAttributes(
				attribute.String("cache.key", cacheKey),
				attribute.Bool("cache.allowed", entry.Allowed),
			))
			return entry.Allowed, entry.Zookie, nil
		}
		span.AddEvent("cache_miss", trace.WithAttributes(
			attribute.String("cache.key", cacheKey),
		))
	}

	// Direct check with required_zookie (ensures consistency)
	span.AddEvent("db_query", trace.WithAttributes(
		attribute.String("db.operation", "CheckDirect"),
	))
	allowed, zookie, err := e.repo.CheckDirect(ctx, tuple, req.RequiredZookie)
	if err != nil {
		span.RecordError(err)
		return false, 0, fmt.Errorf("direct check failed: %w", err)
	}
	span.SetAttributes(
		attribute.Bool("db.allowed", allowed),
	)

	cacheResult := func(final bool, zk int64) {
		if e.cache == nil {
			return
		}
		cacheKey := cache.CheckKey(
			req.SubjectNamespace, req.SubjectID, req.SubjectRelation,
			req.ObjectNamespace, req.ObjectID, req.Relation,
		)
		e.cache.SetCheckWithPatterns(
			cacheKey, final, zk,
			req.ObjectNamespace, req.ObjectID, req.Relation,
			req.SubjectNamespace, req.SubjectID, req.SubjectRelation,
		)
		span.AddEvent("cache_set", trace.WithAttributes(
			attribute.String("cache.key", cacheKey),
			attribute.Bool("cache.allowed", final),
		))
	}

	if allowed {
		cacheResult(true, zookie)
		return true, zookie, nil
	}

	span.AddEvent("db_query", trace.WithAttributes(
		attribute.String("db.operation", "FindUsersetDefinitions"),
	))
	usersets, err := e.repo.FindUsersetDefinitions(ctx, req.ObjectNamespace, req.ObjectID, req.Relation, req.RequiredZookie)
	if err != nil {
		span.RecordError(err)
		return false, 0, fmt.Errorf("failed to find userset definitions: %w", err)
	}
	span.SetAttributes(attribute.Int("db.userset_count", len(usersets)))

	if len(usersets) == 0 {
		cacheResult(false, zookie)
		return false, zookie, nil
	}

	allowed, zookie, err = e.expandUsersets(ctx, req, usersets, visited, depth)
	if err != nil {
		return false, zookie, err
	}
	cacheResult(allowed, zookie)
	return allowed, zookie, nil
}

func usersetCheckRequest(req *CheckRequest, us *models.Tuple) *CheckRequest {
	return &CheckRequest{
		SubjectNamespace: req.SubjectNamespace,
		SubjectID:        req.SubjectID,
		SubjectRelation:  req.SubjectRelation,
		ObjectNamespace:  us.SubjectNamespace,
		ObjectID:         us.SubjectID,
		Relation:         us.SubjectRelation,
		RequiredZookie:   req.RequiredZookie,
	}
}

// expandUsersets expands userset definitions. A single userset (the common case)
// recurses on this goroutine; multiple usersets fan out and OR the results.
func (e *ExpansionEngine) expandUsersets(ctx context.Context, req *CheckRequest, usersets []*models.Tuple, visited *visitedSet, depth int) (bool, int64, error) {
	if len(usersets) == 1 {
		return e.expand(ctx, usersetCheckRequest(req, usersets[0]), visited, depth+1)
	}

	resultChan := make(chan expandResult, len(usersets))
	var wg sync.WaitGroup

	for _, userset := range usersets {
		wg.Add(1)
		go func(us *models.Tuple) {
			defer wg.Done()

			allowed, zookie, err := e.expand(ctx, usersetCheckRequest(req, us), visited, depth+1)
			if err != nil {
				if ctx.Err() != context.DeadlineExceeded {
					resultChan <- expandResult{allowed: false, zookie: 0, err: err}
				}
				return
			}
			resultChan <- expandResult{allowed: allowed, zookie: zookie, err: nil}
		}(userset)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var maxZookie int64
	for result := range resultChan {
		if result.err != nil {
			continue
		}
		if result.zookie > maxZookie {
			maxZookie = result.zookie
		}
		if result.allowed {
			return true, maxZookie, nil
		}
	}

	if maxZookie == 0 {
		zookie, err := e.repo.GetZookie(ctx)
		if err != nil {
			return false, 0, fmt.Errorf("failed to get zookie: %w", err)
		}
		if ctx.Err() != nil {
			return false, zookie, ctx.Err()
		}
		return false, zookie, nil
	}
	if ctx.Err() != nil {
		return false, maxZookie, ctx.Err()
	}
	return false, maxZookie, nil
}

// expandResult represents the result of a recursive expansion
type expandResult struct {
	allowed bool
	zookie  int64
	err     error
}
