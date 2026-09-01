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

# 时区设为东八区：time.Now() 取到的提交时间为北京时间，与前端展示一致
# （DSN 为 loc=Local，DB 读写均按进程本地时间；历史上按 UTC 落库的旧记录需 +8h 迁移）
# font-wqy-zenhei：PDF 导出中文字体（community 仓库，装到 /usr/share/fonts/wqy-zenhei/）
RUN echo 'https://dl-cdn.alpinelinux.org/alpine/v3.20/community' >> /etc/apk/repositories \
    && apk add --no-cache tzdata ca-certificates font-wqy-zenhei \
    && cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime \
    && echo "Asia/Shanghai" > /etc/timezone

ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /app/server .

RUN mkdir -p uploads/evaluations

EXPOSE 8000

CMD ["./server"]
