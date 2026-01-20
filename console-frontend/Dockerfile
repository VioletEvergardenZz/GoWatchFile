# 本文件用于构建 console-frontend 前端镜像
ARG BASE_IMAGE_PREFIX=registry.cn-hangzhou.aliyuncs.com/library
FROM ${BASE_IMAGE_PREFIX}/node:20 AS builder

WORKDIR /app

ARG NPM_REGISTRY=https://registry.npmmirror.com
RUN npm config set registry ${NPM_REGISTRY}

COPY package.json package-lock.json ./
RUN npm ci

COPY . .
RUN npm run build

FROM ${BASE_IMAGE_PREFIX}/nginx:1.27-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 80
