# ---------- 构建阶段 ----------
FROM golang:1.26-alpine AS builder

WORKDIR /build

# 国内镜像加速（如构建环境可直连可删除）
ENV GOPROXY=https://goproxy.cn,direct

COPY go.mod go.sum ./
# 离线构建：依赖已 vendor（vendor/modules.txt 完整时 go 自动启用 vendor 模式）
COPY vendor ./vendor

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -ldflags="-s -w" -o /app/server ./cmd/server

# ---------- 运行阶段 ----------
FROM alpine:3.20

# 时区对齐旧端（python:slim 默认 UTC）：DB 时间按 UTC 原样读写
RUN apk add --no-cache tzdata ca-certificates \
    && cp /usr/share/zoneinfo/UTC /etc/localtime \
    && echo "UTC" > /etc/timezone

WORKDIR /app
COPY --from=builder /app/server .

RUN mkdir -p uploads/evaluations

EXPOSE 8000

CMD ["./server"]
