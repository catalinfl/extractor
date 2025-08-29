FROM golang:1.24-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main .

# Ultra-light Alpine runtime (much smaller than Ubuntu)
FROM alpine:3.18

# Install minimal dependencies for OCR (Alpine packages are much smaller)
RUN apk add --no-cache \
    ca-certificates \
    wget \
    && rm -rf /var/cache/apk/*

# Create non-root user (Alpine way)
RUN addgroup -g 1001 -S appuser && \
    adduser -u 1001 -S appuser -G appuser

# Set working directory
WORKDIR /app

# Copy built application from builder stage
COPY --from=builder /app/main .

# Create tmp directory with proper permissions
RUN mkdir -p /app/tmp && chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose port (Railway uses PORT env variable)
EXPOSE 3000

# Run the application with tmpfs for faster I/O
CMD ["./main"]