# 🚀 Kiro - AWS CodeWhisperer to Claude API Converter

<div align="center">

[![GitHub release](https://img.shields.io/badge/release-v1.4.0-blue)](https://github.com/MamoWorks/Kiro/releases)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.24.0-00ADD8?logo=go)](https://go.dev/)
[![Docker](https://img.shields.io/badge/docker-supported-2496ED?logo=docker)](https://hub.docker.com)
[![Models](https://img.shields.io/badge/models-Opus%20%7C%20Sonnet%20%7C%20Haiku%204.5-8B5CF6)](https://github.com/MamoWorks/Kiro)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/MamoWorks/Kiro/pulls)
[![Stars](https://img.shields.io/github/stars/MamoWorks/Kiro?style=social)](https://github.com/MamoWorks/Kiro)

**将 AWS CodeWhisperer API 转换为 Anthropic Claude API 格式的高性能代理服务**

[快速开始](#-快速开始) · [API文档](#-api-端点) · [Docker部署](#-docker部署) · [报告问题](https://github.com/MamoWorks/Kiro/issues)

</div>

---

## 🌿 分支说明

本项目提供两个分支，满足不同使用场景：

### 🔵 主分支 (main)
- **默认启用思维链** - 所有模型请求自动启用 Thinking Mode
- 适合需要深度推理的场景
- 可通过 `thinking.type = "disabled"` 显式禁用

### 🟢 SillyTavern 分支 (sillytavern)
- **按需启用思维链** - 仅当模型名包含 `-thinking` 后缀时启用
- 支持模型名：`claude-sonnet-4-5-thinking`、`claude-opus-4-5-thinking` 等
- 完全兼容 SillyTavern 和其他客户端

**选择建议**：
- 使用 SillyTavern 等客户端 → 选择 `sillytavern` 分支
- 需要默认思维链功能 → 选择 `main` 分支

---

## ✨ 核心特性

- 🔄 **无缝转换** - 完整兼容 Anthropic Claude API 格式
- 🧠 **默认思维链** - 主分支所有请求默认启用 Thinking Mode，深度推理
- 🎯 **多模型支持** - Claude Opus 4.5、Sonnet 4.5、Haiku 4.5
- ⚡ **流式响应** - 支持 Server-Sent Events (SSE) 流式输出
- 🛠️ **工具调用** - 完整的 Function Calling / Tool Use 支持
- 🖼️ **多模态** - 支持图片输入（Vision）
- 🔐 **灵活认证** - 支持 Kiro 和 AmazonQ 两种 Token 格式
- 🚀 **高性能** - Gin 框架，低延迟，高并发
- 🐳 **容器化** - 开箱即用的 Docker 支持
- 📊 **Token 计数** - 精确的 Token 使用统计

---

## 🎯 支持的模型

| 模型名称 | 描述 |
|---------|------|
| `claude-opus-4-6` | 最新旗舰模型 |
| `claude-sonnet-4-6` | 最新平衡模型 |
| `claude-opus-4-5` | 强大模型，适合复杂任务 |
| `claude-sonnet-4-5` | 平衡性能与速度 |
| `claude-haiku-4-5` | 最快速度，适合简单任务 |

---

## 📡 API 端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/v1/models` | GET | 获取可用模型列表 |
| `/v1/messages` | POST | 发送消息（支持流式/非流式） |
| `/v1/messages/count_tokens` | POST | 计算消息的 Token 数量 |

---

## 🔐 认证方式

通过 `x-api-key` 或 `Authorization: Bearer` 请求头传入 Token。

### Kiro 格式

直接使用 Kiro 的 `refreshToken`：

```bash
x-api-key: YOUR_REFRESH_TOKEN
```

### AmazonQ 格式

使用 `clientId:clientSecret:refreshToken` 组合：

```bash
x-api-key: CLIENT_ID:CLIENT_SECRET:REFRESH_TOKEN
```

---

## 🚀 快速开始

### 前置要求

- Go >= 1.24.0
- Git

### 本地运行

```bash
# 克隆仓库
git clone https://github.com/MamoWorks/Kiro.git
cd Kiro

# 配置环境变量
cp .env.example .env

# 安装依赖
go mod download

# 运行服务
go run ./cmd/server
```

服务将在 `http://localhost:1188` 启动

---

## 🐳 Docker 部署

### 使用 Docker Compose（推荐）

```bash
docker compose -f docker/docker-compose.yml up -d
```

### 手动 Docker 部署

```bash
# 使用预构建镜像
docker run -d \
  -p 1188:1188 \
  -e PORT=1188 \
  -e GIN_MODE=release \
  --name kiro \
  ghcr.io/mamoworks/kiro:latest
```

### 从源码构建

```bash
# 构建镜像
docker build -t kiro:latest .

# 运行容器
docker run -d -p 1188:1188 --name kiro kiro:latest
```

---

## 💻 使用示例

### 基础对话

```bash
curl -X POST http://localhost:1188/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: YOUR_REFRESH_TOKEN" \
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 1024,
    "messages": [
      {"role": "user", "content": "Hello, Claude!"}
    ]
  }'
```

### 流式输出

```bash
curl -X POST http://localhost:1188/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: YOUR_REFRESH_TOKEN" \
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 2048,
    "stream": true,
    "messages": [
      {"role": "user", "content": "讲一个笑话"}
    ]
  }'
```

### 思维链模式（Thinking Mode）

**主分支默认启用思维链**，无需额外配置：

```bash
# 默认行为：自动启用思维链（budget_tokens=16000）
curl -X POST http://localhost:1188/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: YOUR_REFRESH_TOKEN" \
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 4096,
    "messages": [
      {"role": "user", "content": "解释量子计算的工作原理"}
    ]
  }'
```

**自定义思维链预算**：

```bash
curl -X POST http://localhost:1188/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: YOUR_REFRESH_TOKEN" \
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 4096,
    "thinking": {
      "budget_tokens": 32000
    },
    "messages": [
      {"role": "user", "content": "解决这个复杂数学问题"}
    ]
  }'
```

**禁用思维链**（如需要）：

```bash
curl -X POST http://localhost:1188/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: YOUR_REFRESH_TOKEN" \
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 1024,
    "thinking": {
      "type": "disabled"
    },
    "messages": [
      {"role": "user", "content": "简单问候"}
    ]
  }'
```

### 工具调用（Function Calling）

```bash
curl -X POST http://localhost:1188/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: YOUR_REFRESH_TOKEN" \
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 1024,
    "tools": [
      {
        "name": "get_weather",
        "description": "获取指定城市的天气信息",
        "input_schema": {
          "type": "object",
          "properties": {
            "city": {
              "type": "string",
              "description": "城市名称"
            }
          },
          "required": ["city"]
        }
      }
    ],
    "messages": [
      {"role": "user", "content": "北京今天天气怎么样？"}
    ]
  }'
```

### 图片输入（Vision）

```bash
curl -X POST http://localhost:1188/v1/messages \
  -H "Content-Type: application/json" \
  -H "x-api-key: YOUR_REFRESH_TOKEN" \
  -d '{
    "model": "claude-sonnet-4-5",
    "max_tokens": 1024,
    "messages": [
      {
        "role": "user",
        "content": [
          {
            "type": "image",
            "source": {
              "type": "base64",
              "media_type": "image/jpeg",
              "data": "base64_encoded_image_data_here"
            }
          },
          {
            "type": "text",
            "text": "这张图片里有什么？"
          }
        ]
      }
    ]
  }'
```

### Token 计数

```bash
curl -X POST http://localhost:1188/v1/messages/count_tokens \
  -H "Content-Type: application/json" \
  -H "x-api-key: YOUR_REFRESH_TOKEN" \
  -d '{
    "model": "claude-sonnet-4-5",
    "messages": [
      {"role": "user", "content": "Hello, world!"}
    ]
  }'
```

---

## 📂 项目结构

```
Kiro/
├── cmd/
│   └── server/          # 服务入口
├── server/              # HTTP 服务器
├── converter/           # API 格式转换器
├── parser/              # SSE 流解析器
├── auth/                # 认证模块
├── config/              # 配置管理
├── types/               # 类型定义
├── utils/               # 工具函数
├── docker/              # Docker 配置
│   └── docker-compose.yml
├── .env.example         # 环境变量示例
├── go.mod               # Go 依赖
└── README.md            # 项目文档
```

---

## ⚙️ 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `PORT` | 服务监听端口 | `1188` |
| `GIN_MODE` | Gin 运行模式 (`release`/`debug`) | `release` |
| `DEBUG` | 启用调试日志 (`1`/`true`) | - |

### 日志级别

- `GIN_MODE=release`: 仅输出错误日志（生产环境推荐）
- 默认: 输出 INFO 和 ERROR 日志
- `DEBUG=1`: 输出所有日志，包括调试信息

---

## 🛠️ 技术栈

- **语言**: Go 1.24+
- **Web 框架**: Gin
- **HTTP 客户端**: Go 标准库
- **流式处理**: Server-Sent Events (SSE)
- **容器化**: Docker & Docker Compose
- **架构**: RESTful API

---

## 🔧 高级特性

### 默认思维链模式（主分支）

主分支所有请求默认启用思维链，提供更深入的推理能力：

- **默认配置**: `budget_tokens=16000`
- **自定义**: 通过 `thinking.budget_tokens` 参数调整
- **禁用**: 设置 `thinking.type = "disabled"`

```json
// 默认启用，无需配置
{
  "model": "claude-sonnet-4-5",
  "messages": [...]
}

// 自定义预算
{
  "model": "claude-sonnet-4-5",
  "thinking": {"budget_tokens": 32000},
  "messages": [...]
}

// 显式禁用
{
  "model": "claude-sonnet-4-5",
  "thinking": {"type": "disabled"},
  "messages": [...]
}
```

### Agentic 模式

在用户消息前添加 `-agent` 前缀可启用 Agentic 模式，注入防止大文件写入超时的系统提示：

```json
{
  "messages": [
    {"role": "user", "content": "-agent 创建一个包含1000行代码的文件"}
  ]
}
```

### 时间戳注入

所有请求会自动注入当前时间戳上下文，让模型知道当前时间：

```
[Context: Current time is 2026-01-08 12:00:00 UTC]
```

### 工具过滤

自动过滤不支持的工具（如 `web_search`），静默处理，不会报错。

---

## 🚨 注意事项

- 🔑 **Token 安全**: 请妥善保管您的 Refresh Token
- ⏱️ **超时设置**: 大型响应可能需要较长时间
- 💾 **内存要求**: 建议至少 512MB 可用内存
- 🔒 **生产部署**: 建议配置 HTTPS 和反向代理
- 📊 **Token 限制**: 遵守 AWS CodeWhisperer 的使用限制

---

## 📝 API 响应格式

### 标准响应

```json
{
  "id": "msg_01ABC123",
  "type": "message",
  "role": "assistant",
  "model": "claude-sonnet-4-5",
  "content": [
    {
      "type": "text",
      "text": "Hello! How can I help you today?"
    }
  ],
  "stop_reason": "end_turn",
  "usage": {
    "input_tokens": 10,
    "output_tokens": 25
  }
}
```

### 流式响应（SSE）

```
event: message_start
data: {"type":"message_start","message":{"id":"msg_01ABC123","type":"message","role":"assistant"}}

event: content_block_delta
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hello"}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":25}}

event: message_stop
data: {"type":"message_stop"}
```

---

## 🤝 贡献

欢迎贡献代码、报告问题和提出建议！

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 开启 Pull Request

---

## 📞 联系方式

- **问题反馈**: [GitHub Issues](https://github.com/MamoWorks/Kiro/issues)
- **功能建议**: [GitHub Discussions](https://github.com/MamoWorks/Kiro/discussions)
- **Pull Requests**: [GitHub PRs](https://github.com/MamoWorks/Kiro/pulls)

---

## ⚠️ 免责声明

本项目仅供学习和研究使用。使用本服务产生的任何后果由使用者自行承担。请遵守相关法律法规和服务提供商的使用条款。

---

## ⭐ Star History

[![Star History Chart](https://api.star-history.com/svg?repos=MamoWorks/Kiro&type=Date)](https://star-history.com/#MamoWorks/Kiro&Date)

---

<div align="center">

**如果觉得这个项目有帮助，请给一个 ⭐️ Star！**

Made with ❤️ by [MamoWorks](https://github.com/MamoWorks)

</div>
