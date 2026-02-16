FROM golang:1.25-alpine AS builder

ARG VERSION=dev
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.commit=$(git rev-parse --short HEAD 2>/dev/null || echo unknown) -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /tr-engine ./cmd/tr-engine

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /tr-engine /usr/local/bin/tr-engine
COPY schema.sql /opt/tr-engine/schema.sql
COPY sample.env /opt/tr-engine/sample.env

WORKDIR /opt/tr-engine

EXPOSE 8080

ENTRYPOINT ["tr-engine"]
