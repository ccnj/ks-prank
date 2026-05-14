import { ConfigProvider, Layout, Spin, message, theme } from "antd";
import { useCallback, useEffect, useRef, useState } from "react";
import * as WailsApp from "../wailsjs/go/main/App";
import type { types } from "../wailsjs/go/models";
import { EventsOn } from "../wailsjs/runtime/runtime";
import { EventStream } from "./components/EventStream";
import { HeaderBar } from "./components/HeaderBar";
import { LoginPage } from "./components/LoginPage";
import { SidePanel } from "./components/SidePanel";
import { SystemLog } from "./components/SystemLog";
import type { EventItem, LogLevel, PrankRules } from "./types";

const {
  Connect,
  Disconnect,
  GetLastAccountId,
  GetLoginState,
  GetProfile,
  Login,
  Logout,
} = WailsApp;
// d.ts 由 wails dev 重新生成，目前先 any 兜底
const GetPrankRules: (liveAccountId: string) => Promise<PrankRules> =
  (WailsApp as any).GetPrankRules;

const { Content } = Layout;

let eventIdCounter = 0;

function App() {
  const [bootLoading, setBootLoading] = useState(true);
  const [loggedIn, setLoggedIn] = useState(false);
  const [username, setUsername] = useState("");
  const [profile, setProfile] = useState<types.Profile | null>(null);
  const [profileLoading, setProfileLoading] = useState(false);

  const [accountId, setAccountId] = useState("");
  const [status, setStatus] = useState("disconnected");
  const [events, setEvents] = useState<EventItem[]>([]);
  const [connectLoading, setConnectLoading] = useState(false);
  const [rules, setRules] = useState<PrankRules | null>(null);
  const [rulesLoading, setRulesLoading] = useState(false);

  const accountIdRef = useRef("");
  accountIdRef.current = accountId;

  const addEvent = useCallback((type: EventItem["type"], data: any) => {
    setEvents((prev) => {
      const next = [
        ...prev,
        { id: ++eventIdCounter, type, timestamp: Date.now(), data },
      ];
      return next.slice(-300);
    });
  }, []);

  const log = useCallback(
    (level: LogLevel, message: string, detail?: string) => {
      setEvents((prev) => {
        const next = [
          ...prev,
          {
            id: ++eventIdCounter,
            type: "log" as const,
            timestamp: Date.now(),
            data: { level, message, detail },
          },
        ];
        return next.slice(-300);
      });
    },
    [],
  );

  const fetchRules = useCallback(
    async (id: string) => {
      if (!id) {
        setRules(null);
        return;
      }
      setRulesLoading(true);
      try {
        const r = await GetPrankRules(id);
        setRules(r);
        const gn = r?.gift_triggers?.length || 0;
        const cn = r?.chat_triggers?.length || 0;
        log("info", `加载整蛊规则成功: 礼物 ${gn} · 弹幕 ${cn}`);
      } catch (err: any) {
        const detail = err?.message || String(err);
        setRules(null);
        log("error", "加载整蛊规则失败", detail);
      } finally {
        setRulesLoading(false);
      }
    },
    [log],
  );

  const refreshProfile = useCallback(async () => {
    setProfileLoading(true);
    try {
      const p = await GetProfile();
      setProfile(p);
      const siteName = p.site?.name || "(未绑定)";
      const accountCount = (p.live_accounts || []).filter((a) => a.enabled)
        .length;
      log(
        "info",
        `加载账号资料成功: 场地=${siteName} · 启用直播账号 ${accountCount} 个`,
      );
      const last = await GetLastAccountId();
      const enabledAccounts = (p.live_accounts || []).filter((a) => a.enabled);
      let pick = enabledAccounts.find((a) => a.id === last)?.id;
      if (!pick && enabledAccounts.length > 0) pick = enabledAccounts[0].id;
      if (pick) setAccountId(pick);
    } catch (err: any) {
      const detail = err?.message || String(err);
      message.error("加载账号资料失败: " + detail);
      log("error", "加载账号资料失败", detail);
      setLoggedIn(false);
      setProfile(null);
    } finally {
      setProfileLoading(false);
    }
  }, [log]);

  useEffect(() => {
    (async () => {
      try {
        const s = await GetLoginState();
        setLoggedIn(s.loggedIn);
        if (s.loggedIn) {
          if (s.username) setUsername(s.username);
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

  const onSystemLog = useCallback(
    (level: LogLevel, msg: string, detail?: string) => log(level, msg, detail),
    [log],
  );

  useEffect(() => {
    if (loggedIn && accountId) {
      fetchRules(accountId);
    } else {
      setRules(null);
    }
  }, [loggedIn, accountId, fetchRules]);

  const handleLogin = async (name: string, password: string) => {
    try {
      await Login(name, password);
      setUsername(name);
      setLoggedIn(true);
      log("info", `登录成功: ${name}`);
      await refreshProfile();
      message.success("登录成功");
    } catch (err: any) {
      const detail = err?.message || String(err);
      log("error", `登录失败: ${name}`, detail);
      throw err;
    }
  };

  const handleLogout = async () => {
    await Logout();
    setLoggedIn(false);
    setProfile(null);
    setAccountId("");
    setStatus("disconnected");
    setEvents([]);
    setRules(null);
    log("info", "已退出登录");
  };

  const handleConnect = async () => {
    const id = accountIdRef.current;
    if (!id) {
      message.warning("请先选择直播账号");
      return;
    }
    setConnectLoading(true);
    log("info", `开始连接直播账号 (${id})`);
    try {
      await Connect(id);
      message.success("连接成功");
      log("info", "连接成功");
    } catch (err: any) {
      const text = err?.message || String(err);
      if (text.includes("连接已取消")) {
        message.info("连接已取消");
        log("warn", "连接已取消");
      } else {
        message.error("连接失败: " + text);
        log("error", "连接失败", text);
      }
    } finally {
      setConnectLoading(false);
    }
  };

  const handleDisconnect = async () => {
    try {
      await Disconnect();
      log("info", "已断开直播连接");
    } catch (err: any) {
      const detail = err?.message || String(err);
      message.error(detail);
      log("error", "断开直播连接失败", detail);
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

  if (!loggedIn) {
    return (
      <ConfigProvider theme={{ algorithm: theme.defaultAlgorithm }}>
        <LoginPage onLogin={handleLogin} />
      </ConfigProvider>
    );
  }

  const isConnected = status === "connected";
  const inProgress = status === "connecting" || status === "fetching_token";
  const isActive = isConnected || inProgress;
  const displayName = profile?.user?.nickname || username || "账号";

  return (
    <ConfigProvider theme={{ algorithm: theme.defaultAlgorithm }}>
      <Layout style={{ height: "100vh" }}>
        <HeaderBar
          status={status}
          isConnected={isActive}
          connectLoading={connectLoading || inProgress}
          canConnect={!!accountId && !inProgress}
          displayName={displayName}
          onConnect={handleConnect}
          onDisconnect={handleDisconnect}
          onRefresh={refreshProfile}
          onLogout={handleLogout}
        />
        <Content
          style={{ padding: 16, display: "flex", gap: 16, overflow: "hidden" }}
        >
          <SidePanel
            profile={profile}
            profileLoading={profileLoading}
            accountId={accountId}
            onAccountChange={setAccountId}
            isConnected={isActive}
            rules={rules}
            rulesLoading={rulesLoading}
            onRefreshProfile={refreshProfile}
            onRefreshRules={() => fetchRules(accountId)}
            onLog={onSystemLog}
          />
          <div
            style={{
              flex: 1,
              display: "flex",
              flexDirection: "column",
              minWidth: 0,
              overflow: "hidden",
            }}
          >
            <SystemLog events={events} onClear={() => setEvents([])} />
            <EventStream events={events} onClear={() => setEvents([])} />
          </div>
        </Content>
      </Layout>
    </ConfigProvider>
  );
}

export default App;
