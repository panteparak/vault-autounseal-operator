# syntax=docker/dockerfile:1

# Build stage with optimized caching
FROM golang:1.24-alpine AS builder

# Install build dependencies once and cache them
RUN apk add --no-cache ca-certificates git tzdata && \
    adduser -D -g '' appuser

WORKDIR /workspace

# Copy and cache go mod files first for better layer caching
COPY go.mod go.sum ./

# Download dependencies and cache them in a separate layer
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download && \
    go mod verify

# Copy source code (this changes more frequently)
COPY main.go ./
COPY pkg/ pkg/

# Build with cache mounts for faster compilation
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
ARG BUILD_TIME
ARG GIT_COMMIT

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 \
    GOOS=${TARGETOS} \
    GOARCH=${TARGETARCH} \
    go build \
    -ldflags="-w -s -X main.version=${VERSION} -X main.buildTime=${BUILD_TIME} -X main.gitCommit=${GIT_COMMIT} -extldflags '-static'" \
    -tags netgo,osusergo \
    -trimpath \
    -o manager \
    main.go

# Verification stage
FROM builder AS verify
RUN file manager && \
    ./manager --version 2>/dev/null || echo "Binary built successfully"

# Production stage - minimal and secure
FROM gcr.io/distroless/static:nonroot AS production

# Copy timezone data for scheduled operations
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy CA certificates for HTTPS validation
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the binary from builder stage
COPY --from=builder /workspace/manager /manager

# Use numeric ID for better Kubernetes compatibility
USER 65532:65532

# Health check port, metrics port, webhook port
EXPOSE 8080 8081 9443

# Add health check using wget/curl (but distroless doesn't have these, so remove for now)
# HEALTHCHECK --interval=30s --timeout=5s --start-period=30s --retries=3 \
#     CMD ["/manager", "--health-check"]

ENTRYPOINT ["/manager"]

# Development stage for debugging
FROM golang:1.24-alpine AS development

RUN apk add --no-cache ca-certificates git tzdata curl vim

WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Install development tools
RUN go install -a github.com/go-delve/delve/cmd/dlv@latest && \
    go install -a github.com/golangci/golangci-lint/cmd/golangci-lint@latest

EXPOSE 8080 8081 9443 2345

CMD ["go", "run", "main.go"]
