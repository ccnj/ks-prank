export type LogLevel = "info" | "warn" | "error";

export interface LogPayload {
  level: LogLevel;
  message: string;
  detail?: string;
}

export interface EventItem {
  id: number;
  type: "gift" | "comment" | "action" | "status" | "log";
  timestamp: number;
  data: any;
}

// 哪些条目应该归入系统日志面板(排除直播间互动事件)
export const SYSTEM_LOG_TYPES: ReadonlySet<EventItem["type"]> = new Set([
  "log",
  "status",
]);

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
  fetching_token: "获取直播连接信息...",
  connected: "已连接",
};

export const STATUS_COLOR: Record<string, string> = {
  connected: "green",
  connecting: "orange",
  fetching_token: "orange",
  disconnected: "red",
};

// ===== 整蛊规则 =====

export interface ActionChoice {
  action: string;
  weight: number;
  worker_group: number;
  params: any;
}

export interface GiftTrigger {
  gift_name: string;
  choices: ActionChoice[];
}

export interface ChatTrigger {
  keyword: string;
  choices: ActionChoice[];
}

export interface LikeTrigger {
  threshold: number;
  choices: ActionChoice[];
}

export interface PrankRules {
  gift_triggers?: GiftTrigger[] | null;
  chat_triggers?: ChatTrigger[] | null;
  like_trigger?: LikeTrigger | null;
}

const ACTION_LABEL: Record<string, string> = {
  attack_monster_360: "攻击怪物",
  heal_monster: "怪物回血",
  throw_cockroach: "丢蟑螂",
  add_monster: "新增怪物",
  update_aa_level: "武器升降级",
  spin: "旋转",
  pet_feed: "投喂宠物",
  pet_tease: "逗弄宠物",
};

export function actionLabel(action: string): string {
  return ACTION_LABEL[action] || action;
}

// 把一个礼物触发的所有 choices 合成一行显示文本，多动作用 / 拼（按出现顺序去重）
export function joinChoiceLabels(choices: ActionChoice[]): string {
  const seen = new Set<string>();
  const labels: string[] = [];
  for (const c of choices) {
    const l = actionLabel(c.action);
    if (!seen.has(l)) {
      seen.add(l);
      labels.push(l);
    }
  }
  return labels.join(" / ");
}
