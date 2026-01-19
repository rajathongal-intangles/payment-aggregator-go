package errors

import (
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// AppError represents an application-level error
type AppError struct {
	Code    codes.Code
	Message string
	Err     error // Original error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// ToGRPCError converts AppError to gRPC status error
func (e *AppError) ToGRPCError() error {
	return status.Error(e.Code, e.Message)
}

// Common error constructors

func NotFound(resource, id string) *AppError {
	return &AppError{
		Code:    codes.NotFound,
		Message: fmt.Sprintf("%s not found: %s", resource, id),
	}
}

func InvalidArgument(field, reason string) *AppError {
	return &AppError{
		Code:    codes.InvalidArgument,
		Message: fmt.Sprintf("invalid %s: %s", field, reason),
	}
}

func Internal(msg string, err error) *AppError {
	return &AppError{
		Code:    codes.Internal,
		Message: msg,
		Err:     err,
	}
}

func Unavailable(msg string) *AppError {
	return &AppError{
		Code:    codes.Unavailable,
		Message: msg,
	}
}

func AlreadyExists(resource, id string) *AppError {
	return &AppError{
		Code:    codes.AlreadyExists,
		Message: fmt.Sprintf("%s already exists: %s", resource, id),
	}
}

func ResourceExhausted(msg string) *AppError {
	return &AppError{
		Code:    codes.ResourceExhausted,
		Message: msg,
	}
}

// IsRetryable checks if an error should be retried
func IsRetryable(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}

	switch st.Code() {
	case codes.Unavailable, codes.Aborted, codes.ResourceExhausted:
		return true
	default:
		return false
	}
}
