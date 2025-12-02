# Dialogue is All You Need (AM glass 后端服务)

[English](README.md) | 中文

> 一个以对话为核心的多模态后端，让大多数任务无需传统 GUI 或 App，也能高效完成。

---

## 目录
- [概览](#概览)
- ["Dialogue is All You Need"的含义](#dialogue-is-all-you-need的含义)
- [核心特性](#核心特性)
- [项目结构](#项目结构)
- [快速开始](#快速开始)
  - [环境要求](#1-环境要求)
  - [配置文件](#2-配置文件)
  - [Windows 环境配置](#3-windows-环境配置opus-编译)
  - [启动服务](#4-启动服务)
  - [启动 gRPC IM 服务](#5-启动-grpc-im-服务可选)
  - [启动 MQTT 服务器](#6-启动-mqtt-服务器可选)
- [MCP 协议配置](#mcp-协议配置)
- [认证与设备绑定](#认证与设备绑定)
- [API 文档](#api-文档)
- [使用场景](#使用场景)
- [技术栈](#技术栈)
- [开发指南](#开发指南)
- [常见问题](#常见问题)
- [许可证](#许可证)

---

## 概览
Dialogue is All You Need（AM glass 后端服务）是面向语音与多模态交互的端到端、跨平台服务。支持灵活的传输层、可插拔的 ASR/TTS/LLM 模型，以及基于 MCP 的工具接入（如地图、天气）。系统设计目标是让大部分工作通过自然对话完成，必要时再用轻量的可视化补充。

## “Dialogue is All You Need”的含义
- 通过对话可解决 80% 甚至更多过去依赖 GUI 和 App 的需求。无需页面跳转与交互分散，只需直接表达意图。
- 和 AI 的交流、推进任务本就应当像与团队协作：陈述目标、补充上下文、明确步骤、快速迭代，全部在对话中完成。
- 少数确需 GUI 的场景（如富数据展示、结构化输入），在对话中插入 H5 卡片即可：  
  - 更高效率（免切换应用、减少上下文丢失）  
  - 更佳跨平台（HTML5 卡片具备一致渲染）
- 这是一个端到端的跨平台方案；发起端可以非常轻量：不仅限于 PC/手机浏览器，任何运行 Linux 或 RTOS 的嵌入式随身设备也可以接入服务。

## 核心特性

### 🎯 多传输层支持
* [x] **WebSocket** - 实时双向通信，支持浏览器和原生客户端
* [x] **gRPC Gateway** - 高性能 RPC 通信
* [x] **MQTT** - 物联网设备消息传输，支持 UDP 音频传输
* [x] **多协议并行** - 可同时启用多种传输协议

### 🎤 语音处理能力
* [x] **ASR（语音识别）** - 支持豆包流式识别、Deepgram、GoSherpa
* [x] **TTS（语音合成）** - 支持豆包、EdgeTTS、Deepgram、GoSherpa
* [x] **VAD（语音活动检测）** - WebRTC VAD，智能检测语音开始/结束
* [x] **音频格式** - 支持 PCM、Opus 编解码
* [x] **音频转录（AUC）** - 豆包录音文件识别

### 🤖 大语言模型集成
* [x] **OpenAI 兼容接口** - 支持通义千问、智谱 ChatGLM、DeepSeek 等
* [x] **本地模型** - Ollama 本地部署
* [x] **Coze Bot** - 扣子平台 Bot 集成
* [x] **Function Calling** - 工具调用能力
* [x] **流式响应** - 实时流式输出

### 👁️ 视觉能力
* [x] **VLLLM（视觉语言模型）** - 支持 GLM-4V、Qwen2.5VL
* [x] **图像识别** - 语音控制调用摄像头识别
* [x] **图像安全检测** - 文件大小、格式、像素限制

### 🔧 MCP 协议支持
* [x] **MCP 客户端** - 调用外部 MCP 服务器（高德地图、天气等）
* [x] **MCP 服务器** - 对外提供 MCP 服务
* [x] **本地 MCP 功能** - 时间查询、退出意图、角色切换、音乐播放等
* [x] **资源池管理** - MCP 连接池优化性能

### 🎭 角色与配置
* [x] **多角色支持** - 预设角色配置（怒喵、英语老师等）
* [x] **语音切换** - 动态切换 TTS 音色
* [x] **Bot 配置管理** - 用户自定义 Bot 配置（私有/公开）
* [x] **好友系统** - 用户好友管理，Bot 添加机制

### 🔐 认证与安全
* [x] **JWT 认证** - 基于 JWT 的用户认证
* [x] **设备绑定** - 设备 ID 绑定与授权
* [x] **Token 管理** - 内存/文件/Redis 存储
* [x] **权限控制** - 基于用户的访问控制

### 💾 数据存储
* [x] **SQLite** - 轻量级本地数据库
* [x] **PostgreSQL** - 生产环境数据库
* [x] **Redis** - 缓存与会话存储
* [x] **对话历史** - 支持 SQLite/PostgreSQL/Redis 存储

### 🚀 其他特性
* [x] **OTA 升级** - 设备固件在线升级
* [x] **任务管理** - 异步任务队列与调度
* [x] **连接池** - ASR/LLM/TTS/MCP 资源池管理
* [x] **优雅关闭** - 信号处理与资源清理
* [x] **Swagger 文档** - 完整的 API 文档
* [x] **日志系统** - 结构化日志输出

---

## 项目结构

```
angrymiao-ai-server/
├── src/
│   ├── main.go                 # 程序入口
│   ├── configs/                # 配置管理
│   │   ├── config.go           # 配置加载
│   │   ├── database/           # 数据库初始化
│   │   └── casbin/             # JWT 公钥配置
│   ├── core/                   # 核心功能
│   │   ├── connection.go       # 连接处理器
│   │   ├── auth/               # 认证管理
│   │   ├── botconfig/          # Bot 配置服务
│   │   ├── chat/               # 对话管理
│   │   ├── function/           # 函数注册
│   │   ├── image/              # 图像处理
│   │   ├── mcp/                # MCP 协议实现
│   │   ├── pool/               # 资源池管理
│   │   ├── providers/          # AI 提供商
│   │   │   ├── asr/            # 语音识别
│   │   │   ├── tts/            # 语音合成
│   │   │   ├── llm/            # 大语言模型
│   │   │   ├── vlllm/          # 视觉语言模型
│   │   │   ├── vad/            # 语音活动检测
│   │   │   └── auc/            # 音频转录
│   │   ├── transport/          # 传输层
│   │   │   ├── websocket/      # WebSocket 传输
│   │   │   ├── grpcgateway/    # gRPC 传输
│   │   │   └── mqtt/           # MQTT 传输
│   │   └── utils/              # 工具函数
│   ├── httpsvr/                # HTTP 服务
│   │   ├── app/                # 用户好友管理
│   │   ├── bot/                # Bot 配置管理
│   │   ├── device/             # 设备管理
│   │   ├── ota/                # OTA 升级
│   │   └── vision/             # 视觉服务
│   ├── models/                 # 数据模型
│   ├── task/                   # 任务管理
│   └── docs/                   # Swagger 文档
├── im-server/                  # gRPC IM 服务（可选）
├── mqtt-server/                # MQTT 服务器配置
├── .config.yaml                # 主配置文件
└── go.mod                      # Go 模块依赖
```

---

## 快速开始

### 1. 环境要求

* **Go 1.24.2+**
* **Windows 用户需要 CGO 和 Opus 库**（见下文安装说明）
* **可选：PostgreSQL / Redis**（默认使用 SQLite）

### 2. 配置文件

复制配置模板并修改：

```bash
cp config.yaml .config.yaml
```

#### 主要配置项

**传输层**：支持 WebSocket（默认 8000 端口）、gRPC Gateway、MQTT  
**Web 服务**：HTTP API 端口 8080，Swagger 文档自动生成  
**AI 模型**：在 `selected_module` 中选择使用的 ASR/TTS/LLM/VLLLM 提供商  
**数据库**：默认 SQLite，可切换 PostgreSQL  
**认证**：支持 JWT 认证，Token 存储支持 memory/file/redis

详细配置说明请参考 `.config.yaml` 文件中的注释。

**快速配置示例**：

```yaml
# 选择使用的模块
selected_module:
  ASR: DoubaoASR      # 语音识别
  TTS: DoubaoTTS      # 语音合成
  LLM: QwenLLM        # 大语言模型

# 传输层
transport:
  default: "websocket"
  websocket:
    port: 8000

# Web 服务
web:
  port: 8080
```

### 3. Windows 环境配置（Opus 编译）

Windows 用户需要安装 CGO 和 Opus 库以支持音频编解码。

#### 安装 MSYS2

1. 下载并安装 [MSYS2](https://www.msys2.org/)
2. 打开 **MSYS2 MINGW64** 控制台
3. 执行以下命令：

```bash
# 更新系统
pacman -Syu

# 安装编译工具链和 Opus 库
pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-go mingw-w64-x86_64-opus
pacman -S mingw-w64-x86_64-pkg-config
```

#### 设置环境变量

在 PowerShell 或系统环境变量中设置：

```bash
set PKG_CONFIG_PATH=C:\msys64\mingw64\lib\pkgconfig
set CGO_ENABLED=1
```

#### 验证安装

在 MINGW64 环境下运行一次，确保编译通过：

```bash
go run ./src/main.go
```

**提示**：如果 `go mod` 下载缓慢，可以设置国内镜像：

```bash
go env -w GOPROXY=https://goproxy.cn,direct
```

### 4. 启动服务

```bash
# 安装依赖
go mod tidy

# 启动主服务
go run ./src/main.go
```

服务启动后：
- **HTTP API**: `http://localhost:8080`
- **WebSocket**: `ws://localhost:8000`
- **Swagger 文档**: `http://localhost:8080/swagger/index.html`

### 5. 启动 gRPC IM 服务（可选）

如果需要使用 gRPC Gateway 传输层：

```bash
cd im-server
go run main.go
```

### 6. 启动 MQTT 服务器（可选）

如果需要使用 MQTT 传输层：

```bash
cd mqtt-server
docker-compose up -d
```

---

## MCP 协议配置

MCP（Model Context Protocol）允许服务器调用外部工具和服务。

### 配置本地 MCP 功能

```yaml
local_mcp_fun:
  - time           # 获取系统时间
  - exit           # 识别退出意图
  - change_role    # 切换角色
  - play_music     # 播放本地音乐
  - change_voice   # 切换音色
```

### 配置外部 MCP 服务器

详细配置请参考：`src/core/mcp/README.md`

示例配置（在 `.mcp_server_settings.json` 中）：

```json
{
  "mcpServers": {
    "weather": {
      "command": "uvx",
      "args": ["mcp-server-weather"],
      "env": {}
    }
  }
}
```

---

## 认证与设备绑定

### 认证流程

1. **外部授权系统签发 User JWT**
2. **调用设备绑定接口**
3. **获取 Device Token**
4. **使用 Device Token 连接 WebSocket/MQTT**

### 设备绑定 API

```bash
curl -X POST "http://localhost:8080/api/device/bind" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <UserJWT>" \
  -d '{"device_id":"device-001"}'
```

**响应示例**：

```json
{
  "success": true,
  "device_key": "<bind_key>",
  "token": "<device_token>"
}
```

### WebSocket 连接

**推荐方式**（Header 传递）：

```javascript
const ws = new WebSocket('ws://localhost:8000', {
  headers: {
    'Authorization': 'Bearer <device_token>',
    'Device-Id': 'device-001'
  }
});
```

**浏览器模式**（Query 参数）：

```javascript
const ws = new WebSocket('ws://localhost:8000/?device-id=device-001&token=<device_token>');
```

### JWT 公钥配置

将外部授权系统的 JWT 公钥放置在：

```
src/configs/casbin/jwt/public.pem
```

**参考实现文件**：
- `src/httpsvr/device/server.go` - 设备绑定逻辑
- `src/core/transport/websocket/transport.go` - WebSocket 认证

---

## API 文档

### Swagger 文档

启动服务后访问：

```
http://localhost:8080/swagger/index.html
```

### 更新 Swagger 文档

修改代码后重新生成文档：

```bash
cd src
swag init -g main.go
```

### 主要 API 端点

#### 用户好友管理
- `POST /api/friends` - 添加好友/Bot
- `GET /api/friends` - 获取好友列表
- `DELETE /api/friends/:id` - 删除好友

#### Bot 配置管理
- `POST /api/bots` - 创建 Bot 配置
- `GET /api/bots/:id` - 获取 Bot 详情
- `PUT /api/bots/:id` - 更新 Bot 配置
- `DELETE /api/bots/:id` - 删除 Bot 配置
- `GET /api/bots/search` - 搜索 Bot
- `GET /api/bots/my` - 获取我创建的 Bot

#### 模型配置管理
- `POST /api/models` - 创建模型配置
- `GET /api/models` - 获取模型列表
- `PUT /api/models/:id` - 更新模型配置
- `DELETE /api/models/:id` - 删除模型配置

#### 设备管理
- `POST /api/device/bind` - 设备绑定
- `GET /api/device/info` - 获取设备信息

#### OTA 升级
- `GET /api/ota/check` - 检查更新
- `POST /api/ota/upload` - 上传固件

#### 视觉服务
- `POST /api/vision/analyze` - 图像分析

---

## 使用场景

### 智能硬件设备
- 智能音箱、智能眼镜等语音交互设备
- 支持 WebSocket、MQTT、gRPC 多种连接方式
- 低延迟实时语音对话

### AI 应用开发
- 语音助手应用
- 多模态 AI 应用（语音+视觉）
- 自定义 Bot 平台

### 物联网场景
- MQTT 设备接入
- UDP 音频传输优化
- 设备认证与管理

---

## 技术栈

- **语言**: Go 1.24+
- **Web 框架**: Gin
- **数据库**: SQLite / PostgreSQL
- **缓存**: Redis
- **消息队列**: MQTT
- **音频编解码**: Opus
- **AI 模型**: OpenAI API / Ollama / Coze
- **协议**: WebSocket / gRPC / MQTT / MCP

---

## 开发指南

### 添加新的 AI 提供商

1. 在 `src/core/providers/` 对应目录创建新提供商
2. 实现对应的接口（ASR/TTS/LLM/VLLLM）
3. 在 `init()` 函数中注册提供商
4. 在配置文件中添加配置项

示例：

```go
package myprovider

import "angrymiao-ai-server/src/core/providers/llm"

func init() {
    llm.Register("myprovider", NewProvider)
}

func NewProvider(config *llm.Config) (llm.Provider, error) {
    // 实现提供商逻辑
}
```

### 添加新的 MCP 工具

1. 在 `src/core/mcp/` 中实现工具逻辑
2. 在 `local_mcp_fun` 配置中添加工具名称
3. 注册工具到 Function Registry

---

## 常见问题

### Q: 如何切换不同的 AI 模型？

A: 修改 `.config.yaml` 中的 `selected_module` 配置，然后重启服务。

### Q: 支持哪些语音识别服务？

A: 支持豆包、Deepgram、GoSherpa 等多种 ASR 服务。

### Q: 如何启用 VAD（语音活动检测）？

A: 在客户端连接时设置 HTTP Header `Enable-VAD: true`。

### Q: MQTT 连接失败怎么办？

A: 检查 MQTT 服务器是否启动，用户名密码是否正确，防火墙是否开放端口。

### Q: 如何配置多个 LLM 模型？

A: 在 `LLM` 配置节中添加多个模型配置，通过 `selected_module.LLM` 选择使用哪个。

---

## 许可证

本项目采用 MIT 许可证，详见 [LICENSE](LICENSE) 文件。