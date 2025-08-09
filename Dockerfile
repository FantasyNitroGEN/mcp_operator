# syntax=docker/dockerfile:1
# Build the manager binary
# Билдер ВСЕГДА на билдер-платформе (x86_64 runner), без эмуляции
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
ARG BUILDPLATFORM
ARG TARGETPLATFORM
ARG BUILDKIT_INLINE_CACHE=1


# Быстрый, повторяемый билд
ENV CGO_ENABLED=0 \
    GO111MODULE=on \
    GOCACHE=/root/.cache/go-build \
    GOMODCACHE=/go/pkg/mod

# Install git and ca-certificates for go mod download
RUN apk add --no-cache git ca-certificates

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.sum ./

# Cache go mod download with BuildKit cache mount
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && \
    go mod verify

# Copy only necessary source files
COPY cmd/ cmd/
COPY api/ api/
COPY controllers/ controllers/
COPY pkg/ pkg/

# Build with optimization flags and cache mount
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -trimpath -buildvcs=false -a -ldflags='-w -s' -o manager cmd/manager/main.go

# Use scratch as minimal base image for smallest possible size
FROM scratch
WORKDIR /
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /workspace/manager .
USER 10001:10001

ENTRYPOINT ["/manager"]
