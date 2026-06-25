package telemetry

import (
"net/http"
"strconv"
"time"

"go.opentelemetry.io/otel/attribute"
"go.opentelemetry.io/otel/codes"
"go.opentelemetry.io/otel/metric"
)

func HTTPMiddleware(next http.Handler) http.Handler {
return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
if Tracer == nil {
next.ServeHTTP(w, r)
return
}
ctx, span := Tracer.Start(r.Context(), r.Method+" "+r.URL.Path)
defer span.End()
span.SetAttributes(
attribute.String("http.method", r.Method),
attribute.String("http.path", r.URL.Path),
)
rw := &responseWriter{ResponseWriter: w, status: 200}
start := time.Now()
next.ServeHTTP(rw, r.WithContext(ctx))
elapsed := float64(time.Since(start).Milliseconds())
span.SetAttributes(attribute.Int("http.status_code", rw.status))
if rw.status >= 500 {
span.SetStatus(codes.Error, strconv.Itoa(rw.status))
}
if HTTPRequestDuration != nil {
HTTPRequestDuration.Record(ctx, elapsed,
metric.WithAttributes(
attribute.String("method", r.Method),
attribute.String("path", r.URL.Path),
attribute.Int("status", rw.status),
))
}
})
}

type responseWriter struct {
http.ResponseWriter
status int
}

func (rw *responseWriter) WriteHeader(status int) {
rw.status = status
rw.ResponseWriter.WriteHeader(status)
}
