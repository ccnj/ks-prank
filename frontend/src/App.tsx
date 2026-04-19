import { ConfigProvider, Layout, Spin, message, theme } from "antd";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  Connect,
  Disconnect,
  GetLastAccountId,
  GetLoginState,
  GetProfile,
  Login,
  Logout,
} from "../wailsjs/go/main/App";
import type { types } from "../wailsjs/go/models";
import { EventsOn } from "../wailsjs/runtime/runtime";
import { EventStream } from "./components/EventStream";
import { HeaderBar } from "./components/HeaderBar";
import { LoginPage } from "./components/LoginPage";
import { SidePanel } from "./components/SidePanel";
import type { EventItem } from "./types";

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

  const accountIdRef = useRef("");
  accountIdRef.current = accountId;

  const addEvent = useCallback((type: EventItem["type"], data: any) => {
    setEvents((prev) => {
      const next = [
        ...prev,
        { id: ++eventIdCounter, type, timestamp: Date.now(), data },
      ];
      return next.slice(-200);
    });
  }, []);

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

  const handleLogin = async (name: string, password: string) => {
    await Login(name, password);
    setUsername(name);
    setLoggedIn(true);
    await refreshProfile();
    message.success("登录成功");
  };

  const handleLogout = async () => {
    await Logout();
    setLoggedIn(false);
    setProfile(null);
    setAccountId("");
    setStatus("disconnected");
    setEvents([]);
  };

  const handleConnect = async () => {
    const id = accountIdRef.current;
    if (!id) {
      message.warning("请先选择直播账号");
      return;
    }
    setConnectLoading(true);
    try {
      await Connect(id);
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
  const displayName = profile?.user?.nickname || username || "账号";

  return (
    <ConfigProvider theme={{ algorithm: theme.defaultAlgorithm }}>
      <Layout style={{ height: "100vh" }}>
        <HeaderBar
          status={status}
          isConnected={isConnected}
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
            isConnected={isConnected}
            status={status}
          />
          <EventStream events={events} onClear={() => setEvents([])} />
        </Content>
      </Layout>
    </ConfigProvider>
  );
}

export default App;
