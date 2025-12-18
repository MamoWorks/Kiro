# Kiro2API

将 Kiro (AWS CodeWhisperer) API 转换为 Anthropic Claude API 格式的代理服务器。

## 支持的模型

| 模型名称 | 别名 |
|---------|------|
| claude-opus-4-5-20251101 | claude-opus-4-5 |
| claude-sonnet-4-5-20250929 | claude-sonnet-4-5 |
| claude-haiku-4-5-20251001 | claude-haiku-4-5 |

## API 端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/v1/models` | GET | 获取可用模型列表 |
| `/v1/messages` | POST | 发送消息（支持流式/非流式） |
| `/v1/messages/count_tokens` | POST | 计算 Token 数量 |

## 认证方式

通过 `x-api-key` 或 `Authorization: Bearer` 传入 Token，支持两种格式：

### Kiro 格式

直接使用 Kiro 的 `refreshToken`：

```
x-api-key: YOUR_REFRESH_TOKEN
```

### AmazonQ 格式

使用 `clientId:clientSecret:refreshToken` 组合：

```
x-api-key: CLIENT_ID:CLIENT_SECRET:REFRESH_TOKEN
```

## 快速开始

### 直接运行

```bash
# 设置环境变量
cp .env.example .env

# 运行
go run ./cmd/server
```

### Docker

```bash
# 使用 docker-compose（推荐）
docker compose -f docker/docker-compose.yml up -d

# 或直接运行镜像
docker run -d -p 1188:1188 ghcr.io/mamocode/kiro:latest
```

## 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PORT` | 服务端口 | 1188 |
| `GIN_MODE` | 运行模式 (release/debug) | release |
| `DEBUG` | 启用调试日志 (1/true) | - |

### 日志级别

- `GIN_MODE=release`: 只输出错误日志
- 默认: 输出 INFO 和 ERROR 日志
- `DEBUG=1`: 输出所有日志（包括调试信息）

## 使用示例

```bash
curl -X POST http://localhost:1188/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: YOUR_REFRESH_TOKEN" \
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Hello"}
    ]
  }'
```