# Stage 1: Build
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache ca-certificates

ENV GOPROXY=https://goproxy.cn,direct

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /dehydrator ./cmd/server

# Stage 2: Minimal runtime
FROM scratch

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /dehydrator /dehydrator

EXPOSE 8765

ENTRYPOINT ["/dehydrator"]
