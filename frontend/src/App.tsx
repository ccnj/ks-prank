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
  ConfigProvider,
  theme,
  Select,
  Empty,
  Typography,
  Dropdown,
  Avatar,
  Spin,
} from "antd";
import {
  LinkOutlined,
  DisconnectOutlined,
  GiftOutlined,
  MessageOutlined,
  ThunderboltOutlined,
  UserOutlined,
  LogoutOutlined,
  ReloadOutlined,
  LoginOutlined,
} from "@ant-design/icons";

import {
  Connect,
  Disconnect,
  GetLoginState,
  GetProfile,
  GetLastAccountId,
  Login,
  Logout,
} from "../wailsjs/go/main/App";
import { types } from "../wailsjs/go/models";
import { EventsOn } from "../wailsjs/runtime/runtime";

const { Header, Content } = Layout;
const { Text } = Typography;

interface EventItem {
  id: number;
  type: "gift" | "comment" | "action" | "status" | "log";
  timestamp: number;
  data: any;
}

let eventIdCounter = 0;

const PLATFORM_LABEL: Record<string, string> = {
  kuaishou: "快手",
  douyin: "抖音",
};

function App() {
  const [bootLoading, setBootLoading] = useState(true);
  const [loggedIn, setLoggedIn] = useState(false);
  const [username, setUsername] = useState<string>("");
  const [profile, setProfile] = useState<types.Profile | null>(null);
  const [profileLoading, setProfileLoading] = useState(false);

  const [accountId, setAccountId] = useState<string>("");

  const [status, setStatus] = useState<string>("disconnected");
  const [events, setEvents] = useState<EventItem[]>([]);
  const [connectLoading, setConnectLoading] = useState(false);
  const [loginForm] = Form.useForm();

  const eventsEndRef = useRef<HTMLDivElement>(null);

  const addEvent = useCallback((type: EventItem["type"], data: any) => {
    setEvents((prev) => {
      const next = [
        ...prev,
        { id: ++eventIdCounter, type, timestamp: Date.now(), data },
      ];
      return next.slice(-200);
    });
  }, []);

  // 启动：检查登录态 + 拉 profile
  const refreshProfile = useCallback(async () => {
    setProfileLoading(true);
    try {
      const p = await GetProfile();
      setProfile(p);
      const last = await GetLastAccountId();
      const enabledAccounts = (p.live_accounts || []).filter((a) => a.enabled);
      let pick = enabledAccounts.find((a) => a.id === last)?.id;
      if (!pick && enabledAccounts.length > 0) pick = enabledAccounts[0].id;
      if (pick) setAccountId(pick);
    } catch (err: any) {
      message.error("加载 profile 失败: " + (err?.message || err));
      // token 失效场景
      setLoggedIn(false);
      setProfile(null);
    } finally {
      setProfileLoading(false);
    }
  }, []);

  useEffect(() => {
    (async () => {
      try {
        const s = await GetLoginState();
        setLoggedIn(s.loggedIn);
        if (s.loggedIn) {
          await refreshProfile();
        }
      } finally {
        setBootLoading(false);
      }
    })();

    const unsubs: (() => void)[] = [];
    unsubs.push(
      EventsOn("event:gift", (e: any) => addEvent("gift", e.data)),
      EventsOn("event:comment", (e: any) => addEvent("comment", e.data)),
      EventsOn("event:action", (e: any) => addEvent("action", e.data)),
      EventsOn("event:status", (s: string) => {
        setStatus(s);
        addEvent("status", s);
      }),
      EventsOn("event:log", (e: any) => addEvent("log", e)),
    );
    return () => unsubs.forEach((fn) => fn?.());
  }, [addEvent, refreshProfile]);

  useEffect(() => {
    eventsEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [events]);

  // ===== 登录表单 =====
  const handleLogin = async (values: { username: string; password: string }) => {
    try {
      await Login(values.username, values.password);
      setUsername(values.username);
      setLoggedIn(true);
      await refreshProfile();
      message.success("登录成功");
    } catch (err: any) {
      message.error(err?.message || String(err));
    }
  };

  const handleLogout = async () => {
    await Logout();
    setLoggedIn(false);
    setProfile(null);
    setAccountId("");
    setStatus("disconnected");
    loginForm.resetFields();
  };

  const handleConnect = async () => {
    if (!accountId) {
      message.warning("请先选择直播账号");
      return;
    }
    setConnectLoading(true);
    try {
      await Connect(accountId);
      message.success("连接成功");
    } catch (err: any) {
      message.error("连接失败: " + (err?.message || err));
    } finally {
      setConnectLoading(false);
    }
  };

  const handleDisconnect = async () => {
    try {
      await Disconnect();
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

  if (bootLoading) {
    return (
      <ConfigProvider theme={{ algorithm: theme.defaultAlgorithm }}>
        <Layout
          style={{
            height: "100vh",
            alignItems: "center",
            justifyContent: "center",
          }}
        >
          <Spin />
        </Layout>
      </ConfigProvider>
    );
  }

  // ===== 登录页 =====
  if (!loggedIn) {
    return (
      <ConfigProvider theme={{ algorithm: theme.defaultAlgorithm }}>
        <Layout
          style={{
            height: "100vh",
            alignItems: "center",
            justifyContent: "center",
            background: "#f0f2f5",
          }}
        >
          <Card
            style={{ width: 380 }}
            styles={{ body: { padding: 32 } }}
          >
            <Space
              direction="vertical"
              size="middle"
              style={{ width: "100%", textAlign: "center" }}
            >
              <div>
                <div
                  style={{ fontSize: 22, fontWeight: 600, marginBottom: 4 }}
                >
                  萌物·整蛊助手
                </div>
                <Text type="secondary" style={{ fontSize: 13 }}>
                  请使用加盟商账号登录
                </Text>
              </div>
              <Form
                form={loginForm}
                layout="vertical"
                onFinish={handleLogin}
                style={{ textAlign: "left" }}
              >
                <Form.Item
                  name="username"
                  rules={[{ required: true, message: "请输入用户名" }]}
                >
                  <Input
                    size="large"
                    prefix={<UserOutlined />}
                    placeholder="用户名"
                  />
                </Form.Item>
                <Form.Item
                  name="password"
                  rules={[{ required: true, message: "请输入密码" }]}
                >
                  <Input.Password size="large" placeholder="密码" />
                </Form.Item>
                <Form.Item style={{ marginBottom: 0 }}>
                  <Button
                    type="primary"
                    htmlType="submit"
                    size="large"
                    block
                    icon={<LoginOutlined />}
                  >
                    登录
                  </Button>
                </Form.Item>
              </Form>
            </Space>
          </Card>
        </Layout>
      </ConfigProvider>
    );
  }

  // ===== 主页 =====
  const site = profile?.site;
  const accounts = (profile?.live_accounts || []).filter((a) => a.enabled);
  const arBoxes = profile?.ar_boxes || [];
  const monsterBox = arBoxes.find((b) => b.type === "MONSTER");
  const currentAccount = accounts.find((a) => a.id === accountId);

  const userMenuItems = [
    {
      key: "refresh",
      label: "刷新 profile",
      icon: <ReloadOutlined />,
      onClick: refreshProfile,
    },
    {
      key: "logout",
      label: "退出登录",
      icon: <LogoutOutlined />,
      onClick: handleLogout,
      danger: true,
    },
  ];

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
              萌物·整蛊助手
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
                loading={connectLoading}
                onClick={handleConnect}
                disabled={status === "fetching_token" || !accountId}
              >
                连接
              </Button>
            )}
            <Dropdown menu={{ items: userMenuItems }} placement="bottomRight">
              <Space style={{ cursor: "pointer" }}>
                <Avatar size="small" icon={<UserOutlined />} />
                <span>{profile?.user?.nickname || username || "账号"}</span>
              </Space>
            </Dropdown>
          </Space>
        </Header>

        <Content
          style={{ padding: 16, display: "flex", gap: 16, overflow: "hidden" }}
        >
          {/* 左侧：配置面板 */}
          <div style={{ width: 360, flexShrink: 0, overflowY: "auto" }}>
            <Spin spinning={profileLoading}>
              <Card size="small" title="场地信息" style={{ marginBottom: 12 }}>
                {site ? (
                  <Space direction="vertical" size={4}>
                    <div>
                      <Text type="secondary">场地：</Text>
                      <Text strong>{site.name}</Text>
                    </div>
                    <div>
                      <Text type="secondary">AR 盒子：</Text>
                      {monsterBox ? (
                        <Tag color="purple">
                          {monsterBox.name} ({monsterBox.type})
                        </Tag>
                      ) : (
                        <Tag color="red">未找到 MONSTER 盒子</Tag>
                      )}
                    </div>
                  </Space>
                ) : (
                  <Empty
                    description="未绑定场地，请联系管理员"
                    image={Empty.PRESENTED_IMAGE_SIMPLE}
                  />
                )}
              </Card>

              <Card size="small" title="直播账号">
                {accounts.length === 0 ? (
                  <Empty
                    description="暂无直播账号，请在管理后台添加"
                    image={Empty.PRESENTED_IMAGE_SIMPLE}
                  />
                ) : (
                  <Form layout="vertical" size="small">
                    <Form.Item label="选择要连接的账号">
                      <Select
                        value={accountId || undefined}
                        onChange={setAccountId}
                        placeholder="请选择直播账号"
                        disabled={isConnected}
                      >
                        {accounts.map((acc) => (
                          <Select.Option key={acc.id} value={acc.id}>
                            <Space>
                              <Tag
                                color={
                                  acc.platform === "kuaishou"
                                    ? "orange"
                                    : "magenta"
                                }
                                style={{ margin: 0 }}
                              >
                                {PLATFORM_LABEL[acc.platform] || acc.platform}
                              </Tag>
                              <span>{acc.nickname || "（无别名）"}</span>
                            </Space>
                          </Select.Option>
                        ))}
                      </Select>
                    </Form.Item>
                    {currentAccount && (
                      <div
                        style={{
                          background: "#fafafa",
                          border: "1px solid #f0f0f0",
                          padding: 8,
                          borderRadius: 4,
                          fontSize: 12,
                        }}
                      >
                        <Text type="secondary" style={{ fontSize: 11 }}>
                          直播间 URL
                        </Text>
                        <div
                          style={{
                            fontFamily: "monospace",
                            wordBreak: "break-all",
                          }}
                        >
                          {currentAccount.live_url}
                        </div>
                      </div>
                    )}
                  </Form>
                )}
              </Card>
            </Spin>
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
            style={{
              flex: 1,
              display: "flex",
              flexDirection: "column",
              overflow: "hidden",
            }}
            styles={{
              body: { flex: 1, overflowY: "auto", padding: "0 16px" },
            }}
          >
            {events.length === 0 ? (
              <div
                style={{
                  marginTop: 48,
                  textAlign: "center",
                  color: "#999",
                }}
              >
                连接后，礼物和弹幕事件将实时显示在这里
              </div>
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
