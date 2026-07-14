# Build the k8ops manager binary
FROM golang:1.26 AS builder

WORKDIR /workspace

# Copy go mod files first (cache layer)
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Copy source
COPY api/ api/
COPY cmd/ cmd/
COPY internal/ internal/
COPY hack/ hack/

# Build with cache mounts for faster CI builds
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o manager ./cmd/manager

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o k8ops ./cmd/k8ops

# Ensure /tmp has correct perms for SQLite temp files (copied to final image)
RUN chmod 1777 /tmp && touch /tmp/.keep

# Runtime — distroless static (no shell, no package manager, ~2MB base)
# Contains only CA certs + timezone data + minimal runtime files
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /
COPY --from=builder /workspace/manager /manager
COPY --from=builder /workspace/k8ops /usr/local/bin/k8ops

# /data is writable by nonroot (UID 65532) in distroless
# /tmp is required by SQLite (modernc.org/sqlite) for temp files during DB operations
# distroless static-debian12 does not include /tmp by default, causing SQLITE_CANTOPEN (err 14)
# The .keep placeholder ensures the directory is created by COPY
COPY --chown=65532:65532 --from=builder /tmp/.keep /tmp/.keep

ENTRYPOINT ["/manager"]
