package telemetry

import (
"context"
"io"
"time"

"go.opentelemetry.io/otel"
"go.opentelemetry.io/otel/exporters/stdout/stdoutmetric"
"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
"go.opentelemetry.io/otel/metric"
"go.opentelemetry.io/otel/propagation"
sdkmetric "go.opentelemetry.io/otel/sdk/metric"
"go.opentelemetry.io/otel/sdk/resource"
sdktrace "go.opentelemetry.io/otel/sdk/trace"
semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
"go.opentelemetry.io/otel/trace"
)

const ServiceName = "agent-control-plane"

var (
Tracer trace.Tracer
Meter  metric.Meter

WorkflowsCreated    metric.Int64Counter
TasksClaimed        metric.Int64Counter
TasksCompleted      metric.Int64Counter
TasksFailed         metric.Int64Counter
TasksDeadLettered   metric.Int64Counter
CommitDuration      metric.Float64Histogram
BridgeDuration      metric.Float64Histogram
HTTPRequestDuration metric.Float64Histogram
RequeueCount        metric.Int64Counter
)

type Provider struct {
tp *sdktrace.TracerProvider
mp *sdkmetric.MeterProvider
}

func (p *Provider) Shutdown(ctx context.Context) {
_ = p.tp.Shutdown(ctx)
_ = p.mp.Shutdown(ctx)
}

func Init(ctx context.Context, traceW, metricW io.Writer) (*Provider, error) {
res, err := resource.New(ctx,
resource.WithAttributes(
semconv.ServiceName(ServiceName),
semconv.ServiceVersion("0.6.0"),
),
)
if err != nil {
return nil, err
}

var tpOpts []sdktrace.TracerProviderOption
tpOpts = append(tpOpts, sdktrace.WithResource(res))
if traceW != nil {
exp, err := stdouttrace.New(stdouttrace.WithWriter(traceW), stdouttrace.WithPrettyPrint())
if err != nil {
return nil, err
}
tpOpts = append(tpOpts, sdktrace.WithBatcher(exp))
}
tp := sdktrace.NewTracerProvider(tpOpts...)
otel.SetTracerProvider(tp)
otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
propagation.TraceContext{},
propagation.Baggage{},
))
Tracer = tp.Tracer(ServiceName)

var mpOpts []sdkmetric.Option
mpOpts = append(mpOpts, sdkmetric.WithResource(res))
if metricW != nil {
exp, err := stdoutmetric.New(stdoutmetric.WithWriter(metricW))
if err != nil {
return nil, err
}
mpOpts = append(mpOpts, sdkmetric.WithReader(
sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(30*time.Second)),
))
}
mp := sdkmetric.NewMeterProvider(mpOpts...)
otel.SetMeterProvider(mp)
Meter = mp.Meter(ServiceName)

if err := initInstruments(); err != nil {
return nil, err
}
return &Provider{tp: tp, mp: mp}, nil
}

func initInstruments() error {
var err error
if WorkflowsCreated, err = Meter.Int64Counter("acp.workflows.created",
metric.WithDescription("Total workflows created")); err != nil {
return err
}
if TasksClaimed, err = Meter.Int64Counter("acp.tasks.claimed",
metric.WithDescription("Total tasks claimed")); err != nil {
return err
}
if TasksCompleted, err = Meter.Int64Counter("acp.tasks.completed",
metric.WithDescription("Total tasks completed")); err != nil {
return err
}
if TasksFailed, err = Meter.Int64Counter("acp.tasks.failed",
metric.WithDescription("Total tasks failed")); err != nil {
return err
}
if TasksDeadLettered, err = Meter.Int64Counter("acp.tasks.dead_lettered",
metric.WithDescription("Tasks moved to dead letter")); err != nil {
return err
}
if CommitDuration, err = Meter.Float64Histogram("acp.commit.duration_ms",
metric.WithDescription("Commit pipeline duration"),
metric.WithUnit("ms")); err != nil {
return err
}
if BridgeDuration, err = Meter.Float64Histogram("acp.bridge.duration_ms",
metric.WithDescription("FARD bridge call duration"),
metric.WithUnit("ms")); err != nil {
return err
}
if HTTPRequestDuration, err = Meter.Float64Histogram("acp.http.duration_ms",
metric.WithDescription("HTTP request duration"),
metric.WithUnit("ms")); err != nil {
return err
}
if RequeueCount, err = Meter.Int64Counter("acp.tasks.requeued",
metric.WithDescription("Tasks requeued after timeout")); err != nil {
return err
}
return nil
}

func NoopInit() {
tp := sdktrace.NewTracerProvider()
otel.SetTracerProvider(tp)
mp := sdkmetric.NewMeterProvider()
otel.SetMeterProvider(mp)
Tracer = tp.Tracer(ServiceName)
Meter = mp.Meter(ServiceName)
_ = initInstruments()
}
