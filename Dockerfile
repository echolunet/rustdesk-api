# syntax=docker/dockerfile:1

FROM golang:1.24-alpine AS builder

ARG GOPROXY=https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY} \
    CGO_ENABLED=0 \
    GOOS=linux

WORKDIR /src

COPY go.mod ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download || true

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod tidy && go build -trimpath -ldflags="-s -w" -o /out/rustdesk-api ./cmd/server

FROM builder AS test
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go test -v ./...

FROM alpine:3.21 AS runtime

RUN apk add --no-cache ca-certificates tzdata curl && \
    mkdir -p /app/data /app/webclient /data/server

WORKDIR /app

COPY --from=builder /out/rustdesk-api /app/rustdesk-api

EXPOSE 21114

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -fsS http://127.0.0.1:21114/health || exit 1

ENTRYPOINT ["/app/rustdesk-api"]
