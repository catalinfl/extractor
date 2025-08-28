# Multi-stage Dockerfile for Railway deployment
# Optimized for Go app with Tesseract OCR and Poppler PDF support

# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install dependencies for building
RUN apk add --no-cache git ca-certificates tzdata

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Runtime stage
FROM ubuntu:22.04

# Prevent interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=UTC

# Set threading environment variables for Railway (conservative)
ENV OMP_NUM_THREADS=1
ENV OPENBLAS_NUM_THREADS=1
ENV GOMAXPROCS=1
ENV OCR_WORKERS=1

# Install system dependencies
RUN apt-get update && apt-get install -y \
    tesseract-ocr \
    poppler-utils \
    ca-certificates \
    wget \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Download selected language models (use tessdata_fast for smaller size)
ARG TESSDATA_DIR=/usr/share/tessdata
RUN mkdir -p ${TESSDATA_DIR} && \
    for L in eng fra deu spa ita por rus chi_sim jpn kor ron hun; do \
        echo "Downloading $L"; \
        wget -q -O ${TESSDATA_DIR}/$L.traineddata "https://github.com/tesseract-ocr/tessdata_fast/raw/main/$L.traineddata" || \
        wget -q -O ${TESSDATA_DIR}/$L.traineddata "https://github.com/tesseract-ocr/tessdata/raw/main/$L.traineddata"; \
    done && \
    ls -lah ${TESSDATA_DIR}

# Create non-root user
RUN groupadd -r appuser && useradd -r -g appuser appuser

# Set working directory
WORKDIR /app

# Copy built application from builder stage
COPY --from=builder /app/main .

# Create temp directory with proper permissions
RUN mkdir -p /app/tmp && chown -R appuser:appuser /app

# Verify installations
RUN tesseract --version && pdftoppm -h

# Switch to non-root user
USER appuser

# Health check (disable - Railway doesn't need it)
# HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
#     CMD curl -f http://localhost:${PORT:-3000}/health || exit 1

# Expose default port (Railway will set PORT env var at runtime)
EXPOSE 3000

# Run the application
CMD ["./main"]
