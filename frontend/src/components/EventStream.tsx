import {
  DownOutlined,
  GiftOutlined,
  MessageOutlined,
  ThunderboltOutlined,
  UserOutlined,
} from "@ant-design/icons";
import { Avatar, Button, Card, Segmented, Space, Tag } from "antd";
import { useEffect, useMemo, useRef, useState } from "react";
import { type EventItem, STATUS_TEXT } from "../types";

interface EventStreamProps {
  events: EventItem[];
  onClear: () => void;
}

type Filter = "all" | "gift" | "comment";

function giftAccent(price: number): { border: string; bg: string } {
  if (price >= 1000) return { border: "#ff4d4f", bg: "#fff1f0" };
  if (price >= 100) return { border: "#fa8c16", bg: "#fff7e6" };
  if (price >= 10) return { border: "#1677ff", bg: "#e6f4ff" };
  return { border: "#d9d9d9", bg: "#fafafa" };
}

function formatTime(ts: number) {
  return new Date(ts).toLocaleTimeString("zh-CN", { hour12: false });
}

function EventRow({ item }: { item: EventItem }) {
  const time = formatTime(item.timestamp);

  if (item.type === "gift") {
    const d = item.data || {};
    const price: number = d.price || 0;
    const { border, bg } = giftAccent(price);
    return (
      <div
        style={{
          display: "flex",
          gap: 10,
          padding: "8px 10px",
          borderLeft: `3px solid ${border}`,
          background: bg,
          borderRadius: 4,
          marginBottom: 6,
        }}
      >
        <Avatar
          size={32}
          src={d.avatar || undefined}
          icon={!d.avatar ? <UserOutlined /> : undefined}
        />
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
            <GiftOutlined style={{ color: border }} />
            <strong style={{ fontSize: 13 }}>{d.username || "匿名"}</strong>
            <span style={{ color: "#999", fontSize: 11 }}>{time}</span>
          </div>
          <div style={{ fontSize: 13, marginTop: 2 }}>
            送出 <Tag color="volcano" style={{ margin: "0 2px" }}>{d.gift_name}</Tag>
            <span style={{ color: "#888" }}>
              {price} 币 × {d.count || 1}
            </span>
          </div>
        </div>
      </div>
    );
  }

  if (item.type === "comment") {
    const d = item.data || {};
    return (
      <div
        style={{
          display: "flex",
          gap: 10,
          padding: "6px 10px",
          marginBottom: 4,
        }}
      >
        <Avatar
          size={28}
          src={d.avatar || undefined}
          icon={!d.avatar ? <UserOutlined /> : undefined}
        />
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
            <MessageOutlined style={{ color: "#1677ff", fontSize: 12 }} />
            <strong style={{ fontSize: 13 }}>{d.username || "匿名"}</strong>
            <span style={{ color: "#999", fontSize: 11 }}>{time}</span>
          </div>
          <div style={{ fontSize: 13, color: "#333" }}>{d.content}</div>
        </div>
      </div>
    );
  }

  if (item.type === "status") {
    return (
      <div
        style={{
          padding: "4px 10px",
          marginBottom: 4,
          fontSize: 12,
          color: "#666",
          display: "flex",
          alignItems: "center",
          gap: 6,
        }}
      >
        <ThunderboltOutlined style={{ color: "#722ed1" }} />
        <span style={{ color: "#999" }}>{time}</span>
        <span>状态：{STATUS_TEXT[item.data] || item.data}</span>
      </div>
    );
  }

  return (
    <div
      style={{
        padding: "4px 10px",
        marginBottom: 4,
        fontSize: 12,
        color: "#999",
      }}
    >
      <span>{time}</span> · {JSON.stringify(item.data)}
    </div>
  );
}

export function EventStream({ events, onClear }: EventStreamProps) {
  const [filter, setFilter] = useState<Filter>("all");
  const [atBottom, setAtBottom] = useState(true);
  const [unreadCount, setUnreadCount] = useState(0);

  const scrollRef = useRef<HTMLDivElement>(null);
  const prevLenRef = useRef(events.length);

  const filtered = useMemo(() => {
    if (filter === "all") return events;
    return events.filter((e) => e.type === filter);
  }, [events, filter]);

  const stats = useMemo(() => {
    let gifts = 0;
    let comments = 0;
    for (const e of events) {
      if (e.type === "gift") gifts++;
      else if (e.type === "comment") comments++;
    }
    return { gifts, comments };
  }, [events]);

  const handleScroll = () => {
    const el = scrollRef.current;
    if (!el) return;
    const threshold = 32;
    const isBottom =
      el.scrollHeight - el.scrollTop - el.clientHeight <= threshold;
    setAtBottom(isBottom);
    if (isBottom) setUnreadCount(0);
  };

  const scrollToBottom = (smooth = true) => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTo({
      top: el.scrollHeight,
      behavior: smooth ? "smooth" : "auto",
    });
  };

  useEffect(() => {
    const delta = events.length - prevLenRef.current;
    prevLenRef.current = events.length;
    if (delta <= 0) return;
    if (atBottom) {
      scrollToBottom(true);
    } else {
      setUnreadCount((n) => n + delta);
    }
  }, [events.length, atBottom]);

  return (
    <Card
      title={
        <Space>
          <span>事件流</span>
          <Tag color="gold">礼物 {stats.gifts}</Tag>
          <Tag color="blue">弹幕 {stats.comments}</Tag>
        </Space>
      }
      extra={
        <Space>
          <Segmented
            size="small"
            value={filter}
            onChange={(v) => setFilter(v as Filter)}
            options={[
              { label: "全部", value: "all" },
              { label: "礼物", value: "gift" },
              { label: "弹幕", value: "comment" },
            ]}
          />
          <Button size="small" onClick={onClear} disabled={events.length === 0}>
            清空
          </Button>
        </Space>
      }
      style={{
        flex: 1,
        display: "flex",
        flexDirection: "column",
        overflow: "hidden",
        position: "relative",
      }}
      styles={{
        body: {
          flex: 1,
          overflow: "hidden",
          padding: 0,
          position: "relative",
        },
      }}
    >
      <div
        ref={scrollRef}
        onScroll={handleScroll}
        style={{
          height: "100%",
          overflowY: "auto",
          padding: "8px 12px",
        }}
      >
        {filtered.length === 0 ? (
          <div
            style={{
              marginTop: 64,
              textAlign: "center",
              color: "#999",
              fontSize: 13,
            }}
          >
            {events.length === 0
              ? "连接后，礼物和弹幕事件将实时显示在这里"
              : "当前筛选下暂无事件"}
          </div>
        ) : (
          filtered.map((item) => <EventRow key={item.id} item={item} />)
        )}
      </div>

      {!atBottom && unreadCount > 0 && (
        <Button
          type="primary"
          size="small"
          shape="round"
          icon={<DownOutlined />}
          onClick={() => scrollToBottom(true)}
          style={{
            position: "absolute",
            bottom: 12,
            right: 16,
            boxShadow: "0 2px 8px rgba(0,0,0,0.2)",
          }}
        >
          {unreadCount} 条新消息
        </Button>
      )}
    </Card>
  );
}
