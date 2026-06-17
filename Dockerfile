# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache modules.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=docker
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.version=${VERSION}" -o /out/immerle ./cmd/immerle

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
