package errors

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorCode represents a specific error type
type ErrorCode string

const (
	ErrorCodeValidation   ErrorCode = "VALIDATION_ERROR"
	ErrorCodeNotFound     ErrorCode = "NOT_FOUND"
	ErrorCodeConflict     ErrorCode = "CONFLICT"
	ErrorCodeTimeout      ErrorCode = "TIMEOUT"
	ErrorCodeDatabase     ErrorCode = "DATABASE_ERROR"
	ErrorCodeExpansion    ErrorCode = "EXPANSION_ERROR"
	ErrorCodeInternal     ErrorCode = "INTERNAL_ERROR"
	ErrorCodeRateLimited  ErrorCode = "RATE_LIMITED"
)

// ZenithError is a custom error type with context
type ZenithError struct {
	Code    ErrorCode
	Message string
	Context map[string]interface{}
	Err     error // Underlying error
}

func (e *ZenithError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *ZenithError) Unwrap() error {
	return e.Err
}

// NewValidationError creates a validation error
func NewValidationError(message string, context map[string]interface{}) *ZenithError {
	return &ZenithError{
		Code:    ErrorCodeValidation,
		Message: message,
		Context: context,
	}
}

// NewNotFoundError creates a not found error
func NewNotFoundError(resourceType, resourceID string) *ZenithError {
	return &ZenithError{
		Code:    ErrorCodeNotFound,
		Message: fmt.Sprintf("%s not found: %s", resourceType, resourceID),
		Context: map[string]interface{}{
			"resource_type": resourceType,
			"resource_id":   resourceID,
		},
	}
}

// NewConflictError creates a conflict error
func NewConflictError(message string, context map[string]interface{}) *ZenithError {
	return &ZenithError{
		Code:    ErrorCodeConflict,
		Message: message,
		Context: context,
	}
}

// NewTimeoutError creates a timeout error
func NewTimeoutError(operation string, timeoutMs int64) *ZenithError {
	return &ZenithError{
		Code:    ErrorCodeTimeout,
		Message: fmt.Sprintf("operation %s timed out after %dms", operation, timeoutMs),
		Context: map[string]interface{}{
			"operation":  operation,
			"timeout_ms": timeoutMs,
		},
	}
}

// NewDatabaseError creates a database error
func NewDatabaseError(message string, err error) *ZenithError {
	context := make(map[string]interface{})
	if err != nil {
		context["error"] = err.Error()
	}
	return &ZenithError{
		Code:    ErrorCodeDatabase,
		Message: message,
		Err:     err,
		Context: context,
	}
}

// NewExpansionError creates an expansion error
func NewExpansionError(message string, context map[string]interface{}, err error) *ZenithError {
	return &ZenithError{
		Code:    ErrorCodeExpansion,
		Message: message,
		Err:     err,
		Context: context,
	}
}

// NewInternalError creates an internal error
func NewInternalError(message string, err error) *ZenithError {
	return &ZenithError{
		Code:    ErrorCodeInternal,
		Message: message,
		Err:     err,
	}
}

// ToGRPCStatus converts a ZenithError to a gRPC status
func ToGRPCStatus(err error) error {
	if err == nil {
		return nil
	}

	zenithErr, ok := err.(*ZenithError)
	if !ok {
		// Not a ZenithError, wrap as internal error
		return status.Error(codes.Internal, err.Error())
	}

	var grpcCode codes.Code
	switch zenithErr.Code {
	case ErrorCodeValidation:
		grpcCode = codes.InvalidArgument
	case ErrorCodeNotFound:
		grpcCode = codes.NotFound
	case ErrorCodeConflict:
		grpcCode = codes.AlreadyExists
	case ErrorCodeTimeout:
		grpcCode = codes.DeadlineExceeded
	case ErrorCodeDatabase:
		grpcCode = codes.Internal
	case ErrorCodeExpansion:
		grpcCode = codes.Internal
	case ErrorCodeRateLimited:
		grpcCode = codes.ResourceExhausted
	case ErrorCodeInternal:
		grpcCode = codes.Internal
	default:
		grpcCode = codes.Internal
	}

	return status.Error(grpcCode, zenithErr.Message)
}

// AddToSpan adds error information to an OpenTelemetry span
func AddToSpan(span trace.Span, err error) {
	if err == nil {
		return
	}

	span.RecordError(err)

	zenithErr, ok := err.(*ZenithError)
	if !ok {
		span.SetAttributes(attribute.String("error.type", "unknown"))
		return
	}

	span.SetAttributes(
		attribute.String("error.code", string(zenithErr.Code)),
		attribute.String("error.message", zenithErr.Message),
	)

	// Add context as attributes
	for key, value := range zenithErr.Context {
		span.SetAttributes(attribute.String(fmt.Sprintf("error.context.%s", key), fmt.Sprintf("%v", value)))
	}

	if zenithErr.Err != nil {
		span.SetAttributes(attribute.String("error.cause", zenithErr.Err.Error()))
	}
}

// IsValidationError checks if an error is a validation error
func IsValidationError(err error) bool {
	zenithErr, ok := err.(*ZenithError)
	return ok && zenithErr.Code == ErrorCodeValidation
}

// IsNotFoundError checks if an error is a not found error
func IsNotFoundError(err error) bool {
	zenithErr, ok := err.(*ZenithError)
	return ok && zenithErr.Code == ErrorCodeNotFound
}

// IsTimeoutError checks if an error is a timeout error
func IsTimeoutError(err error) bool {
	zenithErr, ok := err.(*ZenithError)
	return ok && zenithErr.Code == ErrorCodeTimeout
}

// IsDatabaseError checks if an error is a database error
func IsDatabaseError(err error) bool {
	zenithErr, ok := err.(*ZenithError)
	return ok && zenithErr.Code == ErrorCodeDatabase
}

