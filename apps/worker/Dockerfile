# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /app/worker ./main.go

# Production stage
FROM alpine:3.19 AS production

RUN apk add --no-cache ca-certificates tzdata ffmpeg

RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /app/worker .
RUN mkdir -p /app/storage && chown -R appuser:appgroup /app

USER appuser

CMD ["./worker"]

# Development stage
FROM golang:1.21-alpine AS development

WORKDIR /app

RUN apk add --no-cache git ca-certificates ffmpeg

RUN go install github.com/cosmtrek/air@latest

COPY go.mod go.sum ./
RUN go mod download

CMD ["air", "-c", ".air.toml"]
