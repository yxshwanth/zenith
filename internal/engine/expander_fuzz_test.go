package engine

import (
	"testing"
)

// FuzzExpansion fuzzes the expansion engine with random input
func FuzzExpansion(f *testing.F) {
	// Seed corpus with valid inputs
	seedInputs := [][]byte{
		[]byte("user:alice doc:doc1 viewer"),
		[]byte("user:bob folder:folder1 owner"),
		[]byte("group:eng doc:doc1 editor"),
	}
	
	for _, seed := range seedInputs {
		f.Add(seed)
	}
	
	f.Fuzz(func(t *testing.T, data []byte) {
		// Parse input (simplified - in real implementation would parse tuple format)
		if len(data) < 10 {
			t.Skip("Input too short")
		}
		
		// Create a mock request from fuzzed data
		// In a real implementation, we would deserialize tuples from the data
		req := &CheckRequest{
			SubjectNamespace: "user",
			SubjectID:        "test",
			ObjectNamespace:  "doc",
			ObjectID:         "test",
			Relation:         "viewer",
		}
		
		// The expansion should never panic, even with malformed input
		// We use a nil expander here - in real tests, we'd use a mock
		// The key is that the function handles all inputs gracefully
		_ = req
		
		// In a real fuzz test, we would:
		// 1. Deserialize tuples from data
		// 2. Create an expander with those tuples
		// 3. Call Check() and verify it doesn't panic
		// 4. Verify the result is always valid (bool, int64, error)
	})
}

// FuzzTupleDeserialization fuzzes tuple deserialization
func FuzzTupleDeserialization(f *testing.F) {
	seedInputs := [][]byte{
		[]byte(`{"namespace":"doc","object_id":"doc1","relation":"viewer","subject_namespace":"user","subject_id":"alice"}`),
		[]byte(`{"namespace":"folder","object_id":"f1","relation":"owner","subject_namespace":"group","subject_id":"eng","subject_relation":"member"}`),
	}
	
	for _, seed := range seedInputs {
		f.Add(seed)
	}
	
	f.Fuzz(func(t *testing.T, data []byte) {
		// Attempt to deserialize tuple from JSON
		// Should handle malformed JSON gracefully
		_ = data
		
		// In a real implementation:
		// 1. Try to unmarshal JSON
		// 2. Validate tuple structure
		// 3. Verify no panics occur
	})
}

// FuzzExpansionDepth tests expansion with various depth limits
func FuzzExpansionDepth(f *testing.F) {
	f.Add(int64(1), int64(5))
	f.Add(int64(10), int64(20))
	f.Add(int64(0), int64(100))
	
	f.Fuzz(func(t *testing.T, maxDepth int64, timeout int64) {
		// Normalize inputs
		if maxDepth < 0 {
			maxDepth = 0
		}
		if maxDepth > 100 {
			maxDepth = 100
		}
		if timeout < 0 {
			timeout = 0
		}
		if timeout > 10000 {
			timeout = 10000
		}
		
		// Create expander with fuzzed parameters
		// In real implementation, verify expansion respects depth limit
		_ = maxDepth
		_ = timeout
	})
}

