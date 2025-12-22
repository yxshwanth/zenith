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

	// Check cache first (if enabled)
	if e.cache != nil {
		cacheKey := cache.CheckKey(
			req.SubjectNamespace, req.SubjectID, req.SubjectRelation,
			req.ObjectNamespace, req.ObjectID, req.Relation,
		)
		if entry, ok := e.cache.GetCheck(cacheKey); ok {
			// Cache hit - return cached result
			return entry.Allowed, entry.Zookie, nil
		}
	}

	// Direct check with required_zookie (ensures consistency)
	allowed, zookie, err := e.repo.CheckDirect(ctx, tuple, req.RequiredZookie)
	if err != nil {
		return false, 0, fmt.Errorf("direct check failed: %w", err)
	}

	// Store in cache (if enabled)
	if e.cache != nil {
		cacheKey := cache.CheckKey(
			req.SubjectNamespace, req.SubjectID, req.SubjectRelation,
			req.ObjectNamespace, req.ObjectID, req.Relation,
		)
		// Use SetCheckWithPatterns to enable selective invalidation
		e.cache.SetCheckWithPatterns(
			cacheKey, allowed, zookie,
			req.ObjectNamespace, req.ObjectID, req.Relation,
			req.SubjectNamespace, req.SubjectID, req.SubjectRelation,
		)
	}

	if allowed {
		// Return with zookie from direct check
		// This zookie respects the required_zookie via AS OF SYSTEM TIME MAX
		return true, zookie, nil
	}

	// Direct check failed, look for userset definitions
	// All queries use the same required_zookie for consistency
	usersets, err := e.repo.FindUsersetDefinitions(ctx, req.ObjectNamespace, req.ObjectID, req.Relation, req.RequiredZookie)
	if err != nil {
		return false, 0, fmt.Errorf("failed to find userset definitions: %w", err)
	}

	// No userset definitions found, permission denied
	// Return zookie from direct check for consistency (even though it was false)
	if len(usersets) == 0 {
		// Use the zookie from the direct check to maintain consistency
		// This ensures the zookie reflects the state at the time of the check
		return false, zookie, nil
	}

	// Recursively check each userset definition concurrently
	return e.expandUsersets(ctx, req, usersets, visited, depth)
}

// expandUsersets expands multiple userset definitions concurrently
func (e *ExpansionEngine) expandUsersets(ctx context.Context, req *CheckRequest, usersets []*models.Tuple, visited *visitedSet, depth int) (bool, int64, error) {
	// Use a channel to collect results
	resultChan := make(chan expandResult, len(usersets))
	var wg sync.WaitGroup

	// Spawn goroutine for each userset definition
	for _, userset := range usersets {
		wg.Add(1)
		go func(us *models.Tuple) {
			defer wg.Done()

			// Create new check request for the userset
			// We're checking if the subject has the userset's relation on the userset's object
			newReq := &CheckRequest{
				SubjectNamespace: req.SubjectNamespace,
				SubjectID:        req.SubjectID,
				SubjectRelation:  req.SubjectRelation,
				ObjectNamespace: us.SubjectNamespace,
				ObjectID:         us.SubjectID,
				Relation:         us.SubjectRelation, // The relation from the userset
				RequiredZookie:   req.RequiredZookie,
			}

			// Recursively expand
			allowed, zookie, err := e.expand(ctx, newReq, visited, depth+1)
			if err != nil {
				// Only send error if not a timeout (timeouts are expected in some cases)
				if ctx.Err() != context.DeadlineExceeded {
					resultChan <- expandResult{allowed: false, zookie: 0, err: err}
				}
				return
			}

			// If allowed, send result immediately (early termination)
			if allowed {
				resultChan <- expandResult{allowed: true, zookie: zookie, err: nil}
			}
		}(userset)
	}

	// Close result channel when all goroutines complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect first true result (OR logic: any path grants permission)
	// Track max zookie across all paths for consistency
	var maxZookie int64
	for result := range resultChan {
		if result.err != nil {
			// Log error but continue checking other paths
			continue
		}
		// Track max zookie for consistency across all paths
		if result.zookie > maxZookie {
			maxZookie = result.zookie
		}
		if result.allowed {
			// Found a path that grants permission
			// Return with max zookie seen so far (ensures consistency)
			// This ensures all concurrent paths contribute to the final zookie
			return true, maxZookie, nil
		}
	}

	// Check if context was cancelled
	select {
	case <-ctx.Done():
		zookie, err := e.repo.GetZookie(ctx)
		if err != nil {
			return false, 0, fmt.Errorf("failed to get zookie: %w", err)
		}
		return false, zookie, ctx.Err()
	default:
	}

	// No path granted permission, return false with max zookie
	if maxZookie == 0 {
		zookie, err := e.repo.GetZookie(ctx)
		if err != nil {
			return false, 0, fmt.Errorf("failed to get zookie: %w", err)
		}
		return false, zookie, nil
	}

	return false, maxZookie, nil
}

// expandResult represents the result of a recursive expansion
type expandResult struct {
	allowed bool
	zookie  int64
	err     error
}


