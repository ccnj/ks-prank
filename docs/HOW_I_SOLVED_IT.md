# 快手直播间礼物抓包：问题解决全过程

## 一、核心思路：逆向工程的三步法

遇到"抓取某个平台的实时数据"这类问题，通用思路是：

1. **先找前人的成果** — 不要从零开始逆向，先看有没有人已经做过
2. **验证协议是否仍然有效** — 平台会频繁更新，开源项目可能过时
3. **用最小代价验证，再逐步完善** — 先跑通 demo，再考虑工程化

下面按实际的时间线回顾每一步。

---

## 二、第一步：调研（5分钟定方向）

### 做了什么

在 GitHub 和技术博客上搜索关键词：
- `kuaishou live websocket`
- `快手直播 弹幕 protobuf`
- `快手 直播间 礼物 抓取`

### 找到了什么

| 项目 | 价值 |
|------|------|
| [wbt5/real-url](https://github.com/wbt5/real-url) (Python) | **最关键** — 包含完整的 `.proto` 文件和工作流程 |
| [zzzzqs/kuaishou-live-barrage](https://github.com/zzzzqs/kuaishou-live-barrage) (JS) | 补充了 WebSocket 连接细节，但已归档 |
| CSDN 多篇博文 | 确认了 GraphQL 获取 token 的方式 |

### 关键收获

从 `wbt5/real-url` 的 Python 代码中，提取出了完整的技术方案：

- **协议**: WebSocket + Protocol Buffers (proto2)
- **连接流程**: 获取 token → 连接 WSS → 发送进房消息 → 心跳保活
- **消息结构**: `SocketMessage` 包裹层，`payloadType` 路由，`SCWebFeedPush.giftFeeds` 是礼物数据
- **Proto 定义**: 直接拿到了 `.proto` 文件，不需要自己逆向

### 思路要点

> **"站在巨人的肩膀上"** — 逆向工程最大的成本是搞清楚协议格式。如果有人已经做过这个工作，直接复用能节省 90% 的时间。即使他的代码不能直接运行（平台更新了），协议格式通常不会大改。

---

## 三、第二步：验证协议（踩坑与调整）

拿到了理论上的方案，但实际跑起来遇到了几个问题。

### 坑1：页面结构已变，`wsFeedInfo` 不存在

**预期**: 调研资料说，访问快手移动端页面 `https://livev.m.chenzhongtech.com/fw/live/{roomId}` 可以从 HTML 中正则提取 `wsFeedInfo`（包含 token 和 WSS 地址）。

**实际**: 页面返回了 HTML，但搜索 `wsFeedInfo`、`webSocketUrl`、`liveStreamId` 全部为空。快手已经改版，移动端页面不再内嵌这些信息。

**应对**: 不纠结于这条路，立刻换方向 — 去看 PC 端页面。

### 坑2：PC 端 `__INITIAL_STATE__` 中 token 为空

**预期**: PC 端页面 `https://live.kuaishou.com/u/{roomId}` 的 `window.__INITIAL_STATE__` 中应该包含 WebSocket 连接信息。

**实际**: 找到了 `__INITIAL_STATE__`，里面有 `websocketUrls: []` 和 `token: ""`，都是空的。但 `liveStreamId: "nWRydQ_je14"` 和 `isLiving: true` 是有的。

**分析**: 说明 WebSocket 信息不是 SSR（服务端渲染）输出的，而是前端 JS 运行后动态请求的。需要找到前端调用的 API。

### 坑3：GraphQL 接口已失效

**预期**: 调研资料提到 `POST /live_graphql` + `WebSocketInfoQuery` 可以获取 token。

**实际**: 返回 `{"result": 1, "message": "活动结束啦~"}`，接口已下线或改路径。

**应对**: 放弃 GraphQL 路线，转向让用户从浏览器抓包。

### 坑4：找到了真实 API，但有签名保护

**转折点**: 让用户在浏览器 Network 面板中搜索 `websocket`，发现了真实接口：

```
GET /live_api/liveroom/websocketinfo?__NS_hxfalcon=...&caver=2&liveStreamId=nWRydQ_je14
```

直接用 curl 调用返回 `{"data": {"result": 2}}`，说明 `__NS_hxfalcon` 是一个请求签名，无法简单伪造。

**决策**: 不在签名算法上花时间（那是另一个大工程），改为让用户从浏览器复制 API 响应中的 token。

### 思路要点

> **"快速失败，快速转向"** — 每条路最多试 2-3 分钟，不通就换。逆向工程中，你的假设有很大概率是错的（因为平台在不断更新），关键是快速验证并调整方向，而不是在一条死路上死磕。

> **"让用户帮你突破最难的环节"** — 浏览器已经帮你处理了签名、Cookie、JS 执行等所有复杂逻辑。与其花几天逆向签名算法，不如让用户在浏览器里复制一个值，30 秒就搞定。

---

## 四、第三步：最小可用 Demo

拿到 token 后，核心代码其实很少：

```
1. 用 protoc 编译 .proto 文件 → 生成 Go 结构体
2. WebSocket 连接 WSS 地址
3. 构造 CSWebEnterRoom 消息，序列化为 protobuf，发送
4. 每 20 秒发送 CSWebHeartbeat
5. 接收消息 → 反序列化 SocketMessage → 按 payloadType 路由 → 解析 SCWebFeedPush
```

第一次运行就成功了：`[进房确认] code=1000`，弹幕和礼物开始涌入。

### 思路要点

> **"先证明路径可行，再补充细节"** — 第一版 demo 硬编码了 token 和 WSS 地址，礼物名称映射也只有几十个。但它 **跑通了**，这就够了。礼物名称映射、配置文件、自动获取 token 这些都是后续可以逐步完善的。

---

## 五、第四步：补充礼物映射（意外顺利）

在浏览器 Network 中发现快手有公开的礼物列表 API：

```
GET /live_api/liveroom/giftlist?liveStreamId=xxx
```

这个接口**不需要签名**，只需要基本的 Cookie，直接返回了 166 个礼物的完整信息（id、name、unitPrice）。一次请求就拿到了完整映射。

---

## 六、总结：解决"看似很难"问题的方法论

### 为什么这个问题看起来难？

- "快手"是大厂，通常有反爬和加密
- "WebSocket + Protobuf"听起来比 HTTP + JSON 复杂得多
- "直播间实时数据"感觉需要特殊权限

### 为什么实际没那么难？

| 看似困难 | 实际情况 |
|----------|----------|
| Protobuf 协议未知 | 开源项目已经逆向过，直接拿 .proto 文件 |
| 需要破解签名算法 | 绕过它 — 让用户从浏览器复制 token |
| 需要理解整个系统 | 只需要理解 5 个 protobuf 消息的结构 |
| 礼物 ID 映射未知 | 快手有公开 API 直接返回完整列表 |

### 通用方法论

1. **搜索优先** — 花 5 分钟搜索能省 5 小时逆向
2. **快速验证** — 每个假设用最小代价验证，不通立刻换路
3. **80/20 法则** — 先解决最关键的 20%（协议格式），剩下 80%（签名、自动化）后续再说
4. **善用"人肉接口"** — 如果某个环节自动化成本太高，先让用户手动提供，验证整体流程可行后再优化
5. **逐步增量** — demo → 补充数据 → 工程化，每一步都在上一步验证可行的基础上推进
