# Build the provider binary
FROM golang:1.23-alpine AS builder

WORKDIR /workspace

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY apis/ apis/
COPY cmd/ cmd/
COPY internal/ internal/

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -installsuffix cgo -ldflags="-s -w" -o provider ./cmd/provider

# Use distroless as minimal base image
FROM gcr.io/distroless/static:nonroot

WORKDIR /

# Copy the binary from the builder stage
COPY --from=builder /workspace/provider .

# Use non-root user from distroless
USER 65532:65532

ENTRYPOINT ["/provider"]