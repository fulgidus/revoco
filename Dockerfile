# Docker build for revoco
# When used with GoReleaser, the pre-built binary is copied into the context.
# For standalone builds, use: docker build --build-arg BINARY=revoco .
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

# Copy pre-built binary from GoReleaser context
COPY revoco /usr/local/bin/revoco

# Switch to non-root user
USER revoco

# Set entrypoint
ENTRYPOINT ["revoco"]
