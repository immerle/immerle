# syntax=docker/dockerfile:1

# ---- web app stage ----
# Pin to the build platform: the export is static web assets (arch-independent),
# so we build it once natively instead of emulating it per target arch.
FROM --platform=$BUILDPLATFORM node:24-alpine AS ui
WORKDIR /ui
COPY ui/package.json ui/package-lock.json ./
RUN --mount=type=cache,id=npm-expo57,target=/root/.npm,sharing=locked npm ci --legacy-peer-deps
COPY ui/ ./
RUN npm run export:web

# ---- build stage ----
# Also pinned to the build platform; Go cross-compiles to $TARGETARCH natively,
# which is far faster than running the toolchain under QEMU emulation.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src
ARG TARGETOS TARGETARCH

# Cache modules.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
# Overlay the freshly exported web app so //go:embed picks it up.
COPY --from=ui /ui/dist ./ui/dist
ARG VERSION=docker
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/immerle ./cmd/immerle

# ---- runtime stage ----
FROM alpine:3.20
RUN apk add --no-cache ffmpeg ca-certificates && \
    adduser -D -u 10001 immerle
WORKDIR /app
COPY --from=build /out/immerle /usr/local/bin/immerle

# Pre-create the mount points owned by the non-root user. A fresh named volume
# inherits this ownership, so the server (running as uid 10001) can create the
# SQLite database and write derived data under /data.
RUN mkdir -p /data /music && chown -R immerle:immerle /data /music /app

# Library (read-only mount) and derived data.
VOLUME ["/music", "/data"]
ENV LIBRARY_DATA_DIR=/data \
    DATABASE_DSN=/data/immerle.db \
    PORT=4533

EXPOSE 4533
USER immerle

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s \
    CMD wget -qO- http://127.0.0.1:4533/ping || exit 1

ENTRYPOINT ["immerle"]
# Config comes from the environment (and an optional /app/.env). Runtime settings
# (provider behaviour, avatars, scan, federation) are managed via the admin API.
CMD []
