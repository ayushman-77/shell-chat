# ------------------------------
# 1️⃣ Build stage – compile the binary
# ------------------------------
FROM golang:alpine AS builder

# Install git (required for go mod tidy) and ca-certificates
RUN apk add --no-cache git ca-certificates

# Set working directory inside container
WORKDIR /app

# Copy go.mod and go.sum first (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the binary (static, no CGO)
ENV CGO_ENABLED=0
RUN go build -o shell-chat ./cmd/server 

# ------------------------------
# 2️⃣ Runtime stage – tiny image with only the binary
# ------------------------------
FROM alpine:3.20

# Add a non‑root user for security (optional but recommended)
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
WORKDIR /app
RUN chown -R appuser:appgroup /app
USER appuser

# Copy the compiled binary from the builder stage
COPY --from=builder /app/shell-chat .

# Expose any ports you might need (none for pure TUI, but keep for completeness)
# EXPOSE 8080

# Entry point – run the binary
ENTRYPOINT ["./shell-chat"]
