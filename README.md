# Dialogue is All You Need (AM glasses Backend)

English | [中文](#中文)

> A voice-first, multimodal backend that uses conversation to accomplish most tasks—without traditional GUIs or apps.

---

## Contents
- [Overview](#overview)
- [What “Dialogue is All You Need” Means](#what-dialogue-is-all-you-need-means)
- [Features](#features)
- [Quick Start](#quick-start)
  - [Configure .config.yaml](#configure-configyaml)
  - [MCP Configuration](#mcp-configuration)
  - [Build from Source](#build-from-source)
  - [Windows Opus Toolchain](#windows-opus-toolchain)
  - [Run](#run)
- [External Auth & Device Binding](#external-auth--device-binding)
- [Swagger](#swagger)
- [中文](#中文)

---

## Overview
Dialogue is All You Need (AM glass backend) is an end-to-end, cross-platform server for voice and multimodal interactions. It supports flexible transports, pluggable ASR/TTS/LLM providers, and MCP-based tool integrations (e.g., maps, weather). The system is designed so that most user workflows are completed through natural conversation, with optional lightweight visual inserts when needed.

---

## What “Dialogue is All You Need” Means
- Conversation can address 80%+ of needs that were previously solved by GUIs and standalone apps. Instead of navigating screens, users simply say what they want.
- Working with AI should feel like collaborating with a teammate—state intent, clarify context, negotiate steps, and iterate quickly, all via dialogue.
- For the minority of cases that truly require a UI (e.g., rich data display, structured input), inject lightweight H5 cards directly into the conversation. This preserves the dialogue-first flow while providing:
  - Higher efficiency (no app switching, minimal context loss)
  - Better cross-platform behavior (HTML5 cards render consistently)
- It’s an end-to-end cross-platform solution. The initiating endpoint can be extremely lightweight—not only PC or mobile browsers, but also any embedded personal device running Linux or even RTOS can access the service.

---

## Features
- [x] Transports: WebSocket, gRPC
- [x] Voice dialog with PCM / Opus
- [x] Models:
  - ASR: Doubao streaming
  - TTS: EdgeTTS / Doubao
  - LLM: OpenAI API, Ollama
- [x] Voice-controlled camera invocation for on-device image recognition
- [x] MCP protocol (client/local/server) with integrations like AMap (Gaode) and weather
- [x] Voice-controlled role voice switching
- [x] Voice-controlled preset role switching
- [x] Single-host deployment
- [x] Local databases: sqlite, postgre

---

## Quick Start

### Configure `.config.yaml`
- Template at project root: `config.yaml`
- Copy to local config: `cp config.yaml .config.yaml`
- Adjust model providers, WebSocket, and server endpoints as needed

WebSocket address:
```yaml
web:
  websocket: ws://your-server-ip:8000
```

Transport:
```yaml
transport:
  default: websocket
```

To use gRPC transport, set `default` to `grpcgateway` and start the IM service:
```bash
cd im-server
go run main.go
```

ASR/LLM/TTS:
- Configure providers following the existing schema in the config file.
- Avoid adding/removing fields to maintain compatibility.

---

### MCP Configuration
See: `src/core/mcp/README.md`

---

## Build from Source

### Prerequisites
- Go 1.24.2+
- On Windows, install CGO and Opus (see below)

Initialize:
```bash
cd angrymiao-ai-server
cp config.yaml .config.yaml
```

---

### Windows Opus Toolchain
1) Install MSYS2: https://www.msys2.org/  
2) In MSYS2 MINGW64:
```bash
pacman -Syu
pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-go mingw-w64-x86_64-opus
pacman -S mingw-w64-x86_64-pkg-config
```
3) Environment variables (PowerShell or System):
```bash
set PKG_CONFIG_PATH=C:\msys64\mingw64\lib\pkgconfig
set CGO_ENABLED=1
```
4) Sanity check (MINGW64):
```bash
go run ./src/main.go
```

If Go module downloads are slow, switch to a domestic mirror/accelerator.

---

## Run
```bash
go mod tidy
go run ./src/main.go
```

---

## External Auth & Device Binding

Flow:
1) External auth issues a User JWT  
2) Client calls POST `/api/device/bind` (Header: `Authorization: Bearer <UserJWT>`, Body: `{"device_id":"..."}`)  
3) Server returns `{device_key, token}`  
4) Use the device token to connect via WebSocket or im-server

Example (curl):
```bash
curl -X POST "http://your-server:8080/api/device/bind" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <UserJWT>" \
  -d '{"device_id":"device-001"}'
```

Success response:
```json
{"success":true,"device_key":"<bind_key>","token":"<device_token>"}
```

WebSocket connection:
- Prefer headers during handshake:
  - `Authorization: Bearer <device_token>`
  - `Device-Id: device-001`
- Browser query alternative:
  - `ws://your-server:8000/?device-id=device-001&token=<device_token>`  
  The server converts the token into an Authorization header.

Reference files:
- `src/device/server.go`
- `src/device/types.go`
- `src/core/transport/websocket/transport.go`  
(Token validity checks, binding logic, and verification points)

Note: Place the external auth system’s JWT public key at `src/configs/casbin/jwt/public.pem` (replace with your own key; none is included).

---

## Swagger
Open:
```
http://localhost:8080/swagger/index.html
```

Refresh API docs:
```bash
cd src
swag init -g main.go
```

---

# 中文

> 一个以对话为核心的多模态后端，让大多数任务无需传统 GUI 或 App，也能高效完成。

## 概览
Dialogue is All You Need（AM glass 后端服务）是面向语音与多模态交互的端到端、跨平台服务。支持灵活的传输层、可插拔的 ASR/TTS/LLM 模型，以及基于 MCP 的工具接入（如地图、天气）。系统设计目标是让大部分工作通过自然对话完成，必要时再用轻量的可视化补充。

## “Dialogue is All You Need”的含义
- 通过对话可解决 80% 甚至更多过去依赖 GUI 和 App 的需求。无需页面跳转与交互分散，只需直接表达意图。
- 和 AI 的交流、推进任务本就应当像与团队协作：陈述目标、补充上下文、明确步骤、快速迭代，全部在对话中完成。
- 少数确需 GUI 的场景（如富数据展示、结构化输入），在对话中插入 H5 卡片即可：  
  - 更高效率（免切换应用、减少上下文丢失）  
  - 更佳跨平台（HTML5 卡片具备一致渲染）
- 这是一个端到端的跨平台方案；发起端可以非常轻量：不仅限于 PC/手机浏览器，任何运行 Linux 或 RTOS 的嵌入式随身设备也可以接入服务。

## 功能清单
- [x] 传输层：WebSocket、gRPC
- [x] 语音对话：PCM / Opus
- [x] 模型能力：
  - ASR：豆包流式
  - TTS：EdgeTTS / 豆包
  - LLM：OpenAI API、Ollama
- [x] 语音控制调用摄像头进行图像识别
- [x] MCP 协议（客户端 / 本地 / 服务器），可接入高德地图、天气查询等
- [x] 语音控制切换角色声音
- [x] 语音控制切换预设角色
- [x] 支持单机部署
- [x] 本地数据库：sqlite、postgre

## 快速开始

### 配置 `.config.yaml`
- 根目录有模板：`config.yaml`
- 复制为本地配置：`cp config.yaml .config.yaml`
- 按需配置模型、WebSocket、Server 地址等字段

WebSocket 地址：
```yaml
web:
  websocket: ws://your-server-ip:8000
```

传输层：
```yaml
transport:
  default: websocket
```

如需使用 gRPC，将 `default` 改为 `grpcgateway`，并在 `im-server` 目录启动 IM 服务：
```bash
cd im-server
go run main.go
```

ASR/LLM/TTS：
- 按配置文件既有结构填写对应服务。
- 尽量不要增减字段以保证兼容性。

---

### MCP 协议配置
参考：`src/core/mcp/README.md`

---

## 源码安装与运行

### 前置条件
- Go 1.24.2+
- Windows 需安装 CGO 与 Opus（见下节）

初始化：
```bash
cd angrymiao-ai-server
cp config.yaml .config.yaml
```

---

### Windows 安装 Opus 编译环境
1) 安装 MSYS2：https://www.msys2.org/  
2) 打开 MSYS2 MINGW64，执行：
```bash
pacman -Syu
pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-go mingw-w64-x86_64-opus
pacman -S mingw-w64-x86_64-pkg-config
```
3) 设置环境变量（PowerShell 或系统变量）：
```bash
set PKG_CONFIG_PATH=C:\msys64\mingw64\lib\pkgconfig
set CGO_ENABLED=1
```
4) 建议先在 MINGW64 环境下运行一次：
```bash
go run ./src/main.go
```

如 Go 模块更新较慢，可配置国内代理镜像源以加速依赖下载。

---

## 运行项目
```bash
go mod tidy
go run ./src/main.go
```

---

## 外部授权与设备绑定说明

流程：
1) 外部授权系统签发 User JWT  
2) 调用 POST `/api/device/bind`（Header: `Authorization: Bearer <UserJWT>`，Body: `{"device_id":"..."}`）  
3) 服务返回 `{device_key, token}`  
4) 使用 device token 连接 WebSocket 或 im-server

示例（curl）：
```bash
curl -X POST "http://your-server:8080/api/device/bind" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <UserJWT>" \
  -d '{"device_id":"device-001"}'
```

返回（成功）：
```json
{"success":true,"device_key":"<bind_key>","token":"<device_token>"}
```

WebSocket 连接：
- 推荐握手时用 Header 传递：
  - `Authorization: Bearer <device_token>`
  - `Device-Id: device-001`
- 浏览器可使用 query：
  - `ws://your-server:8000/?device-id=device-001&token=<device_token>`  
  服务会将 token 转为 Authorization header。

参考实现：
- `src/device/server.go`
- `src/device/types.go`
- `src/core/transport/websocket/transport.go`  
（包含 token 有效期检查、绑定逻辑与关键校验点）

注意：请将外部授权系统的 JWT 公钥文件放到 `src/configs/casbin/jwt/public.pem`（仓库未包含真实密钥，请替换为你的 `public.pem`）。

---

## Swagger 文档
打开：
```
http://localhost:8080/swagger/index.html
```

更新 API 文档：
```bash
cd src
swag init -g main.go
```
