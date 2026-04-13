package errors

import (
	"errors"
	"fmt"
	"testing"
)

func TestSentinelErrors(t *testing.T) {
	sentinels := []struct {
		err  error
		text string
	}{
		{ErrNotFound, "resource not found"},
		{ErrAlreadyExists, "resource already exists"},
		{ErrInvalidInput, "invalid input"},
		{ErrInvalidState, "invalid state"},
		{ErrConcurrency, "concurrency conflict"},
		{ErrUnauthorized, "unauthorized"},
		{ErrForbidden, "forbidden"},
		{ErrInternal, "internal error"},
		{ErrTimeout, "operation timeout"},
		{ErrGraphCycle, "cycle detected in graph"},
		{ErrMaxIterations, "max iterations exceeded"},
	}

	for _, tc := range sentinels {
		t.Run(tc.text, func(t *testing.T) {
			if tc.err.Error() != tc.text {
				t.Errorf("expected %q, got %q", tc.text, tc.err.Error())
			}
		})
	}
}

func TestDomainError_Error(t *testing.T) {
	t.Run("with wrapped error", func(t *testing.T) {
		de := NewDomainError("TEST_CODE", "test message", fmt.Errorf("root cause"))
		want := "TEST_CODE: test message: root cause"
		if de.Error() != want {
			t.Errorf("expected %q, got %q", want, de.Error())
		}
	})

	t.Run("without wrapped error", func(t *testing.T) {
		de := NewDomainError("TEST_CODE", "test message", nil)
		want := "TEST_CODE: test message"
		if de.Error() != want {
			t.Errorf("expected %q, got %q", want, de.Error())
		}
	})
}

func TestDomainError_Unwrap(t *testing.T) {
	root := fmt.Errorf("root")
	de := NewDomainError("CODE", "msg", root)

	if de.Unwrap() != root {
		t.Error("Unwrap should return the wrapped error")
	}

	deNil := NewDomainError("CODE", "msg", nil)
	if deNil.Unwrap() != nil {
		t.Error("Unwrap should return nil when no wrapped error")
	}
}

func TestDomainError_WithDetails(t *testing.T) {
	de := NewDomainError("CODE", "msg", nil)
	if len(de.Details) != 0 {
		t.Fatalf("expected empty details, got %d", len(de.Details))
	}

	result := de.WithDetails("key1", "val1").WithDetails("key2", 42)
	if result != de {
		t.Error("WithDetails should return the same pointer for chaining")
	}
	if de.Details["key1"] != "val1" {
		t.Error("expected key1=val1")
	}
	if de.Details["key2"] != 42 {
		t.Error("expected key2=42")
	}
}

func TestNewDomainError(t *testing.T) {
	de := NewDomainError("MY_CODE", "my message", ErrNotFound)
	if de.Code != "MY_CODE" {
		t.Errorf("expected code MY_CODE, got %s", de.Code)
	}
	if de.Message != "my message" {
		t.Errorf("expected message 'my message', got %s", de.Message)
	}
	if de.Err != ErrNotFound {
		t.Error("expected Err to be ErrNotFound")
	}
	if de.Details == nil {
		t.Error("Details should be initialized to non-nil map")
	}
}

func TestNotFound(t *testing.T) {
	de := NotFound("assistant", "abc-123")
	if de.Code != "NOT_FOUND" {
		t.Errorf("expected code NOT_FOUND, got %s", de.Code)
	}
	if !errors.Is(de, ErrNotFound) {
		t.Error("NotFound should wrap ErrNotFound")
	}
	if de.Details["resource"] != "assistant" {
		t.Error("expected resource=assistant in details")
	}
	if de.Details["id"] != "abc-123" {
		t.Error("expected id=abc-123 in details")
	}
}

func TestAlreadyExists(t *testing.T) {
	de := AlreadyExists("thread", "xyz-789")
	if de.Code != "ALREADY_EXISTS" {
		t.Errorf("expected code ALREADY_EXISTS, got %s", de.Code)
	}
	if !errors.Is(de, ErrAlreadyExists) {
		t.Error("AlreadyExists should wrap ErrAlreadyExists")
	}
	if de.Details["resource"] != "thread" {
		t.Error("expected resource=thread")
	}
	if de.Details["id"] != "xyz-789" {
		t.Error("expected id=xyz-789")
	}
}

func TestInvalidInput(t *testing.T) {
	de := InvalidInput("email", "must contain @")
	if de.Code != "INVALID_INPUT" {
		t.Errorf("expected code INVALID_INPUT, got %s", de.Code)
	}
	if !errors.Is(de, ErrInvalidInput) {
		t.Error("InvalidInput should wrap ErrInvalidInput")
	}
	if de.Details["field"] != "email" {
		t.Error("expected field=email")
	}
	if de.Details["reason"] != "must contain @" {
		t.Error("expected reason='must contain @'")
	}
}

func TestInvalidState(t *testing.T) {
	de := InvalidState("queued", "complete")
	if de.Code != "INVALID_STATE" {
		t.Errorf("expected code INVALID_STATE, got %s", de.Code)
	}
	if !errors.Is(de, ErrInvalidState) {
		t.Error("InvalidState should wrap ErrInvalidState")
	}
	if de.Details["current_state"] != "queued" {
		t.Error("expected current_state=queued")
	}
	if de.Details["attempted_operation"] != "complete" {
		t.Error("expected attempted_operation=complete")
	}
}

func TestInternal(t *testing.T) {
	root := fmt.Errorf("db connection failed")
	de := Internal("database error", root)
	if de.Code != "INTERNAL_ERROR" {
		t.Errorf("expected code INTERNAL_ERROR, got %s", de.Code)
	}
	if de.Err != root {
		t.Error("Internal should wrap the provided error")
	}
}

func TestIs(t *testing.T) {
	de := NotFound("x", "1")
	if !Is(de, ErrNotFound) {
		t.Error("Is should find ErrNotFound in chain")
	}
	if Is(de, ErrAlreadyExists) {
		t.Error("Is should not match ErrAlreadyExists")
	}
}

func TestAs(t *testing.T) {
	de := NotFound("x", "1")
	var target *DomainError
	if !As(de, &target) {
		t.Error("As should find DomainError in chain")
	}
	if target.Code != "NOT_FOUND" {
		t.Errorf("expected NOT_FOUND, got %s", target.Code)
	}

	plain := fmt.Errorf("plain error")
	var target2 *DomainError
	if As(plain, &target2) {
		t.Error("As should not find DomainError in a plain error")
	}
}

func TestErrorsIs_WithWrappedDomainError(t *testing.T) {
	de := NotFound("run", "abc")
	wrapped := fmt.Errorf("outer: %w", de)

	if !errors.Is(wrapped, ErrNotFound) {
		t.Error("errors.Is should traverse through wrapped DomainError")
	}

	var target *DomainError
	if !errors.As(wrapped, &target) {
		t.Error("errors.As should find DomainError through wrapping")
	}
}
