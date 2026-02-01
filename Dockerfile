# Build stage
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=1 GOOS=linux go build -a -ldflags '-linkmode external -extldflags "-static"' -o requestarr ./cmd/server

# Runtime stage
FROM alpine:3.19

# Add labels for container registries
LABEL org.opencontainers.image.title="Requestarr"
LABEL org.opencontainers.image.description="A fast, lightweight media request gateway for Sonarr & Radarr"
LABEL org.opencontainers.image.source="https://github.com/IcarusCore/Requestarr"
LABEL org.opencontainers.image.licenses="MIT"

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/requestarr .

# Create config directory
RUN mkdir -p /config && \
    chmod 755 /config

# Create non-root user
RUN addgroup -S requestarr && adduser -S requestarr -G requestarr && \
    chown -R requestarr:requestarr /app /config

USER requestarr

# Expose port
EXPOSE 5000

# Environment variables with defaults
ENV PORT=5000
ENV DB_PATH=/config/requestarr.db
ENV ADMIN_PASSWORD=admin
ENV SECRET_KEY=change-me-in-production-please
ENV TZ=UTC

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:5000/api/health || exit 1

# Run the binary
CMD ["./requestarr"]
