# Go mock worker Dockerfile for E2E testing
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download || true

# Copy source
COPY . .

# Download dependencies
RUN go mod tidy

# Build binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/go_worker .

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/go_worker .

# Environment defaults
ENV CONTROL_PLANE_URL=http://localhost:8081
ENV MOCK_WORKER_GRAPH=simple_echo
ENV MOCK_WORKER_DELAY_MS=100
ENV LOG_LEVEL=info

# Run worker
CMD ["./go_worker"]
