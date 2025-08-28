FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .


FROM ubuntu:22.04

# Prevent interactive prompts during package installation
ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=UTC

# Install system dependencies
RUN apt-get update && apt-get install -y \
    tesseract-ocr \
    poppler-utils \
    ca-certificates \
    wget \
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

# Create tmp directory with proper permissions
RUN mkdir -p /app/tmp && chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Verify installations
RUN tesseract --version && pdftoppm -h

# Expose port (Railway uses PORT env variable)
EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:3000/health || exit 1

# Run the application
CMD ["./main"]