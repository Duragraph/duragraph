package errors

import (
	"errors"
	"fmt"
)

// Domain error types
var (
	// ErrNotFound indicates a resource was not found
	ErrNotFound = errors.New("resource not found")

	// ErrAlreadyExists indicates a resource already exists
	ErrAlreadyExists = errors.New("resource already exists")

	// ErrInvalidInput indicates invalid input was provided
	ErrInvalidInput = errors.New("invalid input")

	// ErrInvalidState indicates an invalid state transition or operation
	ErrInvalidState = errors.New("invalid state")

	// ErrConcurrency indicates a concurrency conflict (optimistic locking)
	ErrConcurrency = errors.New("concurrency conflict")

	// ErrUnauthorized indicates unauthorized access
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden indicates forbidden access
	ErrForbidden = errors.New("forbidden")

	// ErrInternal indicates an internal system error
	ErrInternal = errors.New("internal error")

	// ErrTimeout indicates an operation timeout
	ErrTimeout = errors.New("operation timeout")

	// ErrGraphCycle indicates a cycle detected in graph
	ErrGraphCycle = errors.New("cycle detected in graph")

	// ErrMaxIterations indicates max iterations exceeded in loop
	ErrMaxIterations = errors.New("max iterations exceeded")
)

// DomainError wraps an error with additional context
type DomainError struct {
	Code    string
	Message string
	Err     error
	Details map[string]interface{}
}

func (e *DomainError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *DomainError) Unwrap() error {
	return e.Err
}

// NewDomainError creates a new domain error
func NewDomainError(code, message string, err error) *DomainError {
	return &DomainError{
		Code:    code,
		Message: message,
		Err:     err,
		Details: make(map[string]interface{}),
	}
}

// WithDetails adds details to a domain error
func (e *DomainError) WithDetails(key string, value interface{}) *DomainError {
	e.Details[key] = value
	return e
}

// Helper functions for common error scenarios

// NotFound creates a not found error
func NotFound(resource, id string) *DomainError {
	return NewDomainError(
		"NOT_FOUND",
		fmt.Sprintf("%s not found", resource),
		ErrNotFound,
	).WithDetails("resource", resource).WithDetails("id", id)
}

// AlreadyExists creates an already exists error
func AlreadyExists(resource, id string) *DomainError {
	return NewDomainError(
		"ALREADY_EXISTS",
		fmt.Sprintf("%s already exists", resource),
		ErrAlreadyExists,
	).WithDetails("resource", resource).WithDetails("id", id)
}

// InvalidInput creates an invalid input error
func InvalidInput(field, reason string) *DomainError {
	return NewDomainError(
		"INVALID_INPUT",
		fmt.Sprintf("invalid input for field %s", field),
		ErrInvalidInput,
	).WithDetails("field", field).WithDetails("reason", reason)
}

// InvalidState creates an invalid state error
func InvalidState(current, attempted string) *DomainError {
	return NewDomainError(
		"INVALID_STATE",
		fmt.Sprintf("cannot perform operation in state %s", current),
		ErrInvalidState,
	).WithDetails("current_state", current).WithDetails("attempted_operation", attempted)
}

// Internal creates an internal error
func Internal(message string, err error) *DomainError {
	return NewDomainError("INTERNAL_ERROR", message, err)
}

// Is checks if an error is of a specific type
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// As finds the first error in err's chain that matches target
func As(err error, target interface{}) bool {
	return errors.As(err, target)
}
