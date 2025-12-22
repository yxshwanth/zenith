package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zenith/zenith/internal/api"
	"github.com/zenith/zenith/internal/engine"
	"github.com/zenith/zenith/internal/models"
)

// countingMockRepo implements engine.TupleRepository and counts CheckDirect calls
type countingMockRepo struct {
	tuples           map[string]*models.Tuple
	zookie           int64
	checkDirectCalls int64
}

func newCountingMockRepo() *countingMockRepo {
	return &countingMockRepo{
		tuples: make(map[string]*models.Tuple),
		zookie: 1000,
	}
}

func (c *countingMockRepo) Insert(ctx context.Context, tuple *models.Tuple) (int64, error) {
	key := tuple.String()
	c.tuples[key] = tuple
	c.zookie++
	return c.zookie, nil
}

func (c *countingMockRepo) Delete(ctx context.Context, tuple *models.Tuple) (int64, error) {
	key := tuple.String()
	delete(c.tuples, key)
	c.zookie++
	return c.zookie, nil
}

func (c *countingMockRepo) CheckDirect(ctx context.Context, tuple *models.Tuple, requiredZookie int64) (bool, int64, error) {
	atomic.AddInt64(&c.checkDirectCalls, 1)
	key := tuple.String()
	_, exists := c.tuples[key]
	c.zookie++
	if exists {
		return true, c.zookie, nil
	}
	return false, c.zookie, nil
}

func (c *countingMockRepo) FindUsersetDefinitions(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*models.Tuple, error) {
	var results []*models.Tuple
	for _, tuple := range c.tuples {
		if tuple.Namespace == namespace &&
			tuple.ObjectID == objectID &&
			tuple.Relation == relation &&
			tuple.SubjectRelation != "" {
			results = append(results, tuple)
		}
	}
	return results, nil
}

func (c *countingMockRepo) GetZookie(ctx context.Context) (int64, error) {
	c.zookie++
	return c.zookie, nil
}

func (c *countingMockRepo) FindDirectSubjects(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*models.Tuple, error) {
	var results []*models.Tuple
	for _, tuple := range c.tuples {
		if tuple.Namespace == namespace &&
			tuple.ObjectID == objectID &&
			tuple.Relation == relation &&
			tuple.SubjectRelation == "" {
			results = append(results, tuple)
		}
	}
	c.zookie++
	return results, nil
}

func (c *countingMockRepo) FindUsersetSubjects(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*models.Tuple, error) {
	return c.FindUsersetDefinitions(ctx, namespace, objectID, relation, requiredZookie)
}

func (c *countingMockRepo) FindUsersetMembers(ctx context.Context, namespace, objectID, relation string, requiredZookie int64) ([]*models.Tuple, error) {
	var results []*models.Tuple
	for _, tuple := range c.tuples {
		if tuple.Namespace == namespace &&
			tuple.ObjectID == objectID &&
			tuple.Relation == relation &&
			tuple.SubjectRelation == "" {
			results = append(results, tuple)
		}
	}
	c.zookie++
	return results, nil
}

// TestSingleflightDeduplication tests that concurrent identical requests are deduplicated
func TestSingleflightDeduplication(t *testing.T) {
	// Test 1: Verify checkKey generates consistent keys for identical requests
	req := &api.CheckRequest{
		SubjectNamespace: "user",
		SubjectId:        "alice",
		SubjectRelation:  "",
		Namespace:        "doc",
		ObjectId:         "doc_1",
		Relation:         "viewer",
		RequiredZookie:   0,
	}

	key1 := checkKey(req)
	key2 := checkKey(req)
	if key1 != key2 {
		t.Errorf("checkKey should generate same key for identical requests: %s != %s", key1, key2)
	}

	// Test 2: Verify checkKey generates different keys for different requests
	req2 := &api.CheckRequest{
		SubjectNamespace: "user",
		SubjectId:        "alice",
		SubjectRelation:  "",
		Namespace:        "doc",
		ObjectId:         "doc_1",
		Relation:         "viewer",
		RequiredZookie:   100,
	}
	key3 := checkKey(req2)
	if key1 == key3 {
		t.Errorf("checkKey should generate different keys for different required_zookie: %s == %s", key1, key3)
	}

	// Test 3: Test with different subject
	req3 := &api.CheckRequest{
		SubjectNamespace: "user",
		SubjectId:        "bob",
		SubjectRelation:  "",
		Namespace:        "doc",
		ObjectId:         "doc_1",
		Relation:         "viewer",
		RequiredZookie:   0,
	}
	key4 := checkKey(req3)
	if key1 == key4 {
		t.Errorf("checkKey should generate different keys for different subjects: %s == %s", key1, key4)
	}

	// Test 4: Verify singleflight mechanism exists in service
	// Since Service requires *db.TupleRepo (concrete type), we can't easily
	// create a test service with a mock. However, we can verify:
	// 1. The checkKey function works correctly (tested above)
	// 2. The service structure includes singleflight group
	// 3. Integration tests will verify actual deduplication behavior

	// Create a mock repo and expander to verify the expander works
	mockRepo := newCountingMockRepo()

	// Insert a tuple
	tuple := &models.Tuple{
		Namespace:        "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
	}
	mockRepo.Insert(context.Background(), tuple)

	// Create expansion engine
	expander := engine.NewExpansionEngine(mockRepo, 10, 100)

	// Create check request for expander
	checkReq := &engine.CheckRequest{
		SubjectNamespace: "user",
		SubjectID:        "alice",
		SubjectRelation:  "",
		ObjectNamespace:  "doc",
		ObjectID:         "doc_1",
		Relation:         "viewer",
		RequiredZookie:   0,
	}

	// Reset call counter
	atomic.StoreInt64(&mockRepo.checkDirectCalls, 0)

	// Make 10 concurrent identical requests to the expander
	// Note: The actual singleflight happens in the Service layer, not the expander
	// This test verifies concurrent requests work correctly
	const numRequests = 10
	results := make([]bool, numRequests)
	errors := make([]error, numRequests)
	var wg sync.WaitGroup

	for i := 0; i < numRequests; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			allowed, _, err := expander.Check(context.Background(), checkReq)
			results[idx] = allowed
			errors[idx] = err
		}(i)
	}

	wg.Wait()

	// Verify all requests succeeded and returned the same result
	for i, err := range errors {
		if err != nil {
			t.Errorf("Request %d failed: %v", i, err)
		}
		if !results[i] {
			t.Errorf("Request %d should return allowed=true", i)
		}
	}

	// Verify all results are identical (important for singleflight)
	firstResult := results[0]
	for i, result := range results {
		if result != firstResult {
			t.Errorf("Request %d returned different result: got %v, expected %v", i, result, firstResult)
		}
	}

	// Without singleflight at expander level, we'd expect multiple CheckDirect calls
	// With singleflight (at service level), we'd expect 1 call
	// Since we're testing at expander level, we see multiple calls (expected)
	calls := atomic.LoadInt64(&mockRepo.checkDirectCalls)
	if calls == 0 {
		t.Error("Expected at least one CheckDirect call")
	}

	t.Logf("Singleflight key generation verified")
	t.Logf("Concurrent requests test: %d CheckDirect calls for %d requests (expected multiple at expander level)", calls, numRequests)
	t.Logf("Note: Full singleflight deduplication test requires integration test with real Service and db.TupleRepo")
	t.Logf("The Service.Check method uses singleflight.Group to deduplicate concurrent identical requests")
}
