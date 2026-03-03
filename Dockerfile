# Multi-stage Docker build for revoco
# Stage 1: Builder
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy source code
COPY . .

# Build the binary with ldflags for version info
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w \
      -X main.Version=$(git describe --tags --always --dirty 2>/dev/null || echo 'dev') \
      -X main.Commit=$(git rev-parse --short HEAD 2>/dev/null || echo 'none') \
      -X main.BuildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /app/revoco .

# Stage 2: Runtime
FROM alpine:3.19

# Add OCI image metadata labels
LABEL org.opencontainers.image.source="https://github.com/fulgidus/revoco" \
      org.opencontainers.image.title="revoco" \
      org.opencontainers.image.description="Data liberation tool for escaping big tech walled gardens" \
      org.opencontainers.image.licenses="GPL-3.0"

# Update package index and install runtime dependencies
RUN apk update && \
    apk add --no-cache exiftool ffmpeg && \
    rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 revoco && \
    adduser -D -u 1000 -G revoco revoco

# Copy binary from builder
COPY --from=builder /app/revoco /usr/local/bin/revoco

# Switch to non-root user
USER revoco

# Set entrypoint
ENTRYPOINT ["revoco"]
