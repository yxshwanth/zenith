package errors

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestZenithError(t *testing.T) {
	err := NewValidationError("invalid input", map[string]interface{}{
		"field": "namespace",
		"value": "",
	})

	if err.Code != ErrorCodeValidation {
		t.Errorf("Expected ErrorCodeValidation, got %v", err.Code)
	}
	if err.Message != "invalid input" {
		t.Errorf("Expected message 'invalid input', got %s", err.Message)
	}
	if err.Context["field"] != "namespace" {
		t.Errorf("Expected context field 'namespace', got %v", err.Context["field"])
	}
}

func TestToGRPCStatus(t *testing.T) {
	tests := []struct {
		name     string
		err      *ZenithError
		expected codes.Code
	}{
		{
			name:     "validation error",
			err:      NewValidationError("invalid", nil),
			expected: codes.InvalidArgument,
		},
		{
			name:     "not found error",
			err:      NewNotFoundError("tuple", "123"),
			expected: codes.NotFound,
		},
		{
			name:     "conflict error",
			err:      NewConflictError("duplicate", nil),
			expected: codes.AlreadyExists,
		},
		{
			name:     "timeout error",
			err:      NewTimeoutError("check", 10),
			expected: codes.DeadlineExceeded,
		},
		{
			name:     "database error",
			err:      NewDatabaseError("connection failed", nil),
			expected: codes.Internal,
		},
		{
			name:     "internal error",
			err:      NewInternalError("unexpected", nil),
			expected: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grpcErr := ToGRPCStatus(tt.err)
			st, ok := status.FromError(grpcErr)
			if !ok {
				t.Fatal("Expected gRPC status error")
			}
			if st.Code() != tt.expected {
				t.Errorf("Expected code %v, got %v", tt.expected, st.Code())
			}
		})
	}
}

func TestErrorHelpers(t *testing.T) {
	validationErr := NewValidationError("test", nil)
	if !IsValidationError(validationErr) {
		t.Error("Expected IsValidationError to return true")
	}
	if IsValidationError(NewNotFoundError("test", "123")) {
		t.Error("Expected IsValidationError to return false for NotFoundError")
	}

	notFoundErr := NewNotFoundError("tuple", "123")
	if !IsNotFoundError(notFoundErr) {
		t.Error("Expected IsNotFoundError to return true")
	}

	timeoutErr := NewTimeoutError("check", 10)
	if !IsTimeoutError(timeoutErr) {
		t.Error("Expected IsTimeoutError to return true")
	}

	dbErr := NewDatabaseError("test", nil)
	if !IsDatabaseError(dbErr) {
		t.Error("Expected IsDatabaseError to return true")
	}
}

