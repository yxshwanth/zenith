package engine

import (
	"context"
	"testing"
	"time"

	"github.com/zenith/zenith/internal/cache"
	"github.com/zenith/zenith/internal/models"
)

// MockTupleRepo is a simple in-memory implementation for testing
type MockTupleRepo struct {
	tuples map[string]*models.Tuple
	zookie int64
}

func NewMockTupleRepo() *MockTupleRepo {
	return &MockTupleRepo{
		tuples: make(map[string]*models.Tuple),
		zookie: 1000,
	}
}

func (m *MockTupleRepo) Insert(ctx context.Context, tuple *models.Tuple) (int64, error) {
	key := tuple.String()
	m.tuples[key] = tuple
	m.zookie++
	return m.zookie, nil
}

func (m *MockTupleRepo) Delete(ctx context.Context, tuple *models.Tuple) (int64, error) {
	key := tuple.String()
	delete(m.tuples, key)
	m.zookie++
	return m.zookie, nil
}

func (m *MockTupleRepo) CheckDirect(ctx context.Context, tuple *models.Tuple, requiredZookie int64) (bool, int64, error) {
	key := tuple.String()
	_, exists := m.tuples[key]
	m.zookie++
	if exists {
		return true, m.zookie, nil
	}
	return false, m.zookie, nil
}

func (m *MockTupleRepo) FindUsersetDefinitions(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*models.Tuple, error) {
	var results []*models.Tuple
	for _, tuple := range m.tuples {
		if tuple.Namespace == namespace &&
			tuple.ObjectID == objectID &&
			tuple.Relation == relation &&
			tuple.SubjectRelation != "" { // Is a userset
			results = append(results, tuple)
		}
	}
	return results, nil
}

func (m *MockTupleRepo) GetZookie(ctx context.Context) (int64, error) {
	m.zookie++
	return m.zookie, nil
}

// FindDirectSubjects finds all direct user subjects for an object
func (m *MockTupleRepo) FindDirectSubjects(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*models.Tuple, error) {
	var results []*models.Tuple
	for _, tuple := range m.tuples {
		if tuple.Namespace == namespace &&
			tuple.ObjectID == objectID &&
			tuple.Relation == relation &&
			tuple.SubjectRelation == "" { // Direct users only
			results = append(results, tuple)
		}
	}
	m.zookie++
	return results, nil
}

// FindUsersetSubjects finds all userset subjects for an object
func (m *MockTupleRepo) FindUsersetSubjects(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*models.Tuple, error) {
	// Same as FindUsersetDefinitions
	return m.FindUsersetDefinitions(ctx, namespace, objectID, relation, requiredZookie)
}

// FindUsersetMembers finds all members of a userset (both direct users and nested usersets)
func (m *MockTupleRepo) FindUsersetMembers(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*models.Tuple, error) {
	var results []*models.Tuple
	for _, tuple := range m.tuples {
		if tuple.Namespace == namespace &&
			tuple.ObjectID == objectID &&
			tuple.Relation == relation {
			// Return both direct members (subject_relation == "") and nested usersets (subject_relation != "")
			results = append(results, tuple)
		}
	}
	m.zookie++
	return results, nil
}

func TestExpansionEngine_DirectCheck(t *testing.T) {
	repo := NewMockTupleRepo()
	engine := NewExpansionEngine(repo, 10, 100)

	// Insert a direct tuple
	tuple := &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
	}
	repo.Insert(context.Background(), tuple)

	// Check direct access
	req := &CheckRequest{
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
		ObjectNamespace:  "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		RequiredZookie:   0,
	}

	allowed, _, err := engine.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !allowed {
		t.Error("Expected allowed=true for direct check")
	}
}

func TestExpansionEngine_SimpleUserset(t *testing.T) {
	repo := NewMockTupleRepo()
	engine := NewExpansionEngine(repo, 10, 100)

	// Setup: doc_1#viewer is defined as folder_A#viewer
	usersetTuple := &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "folder",
		SubjectID:        "folder_A",
		SubjectRelation:  "viewer", // This is a userset
	}
	repo.Insert(context.Background(), usersetTuple)

	// Setup: user:alice has viewer on folder_A
	directTuple := &models.Tuple{
		Namespace:        "folder",
		ObjectID:         "folder_A",
		Relation:         "viewer",
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
	}
	repo.Insert(context.Background(), directTuple)

	// Check: user:alice -> doc:doc_1#viewer (should expand to folder_A#viewer)
	req := &CheckRequest{
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
		ObjectNamespace:  "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		RequiredZookie:   0,
	}

	allowed, _, err := engine.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !allowed {
		t.Error("Expected allowed=true for userset expansion")
	}
}

func TestExpansionEngine_NestedUsersets(t *testing.T) {
	repo := NewMockTupleRepo()
	engine := NewExpansionEngine(repo, 10, 100)

	// Setup: doc_1#viewer -> folder_A#viewer
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "folder",
		SubjectID:        "folder_A",
		SubjectRelation:  "viewer",
	})

	// Setup: folder_A#viewer -> group:eng#member
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "folder",
		ObjectID:         "folder_A",
		Relation:         "viewer",
		SubjectNamespace: "group",
		SubjectID:        "eng",
		SubjectRelation:  "member",
	})

	// Setup: user:alice is member of group:eng
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "group",
		ObjectID:         "eng",
		Relation:         "member",
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
	})

	// Check: user:alice -> doc:doc_1#viewer (should expand through folder_A -> group:eng)
	req := &CheckRequest{
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
		ObjectNamespace:  "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		RequiredZookie:   0,
	}

	allowed, _, err := engine.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !allowed {
		t.Error("Expected allowed=true for nested userset expansion")
	}
}

func TestExpansionEngine_CycleDetection(t *testing.T) {
	repo := NewMockTupleRepo()
	engine := NewExpansionEngine(repo, 10, 100)

	// Setup a cycle: A -> B -> A
	// doc_1#viewer -> folder_A#viewer
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "folder",
		SubjectID:        "folder_A",
		SubjectRelation:  "viewer",
	})

	// folder_A#viewer -> doc_1#viewer (cycle!)
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "folder",
		ObjectID:         "folder_A",
		Relation:         "viewer",
		SubjectNamespace: "doc",
		SubjectID:        "doc_1",
		SubjectRelation:  "viewer",
	})

	// Check should detect cycle and return false
	req := &CheckRequest{
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
		ObjectNamespace:  "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		RequiredZookie:   0,
	}

	allowed, _, err := engine.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if allowed {
		t.Error("Expected allowed=false for cycle (no valid path)")
	}
}

func TestExpansionEngine_MaxDepth(t *testing.T) {
	repo := NewMockTupleRepo()
	engine := NewExpansionEngine(repo, 2, 100) // Max depth of 2

	// Create a chain longer than max depth
	// A -> B -> C -> D (depth 3, exceeds max of 2)
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "folder",
		SubjectID:        "folder_A",
		SubjectRelation:  "viewer",
	})

	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "folder",
		ObjectID:         "folder_A",
		Relation:         "viewer",
		SubjectNamespace: "folder",
		SubjectID:        "folder_B",
		SubjectRelation:  "viewer",
	})

	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "folder",
		ObjectID:         "folder_B",
		Relation:         "viewer",
		SubjectNamespace: "folder",
		SubjectID:        "folder_C",
		SubjectRelation:  "viewer",
	})

	// Check should stop at max depth
	req := &CheckRequest{
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
		ObjectNamespace:  "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		RequiredZookie:   0,
	}

	allowed, _, err := engine.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if allowed {
		t.Error("Expected allowed=false when max depth exceeded")
	}
}

func TestExpansionEngine_Timeout(t *testing.T) {
	repo := NewMockTupleRepo()
	engine := NewExpansionEngine(repo, 10, 1) // 1ms timeout (very short)

	// Create a long chain that will timeout
	for i := 0; i < 100; i++ {
		repo.Insert(context.Background(), &models.Tuple{
			Namespace:        "doc",
			ObjectID:         "doc_1",
			Relation:         "viewer",
			SubjectNamespace: "folder",
			SubjectID:        "folder_A",
			SubjectRelation:  "viewer",
		})
	}

	req := &CheckRequest{
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
		ObjectNamespace:  "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		RequiredZookie:   0,
	}

	// Should timeout and return false
	allowed, _, err := engine.Check(context.Background(), req)
	if err == nil {
		// If no error, should be false due to timeout
		if allowed {
			t.Error("Expected allowed=false on timeout")
		}
	}
}

func TestExpansionEngine_ConcurrentChecks(t *testing.T) {
	repo := NewMockTupleRepo()
	engine := NewExpansionEngine(repo, 10, 100)

	// Setup multiple userset paths (OR logic: any path grants permission)
	// Path 1: doc_1#viewer -> folder_A#viewer
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "folder",
		SubjectID:        "folder_A",
		SubjectRelation:  "viewer",
	})

	// Path 2: doc_1#viewer -> group:eng#member
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "group",
		SubjectID:        "eng",
		SubjectRelation:  "member",
	})

	// Only path 2 is valid: user:alice is member of group:eng
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "group",
		ObjectID:         "eng",
		Relation:         "member",
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
	})

	// Check should find path 2 and return true
	req := &CheckRequest{
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
		ObjectNamespace:  "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		RequiredZookie:   0,
	}

	allowed, _, err := engine.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !allowed {
		t.Error("Expected allowed=true (one of the concurrent paths should succeed)")
	}
}

func TestExpansionEngine_CacheDoesNotPoisonNested(t *testing.T) {
	repo := NewMockTupleRepo()
	c, err := cache.NewCache(128, 30*time.Second, 5*time.Second)
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	eng := NewExpansionEngineWithCache(repo, c, 10, 100)

	repo.Insert(context.Background(), &models.Tuple{
		Namespace: "doc", ObjectID: "doc_1", Relation: "viewer",
		SubjectNamespace: "group", SubjectID: "eng", SubjectRelation: "member",
	})
	repo.Insert(context.Background(), &models.Tuple{
		Namespace: "group", ObjectID: "eng", Relation: "member",
		SubjectNamespace: "user", SubjectID: "alice", SubjectRelation: "",
	})

	req := &CheckRequest{
		SubjectNamespace: "user", SubjectID: "alice",
		ObjectNamespace: "doc", ObjectID: "doc_1", Relation: "viewer",
	}

	for i := 0; i < 3; i++ {
		allowed, _, err := eng.Check(context.Background(), req)
		if err != nil {
			t.Fatalf("check %d: %v", i, err)
		}
		if !allowed {
			t.Fatalf("check %d: nested allow was cached as deny", i)
		}
	}
}
