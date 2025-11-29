# Stage 1: Build
FROM golang:1.20-alpine AS builder
WORKDIR /app
COPY . .
RUN go mod init constellation-simulator || true
RUN go build -o simulator main.go

# Stage 2: Runtime
FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/simulator .
COPY configs ./configs
CMD ["./simulator"]
