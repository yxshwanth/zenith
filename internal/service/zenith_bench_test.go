package service

import (
	"context"
	"testing"

	"github.com/zenith/zenith/internal/api"
	"github.com/zenith/zenith/internal/engine"
	"github.com/zenith/zenith/internal/models"
)

// BenchmarkCheck_Direct benchmarks direct permission checks
func BenchmarkCheck_Direct(b *testing.B) {
	repo := newCountingMockRepo()
	
	// Insert a tuple
	tuple := &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
	}
	repo.Insert(context.Background(), tuple)

	expander := engine.NewExpansionEngine(repo, 10, 100)
	checkReq := &engine.CheckRequest{
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
		ObjectNamespace: "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		RequiredZookie:   0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = expander.Check(context.Background(), checkReq)
	}
}

// BenchmarkCheck_Nested benchmarks nested userset checks
func BenchmarkCheck_Nested(b *testing.B) {
	repo := newCountingMockRepo()
	
	// Set up 3-level nesting
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
		SubjectNamespace: "group",
		SubjectID:        "eng",
		SubjectRelation:  "member",
	})
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "group",
		ObjectID:         "eng",
		Relation:         "member",
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
	})

	expander := engine.NewExpansionEngine(repo, 10, 100)
	checkReq := &engine.CheckRequest{
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
		ObjectNamespace: "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		RequiredZookie:   0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = expander.Check(context.Background(), checkReq)
	}
}

// BenchmarkCheckKey benchmarks checkKey generation
func BenchmarkCheckKey(b *testing.B) {
	req := &api.CheckRequest{
		SubjectNamespace: "user",
		SubjectId:         "alice",
		SubjectRelation:   "",
		Namespace:        "doc",
		ObjectId:         "doc_1",
		Relation:         "viewer",
		RequiredZookie:   0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = checkKey(req)
	}
}

