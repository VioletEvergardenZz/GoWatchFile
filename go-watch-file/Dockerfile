# 本文件用于构建 go-watch-file 后端镜像
ARG BASE_IMAGE_PREFIX=docker.io/library
ARG GOLANG_IMAGE=${BASE_IMAGE_PREFIX}/golang:1.23
ARG DEBIAN_IMAGE=${BASE_IMAGE_PREFIX}/debian:bookworm-slim
FROM ${GOLANG_IMAGE} AS builder

WORKDIR /src

ARG GOPROXY=https://mirrors.aliyun.com/goproxy/,direct
ENV GOPROXY=${GOPROXY}

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o /out/file-watch cmd/main.go

FROM ${DEBIAN_IMAGE}

WORKDIR /app

ARG APT_MIRROR=mirrors.aliyun.com
RUN sed -i "s#deb.debian.org#${APT_MIRROR}#g" /etc/apt/sources.list.d/debian.sources \
    && apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /out/file-watch /app/file-watch
COPY config.yaml /etc/gwf/config.yaml

EXPOSE 8080

CMD ["/app/file-watch", "-config", "/etc/gwf/config.yaml"]
