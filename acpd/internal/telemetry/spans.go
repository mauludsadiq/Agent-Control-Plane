package telemetry

import (
"context"
"time"

"go.opentelemetry.io/otel/attribute"
"go.opentelemetry.io/otel/codes"
"go.opentelemetry.io/otel/metric"
)

func TraceCommit(ctx context.Context, workflowID string, fn func(context.Context) error) error {
if Tracer == nil {
return fn(ctx)
}
ctx, span := Tracer.Start(ctx, "acp.commit_transition")
defer span.End()
span.SetAttributes(attribute.String("workflow_id", workflowID))
start := time.Now()
err := fn(ctx)
elapsed := float64(time.Since(start).Milliseconds())
if err != nil {
span.SetStatus(codes.Error, err.Error())
span.RecordError(err)
}
if CommitDuration != nil {
CommitDuration.Record(ctx, elapsed,
metric.WithAttributes(attribute.String("workflow_id", workflowID)))
}
return err
}

func TraceBridge(ctx context.Context, program string, fn func(context.Context) error) error {
if Tracer == nil {
return fn(ctx)
}
ctx, span := Tracer.Start(ctx, "acp.bridge."+program)
defer span.End()
span.SetAttributes(attribute.String("bridge.program", program))
start := time.Now()
err := fn(ctx)
elapsed := float64(time.Since(start).Milliseconds())
if err != nil {
span.SetStatus(codes.Error, err.Error())
span.RecordError(err)
}
if BridgeDuration != nil {
BridgeDuration.Record(ctx, elapsed,
metric.WithAttributes(attribute.String("program", program)))
}
return err
}

func TraceClaimTask(ctx context.Context, agent string, fn func(context.Context) error) error {
if Tracer == nil {
return fn(ctx)
}
ctx, span := Tracer.Start(ctx, "acp.task.claim")
defer span.End()
span.SetAttributes(attribute.String("agent", agent))
err := fn(ctx)
if err != nil {
span.SetStatus(codes.Error, err.Error())
} else if TasksClaimed != nil {
TasksClaimed.Add(ctx, 1,
metric.WithAttributes(attribute.String("agent", agent)))
}
return err
}
