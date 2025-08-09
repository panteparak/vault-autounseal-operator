# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /workspace

# Install git for go mod download
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY main.go ./
COPY pkg/ pkg/

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo \
    -o manager main.go

# Production stage
FROM gcr.io/distroless/static:nonroot

WORKDIR /

# Copy the binary from builder stage
COPY --from=builder /workspace/manager .

USER 65532:65532

EXPOSE 8080 8081 9443

ENTRYPOINT ["/manager"]
