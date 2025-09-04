# Multi-stage Dockerfile for Go-Fitz PDF Extractor
# Stage 1: Builder with full C development environment
FROM golang:1.24.3-bullseye AS builder

# Install build tools and MuPDF development dependencies (glibc-based)
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    pkg-config \
    libmupdf-dev \
    mupdf-tools \
    git \
    ca-certificates \
    make \
    wget \
 && rm -rf /var/lib/apt/lists/*

# Enable CGO
ENV CGO_ENABLED=1
ENV GOOS=linux
ENV GOARCH=amd64

# Set working directory
WORKDIR /app

# Copy go mod and sum files first for better layer caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the application (dynamic linking — rely on system MuPDF)
RUN go build -o main .

# Stage 2: Minimal runtime image
FROM debian:bullseye-slim

# Install runtime dependencies (MuPDF)
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    pkg-config \
    libmupdf-dev \
    mupdf-tools \
    git \
    ca-certificates \
    make \
    wget \
 && rm -rf /var/lib/apt/lists/*
# Create non-root user for security
RUN groupadd -r appgroup && useradd -r -g appgroup appuser

# Create app directory and set ownership
WORKDIR /app
RUN chown appuser:appgroup /app

# Copy the binary from builder stage
COPY --from=builder /app/main .
RUN chmod +x main && chown appuser:appgroup main

# Create tmp directory with proper permissions
RUN mkdir -p /app/tmp && chown -R appuser:appgroup /app/tmp

# Switch to non-root user
USER appuser

# Expose port (Railway uses PORT env var)
EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://localhost:${PORT:-3000}/health || exit 1

# Command to run the application
CMD ["./main"]