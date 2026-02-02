# 本文件用于构建 console-frontend 前端镜像
ARG BASE_IMAGE_PREFIX=docker.io/library
FROM ${BASE_IMAGE_PREFIX}/node:22.12.0-slim AS builder

WORKDIR /app

ARG NPM_REGISTRY=https://registry.npmmirror.com
RUN npm config set registry ${NPM_REGISTRY}

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

FROM ${BASE_IMAGE_PREFIX}/nginx:1.28-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80
