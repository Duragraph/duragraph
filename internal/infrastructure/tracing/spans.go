package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func StartRunSpan(ctx context.Context, operation, runID, threadID, assistantID string) (context.Context, trace.Span) {
	ctx, span := Tracer().Start(ctx, "run."+operation,
		trace.WithAttributes(
			attribute.String("run.id", runID),
			attribute.String("thread.id", threadID),
			attribute.String("assistant.id", assistantID),
		),
	)
	return ctx, span
}

func StartNodeSpan(ctx context.Context, nodeID, nodeType string) (context.Context, trace.Span) {
	ctx, span := Tracer().Start(ctx, "node.execute",
		trace.WithAttributes(
			attribute.String("node.id", nodeID),
			attribute.String("node.type", nodeType),
		),
	)
	return ctx, span
}

func StartDBSpan(ctx context.Context, operation, table string) (context.Context, trace.Span) {
	ctx, span := Tracer().Start(ctx, "db."+operation,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.operation", operation),
			attribute.String("db.sql.table", table),
		),
	)
	return ctx, span
}

func StartLLMSpan(ctx context.Context, provider, model string) (context.Context, trace.Span) {
	ctx, span := Tracer().Start(ctx, "llm.completion",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("llm.provider", provider),
			attribute.String("llm.model", model),
		),
	)
	return ctx, span
}

func RecordError(span trace.Span, err error) {
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}
