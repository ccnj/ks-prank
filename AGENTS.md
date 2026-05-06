# AGENTS.md

This file gives Codex working context for this repository. Keep it aligned with the code, not with older `CLAUDE.md` or historical config examples.

## What Is ks-prank

`ks-prank` is a Wails v2 desktop app for LuckPets live-room prank effects. It has a Go backend and a React/TypeScript/Ant Design frontend.

Despite the name, the current app supports both:

- Kuaishou live rooms via `internal/service/kuaishou.go` and `proto/kuaishou.proto`
- Douyin live rooms via `internal/service/douyin.go` and `pkg/douyincrawler`

The app logs into LuckPets admin APIs, loads the current user's site, AR boxes, live accounts, and prank rules, connects to the selected live account, listens for gifts/comments/likes, and dispatches configured prank actions through MQTT and low-security HTTP APIs.

## Build And Run

```bash
# Development. Requires Wails CLI:
# go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails dev

# Local production build
wails build

# Cross-compile a simple Windows exe with go build
./scripts/build_windows.sh

# Build macOS app, Windows exe, and Windows installer
# Requires Wails and makensis; ImageMagick is optional for icon conversion.
./scripts/build_all.sh
```

Frontend-only commands live under `frontend/`:

```bash
npm install
npm run dev
npm run build
```

`wails.json` runs `npm install`, `npm run dev`, and `npm run build` for the frontend.

## Runtime Config

The persisted local config is intentionally small. It is stored at:

- macOS: `~/Library/Application Support/ks-prank/config-ks.yaml`
- Windows: `%APPDATA%/ks-prank/config-ks.yaml`

Current persisted fields are defined in `config/config.go`:

- `server_url`
- `auth_token`
- `last_account_id`

Business data such as `site_id`, `ar_box_id`, live URLs, gift triggers, chat triggers, like triggers, and prank device SN comes from LuckPets server APIs after login. Do not reintroduce old local YAML-driven `gift_actions` logic unless the user explicitly asks for an offline mode.

Note: `config.yaml.example` is only a minimal local-config sample. Business fields such as `ar_box_id`, `site_id`, `live_url`, and `gift_actions` should not be reintroduced there unless the user explicitly asks for an offline mode.

## Architecture

```text
main.go                         # Wails entrypoint: embeds frontend assets and binds App
app.go                          # App methods exposed to frontend; login/profile/connect orchestration
config/config.go                # Small persisted config: server URL, auth token, last account
frontend/
  src/App.tsx                   # Main React shell: login gate, profile, connection, events
  src/components/               # HeaderBar, LoginPage, SidePanel, EventStream
  src/types.ts                  # UI-side event/rule/action display types
  wailsjs/                      # Generated Wails bindings; not committed
internal/
  global/global.go              # Resty client, MQTT client, persisted config, runtime config
  initialize/
    http.go                     # Resty HTTP client setup
    mqtt.go                     # FetchMqttConfig and MQTT client setup
    chrome.go                   # chromedp capture for Kuaishou token and Douyin WSS/cookies
  service/
    admin_api.go                # Admin login, profile, prank config APIs
    kuaishou.go                 # Kuaishou WebSocket client and event dispatch
    douyin.go                   # Douyin crawler bridge and event dispatch
    gift_deduper.go             # Douyin gift de-duplication helper
  protocol/kuaishou.go          # Kuaishou socket frame/protobuf helpers
  worker/dispatcher.go          # Per-worker_group serial queues
  types/prank.go                # Server-driven profile, live account, and prank config types
  handler/                      # Action registry and action implementations
  consts/                       # Low-security key and Kuaishou gift map
proto/                          # Kuaishou protobuf definitions/generated Go
pkg/douyincrawler/              # Douyin crawler client, protobufs, and message handlers
scripts/                        # Build scripts and NSIS installer config
```

## Wails API Surface

Frontend imports Go methods from `frontend/wailsjs/go/main/App`. The generated files are not committed, so run `wails dev` or `wails generate module` when bindings are stale.

Current `app.go` methods used by the frontend:

| Go method | Purpose |
| --- | --- |
| `GetLoginState()` | Check persisted auth token and cached username state |
| `Login(username, password)` | Call admin login and persist JWT |
| `Logout()` | Disconnect, clear token/profile, and persist config |
| `GetProfile()` | Load current user, site, AR boxes, live accounts, and prank device SN |
| `GetLastAccountId()` | Restore last selected live account |
| `GetPrankRules(liveAccountId)` | Load server-side gift/chat/like rules for the selected account's platform |
| `Connect(liveAccountId)` | Fetch prank config, MQTT config, live WSS info, then connect selected platform |
| `Disconnect()` | Close the active platform client |
| `GetStatus()` | Return current connection status |

Older methods such as `GetConfig`, `SaveConfig`, and `FetchToken` are not present in current `app.go`.

Go emits frontend events through `runtime.EventsEmit`:

- `event:status` with a string such as `disconnected`, `connecting`, `fetching_token`, or `connected`
- `event:gift` with `service.GiftEventData`
- `event:comment` with `service.CommentEventData`
- `event:action` and `event:log` are part of the UI event model, but are not widely emitted by current backend code

## Data Flow

1. User logs in through `/api/v1/admin/user/login`; JWT is stored in local config.
2. `GetProfile()` loads site, AR boxes, live accounts, and optional prank device SN through `/api/v1/admin/prank/my_profile`.
3. User selects a live account. `GetPrankRules()` pulls server-side rules through `/api/v1/admin/prank_config/get`.
4. `Connect(liveAccountId)` validates profile/site/account and builds `global.Runtime`.
5. MQTT credentials are fetched from `/api/v1/fight/low_security/get_mqtt_config`.
6. Platform-specific WSS setup runs:
   - Kuaishou: chromedp captures `websocketinfo`, token, WSS URL, and `liveStreamId`.
   - Douyin: chromedp ensures logged-in cookies, captures the IM push WSS URL, and exports `.douyin.com` cookies.
7. Gifts/comments/likes become frontend events and are matched against server-provided triggers.
8. `worker.Dispatcher` executes selected action choices. Same `worker_group` is serial; different groups run independently.

## Trigger And Action Model

Server-side prank config is represented by `internal/types/prank.go`:

- `GiftTrigger`: exact gift-name match
- `ChatTrigger`: exact comment keyword match
- `LikeTrigger`: threshold-based likes
- `ActionChoice`: weighted action selection with `worker_group` and raw JSON `params`

Both Kuaishou and Douyin use the same action dispatch path:

```text
live event -> trigger map -> pickChoice -> Dispatcher -> handler.RunChoice
```

Kuaishou gift IDs are mapped to names/prices through `internal/consts/gifts.go`. Douyin gift names are parsed from the message description and gift value from diamond count.

## Adding A New Prank Action

1. Add an implementation file under `internal/handler/`.
2. Register the action name in `internal/handler/registry.go`.
3. Parse that action's `params` in the registry wrapper.
4. If it needs runtime context, use `global.Runtime` and handle missing `ArBoxId` or `PrankDeviceSn` gracefully.
5. If it publishes MQTT, keep topics consistent with server ACLs.
6. Configure the action on the server-side prank config; the local config file is not the source of truth.

Current registered actions:

- `attack_monster_360`
- `heal_monster`
- `throw_cockroach`
- `add_monster`
- `update_aa_level`
- `spin`
- `pet_feed`
- `pet_tease`

## Worker Groups

`internal/worker/dispatcher.go` routes every task by `worker_group`:

- Same group: one queue, serial execution
- Different groups: separate queues, parallel execution

This is important for long-running actions such as `pet_feed`, `pet_tease`, and `spin`. Choose worker groups in server config according to whether actions should block each other.

## MQTT Topics

MQTT credentials are fetched dynamically for every connection and are not persisted locally.

Known publish targets in the current handlers:

| Topic template | Used by |
| --- | --- |
| `BOX/{ar_box_id}/fight` | `attack_monster_360` |
| `SITE/{site_id}/prank_event` | `throw_cockroach` |
| `SITE/{site_id}/live_room_gift` | `publishLiveRoomGiftInfo`, used by most actions |
| `RC/{sn}/ctrl` | `pet_feed`, `pet_tease`, `spin` |

If a new action publishes a new topic, confirm EMQX ACL support server-side.

## HTTP API Calls

Admin APIs use the JWT from login:

| Path | Purpose |
| --- | --- |
| `/api/v1/admin/user/login` | Login; token returned in `Authorization` header |
| `/api/v1/admin/prank/my_profile` | Current user/site/AR boxes/live accounts/prank device |
| `/api/v1/admin/prank_config/get` | Server-side triggers for site + platform |

Low-security fight APIs include `sec_key`:

| Path | Purpose |
| --- | --- |
| `/api/v1/fight/low_security/get_mqtt_config` | MQTT connection credentials |
| `/api/v1/fight/low_security/get_current_monster` | Current monster for attack target |
| `/api/v1/fight/low_security/heal_monster` | Heal monster |
| `/api/v1/fight/low_security/add_monster` | Add monster |
| `/api/v1/fight/low_security/update_user_aa_level` | Update weapon level |
| `/api/v1/fight/low_security/get_using_sn_by_uid` | Resolve current RC device for spin |
| `/api/v1/fight/low_security/add_ks_gift_log` | Log Kuaishou gifts |
| `/api/v1/fight/low_security/add_dy_gift_log` | Log Douyin gifts |

The low-security key currently appears in both `internal/consts/consts.go` and `internal/handler/common.go`; prefer consolidating instead of adding another copy.

## Douyin Login Caveat

Douyin live WS behavior depends on logged-in cookies:

- Anonymous/unauthenticated sessions may receive public chat/like messages but not the full gift/member stream.
- Logged-in `.douyin.com` cookies must be present before the live room creates its WebSocket.

Current implementation in `internal/initialize/chrome.go`:

1. Uses persistent Chrome data dir `~/.ks-prank/chrome-user-data-dy`.
2. Navigates to `https://www.douyin.com/` first.
3. Polls cookies until `sessionid` exists, prompting the user to log in if needed.
4. Waits 3 seconds for related cookies such as `sessionid_ss`, `sid_guard`, and `ttwid`.
5. Navigates to the live room.
6. Accepts only WebSocket URLs containing `app_name=douyin_web` and `/webcast/im/push/`.
7. Chooses the best candidate with `pickBestDouyinWss`, preferring `/webcast/im/push/v2/` and `identity=audience`.
8. Extracts `.douyin.com` cookies and passes them to `douyincrawler.NewDouyinClient`.

Do not fall back to old hard-coded anonymous cookies. A copied WSS URL alone is not enough; the Cookie header is part of the identity.

## Kuaishou Token Capture

Kuaishou uses `FetchWssInfo()` in `internal/initialize/chrome.go`:

- Persistent Chrome data dir: `~/.ks-prank/chrome-user-data`
- Opens the live URL and listens for `websocketinfo`
- Handles slider verification by prompting via system speech
- Extracts token, first WSS URL, and `liveStreamId`
- `service.KuaishouClient.Connect()` sends enter-room protobuf and starts heartbeat

Kuaishou does not use the Douyin cookie login flow.

## Generated Files And Local Changes

- `frontend/wailsjs/` is generated by Wails and ignored.
- Protobuf generated files are committed: `proto/kuaishou.pb.go` and `pkg/douyincrawler/proto/douyin.pb.go`.
- Before editing generated protobuf output, update the `.proto` source and regenerate intentionally.
- Be careful with untracked files. At the time this file was written, `AGENTS.md` itself was untracked.

## Testing And Verification

There are no dedicated test commands in the repository today. Useful checks:

```bash
go test ./...
cd frontend && npm run build
```

For Wails binding or frontend integration changes, run `wails dev` or `wails build` when feasible. For Chrome capture changes, expect manual verification with real Kuaishou/Douyin live URLs and login state.
