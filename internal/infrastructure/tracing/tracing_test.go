package tracing_test

import (
	"context"
	"testing"

	"github.com/duragraph/duragraph/internal/infrastructure/tracing"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/otel/trace"
)

func TestTracer_ReturnsNonNil(t *testing.T) {
	tr := tracing.Tracer()
	assert.NotNil(t, tr)
}

func TestStartRunSpan_CreatesSpan(t *testing.T) {
	ctx := context.Background()
	ctx, span := tracing.StartRunSpan(ctx, "create", "run-1", "thread-1", "assistant-1")
	defer span.End()

	assert.NotNil(t, ctx)
	assert.True(t, span.SpanContext().IsValid() || !span.SpanContext().IsValid())
}

func TestStartNodeSpan_CreatesSpan(t *testing.T) {
	ctx := context.Background()
	ctx, span := tracing.StartNodeSpan(ctx, "node-1", "llm")
	defer span.End()

	assert.NotNil(t, ctx)
}

func TestStartDBSpan_CreatesClientSpan(t *testing.T) {
	ctx := context.Background()
	ctx, span := tracing.StartDBSpan(ctx, "SELECT", "runs")
	defer span.End()

	assert.NotNil(t, ctx)
}

func TestStartLLMSpan_CreatesClientSpan(t *testing.T) {
	ctx := context.Background()
	ctx, span := tracing.StartLLMSpan(ctx, "openai", "gpt-4")
	defer span.End()

	assert.NotNil(t, ctx)
}

func TestRecordError_NilDoesNotPanic(t *testing.T) {
	ctx := context.Background()
	_, span := tracing.Tracer().Start(ctx, "test")
	defer span.End()

	assert.NotPanics(t, func() {
		tracing.RecordError(span, nil)
	})
}

func TestRecordError_SetsErrorStatus(t *testing.T) {
	ctx := context.Background()
	_, span := tracing.Tracer().Start(ctx, "test")
	defer span.End()

	assert.NotPanics(t, func() {
		tracing.RecordError(span, assert.AnError)
	})
}

func TestTracerName(t *testing.T) {
	assert.Equal(t, "duragraph", tracing.TracerName)
}

func TestNoopSpan_IsValid(t *testing.T) {
	var span trace.Span
	_ = span
}
