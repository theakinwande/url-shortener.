# ===========================================
# Dockerfile - Multi-Stage Build
# ===========================================
# Multi-stage builds create minimal production images.
#
# Stage 1 (builder): Compile the Go binary
# Stage 2 (runner): Copy binary into minimal image
#
# RESULT:
# - Development image: ~1GB (with Go compiler)
# - Production image: ~15MB (just the binary)
#
# WHY MULTI-STAGE?
# 1. Smaller image = faster deployments
# 2. Smaller image = less attack surface
# 3. No build tools in production = more secure
# ===========================================

# ===========================================
# Stage 1: Builder
# ===========================================
# Start with official Go image
FROM golang:1.21-alpine AS builder

# Install build dependencies
# - git: for `go mod download` (some deps use git)
# - ca-certificates: for HTTPS requests
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy dependency files first (for layer caching)
# If go.mod/go.sum don't change, this layer is cached
COPY go.mod go.sum ./

# Download dependencies
# This is cached unless go.mod or go.sum changes
RUN go mod download

# Copy source code
COPY . .

# Build the binary
# CGO_ENABLED=0: Pure Go, no C dependencies (more portable)
# -ldflags: Remove debug info, set version
# -o: Output binary name
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.Version=${VERSION:-dev}" \
    -o /app/server \
    ./cmd/server

# ===========================================
# Stage 2: Runner
# ===========================================
# Use minimal Alpine image (5MB base)
# Alternative: scratch (0MB, but harder to debug)
FROM alpine:3.19

# Add non-root user for security
# Running as root in containers is a bad practice
RUN addgroup -g 1000 app && \
    adduser -u 1000 -G app -D app

# Install runtime dependencies
# - ca-certificates: for HTTPS (external URLs)
# - tzdata: for timezone support
RUN apk add --no-cache ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/server .

# Copy migrations (if running in-container)
COPY --from=builder /app/migrations ./migrations

# Switch to non-root user
USER app

# Expose port (documentation, doesn't actually open port)
EXPOSE 8080

# Health check (Docker-level, distinct from K8s probes)
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/live || exit 1

# Run the server
CMD ["./server"]

# ===========================================
# LEARNING NOTES - Docker Best Practices:
# ===========================================
#
# 1. LAYER CACHING:
#    Each RUN/COPY creates a layer. Order matters!
#    Put rarely-changing steps first (go mod download).
#    Put frequently-changing steps last (COPY . .).
#
# 2. MINIMAL BASE IMAGES:
#    alpine (5MB) vs ubuntu (77MB) vs scratch (0MB)
#    Alpine is a good balance of size and usability.
#
# 3. NON-ROOT USER:
#    If attacker breaks into container, they're not root.
#    Limits potential damage.
#
# 4. .dockerignore:
#    Create .dockerignore to exclude:
#    - .git
#    - vendor
#    - *.md
#    This speeds up COPY and reduces image size.
#
# 5. BUILD ARGS:
#    docker build --build-arg VERSION=1.0.0 .
#    Injects version at build time.
#
# ===========================================
# COMMANDS:
# ===========================================
#
# Build:
#   docker build -t urlshortener:latest .
#
# Run:
#   docker run -p 8080:8080 \
#     -e DATABASE_URL=postgres://... \
#     -e REDIS_URL=redis://... \
#     urlshortener:latest
#
# Build with version:
#   docker build --build-arg VERSION=1.0.0 -t urlshortener:1.0.0 .
#
# ===========================================
