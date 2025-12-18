# 构建阶段
FROM golang:1.24-alpine AS builder

WORKDIR /app

# 复制依赖文件
COPY go.mod go.sum ./
RUN go mod download

# 复制源代码
COPY . .

# 构建应用（完全静态编译）
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -a -ldflags '-extldflags "-static"' -o kiro ./cmd/server

# 运行阶段 - 使用 scratch 最小镜像
FROM scratch

# 从构建阶段复制 ca-certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# 从构建阶段复制可执行文件
COPY --from=builder /app/kiro /kiro

# 暴露端口
EXPOSE 8000

# 运行应用
ENTRYPOINT ["/kiro"]
