import {
  CheckCircleFilled,
  CloseCircleFilled,
  ExclamationCircleFilled,
  ThunderboltFilled,
} from "@ant-design/icons";
import { Button, Card, Space, Tag, Typography } from "antd";
import { useEffect, useRef, useState } from "react";
import {
  type EventItem,
  type LogLevel,
  type LogPayload,
  STATUS_TEXT,
  SYSTEM_LOG_TYPES,
} from "../types";

const { Text } = Typography;

interface SystemLogProps {
  events: EventItem[];
  onClear: () => void;
}

const LEVEL_COLOR: Record<LogLevel, string> = {
  info: "#1677ff",
  warn: "#fa8c16",
  error: "#ff4d4f",
};

function formatTime(ts: number) {
  return new Date(ts).toLocaleTimeString("zh-CN", { hour12: false });
}

function asLogPayload(data: any): LogPayload {
  if (data && typeof data === "object" && typeof data.message === "string") {
    const level: LogLevel =
      data.level === "warn" || data.level === "error" ? data.level : "info";
    return { level, message: data.message, detail: data.detail };
  }
  return { level: "info", message: String(data) };
}

function LogRow({ item }: { item: EventItem }) {
  const time = formatTime(item.timestamp);

  if (item.type === "status") {
    const s = String(item.data || "");
    return (
      <div style={{ padding: "2px 8px", fontSize: 12, display: "flex", gap: 6 }}>
        <span style={{ color: "#999", fontFamily: "monospace" }}>{time}</span>
        <ThunderboltFilled style={{ color: "#722ed1", fontSize: 11 }} />
        <span>状态:{STATUS_TEXT[s] || s}</span>
      </div>
    );
  }

  if (item.type === "log") {
    const payload = asLogPayload(item.data);
    const color = LEVEL_COLOR[payload.level];
    const Icon =
      payload.level === "error"
        ? CloseCircleFilled
        : payload.level === "warn"
          ? ExclamationCircleFilled
          : CheckCircleFilled;
    return (
      <div
        style={{
          padding: "2px 8px",
          fontSize: 12,
          display: "flex",
          gap: 6,
          alignItems: "flex-start",
        }}
      >
        <span style={{ color: "#999", fontFamily: "monospace" }}>{time}</span>
        <Icon style={{ color, fontSize: 11, marginTop: 4 }} />
        <div style={{ flex: 1, minWidth: 0 }}>
          <span>{payload.message}</span>
          {payload.detail && (
            <div
              style={{
                color: "#999",
                fontSize: 10,
                wordBreak: "break-all",
                whiteSpace: "pre-wrap",
                lineHeight: 1.3,
              }}
            >
              {payload.detail}
            </div>
          )}
        </div>
      </div>
    );
  }

  return null;
}

export function SystemLog({ events, onClear }: SystemLogProps) {
  const logs = events.filter((e) => SYSTEM_LOG_TYPES.has(e.type));
  const scrollRef = useRef<HTMLDivElement>(null);
  const [atBottom, setAtBottom] = useState(true);
  const prevLenRef = useRef(logs.length);

  const errCount = logs.reduce(
    (n, e) =>
      e.type === "log" && asLogPayload(e.data).level === "error" ? n + 1 : n,
    0,
  );

  const handleScroll = () => {
    const el = scrollRef.current;
    if (!el) return;
    const isBottom =
      el.scrollHeight - el.scrollTop - el.clientHeight <= 24;
    setAtBottom(isBottom);
  };

  useEffect(() => {
    if (logs.length === prevLenRef.current) return;
    prevLenRef.current = logs.length;
    if (atBottom && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [logs.length, atBottom]);

  return (
    <Card
      size="small"
      title={
        <Space size={6}>
          <span>系统日志</span>
          {errCount > 0 && <Tag color="red">{errCount} 错误</Tag>}
        </Space>
      }
      extra={
        <Button
          size="small"
          type="text"
          onClick={onClear}
          disabled={logs.length === 0}
        >
          清空
        </Button>
      }
      style={{
        height: 140,
        display: "flex",
        flexDirection: "column",
        marginBottom: 12,
      }}
      styles={{ body: { flex: 1, overflow: "hidden", padding: 0 } }}
    >
      <div
        ref={scrollRef}
        onScroll={handleScroll}
        style={{
          height: "100%",
          overflowY: "auto",
          padding: "4px 0",
        }}
      >
        {logs.length === 0 ? (
          <div
            style={{
              textAlign: "center",
              color: "#bbb",
              fontSize: 12,
              padding: "32px 0",
            }}
          >
            <Text type="secondary" style={{ fontSize: 12 }}>
              这里会显示登录 / 连接 / 探活等系统事件
            </Text>
          </div>
        ) : (
          logs.map((item) => <LogRow key={item.id} item={item} />)
        )}
      </div>
    </Card>
  );
}
