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

// Subject represents a subject (user or userset) with optional relation
// Alias to cache.Subject for consistency
type Subject = cache.Subject

// subjectSet is a thread-safe set for tracking subjects
type subjectSet struct {
	mu       sync.RWMutex
	subjects map[string]*Subject // Key: "namespace:id:relation"
}

func newSubjectSet() *subjectSet {
	return &subjectSet{
		subjects: make(map[string]*Subject),
	}
}

func (ss *subjectSet) add(subject *Subject) {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	key := fmt.Sprintf("%s:%s:%s", subject.Namespace, subject.ID, subject.Relation)
	ss.subjects[key] = subject
}

func (ss *subjectSet) contains(subject *Subject) bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	key := fmt.Sprintf("%s:%s:%s", subject.Namespace, subject.ID, subject.Relation)
	_, exists := ss.subjects[key]
	return exists
}

func (ss *subjectSet) toSlice() []*Subject {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	result := make([]*Subject, 0, len(ss.subjects))
	for _, subject := range ss.subjects {
		result = append(result, subject)
	}
	return result
}

// ReverseExpansionEngine handles reverse expansion: "Who has access to object X?"
type ReverseExpansionEngine struct {
	repo     TupleRepository
	cache    *cache.Cache // Optional cache for performance
	maxDepth int
	timeout  int64 // timeout in milliseconds
}

// NewReverseExpansionEngine creates a new reverse expansion engine
func NewReverseExpansionEngine(repo TupleRepository, maxDepth int, timeoutMs int64) *ReverseExpansionEngine {
	if maxDepth <= 0 {
		maxDepth = 10 // default max depth
	}
	if timeoutMs <= 0 {
		timeoutMs = 100 // default 100ms timeout (reverse expansion can take longer)
	}
	return &ReverseExpansionEngine{
		repo:     repo,
		maxDepth: maxDepth,
		timeout:  timeoutMs,
	}
}

// NewReverseExpansionEngineWithCache creates a new reverse expansion engine with caching
func NewReverseExpansionEngineWithCache(repo TupleRepository, cache *cache.Cache, maxDepth int, timeoutMs int64) *ReverseExpansionEngine {
	engine := NewReverseExpansionEngine(repo, maxDepth, timeoutMs)
	engine.cache = cache
	return engine
}

// ListSubjects performs reverse expansion to find all subjects with a relation on an object
func (e *ReverseExpansionEngine) ListSubjects(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*Subject, int64, error) {
	// Create span for reverse expansion
	ctx, span := observability.StartSpan(ctx, "ReverseExpand",
		trace.WithAttributes(
			attribute.String("reverse.object", fmt.Sprintf("%s:%s#%s", namespace, objectID, relation)),
		),
	)
	defer span.End()

	if requiredZookie > 0 {
		observability.AddZookieToSpan(span, requiredZookie)
	}

	// Check cache first (if enabled)
	if e.cache != nil {
		cacheKey := cache.ObjectSubjectsKey(namespace, objectID, relation)
		if entry, ok := e.cache.GetObjectSubjects(cacheKey); ok {
			span.SetAttributes(attribute.Bool("reverse.cache_hit", true))
			observability.AddZookieToSpan(span, entry.Zookie)
			return entry.Subjects, entry.Zookie, nil
		}
	}

	// Create context with timeout
	checkCtx, cancel := context.WithTimeout(ctx, time.Duration(e.timeout)*time.Millisecond)
	defer cancel()

	// Track visited paths to prevent cycles
	visited := newVisitedSet()

	// Perform reverse expansion
	subjects, zookie, err := e.expandObjectSubjects(checkCtx, namespace, objectID, relation, requiredZookie, visited, 0)
	if err != nil {
		// If timeout, return partial results
		if checkCtx.Err() == context.DeadlineExceeded {
			span.SetAttributes(attribute.Bool("reverse.timeout", true))
			// Return what we have so far
			if subjects == nil {
				subjects = []*Subject{}
			}
			// Get current zookie
			if zookie == 0 {
				zookie, _ = e.repo.GetZookie(ctx)
			}
		} else {
			span.RecordError(err)
			return nil, 0, err
		}
	}

	// Store in cache (if enabled)
	if e.cache != nil && err == nil {
		cacheKey := cache.ObjectSubjectsKey(namespace, objectID, relation)
		// Convert engine.Subject to cache.Subject (they're the same type now)
		cacheSubjects := make([]*cache.Subject, len(subjects))
		for i, s := range subjects {
			cacheSubjects[i] = s
		}
		e.cache.SetObjectSubjects(cacheKey, cacheSubjects, zookie)
	}

	span.SetAttributes(
		attribute.Int("reverse.subject_count", len(subjects)),
	)
	if zookie > 0 {
		observability.AddZookieToSpan(span, zookie)
	}

	return subjects, zookie, nil
}

// expandObjectSubjects performs reverse expansion starting from an object
func (e *ReverseExpansionEngine) expandObjectSubjects(ctx context.Context, namespace, objectID, relation string, requiredZookie int64, visited *visitedSet, depth int) ([]*Subject, int64, error) {
	// Create nested span for this expansion level
	ctx, span := observability.StartSpan(ctx, "ReverseExpand.Level",
		trace.WithAttributes(
			attribute.Int("reverse.depth", depth),
			attribute.String("reverse.path", fmt.Sprintf("%s:%s#%s", namespace, objectID, relation)),
		),
	)
	defer span.End()

	// Check depth limit
	if depth > e.maxDepth {
		span.SetAttributes(attribute.Bool("reverse.max_depth_exceeded", true))
		return []*Subject{}, 0, nil
	}

	// Check for timeout
	select {
	case <-ctx.Done():
		span.SetAttributes(attribute.Bool("reverse.timeout", true))
		return nil, 0, ctx.Err()
	default:
	}

	// Create path key for cycle detection
	pathKey := fmt.Sprintf("%s:%s#%s", namespace, objectID, relation)

	// Check for cycles
	if visited.contains(pathKey) {
		span.SetAttributes(attribute.Bool("reverse.cycle_detected", true))
		return []*Subject{}, 0, nil
	}

	// Mark as visited
	visited.add(pathKey)

	// Collect all subjects
	subjectSet := newSubjectSet()
	var maxZookie int64

	// 1. Find direct subjects (users)
	directSubjects, err := e.repo.FindDirectSubjects(ctx, namespace, objectID, relation, requiredZookie)
	if err != nil {
		span.RecordError(err)
		return nil, 0, fmt.Errorf("failed to find direct subjects: %w", err)
	}

	for _, tuple := range directSubjects {
		subject := &Subject{
			Namespace: tuple.SubjectNamespace,
			ID:        tuple.SubjectID,
			Relation:  "", // Direct users have no relation
		}
		subjectSet.add(subject)
		// Get zookie from tuple (would need to query, but for now use requiredZookie)
		if requiredZookie > maxZookie {
			maxZookie = requiredZookie
		}
	}

	// 2. Find userset subjects and expand them
	usersetSubjects, err := e.repo.FindUsersetSubjects(ctx, namespace, objectID, relation, requiredZookie)
	if err != nil {
		span.RecordError(err)
		return nil, 0, fmt.Errorf("failed to find userset subjects: %w", err)
	}

	// Expand each userset concurrently
	if len(usersetSubjects) > 0 {
		members, zookie, err := e.expandUsersetMembers(ctx, usersetSubjects, requiredZookie, visited, depth)
		if err != nil {
			// Continue with partial results
			span.RecordError(err)
		}
		// Add all members to the set
		for _, member := range members {
			subjectSet.add(member)
		}
		if zookie > maxZookie {
			maxZookie = zookie
		}
	}

	// Get final zookie if we don't have one
	if maxZookie == 0 {
		maxZookie, _ = e.repo.GetZookie(ctx)
	}

	return subjectSet.toSlice(), maxZookie, nil
}

// expandUsersetMembers recursively expands userset members
func (e *ReverseExpansionEngine) expandUsersetMembers(ctx context.Context, usersets []*models.Tuple, requiredZookie int64, visited *visitedSet, depth int) ([]*Subject, int64, error) {
	// Create span for userset member expansion
	ctx, span := observability.StartSpan(ctx, "ExpandUsersetMembers",
		trace.WithAttributes(
			attribute.Int("reverse.userset_count", len(usersets)),
		),
	)
	defer span.End()

	// Use a channel to collect results
	resultChan := make(chan expandMembersResult, len(usersets))
	var wg sync.WaitGroup

	// Spawn goroutine for each userset
	for _, userset := range usersets {
		wg.Add(1)
		go func(us *models.Tuple) {
			defer wg.Done()

			// Find direct members of this userset
			members, err := e.repo.FindUsersetMembers(ctx, us.SubjectNamespace, us.SubjectID, us.SubjectRelation, requiredZookie)
			if err != nil {
				resultChan <- expandMembersResult{members: []*Subject{}, zookie: 0, err: err}
				return
			}

			// Convert tuples to subjects and separate direct users from nested usersets
			directSubjects := make([]*Subject, 0, len(members))
			nestedUsersets := make([]*models.Tuple, 0)
			
			for _, member := range members {
				if member.SubjectRelation == "" {
					// Direct user member
					subject := &Subject{
						Namespace: member.SubjectNamespace,
						ID:        member.SubjectID,
						Relation:  "",
					}
					directSubjects = append(directSubjects, subject)
				} else {
					// Nested userset - needs recursive expansion
					nestedUsersets = append(nestedUsersets, member)
				}
			}

			// Collect all members (start with direct subjects)
			allMembers := make([]*Subject, 0, len(directSubjects))
			allMembers = append(allMembers, directSubjects...)

			// Recursively expand nested usersets
			if len(nestedUsersets) > 0 {
				// For each nested userset, we need to find all subjects that have
				// the userset's relation on the userset's object
				// This is done by calling expandObjectSubjects recursively
				for _, nestedUserset := range nestedUsersets {
					// Recursively expand: find all subjects with nestedUserset.SubjectRelation
					// on nestedUserset.SubjectNamespace:nestedUserset.SubjectID
					nestedSubjects, nestedZookie, nestedErr := e.expandObjectSubjects(
						ctx,
						nestedUserset.SubjectNamespace,
						nestedUserset.SubjectID,
						nestedUserset.SubjectRelation,
						requiredZookie,
						visited,
						depth+1,
					)
					
					if nestedErr != nil {
						// Log error but continue with other nested usersets
						if ctx.Err() != context.DeadlineExceeded {
							// Only log non-timeout errors
							continue
						}
					}
					
					// Add nested subjects to all members
					allMembers = append(allMembers, nestedSubjects...)
					
					// Track max zookie
					if nestedZookie > requiredZookie {
						requiredZookie = nestedZookie
					}
				}
			}

			resultChan <- expandMembersResult{members: allMembers, zookie: requiredZookie, err: nil}
		}(userset)
	}

	// Close result channel when all goroutines complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect all results
	allMembers := newSubjectSet()
	var maxZookie int64
	var lastErr error

	for result := range resultChan {
		if result.err != nil {
			lastErr = result.err
			continue
		}
		for _, member := range result.members {
			allMembers.add(member)
		}
		if result.zookie > maxZookie {
			maxZookie = result.zookie
		}
	}

	if maxZookie == 0 {
		maxZookie = requiredZookie
	}

	return allMembers.toSlice(), maxZookie, lastErr
}

// expandMembersResult holds the result of expanding userset members
type expandMembersResult struct {
	members []*Subject
	zookie  int64
	err     error
}

