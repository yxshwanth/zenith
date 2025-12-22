package engine

import (
	"context"
	"testing"

	"github.com/zenith/zenith/internal/models"
)

// BenchmarkExpansion_Direct benchmarks direct tuple checks
func BenchmarkExpansion_Direct(b *testing.B) {
	repo := NewMockTupleRepo()
	tuple := &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
	}
	repo.Insert(context.Background(), tuple)

	engine := NewExpansionEngine(repo, 10, 100)
	req := &CheckRequest{
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
		_, _, _ = engine.Check(context.Background(), req)
	}
}

// BenchmarkExpansion_2Level benchmarks 2-level userset expansion
func BenchmarkExpansion_2Level(b *testing.B) {
	repo := NewMockTupleRepo()
	
	repo.Insert(context.Background(), &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
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

	engine := NewExpansionEngine(repo, 10, 100)
	req := &CheckRequest{
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
		_, _, _ = engine.Check(context.Background(), req)
	}
}

// BenchmarkExpansion_3Level benchmarks 3-level userset expansion
func BenchmarkExpansion_3Level(b *testing.B) {
	repo := NewMockTupleRepo()
	
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

	engine := NewExpansionEngine(repo, 10, 100)
	req := &CheckRequest{
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
		_, _, _ = engine.Check(context.Background(), req)
	}
}

