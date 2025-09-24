# Use official Go image as base
FROM golang:1.24.7-alpine AS builder

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -a -installsuffix cgo -o mlb-controller ./cmd/mlb-controller

# Use a minimal alpine image for the final stage
FROM alpine:3.22.1

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Set working directory
WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/mlb-controller .

# Copy configuration files
COPY configs /app/configs

# Set environment variables
ENV CONFIG_PATH=/app/configs/config.yaml

# Expose any necessary ports (modify as needed)
EXPOSE 8080

RUN addgroup -S appgroup && adduser -S appuser -G appgroup
USER appuser

# Command to run the binary
CMD ["./mlb-controller"]

