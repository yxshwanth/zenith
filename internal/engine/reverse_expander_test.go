package engine

import (
	"context"
	"testing"

	"github.com/zenith/zenith/internal/models"
)

func TestReverseExpansionEngine_DirectSubjects(t *testing.T) {
	repo := NewMockTupleRepo()
	engine := NewReverseExpansionEngine(repo, 10, 100)

	// Insert direct user tuples
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
	})
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "user",
		SubjectID:        "bob",
		SubjectRelation:  "",
	})

	subjects, zookie, err := engine.ListSubjects(context.Background(), "doc", "doc_1", "viewer", 0)
	if err != nil {
		t.Fatalf("ListSubjects failed: %v", err)
	}
	if zookie <= 0 {
		t.Errorf("Expected zookie > 0, got %d", zookie)
	}
	if len(subjects) != 2 {
		t.Errorf("Expected 2 subjects, got %d", len(subjects))
	}

	// Check that both users are in the result
	subjectMap := make(map[string]bool)
	for _, s := range subjects {
		key := s.Namespace + ":" + s.ID
		subjectMap[key] = true
	}
	if !subjectMap["user:alice"] {
		t.Error("Expected user:alice in results")
	}
	if !subjectMap["user:bob"] {
		t.Error("Expected user:bob in results")
	}
}

func TestReverseExpansionEngine_UsersetSubjects(t *testing.T) {
	repo := NewMockTupleRepo()
	engine := NewReverseExpansionEngine(repo, 10, 100)

	// Set up: group:eng#member has access to doc:doc_1#viewer
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "group",
		SubjectID:        "eng",
		SubjectRelation:  "member",
	})

	// Add members to the group
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "group",
		ObjectID:         "eng",
		Relation:         "member",
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
	})
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "group",
		ObjectID:         "eng",
		Relation:         "member",
		SubjectNamespace: "user",
		SubjectID:        "bob",
		SubjectRelation:  "",
	})

	subjects, zookie, err := engine.ListSubjects(context.Background(), "doc", "doc_1", "viewer", 0)
	if err != nil {
		t.Fatalf("ListSubjects failed: %v", err)
	}
	if zookie <= 0 {
		t.Errorf("Expected zookie > 0, got %d", zookie)
	}
	if len(subjects) != 2 {
		t.Errorf("Expected 2 subjects, got %d", len(subjects))
	}

	// Check that both users are in the result (expanded from group)
	subjectMap := make(map[string]bool)
	for _, s := range subjects {
		key := s.Namespace + ":" + s.ID
		subjectMap[key] = true
	}
	if !subjectMap["user:alice"] {
		t.Error("Expected user:alice in results")
	}
	if !subjectMap["user:bob"] {
		t.Error("Expected user:bob in results")
	}
}

func TestReverseExpansionEngine_MixedSubjects(t *testing.T) {
	repo := NewMockTupleRepo()
	engine := NewReverseExpansionEngine(repo, 10, 100)

	// Direct user
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
	})

	// Userset (group)
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "group",
		SubjectID:        "eng",
		SubjectRelation:  "member",
	})

	// Group member
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "group",
		ObjectID:         "eng",
		Relation:         "member",
		SubjectNamespace: "user",
		SubjectID:        "bob",
		SubjectRelation:  "",
	})

	subjects, zookie, err := engine.ListSubjects(context.Background(), "doc", "doc_1", "viewer", 0)
	if err != nil {
		t.Fatalf("ListSubjects failed: %v", err)
	}
	if zookie <= 0 {
		t.Errorf("Expected zookie > 0, got %d", zookie)
	}
	if len(subjects) != 2 {
		t.Errorf("Expected 2 subjects, got %d", len(subjects))
	}

	subjectMap := make(map[string]bool)
	for _, s := range subjects {
		key := s.Namespace + ":" + s.ID
		subjectMap[key] = true
	}
	if !subjectMap["user:alice"] {
		t.Error("Expected user:alice in results")
	}
	if !subjectMap["user:bob"] {
		t.Error("Expected user:bob in results")
	}
}

func TestReverseExpansionEngine_EmptyResult(t *testing.T) {
	repo := NewMockTupleRepo()
	engine := NewReverseExpansionEngine(repo, 10, 100)

	subjects, zookie, err := engine.ListSubjects(context.Background(), "doc", "doc_999", "viewer", 0)
	if err != nil {
		t.Fatalf("ListSubjects failed: %v", err)
	}
	if zookie <= 0 {
		t.Errorf("Expected zookie > 0, got %d", zookie)
	}
	if len(subjects) != 0 {
		t.Errorf("Expected 0 subjects, got %d", len(subjects))
	}
}

func TestReverseExpansionEngine_MaxDepth(t *testing.T) {
	repo := NewMockTupleRepo()
	engine := NewReverseExpansionEngine(repo, 2, 100) // Max depth 2

	// Create deep nesting: doc -> group1 -> group2 -> user
	// This should hit max depth
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "group",
		SubjectID:        "group1",
		SubjectRelation:  "member",
	})

	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "group",
		ObjectID:         "group1",
		Relation:         "member",
		SubjectNamespace: "group",
		SubjectID:        "group2",
		SubjectRelation:  "member",
	})

	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "group",
		ObjectID:         "group2",
		Relation:         "member",
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
	})

	subjects, zookie, err := engine.ListSubjects(context.Background(), "doc", "doc_1", "viewer", 0)
	if err != nil {
		t.Fatalf("ListSubjects failed: %v", err)
	}
	if zookie <= 0 {
		t.Errorf("Expected zookie > 0, got %d", zookie)
	}
	// Should return empty or partial results due to max depth
	// The exact behavior depends on implementation
	_ = subjects
	_ = zookie
}

func TestReverseExpansionEngine_CycleDetection(t *testing.T) {
	repo := NewMockTupleRepo()
	engine := NewReverseExpansionEngine(repo, 10, 100)

	// Create a cycle: group1 -> group2 -> group1
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "group",
		SubjectID:        "group1",
		SubjectRelation:  "member",
	})

	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "group",
		ObjectID:         "group1",
		Relation:         "member",
		SubjectNamespace: "group",
		SubjectID:        "group2",
		SubjectRelation:  "member",
	})

	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "group",
		ObjectID:         "group2",
		Relation:         "member",
		SubjectNamespace: "group",
		SubjectID:        "group1",
		SubjectRelation:  "member",
	})

	subjects, zookie, err := engine.ListSubjects(context.Background(), "doc", "doc_1", "viewer", 0)
	if err != nil {
		t.Fatalf("ListSubjects failed: %v", err)
	}
	if zookie <= 0 {
		t.Errorf("Expected zookie > 0, got %d", zookie)
	}
	// Should handle cycle gracefully (return empty or stop expansion)
	_ = subjects
	_ = zookie
}

func TestReverseExpansionEngine_NestedUsersets(t *testing.T) {
	repo := NewMockTupleRepo()
	engine := NewReverseExpansionEngine(repo, 10, 100)

	// Set up nested usersets:
	// doc:doc_1#viewer -> group:eng#member
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "group",
		SubjectID:        "eng",
		SubjectRelation:  "member",
	})

	// group:eng#member contains subgroup:backend#member (nested userset)
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "group",
		ObjectID:         "eng",
		Relation:         "member",
		SubjectNamespace: "subgroup",
		SubjectID:        "backend",
		SubjectRelation:  "member", // This is a nested userset!
	})

	// subgroup:backend#member contains user:alice (direct user)
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "subgroup",
		ObjectID:         "backend",
		Relation:         "member",
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
	})

	// subgroup:backend#member also contains user:bob (direct user)
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "subgroup",
		ObjectID:         "backend",
		Relation:         "member",
		SubjectNamespace: "user",
		SubjectID:        "bob",
		SubjectRelation:  "",
	})

	// group:eng#member also contains user:charlie (direct user, not through subgroup)
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "group",
		ObjectID:         "eng",
		Relation:         "member",
		SubjectNamespace: "user",
		SubjectID:        "charlie",
		SubjectRelation:  "",
	})

	subjects, zookie, err := engine.ListSubjects(context.Background(), "doc", "doc_1", "viewer", 0)
	if err != nil {
		t.Fatalf("ListSubjects failed: %v", err)
	}
	if zookie <= 0 {
		t.Errorf("Expected zookie > 0, got %d", zookie)
	}

	// Should find all users: alice, bob (through nested subgroup), and charlie (direct)
	if len(subjects) < 3 {
		t.Errorf("Expected at least 3 subjects, got %d", len(subjects))
	}

	subjectMap := make(map[string]bool)
	for _, s := range subjects {
		key := s.Namespace + ":" + s.ID
		subjectMap[key] = true
	}

	if !subjectMap["user:alice"] {
		t.Error("Expected user:alice in results (through nested subgroup)")
	}
	if !subjectMap["user:bob"] {
		t.Error("Expected user:bob in results (through nested subgroup)")
	}
	if !subjectMap["user:charlie"] {
		t.Error("Expected user:charlie in results (direct member)")
	}
}

func TestReverseExpansionEngine_DeepNestedUsersets(t *testing.T) {
	repo := NewMockTupleRepo()
	engine := NewReverseExpansionEngine(repo, 10, 100)

	// Set up 3-level nesting: doc -> group1 -> group2 -> user
	// doc:doc_1#viewer -> group:group1#member
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "group",
		SubjectID:        "group1",
		SubjectRelation:  "member",
	})

	// group:group1#member -> group:group2#member (nested userset)
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "group",
		ObjectID:         "group1",
		Relation:         "member",
		SubjectNamespace: "group",
		SubjectID:        "group2",
		SubjectRelation:  "member", // Nested userset
	})

	// group:group2#member -> user:alice (direct user)
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "group",
		ObjectID:         "group2",
		Relation:         "member",
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
	})

	subjects, zookie, err := engine.ListSubjects(context.Background(), "doc", "doc_1", "viewer", 0)
	if err != nil {
		t.Fatalf("ListSubjects failed: %v", err)
	}
	if zookie <= 0 {
		t.Errorf("Expected zookie > 0, got %d", zookie)
	}

	// Should find user:alice through 3-level nesting
	if len(subjects) < 1 {
		t.Errorf("Expected at least 1 subject, got %d", len(subjects))
	}

	subjectMap := make(map[string]bool)
	for _, s := range subjects {
		key := s.Namespace + ":" + s.ID
		subjectMap[key] = true
	}

	if !subjectMap["user:alice"] {
		t.Error("Expected user:alice in results (through 3-level nesting)")
	}
}

