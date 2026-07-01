# Build the k8ops manager binary
FROM golang:1.26 AS builder

WORKDIR /workspace

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY api/ api/
COPY cmd/ cmd/
COPY internal/ internal/
COPY hack/ hack/

# Build
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=${VERSION}" -o manager ./cmd/manager
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -X main.version=${VERSION}" -o k8ops ./cmd/k8ops

# Runtime — alpine for ca-certificates support
FROM alpine:3.21

RUN apk add --no-cache ca-certificates

WORKDIR /
COPY --from=builder /workspace/manager /manager
COPY --from=builder /workspace/k8ops /usr/local/bin/k8ops

USER 65532:65532

ENTRYPOINT ["/manager"]
