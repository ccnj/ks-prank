# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is ks-prank

快手直播间整蛊插件。连接快手直播间 WebSocket，监听礼物和弹幕事件，通过 MQTT 和 HTTP API 触发 AR 战斗场景中的整蛊效果（攻击怪物、回血、丢蟑螂等）。属于 LuckPets 项目的子模块，与 `dy-prank`（抖音版）功能对等。

## Build & Run

```bash
# 开发运行
go run .

# 交叉编译 Windows
./scripts/build_windows.sh    # 输出到 build/ks-prank.exe

# 本地编译
go build -o ks-prank .
```

运行前必须配置 `config.yaml`（参考 `config.yaml.example`），需要填入：
- `token` 和 `live_stream_id`：从浏览器开发者工具 Network 面板搜索 `websocketinfo` 获取
- `ar_box_id` 和 `site_id`：从数据库获取
- MQTT broker 连接信息

也可通过命令行参数覆盖快手连接信息：`./ks-prank <wss_url> <token> <live_stream_id>`

## Architecture

```
main.go                          # 入口：快手 WebSocket 连接、protobuf 解析、礼物/弹幕分发
├── config/config.go             # Viper 加载 config.yaml，init() 时自动执行
├── proto/                       # 快手 WebSocket protobuf 定义（proto2）
│   ├── kuaishou.proto
│   └── kuaishou.pb.go
├── internal/
│   ├── global/global.go         # 全局单例：HttpClient (resty)、MQTTClient
│   ├── initialize/              # 初始化 HTTP client 和 MQTT 连接
│   ├── worker/dispatcher.go     # 礼物任务调度器（按 worker_group 分队列串行执行）
│   ├── handler/
│   │   ├── registry.go          # Action 注册表：action name → ActionFactory
│   │   ├── common.go            # 公共工具：MQTT 发布、HTTP 上报礼物日志
│   │   ├── attack_monster_360.go  # 360度攻击怪物（先 HTTP 获取当前怪物 ID，再 MQTT 发射弹道）
│   │   ├── heal_monster.go      # 怪物回血（HTTP API 调用）
│   │   └── throw_cockroach.go   # 丢蟑螂整蛊（MQTT 发布事件）
│   └── consts/consts.go
└── scripts/build_windows.sh
```

### Key data flow

1. **快手 WebSocket** → protobuf `SCWebFeedPush` → `handleFeedPush()` 解析出礼物/弹幕
2. 礼物名称通过 `giftMap`（main.go 中硬编码）映射快手礼物 ID → 中文名
3. 中文名匹配 `config.yaml` 中的 `gift_actions` → 查找对应 action
4. `GiftDispatcher` 按 `worker_group` 分配到不同队列，同组任务串行执行，不同组并行
5. Action handler 通过 MQTT 发消息到 `BOX/{ar_box_id}/fight` 或 `SITE/{site_id}/prank_event`，同时通过 `SITE/{site_id}/live_room_gift` 发送礼物通知

### Adding a new prank action

1. 在 `internal/handler/` 新建文件，实现 handler 函数
2. 在 `registry.go` 的 `ActionRegistry` 中注册 action name → factory
3. 在 `config.yaml` 的 `gift_actions` 中配置触发规则

### Worker group 机制

`worker_group` 控制并发隔离：相同 group 的礼物共享一个队列（串行），不同 group 的队列互不阻塞。例如攻击（group 0）不会阻塞回血（group 1）。

### MQTT topics

| Topic 模板 | 用途 | 发送者 |
|---|---|---|
| `BOX/{ar_box_id}/fight` | AR 战斗指令（攻击弹道） | attack_monster_360 |
| `SITE/{site_id}/prank_event` | 整蛊事件（丢蟑螂） | throw_cockroach |
| `SITE/{site_id}/live_room_gift` | 礼物通知（展示在 AR 页面） | 所有 action |

### HTTP API 调用（→ luck-pets-server）

| Path | 用途 |
|---|---|
| `/api/v1/fight/low_security/get_current_monster` | 获取当前怪物 ID |
| `/api/v1/fight/low_security/heal_monster` | 怪物回血 |
| `/api/v1/fight/low_security/add_ks_gift_log` | 上报快手礼物日志 |

所有 low_security 接口使用 `sec_key` 认证（见 `internal/consts/consts.go`）。
