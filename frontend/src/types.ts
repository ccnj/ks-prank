export interface EventItem {
  id: number;
  type: "gift" | "comment" | "action" | "status" | "log";
  timestamp: number;
  data: any;
}

export const PLATFORM_LABEL: Record<string, string> = {
  kuaishou: "快手",
  douyin: "抖音",
};

export const PLATFORM_COLOR: Record<string, string> = {
  kuaishou: "orange",
  douyin: "magenta",
};

export const STATUS_TEXT: Record<string, string> = {
  disconnected: "未连接",
  connecting: "连接 MQTT...",
  fetching_token: "获取直播间 Token...",
  connected: "已连接",
};

export const STATUS_COLOR: Record<string, string> = {
  connected: "green",
  connecting: "orange",
  fetching_token: "orange",
  disconnected: "red",
};
