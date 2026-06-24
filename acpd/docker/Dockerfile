# ── Stage 1: build Go binary ──────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

# sqlite3 requires cgo
RUN apk add --no-cache gcc musl-dev

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build static binary
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags="-s -w" \
    -o acpd \
    ./cmd/acpd

# ── Stage 2: minimal runtime image ───────────────────────────────────────────
FROM alpine:3.19

RUN apk add --no-cache ca-certificates fardrun || true

WORKDIR /app

# Copy binary
COPY --from=builder /build/acpd .

# Copy migrations
COPY migrations/ migrations/

# Copy FARD bridge programs and ACP source
COPY fard/ fard/
COPY --from=builder /build/../src/ src/

# Data directory for SQLite
RUN mkdir -p /data

ENV ACP_DB=/data/acp.db
ENV ACP_ADDR=:8080
ENV ACP_MIGRATIONS=/app/migrations
ENV ACP_FARD_DIR=/app/fard/bridge
ENV ACP_SNAPSHOT_EVERY=10

EXPOSE 8080

ENTRYPOINT ["./acpd"]
CMD ["--db", "/data/acp.db", \
     "--addr", ":8080", \
     "--migrations", "/app/migrations", \
     "--fard-dir", "/app/fard/bridge", \
     "--snapshot-every", "10"]
