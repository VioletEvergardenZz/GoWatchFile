#!/bin/bash
# docker run --rm -v "$(pwd)":/app -v "$(pwd)/build":/build -w /app golang:1.23-alpine sh -c "apk add --no-cache git ca-certificates tzdata && go mod tidy && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags='-w -s -extldflags \"-static\"' -a -installsuffix cgo -o /build/file-watch cmd/main.go"

# 海外环境
docker run --rm -v "$(pwd)":/app -v "$(pwd)/build":/app/build -w /app golang:1.23-alpine sh -c "make build"

# 国内环境
docker run --rm -e GOPROXY="https://goproxy.cn,direct" -v "$(pwd)":/app -v "$(pwd)/build":/app/build -w /app golang:1.23-alpine sh -c "make build"