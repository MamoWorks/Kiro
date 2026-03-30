# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is Kiro

Kiro is a Go proxy server that converts AWS CodeWhisperer API into Anthropic Claude API format. It accepts requests in Claude API format (`/v1/messages`, `/v1/models`, `/v1/messages/count_tokens`), converts them to CodeWhisperer requests, and translates the responses back. The main branch enables thinking mode (chain-of-thought) by default on all requests.

## Commands

```bash
# Run the server (default port 1188)
go run ./cmd/server

# Build
go build ./cmd/server

# Download dependencies
go mod download

# Docker
docker compose -f docker/docker-compose.yml up -d
```

There are no tests or linting configured in this project.

## Environment Variables

- `PORT` - Server port (default: `1188`)
- `GIN_MODE` - `release` (default) or `debug`
- `DEBUG` - Set to `1` for verbose logging
- Copy `.env.example` to `.env` for local config

## Architecture

**Request flow:** Client (Claude API format) -> Gin HTTP server -> Auth middleware -> Converter -> CodeWhisperer API -> Parser -> Response rewriter -> Client (Claude API format)

Key packages:

- **`cmd/server`** - Entry point. Loads `.env`, starts token refresher, launches Gin server.
- **`server/`** - HTTP handlers, middleware, SSE stream processing, response rewriting. `server.go` sets up routes and middleware. `handlers.go` handles `/v1/messages`. Stream processing is split across `stream_processor.go`, `sse_state_manager.go`, `thinking_extractor.go`, and `stop_reason_manager.go`.
- **`converter/`** - Translates between Anthropic and CodeWhisperer formats. `codewhisperer.go` builds the CodeWhisperer request (model mapping, thinking config, agentic mode). `content.go` converts message content blocks. `tools.go` handles tool/function calling conversion.
- **`parser/`** - Parses CodeWhisperer's binary event stream responses. `compliant_event_stream_parser.go` handles the binary framing protocol. `compliant_message_processor.go` processes parsed events. `sonic_streaming_aggregator.go` aggregates streaming chunks into complete responses.
- **`auth/`** - Token management. Handles both Kiro (`refreshToken`) and AmazonQ (`clientId:clientSecret:refreshToken`) authentication formats. Manages OAuth token refresh.
- **`proxy/`** - HTTP proxy manager. Supports SOCKS5/HTTP proxies with per-key binding, error tracking, and hot-reload from config files in `data/`.
- **`types/`** - Shared type definitions for Anthropic API types, CodeWhisperer types, SSE events, model mappings.
- **`config/`** - Model name mapping (Anthropic model IDs to CodeWhisperer IDs), constants, tuning parameters.
- **`cache/`** - Prompt cache using prefix-based accumulation with SQLite storage.
- **`utils/`** - HTTP client, logging, token estimation, image processing, conversation ID generation.

## Key Behaviors

- **Default thinking mode**: Main branch auto-injects `thinking.budget_tokens=16000` unless explicitly disabled via `thinking.type = "disabled"`.
- **Agentic mode**: Messages prefixed with `-agent` get a system prompt injected to prevent large file write timeouts.
- **Timestamp injection**: All requests get current UTC timestamp injected as context.
- **Tool filtering**: Unsupported tools (e.g., `web_search`) are silently filtered out.
- **Model mapping**: Anthropic model names are mapped to CodeWhisperer model IDs via `config.ModelMap`. Unmapped model names are passed through as-is.
- **New request fields**: `agentContinuationId` (UUID) and `agentTaskType` (`"vibe"`) are injected into `conversationState`. `profileArn` from token refresh is set at the top level.
