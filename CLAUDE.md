# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is ks-prank

快手直播间整蛊插件，基于 Wails v2 构建的桌面应用（Go 后端 + React/TypeScript/Ant Design 前端）。连接快手直播间 WebSocket，监听礼物和弹幕事件，通过 MQTT 和 HTTP API 触发 AR 战斗场景中的整蛊效果（攻击怪物、回血、丢蟑螂等）。属于 LuckPets 项目的子模块，与 `dy-prank`（抖音版）功能对等。

## Build & Run

```bash
# 开发运行（需先安装 Wails CLI：go install github.com/wailsapp/wails/v2/cmd/wails@latest）
wails dev

# 交叉编译 Windows
./scripts/build_windows.sh    # 输出到 build/ks-prank.exe

# 本地编译
wails build
```

首次启动时，程序会将项目根目录下的 `config.yaml` 复制到 OS 标准配置目录作为用户配置：
- **macOS**: `~/Library/Application Support/ks-prank/config-ks.yaml`
- **Windows**: `%APPDATA%/ks-prank/config-ks.yaml`

用户配置中需要填入：
- `ar_box_id` 和 `site_id`：从数据库获取
- `live_url`：快手直播间地址（程序通过 Chrome 自动抓取 token 和 wss 地址）

MQTT 连接凭证从 luck-pets-server 动态获取，无需手动配置。

## Architecture

```
main.go                          # Wails 入口：嵌入前端资源、绑定 App 实例
app.go                           # App 结构体：前端绑定方法（GetConfig/SaveConfig/Connect/Disconnect/FetchToken）
├── config/config.go             # Viper 加载配置，yaml 序列化保存
├── frontend/                    # React + TypeScript + Ant Design 前端
│   ├── src/App.tsx              # 单页应用：配置面板 + 事件流展示
│   └── wailsjs/                 # Wails 自动生成的 Go 绑定和类型定义（.gitignore）
├── proto/                       # 快手 WebSocket protobuf 定义（proto2）
│   ├── kuaishou.proto
│   └── kuaishou.pb.go
├── internal/
│   ├── global/global.go         # 全局单例：HttpClient (resty)、MQTTClient、Config
│   ├── initialize/
│   │   ├── http.go              # HTTP client 初始化
│   │   ├── mqtt.go              # MQTT 连接初始化 + FetchMqttConfig（从 server 获取凭证）
│   │   └── chrome.go            # chromedp 自动获取快手 WSS token
│   ├── protocol/kuaishou.go     # protobuf 消息构建（进房、心跳）和解析（gzip 解压）
│   ├── service/kuaishou.go      # 快手 WebSocket 客户端：连接、监听、事件分发到前端
│   ├── worker/dispatcher.go     # 礼物任务调度器（按 worker_group 分队列串行执行）
│   ├── handler/
│   │   ├── registry.go          # Action 注册表：action name → ActionFactory
│   │   ├── common.go            # 公共工具：MQTT 发布、HTTP 上报礼物日志
│   │   ├── attack_monster_360.go  # 360度攻击怪物（先 HTTP 获取当前怪物 ID，再 MQTT 发射弹道）
│   │   ├── heal_monster.go      # 怪物回血（HTTP API 调用）
│   │   └── throw_cockroach.go   # 丢蟑螂整蛊（MQTT 发布事件）
│   └── consts/
│       ├── consts.go            # 常量（sec_key）
│       └── gifts.go             # 快手礼物 ID → 名称/价格映射表
└── scripts/build_windows.sh
```

### Go ↔ 前端交互

采用 Wails v2 绑定机制，前端通过自动生成的 TypeScript 函数调用 Go 方法：

| Go 方法 (app.go) | 前端调用 | 用途 |
|---|---|---|
| `GetConfig()` | `import { GetConfig } from "wailsjs/go/main/App"` | 加载配置到表单 |
| `SaveConfig(cfg)` | `SaveConfig(new config.Config({...}))` | 保存配置到磁盘 |
| `FetchToken(liveUrl)` | `FetchToken(url)` | Chrome 自动获取快手 WSS 信息 |
| `Connect()` | `Connect()` | 连接快手直播间 |
| `Disconnect()` | `Disconnect()` | 断开连接 |

Go → 前端事件推送通过 `runtime.EventsEmit`，前端用 `EventsOn` 监听：
- `event:status` — 连接状态变更（string）
- `event:gift` — 礼物事件（EventPayload）
- `event:comment` — 弹幕事件（EventPayload）

### Key data flow

1. **快手 WebSocket** → protobuf `SCWebFeedPush` → `handleFeedPush()` 解析出礼物/弹幕
2. 礼物 ID 通过 `consts.GiftMap` 映射快手礼物 ID → 中文名和价格
3. 中文名匹配配置中的 `gift_actions` → 查找对应 action
4. `GiftDispatcher` 按 `worker_group` 分配到不同队列，同组任务串行执行，不同组并行
5. Action handler 通过 MQTT 发消息到 `BOX/{ar_box_id}/fight` 或 `SITE/{site_id}/prank_event`，同时通过 `SITE/{site_id}/live_room_gift` 发送礼物通知

### Adding a new prank action

1. 在 `internal/handler/` 新建文件，实现 handler 函数
2. 在 `registry.go` 的 `ActionRegistry` 中注册 action name → factory
3. 在配置文件的 `gift_actions` 中配置触发规则

### Worker group 机制

`worker_group` 控制并发隔离：相同 group 的礼物共享一个队列（串行），不同 group 的队列互不阻塞。例如攻击（group 0）不会阻塞回血（group 1）。

### MQTT

MQTT 凭证在每次连接时从 luck-pets-server 动态获取（`FetchMqttConfig`），不存储在本地配置文件中。EMQX broker 为 ks-prank 分配了专用账号，ACL 限制只能 publish 到以下 topic：

| Topic 模板 | 用途 | 发送者 |
|---|---|---|
| `BOX/{ar_box_id}/fight` | AR 战斗指令（攻击弹道） | attack_monster_360 |
| `SITE/{site_id}/prank_event` | 整蛊事件（丢蟑螂） | throw_cockroach |
| `SITE/{site_id}/live_room_gift` | 礼物通知（展示在 AR 页面） | 所有 action |

### HTTP API 调用（→ luck-pets-server）

| Path | 用途 |
|---|---|
| `/api/v1/fight/low_security/get_mqtt_config` | 获取 MQTT 连接凭证 |
| `/api/v1/fight/low_security/get_current_monster` | 获取当前怪物 ID |
| `/api/v1/fight/low_security/heal_monster` | 怪物回血 |
| `/api/v1/fight/low_security/add_ks_gift_log` | 上报快手礼物日志 |

所有 low_security 接口使用 `sec_key` 认证（见 `internal/consts/consts.go`）。
