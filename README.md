# Dialogue is All You Need (AM glasses Backend)

English | [中文](README_zh.md)

> A voice-first, multimodal backend that uses conversation to accomplish most tasks—without traditional GUIs or apps.

---

## Contents
- [Overview](#overview)
- [What "Dialogue is All You Need" Means](#what-dialogue-is-all-you-need-means)
- [Features](#features)
- [Project Structure](#project-structure)
- [Quick Start](#quick-start)
  - [Prerequisites](#1-prerequisites)
  - [Configuration](#2-configuration)
  - [Windows Setup (Opus)](#3-windows-setup-opus-compilation)
  - [Run the Service](#4-run-the-service)
  - [Start gRPC IM Service](#5-start-grpc-im-service-optional)
  - [Start MQTT Server](#6-start-mqtt-server-optional)
- [MCP Configuration](#mcp-configuration)
- [Authentication & Device Binding](#authentication--device-binding)
- [API Documentation](#api-documentation)
- [Use Cases](#use-cases)
- [Tech Stack](#tech-stack)
- [Development Guide](#development-guide)
- [FAQ](#faq)
- [License](#license)

---

## Overview
Dialogue is All You Need (AM glass backend) is an end-to-end, cross-platform server for voice and multimodal interactions. It supports flexible transports, pluggable ASR/TTS/LLM providers, and MCP-based tool integrations (e.g., maps, weather). The system is designed so that most user workflows are completed through natural conversation, with optional lightweight visual inserts when needed.

---

## What "Dialogue is All You Need" Means
- Conversation can address 80%+ of needs that were previously solved by GUIs and standalone apps. Instead of navigating screens, users simply say what they want.
- Working with AI should feel like collaborating with a teammate—state intent, clarify context, negotiate steps, and iterate quickly, all via dialogue.
- For the minority of cases that truly require a UI (e.g., rich data display, structured input), inject lightweight H5 cards directly into the conversation. This preserves the dialogue-first flow while providing:
  - Higher efficiency (no app switching, minimal context loss)
  - Better cross-platform behavior (HTML5 cards render consistently)
- It's an end-to-end cross-platform solution. The initiating endpoint can be extremely lightweight—not only PC or mobile browsers, but also any embedded personal device running Linux or even RTOS can access the service.

---

## Features

### 🎯 Multi-Transport Support
* [x] **WebSocket** - Real-time bidirectional communication for browsers and native clients
* [x] **gRPC Gateway** - High-performance RPC communication
* [x] **MQTT** - IoT device messaging with UDP audio transport
* [x] **Multi-Protocol** - Enable multiple transport protocols simultaneously

### 🎤 Voice Processing
* [x] **ASR (Speech Recognition)** - Doubao streaming, Deepgram, GoSherpa
* [x] **TTS (Text-to-Speech)** - Doubao, EdgeTTS, Deepgram, GoSherpa
* [x] **VAD (Voice Activity Detection)** - WebRTC VAD for smart speech start/end detection
* [x] **Audio Formats** - PCM and Opus codec support
* [x] **AUC (Audio Transcription)** - Doubao audio file recognition

### 🤖 LLM Integration
* [x] **OpenAI-Compatible API** - Qwen, ChatGLM, DeepSeek, etc.
* [x] **Local Models** - Ollama local deployment
* [x] **Coze Bot** - Coze platform bot integration
* [x] **Function Calling** - Tool invocation capabilities
* [x] **Streaming Response** - Real-time streaming output

### 👁️ Vision Capabilities
* [x] **VLLLM (Vision Language Models)** - GLM-4V, Qwen2.5VL support
* [x] **Image Recognition** - Voice-controlled camera invocation
* [x] **Image Security** - File size, format, and pixel validation

### 🔧 MCP Protocol Support
* [x] **MCP Client** - Call external MCP servers (AMap, weather, etc.)
* [x] **MCP Server** - Provide MCP services to external clients
* [x] **Local MCP Functions** - Time query, exit intent, role switching, music playback, voice changing
* [x] **Resource Pool** - MCP connection pool for performance optimization

### 🎭 Roles & Configuration
* [x] **Multi-Role Support** - Preset role configurations (AngryMiao, English Teacher, etc.)
* [x] **Voice Switching** - Dynamic TTS voice changing
* [x] **Bot Configuration** - User-defined bot configs (private/public)
* [x] **Friend System** - User friend management and bot addition

### 🔐 Authentication & Security
* [x] **JWT Authentication** - JWT-based user authentication
* [x] **Device Binding** - Device ID binding and authorization
* [x] **Token Management** - Memory/file/Redis storage options
* [x] **Access Control** - User-based permission control

### 💾 Data Storage
* [x] **SQLite** - Lightweight local database
* [x] **PostgreSQL** - Production database
* [x] **Redis** - Cache and session storage
* [x] **Dialogue History** - SQLite/PostgreSQL/Redis storage support

### 🚀 Additional Features
* [x] **OTA Updates** - Device firmware over-the-air updates
* [x] **Task Management** - Async task queue and scheduling
* [x] **Connection Pools** - ASR/LLM/TTS/MCP resource pool management
* [x] **Graceful Shutdown** - Signal handling and resource cleanup
* [x] **Swagger Docs** - Complete API documentation
* [x] **Logging System** - Structured logging output

---

## Project Structure

```
angrymiao-ai-server/
├── src/
│   ├── main.go                 # Program entry point
│   ├── configs/                # Configuration management
│   │   ├── config.go           # Config loading
│   │   ├── database/           # Database initialization
│   │   └── casbin/             # JWT public key config
│   ├── core/                   # Core functionality
│   │   ├── connection.go       # Connection handler
│   │   ├── auth/               # Authentication management
│   │   ├── botconfig/          # Bot configuration service
│   │   ├── chat/               # Dialogue management
│   │   ├── function/           # Function registry
│   │   ├── image/              # Image processing
│   │   ├── mcp/                # MCP protocol implementation
│   │   ├── pool/               # Resource pool management
│   │   ├── providers/          # AI providers
│   │   │   ├── asr/            # Speech recognition
│   │   │   ├── tts/            # Text-to-speech
│   │   │   ├── llm/            # Large language models
│   │   │   ├── vlllm/          # Vision language models
│   │   │   ├── vad/            # Voice activity detection
│   │   │   └── auc/            # Audio transcription
│   │   ├── transport/          # Transport layer
│   │   │   ├── websocket/      # WebSocket transport
│   │   │   ├── grpcgateway/    # gRPC transport
│   │   │   └── mqtt/           # MQTT transport
│   │   └── utils/              # Utility functions
│   ├── httpsvr/                # HTTP services
│   │   ├── app/                # User friend management
│   │   ├── bot/                # Bot configuration management
│   │   ├── device/             # Device management
│   │   ├── ota/                # OTA updates
│   │   └── vision/             # Vision services
│   ├── models/                 # Data models
│   ├── task/                   # Task management
│   └── docs/                   # Swagger documentation
├── im-server/                  # gRPC IM service (optional)
├── mqtt-server/                # MQTT server configuration
├── .config.yaml                # Main configuration file
└── go.mod                      # Go module dependencies
```

---

## Quick Start

### 1. Prerequisites

* **Go 1.24.2+**
* **Windows users need CGO and Opus library** (see installation below)
* **Optional: PostgreSQL / Redis** (defaults to SQLite)

### 2. Configuration

Copy the config template and modify:

```bash
cp config.yaml .config.yaml
```

#### Key Configuration Items

**Transport Layer**: WebSocket (default port 8000), gRPC Gateway, MQTT  
**Web Service**: HTTP API port 8080, auto-generated Swagger docs  
**AI Models**: Select ASR/TTS/LLM/VLLLM providers in `selected_module`  
**Database**: Default SQLite, switchable to PostgreSQL  
**Authentication**: JWT auth with memory/file/redis token storage

See comments in `.config.yaml` for detailed configuration.

**Quick Config Example**:

```yaml
# Select modules to use
selected_module:
  ASR: DoubaoASR      # Speech recognition
  TTS: DoubaoTTS      # Text-to-speech
  LLM: QwenLLM        # Large language model

# Transport layer
transport:
  default: "websocket"
  websocket:
    port: 8000

# Web service
web:
  port: 8080
```

### 3. Windows Setup (Opus Compilation)

Windows users need CGO and Opus library for audio codec support.

#### Install MSYS2

1. Download and install [MSYS2](https://www.msys2.org/)
2. Open **MSYS2 MINGW64** console
3. Run the following commands:

```bash
# Update system
pacman -Syu

# Install toolchain and Opus library
pacman -S mingw-w64-x86_64-gcc mingw-w64-x86_64-go mingw-w64-x86_64-opus
pacman -S mingw-w64-x86_64-pkg-config
```

#### Set Environment Variables

In PowerShell or system environment variables:

```bash
set PKG_CONFIG_PATH=C:\msys64\mingw64\lib\pkgconfig
set CGO_ENABLED=1
```

#### Verify Installation

Run once in MINGW64 environment to ensure compilation succeeds:

```bash
go run ./src/main.go
```

**Tip**: If `go mod` downloads slowly, set a domestic mirror:

```bash
go env -w GOPROXY=https://goproxy.cn,direct
```

### 4. Run the Service

```bash
# Install dependencies
go mod tidy

# Start main service
go run ./src/main.go
```

After service starts:
- **HTTP API**: `http://localhost:8080`
- **WebSocket**: `ws://localhost:8000`
- **Swagger Docs**: `http://localhost:8080/swagger/index.html`

### 5. Start gRPC IM Service (Optional)

If using gRPC Gateway transport:

```bash
cd im-server
go run main.go
```

### 6. Start MQTT Server (Optional)

If using MQTT transport:

```bash
cd mqtt-server
docker-compose up -d
```

---

## MCP Configuration

MCP (Model Context Protocol) allows the server to call external tools and services.

### Configure Local MCP Functions

```yaml
local_mcp_fun:
  - time           # Get system time
  - exit           # Recognize exit intent
  - change_role    # Switch roles
  - play_music     # Play local music
  - change_voice   # Change voice
```

### Configure External MCP Servers

See detailed configuration: `src/core/mcp/README.md`

Example configuration (in `.mcp_server_settings.json`):

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

## Authentication & Device Binding

### Authentication Flow

1. **External auth system issues User JWT**
2. **Call device binding API**
3. **Get Device Token**
4. **Use Device Token to connect WebSocket/MQTT**

### Device Binding API

```bash
curl -X POST "http://localhost:8080/api/device/bind" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <UserJWT>" \
  -d '{"device_id":"device-001"}'
```

**Response Example**:

```json
{
  "success": true,
  "device_key": "<bind_key>",
  "token": "<device_token>"
}
```

### WebSocket Connection

**Recommended (Header)**:

```javascript
const ws = new WebSocket('ws://localhost:8000', {
  headers: {
    'Authorization': 'Bearer <device_token>',
    'Device-Id': 'device-001'
  }
});
```

**Browser Mode (Query)**:

```javascript
const ws = new WebSocket('ws://localhost:8000/?device-id=device-001&token=<device_token>');
```

### JWT Public Key Configuration

Place the external auth system's JWT public key at:

```
src/configs/casbin/jwt/public.pem
```

**Reference Implementation**:
- `src/httpsvr/device/server.go` - Device binding logic
- `src/core/transport/websocket/transport.go` - WebSocket authentication

---

## API Documentation

### Swagger Documentation

After starting the service, visit:

```
http://localhost:8080/swagger/index.html
```

### Update Swagger Docs

Regenerate docs after code changes:

```bash
cd src
swag init -g main.go
```

### Main API Endpoints

#### User Friend Management
- `POST /api/friends` - Add friend/bot
- `GET /api/friends` - Get friend list
- `DELETE /api/friends/:id` - Delete friend

#### Bot Configuration Management
- `POST /api/bots` - Create bot config
- `GET /api/bots/:id` - Get bot details
- `PUT /api/bots/:id` - Update bot config
- `DELETE /api/bots/:id` - Delete bot config
- `GET /api/bots/search` - Search bots
- `GET /api/bots/my` - Get my created bots

#### Model Configuration Management
- `POST /api/models` - Create model config
- `GET /api/models` - Get model list
- `PUT /api/models/:id` - Update model config
- `DELETE /api/models/:id` - Delete model config

#### Device Management
- `POST /api/device/bind` - Device binding
- `GET /api/device/info` - Get device info

#### OTA Updates
- `GET /api/ota/check` - Check for updates
- `POST /api/ota/upload` - Upload firmware

#### Vision Services
- `POST /api/vision/analyze` - Image analysis

---

## Use Cases

### Smart Hardware Devices
- Smart speakers, smart glasses, and other voice interaction devices
- Support for WebSocket, MQTT, gRPC connection methods
- Low-latency real-time voice dialogue

### AI Application Development
- Voice assistant applications
- Multimodal AI applications (voice + vision)
- Custom bot platforms

### IoT Scenarios
- MQTT device access
- UDP audio transmission optimization
- Device authentication and management

---

## Tech Stack

- **Language**: Go 1.24+
- **Web Framework**: Gin
- **Database**: SQLite / PostgreSQL
- **Cache**: Redis
- **Message Queue**: MQTT
- **Audio Codec**: Opus
- **AI Models**: OpenAI API / Ollama / Coze
- **Protocols**: WebSocket / gRPC / MQTT / MCP

---

## Development Guide

### Adding New AI Providers

1. Create new provider in `src/core/providers/` directory
2. Implement corresponding interface (ASR/TTS/LLM/VLLLM)
3. Register provider in `init()` function
4. Add configuration in config file

Example:

```go
package myprovider

import "angrymiao-ai-server/src/core/providers/llm"

func init() {
    llm.Register("myprovider", NewProvider)
}

func NewProvider(config *llm.Config) (llm.Provider, error) {
    // Implement provider logic
}
```

### Adding New MCP Tools

1. Implement tool logic in `src/core/mcp/`
2. Add tool name to `local_mcp_fun` config
3. Register tool to Function Registry

---

## FAQ

### Q: How to switch between different AI models?

A: Modify the `selected_module` config in `.config.yaml`, then restart the service.

### Q: Which speech recognition services are supported?

A: Supports Doubao, Deepgram, GoSherpa, and other ASR services.

### Q: How to enable VAD (Voice Activity Detection)?

A: Set HTTP Header `Enable-VAD: true` when client connects.

### Q: What to do if MQTT connection fails?

A: Check if MQTT server is running, verify username/password, and ensure firewall ports are open.

### Q: How to configure multiple LLM models?

A: Add multiple model configs in the `LLM` section, select which to use via `selected_module.LLM`.

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
