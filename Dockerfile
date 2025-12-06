# syntax=docker/dockerfile:1

# Stage 1: build the nbi-server binary
FROM golang:1.24-alpine AS builder
WORKDIR /src

# Install CA certificates for any HTTPS module downloads during build.
RUN apk add --no-cache ca-certificates

# Cache dependencies before copying the entire repo.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source and build a statically linked binary.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/nbi-server ./cmd/nbi-server

# Stage 2: minimal runtime image
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /out/nbi-server /nbi-server
COPY --from=builder /src/configs /configs

EXPOSE 50051 9090
USER nonroot:nonroot

ENTRYPOINT ["/nbi-server"]
CMD ["--listen-address=0.0.0.0:50051", "--metrics-address=:9090"]
