# Multi-Stage Dockerfile for BeastDB
# Stage 1: Build static Linux binary
FROM golang:alpine AS builder

WORKDIR /app

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source tree and compile zero-dependency static executable
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /beastdb ./cmd/server

# Stage 2: Ultra-minimal production container
FROM alpine:3.21

# Install ca-certificates and tzdata for production reliability
RUN apk --no-cache add ca-certificates tzdata && mkdir -p /data

COPY --from=builder /beastdb /usr/local/bin/beastdb

EXPOSE 50051
VOLUME ["/data"]

ENTRYPOINT ["/usr/local/bin/beastdb"]
CMD ["-role", "leader", "-port", "50051", "-data-dir", "/data"]
