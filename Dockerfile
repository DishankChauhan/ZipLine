# Build stage
FROM golang:1.21-alpine AS builder

# Install git and ca-certificates (needed for building and fetching dependencies)
RUN apk update && apk add --no-cache git ca-certificates && update-ca-certificates

# Create a non-root user
RUN adduser -D -g '' appuser

# Set the working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download
RUN go mod verify

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -a -installsuffix cgo -o main ./cmd/main.go

# Final stage
FROM scratch

# Import ca-certificates from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Import the user and group files from the builder
COPY --from=builder /etc/passwd /etc/passwd

# Copy the binary from builder
COPY --from=builder /build/main /app/main

# Use the non-root user
USER appuser

# Expose the default ports
EXPOSE 8080 9090 9091

# Set the working directory
WORKDIR /app

# Run the binary
ENTRYPOINT ["./main"] 