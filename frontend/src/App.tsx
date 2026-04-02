import { useState, useEffect, useRef, useCallback } from "react";
import {
  Layout,
  Button,
  Card,
  Form,
  Input,
  Badge,
  List,
  Tag,
  Space,
  message,
  Descriptions,
  ConfigProvider,
  theme,
} from "antd";
import {
  LinkOutlined,
  DisconnectOutlined,
  ChromeOutlined,
  GiftOutlined,
  MessageOutlined,
  ThunderboltOutlined,
} from "@ant-design/icons";

import {
  Connect,
  Disconnect,
  FetchToken,
  GetConfig,
  SaveConfig,
} from "../wailsjs/go/main/App";
import { config } from "../wailsjs/go/models";
import { EventsOn } from "../wailsjs/runtime/runtime";

const { Header, Content } = Layout;

interface EventItem {
  id: number;
  type: "gift" | "comment" | "action" | "status" | "log";
  timestamp: number;
  data: any;
}

let eventIdCounter = 0;

function App() {
  const [status, setStatus] = useState<string>("disconnected");
  const [events, setEvents] = useState<EventItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [fetchingToken, setFetchingToken] = useState(false);
  const [form] = Form.useForm();
  const eventsEndRef = useRef<HTMLDivElement>(null);
  const fullConfigRef = useRef<config.Config>(new config.Config());

  const addEvent = useCallback((type: EventItem["type"], data: any) => {
    setEvents((prev) => {
      const next = [
        ...prev,
        { id: ++eventIdCounter, type, timestamp: Date.now(), data },
      ];
      return next.slice(-200); // 保留最近 200 条
    });
  }, []);

  useEffect(() => {
    // 加载配置
    GetConfig().then((cfg) => {
      if (cfg) {
        fullConfigRef.current = cfg;
        form.setFieldsValue(cfg);
      }
    }).catch(() => {});

    // 监听事件
    const unsubs: (() => void)[] = [];
    unsubs.push(
      EventsOn("event:gift", (e: any) => addEvent("gift", e.data)),
      EventsOn("event:comment", (e: any) => addEvent("comment", e.data)),
      EventsOn("event:action", (e: any) => addEvent("action", e.data)),
      EventsOn("event:status", (s: string) => {
        setStatus(s);
        addEvent("status", s);
      }),
      EventsOn("event:log", (e: any) => addEvent("log", e))
    );

    return () => unsubs.forEach((fn) => fn?.());
  }, [form, addEvent]);

  useEffect(() => {
    eventsEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [events]);

  const saveCurrentConfig = async () => {
    const formValues = form.getFieldsValue();
    const merged = new config.Config({ ...fullConfigRef.current, ...formValues });
    fullConfigRef.current = merged;
    try {
      await SaveConfig(merged);
    } catch (err: any) {
      message.error("保存配置失败: " + (err?.message || err));
    }
  };

  const handleFetchToken = async () => {
    const liveUrl = form.getFieldValue("live_url");
    if (!liveUrl) {
      message.warning("请先填写直播间 URL");
      return;
    }
    // 先保存当前表单（包含 live_url 等手动编辑的字段）
    await saveCurrentConfig();
    setFetchingToken(true);
    try {
      const info = await FetchToken(liveUrl);
      if (info) {
        form.setFieldsValue({
          token: info.Token,
          live_stream_id: info.LiveStreamId,
          wss_url: info.WssUrl || form.getFieldValue("wss_url"),
        });
        // FetchToken 成功后自动保存获取到的 token 信息
        await saveCurrentConfig();
        message.success("Token 获取成功");
      }
    } catch (err: any) {
      message.error("获取失败: " + (err?.message || err));
    } finally {
      setFetchingToken(false);
    }
  };

  const handleConnect = async () => {
    setLoading(true);
    try {
      await Connect();
      message.success("连接成功");
    } catch (err: any) {
      message.error("连接失败: " + (err?.message || err));
    } finally {
      setLoading(false);
    }
  };

  const handleDisconnect = async () => {
    try {
      await Disconnect();
      message.info("已断开");
    } catch (err: any) {
      message.error(err?.message || err);
    }
  };

  const isConnected = status === "connected";

  const statusColor =
    status === "connected"
      ? "green"
      : status === "connecting" || status === "fetching_token"
      ? "orange"
      : "red";

  const statusText: Record<string, string> = {
    disconnected: "未连接",
    connecting: "连接中...",
    connected: "已连接",
    fetching_token: "获取 Token 中...",
  };

  const renderEventItem = (item: EventItem) => {
    const time = new Date(item.timestamp).toLocaleTimeString("zh-CN");

    switch (item.type) {
      case "gift": {
        const d = item.data;
        return (
          <List.Item style={{ padding: "6px 0" }}>
            <Space>
              <Tag color="gold" style={{ margin: 0 }}>
                <GiftOutlined /> 礼物
              </Tag>
              <span style={{ color: "#999", fontSize: 12 }}>{time}</span>
              <span>
                <strong>{d?.username}</strong> 送出{" "}
                <Tag color="volcano">{d?.gift_name}</Tag>
                {d?.price}快币 x{d?.count}
              </span>
            </Space>
          </List.Item>
        );
      }
      case "comment": {
        const d = item.data;
        return (
          <List.Item style={{ padding: "6px 0" }}>
            <Space>
              <Tag color="blue" style={{ margin: 0 }}>
                <MessageOutlined /> 弹幕
              </Tag>
              <span style={{ color: "#999", fontSize: 12 }}>{time}</span>
              <span>
                <strong>{d?.username}</strong>: {d?.content}
              </span>
            </Space>
          </List.Item>
        );
      }
      case "status":
        return (
          <List.Item style={{ padding: "6px 0" }}>
            <Space>
              <Tag color="purple" style={{ margin: 0 }}>
                <ThunderboltOutlined /> 状态
              </Tag>
              <span style={{ color: "#999", fontSize: 12 }}>{time}</span>
              <span>{statusText[item.data] || item.data}</span>
            </Space>
          </List.Item>
        );
      default:
        return (
          <List.Item style={{ padding: "6px 0" }}>
            <Space>
              <Tag style={{ margin: 0 }}>日志</Tag>
              <span style={{ color: "#999", fontSize: 12 }}>{time}</span>
              <span>{JSON.stringify(item.data)}</span>
            </Space>
          </List.Item>
        );
    }
  };

  return (
    <ConfigProvider theme={{ algorithm: theme.defaultAlgorithm }}>
      <Layout style={{ height: "100vh" }}>
        <Header
          style={{
            background: "#fff",
            padding: "0 24px",
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            borderBottom: "1px solid #f0f0f0",
          }}
        >
          <Space size="middle">
            <span style={{ fontSize: 18, fontWeight: 600 }}>
              快手整蛊助手
            </span>
            <Badge color={statusColor} text={statusText[status] || status} />
          </Space>
          <Space>
            {isConnected ? (
              <Button
                icon={<DisconnectOutlined />}
                danger
                onClick={handleDisconnect}
              >
                断开
              </Button>
            ) : (
              <Button
                type="primary"
                icon={<LinkOutlined />}
                loading={loading}
                onClick={handleConnect}
                disabled={status === "fetching_token"}
              >
                连接
              </Button>
            )}
          </Space>
        </Header>

        <Content style={{ padding: 16, display: "flex", gap: 16, overflow: "hidden" }}>
          {/* 左侧：配置面板 */}
          <div style={{ width: 360, flexShrink: 0, overflowY: "auto" }}>
            <Form form={form} layout="vertical" size="small">
              <Form.Item label="直播间 URL" name="live_url">
                <Input
                  placeholder="https://live.kuaishou.com/u/xxx"
                  addonAfter={
                    <ChromeOutlined
                      style={{ cursor: "pointer" }}
                      onClick={handleFetchToken}
                      spin={fetchingToken}
                    />
                  }
                />
              </Form.Item>
              <Form.Item label="WSS 地址" name="wss_url">
                <Input disabled placeholder="自动获取" />
              </Form.Item>
              <Form.Item label="Token" name="token">
                <Input.TextArea rows={2} disabled placeholder="自动获取" />
              </Form.Item>
              <Form.Item label="Live Stream ID" name="live_stream_id">
                <Input disabled placeholder="自动获取" />
              </Form.Item>
            </Form>
          </div>

          {/* 右侧：事件流 */}
          <Card
            title={
              <Space>
                <span>事件流</span>
                <Tag>{events.length} 条</Tag>
              </Space>
            }
            extra={
              <Button size="small" onClick={() => setEvents([])}>
                清空
              </Button>
            }
            style={{ flex: 1, display: "flex", flexDirection: "column", overflow: "hidden" }}
            styles={{ body: { flex: 1, overflowY: "auto", padding: "0 16px" } }}
          >
            {events.length === 0 ? (
              <Descriptions style={{ marginTop: 24 }}>
                <Descriptions.Item>
                  连接快手直播间后，礼物和弹幕事件将实时显示在这里
                </Descriptions.Item>
              </Descriptions>
            ) : (
              <List
                dataSource={events}
                renderItem={renderEventItem}
                split={false}
              />
            )}
            <div ref={eventsEndRef} />
          </Card>
        </Content>
      </Layout>
    </ConfigProvider>
  );
}

export default App;
