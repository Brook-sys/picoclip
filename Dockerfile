# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.25

FROM golang:${GO_VERSION}-alpine AS builder

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

RUN --mount=type=cache,target=/go/pkg/mod go install github.com/a-h/templ/cmd/templ@v0.3.1020

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build templ generate
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/picoclip cmd/picoclip/main.go

FROM alpine:3.24

LABEL org.opencontainers.image.description="Default Alpine/musl PicoClip image. Claurst official Linux binaries require glibc; use the Debian image variant when Claurst is required."

RUN apk add --no-cache ca-certificates tzdata && addgroup -S picoclip && adduser -S -G picoclip picoclip

WORKDIR /app
COPY --from=builder /out/picoclip /usr/local/bin/picoclip

RUN mkdir -p /app/data /app/workspaces && chown -R picoclip:picoclip /app

USER picoclip

ENV BIND=0.0.0.0 \
    PORT=8080 \
    PICOCLIP_DB_PATH=/app/data/picoclip.db \
    PICOCLIP_RUNTIMES=/app/data/runtimes \
    PICOCLIP_WORKSPACES=/app/workspaces

EXPOSE 8080
VOLUME ["/app/data", "/app/workspaces"]

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 CMD wget -qO- "http://127.0.0.1:${PORT}/" >/dev/null || exit 1

ENTRYPOINT ["picoclip"]
