# Build stage
FROM golang:1.23-alpine AS builder

ARG TARGETARCH
ARG VERSION=dev

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build with version injected via ldflags
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -ldflags="-w -s -X main.Version=${VERSION#v}" -o tr-engine .

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

# Create non-root user (PostgreSQL refuses to run as root)
RUN addgroup -g 1000 trengine && \
    adduser -u 1000 -G trengine -h /app -D trengine

WORKDIR /app

# Copy binary
COPY --from=builder /app/tr-engine .

# Create directories and set ownership
RUN mkdir -p /data/audio /data/postgres /app/config && \
    chown -R trengine:trengine /app /data

# Default config location
ENV TR_ENGINE_CONFIG=/app/config/config.yaml

# Switch to non-root user
USER trengine

EXPOSE 8080 1883 5432

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["./tr-engine"]
CMD ["--config", "/app/config/config.yaml"]
