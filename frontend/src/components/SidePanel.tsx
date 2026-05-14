import { EditOutlined, PlayCircleOutlined, ReloadOutlined } from "@ant-design/icons";
import {
  Badge,
  Button,
  Card,
  Empty,
  Form,
  message,
  Select,
  Space,
  Spin,
  Tag,
  Tooltip,
  Typography,
} from "antd";
import { useCallback, useEffect, useRef, useState } from "react";
import { CheckCarStream, PlayCarStream } from "../../wailsjs/go/main/App";
import type { types } from "../../wailsjs/go/models";
import { BrowserOpenURL } from "../../wailsjs/runtime/runtime";
import {
  type LogLevel,
  PLATFORM_COLOR,
  PLATFORM_LABEL,
  type PrankRules,
  joinChoiceLabels,
} from "../types";

const { Text } = Typography;

const PRANK_CONFIG_URL = "https://adm.ybkc.cc/ar/prank-configs";
const LIVE_ACCOUNTS_URL = "https://adm.ybkc.cc/site/live-accounts";

interface SidePanelProps {
  profile: types.Profile | null;
  profileLoading: boolean;
  accountId: string;
  onAccountChange: (id: string) => void;
  isConnected: boolean;
  rules: PrankRules | null;
  rulesLoading: boolean;
  onRefreshProfile: () => void;
  onRefreshRules: () => void;
  onLog?: (level: LogLevel, message: string, detail?: string) => void;
}

export function SidePanel({
  profile,
  profileLoading,
  accountId,
  onAccountChange,
  isConnected,
  rules,
  rulesLoading,
  onRefreshProfile,
  onRefreshRules,
  onLog,
}: SidePanelProps) {
  const site = profile?.site;
  const accounts = (profile?.live_accounts || []).filter((a) => a.enabled);
  const arBoxes = profile?.ar_boxes || [];
  const monsterBox = arBoxes.find((b) => b.type === "MONSTER");
  const currentAccount = accounts.find((a) => a.id === accountId);
  const prankDeviceSn = profile?.prank_device_sn || "";
  const prankDeviceIp = profile?.prank_device_last_wlan_ip || "";

  type CarOnline = "unknown" | "checking" | "online" | "offline";
  const [carOnline, setCarOnline] = useState<CarOnline>("unknown");
  const [carOnlineErr, setCarOnlineErr] = useState<string>("");
  const probeSeqRef = useRef(0);

  const probeCarOnline = useCallback(async () => {
    const seq = ++probeSeqRef.current;
    if (!prankDeviceIp) {
      setCarOnline("unknown");
      setCarOnlineErr("");
      return;
    }
    setCarOnline("checking");
    setCarOnlineErr("");
    try {
      await CheckCarStream(prankDeviceIp);
      if (seq !== probeSeqRef.current) return;
      setCarOnline("online");
      setCarOnlineErr("");
      onLog?.("info", `整蛊设备在线 (${prankDeviceIp}:554)`);
    } catch (e: any) {
      if (seq !== probeSeqRef.current) return;
      const msg = String(e?.message || e || "未知错误");
      setCarOnline("offline");
      setCarOnlineErr(msg);
      onLog?.("warn", `整蛊设备离线 (${prankDeviceIp}:554)`, msg);
    }
  }, [prankDeviceIp, onLog]);

  useEffect(() => {
    probeCarOnline();
  }, [probeCarOnline]);

  const offlineFriendlyMsg =
    "获取视频连接失败,请确保整蛊设备在线或尝试重启设备";
  const playDisabled = !prankDeviceIp || carOnline !== "online";
  const playTooltip = !prankDeviceIp
    ? "整蛊设备还没上报局域网地址,暂时无法播放"
    : carOnline === "online"
      ? "打开整蛊设备视频(同局域网直拉 RTSP)"
      : carOnline === "checking"
        ? "正在检测整蛊设备..."
        : carOnline === "offline"
          ? offlineFriendlyMsg
          : "尚未检测整蛊设备";

  const handlePlayCarStream = async () => {
    if (!prankDeviceIp) return;
    try {
      await PlayCarStream(prankDeviceIp);
      message.success("已打开整蛊设备视频窗口");
      onLog?.("info", `已打开直播画面 (rtsp://${prankDeviceIp}/live/0)`);
    } catch (e: any) {
      const raw = e?.message || String(e);
      message.error({
        content: `无法启动播放器: ${raw}(请确认本机已安装 ffplay 并加入 PATH)`,
        duration: 8,
      });
      onLog?.("error", "无法启动播放器(请确认 ffplay 已安装并加入 PATH)", raw);
    }
  };

  const giftTriggers = rules?.gift_triggers || [];

  return (
    <div style={{ width: 360, flexShrink: 0, overflowY: "auto" }}>
      <Spin spinning={profileLoading}>
        <Card
          size="small"
          title="场地信息"
          style={{ marginBottom: 12 }}
          extra={
            <Button
              size="small"
              type="text"
              icon={<ReloadOutlined />}
              onClick={onRefreshProfile}
              loading={profileLoading}
            />
          }
        >
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
                  <Tag color="default">未绑定（不影响连接）</Tag>
                )}
              </div>
              <div>
                <Text type="secondary">整蛊设备：</Text>
                {prankDeviceSn ? (
                  <Space direction="vertical" size={2} style={{ width: "100%" }}>
                    <Space size={4} wrap>
                      <Tag
                        color="orange"
                        style={{ fontFamily: "monospace", margin: 0 }}
                      >
                        {prankDeviceSn}
                      </Tag>
                      {prankDeviceIp ? (
                        <Tag style={{ fontFamily: "monospace", margin: 0 }}>
                          {prankDeviceIp}
                        </Tag>
                      ) : (
                        <Tooltip title="整蛊设备还没上报局域网地址,可能未连接 WiFi 或尚未注册">
                          <Tag color="default" style={{ margin: 0 }}>
                            无 IP
                          </Tag>
                        </Tooltip>
                      )}
                      <Tooltip
                        title={
                          !prankDeviceIp
                            ? "整蛊设备未上报 IP,无法检测"
                            : carOnline === "online"
                              ? "整蛊设备在线,可以播放视频"
                              : carOnline === "checking"
                                ? "正在检测整蛊设备..."
                                : carOnline === "offline"
                                  ? offlineFriendlyMsg
                                  : "尚未检测整蛊设备"
                        }
                      >
                        <Badge
                          status={
                            !prankDeviceIp
                              ? "default"
                              : carOnline === "online"
                                ? "success"
                                : carOnline === "checking"
                                  ? "processing"
                                  : carOnline === "offline"
                                    ? "error"
                                    : "default"
                          }
                        />
                      </Tooltip>
                      <Button
                        size="small"
                        type="text"
                        icon={<ReloadOutlined />}
                        onClick={probeCarOnline}
                        disabled={!prankDeviceIp || carOnline === "checking"}
                        loading={carOnline === "checking"}
                        title="重新检测整蛊设备"
                        style={{ padding: "0 4px" }}
                      />
                      <Tooltip title={playTooltip}>
                        <span>
                          <Button
                            size="small"
                            type="link"
                            icon={<PlayCircleOutlined />}
                            onClick={handlePlayCarStream}
                            disabled={playDisabled}
                            style={{ padding: 0 }}
                          >
                            打开直播画面
                          </Button>
                        </span>
                      </Tooltip>
                    </Space>
                    {carOnline === "offline" && (
                      <div style={{ lineHeight: 1.4 }}>
                        <Text
                          type="danger"
                          style={{ fontSize: 12, fontWeight: 500 }}
                        >
                          {offlineFriendlyMsg}
                        </Text>
                        {carOnlineErr && (
                          <div
                            style={{
                              fontSize: 10,
                              color: "#999",
                              wordBreak: "break-all",
                              whiteSpace: "pre-wrap",
                              marginTop: 2,
                            }}
                          >
                            技术细节:{carOnlineErr}
                          </div>
                        )}
                      </div>
                    )}
                  </Space>
                ) : (
                  <Tag color="default">未配置（pet_feed/pet_tease 触发会跳过）</Tag>
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

        <Card
          size="small"
          title="直播账号"
          style={{ marginBottom: 12 }}
          extra={
            <Space size={0}>
              <Button
                size="small"
                type="text"
                icon={<ReloadOutlined />}
                onClick={onRefreshProfile}
                loading={profileLoading}
              />
              <Button
                size="small"
                type="text"
                icon={<EditOutlined />}
                onClick={() => BrowserOpenURL(LIVE_ACCOUNTS_URL)}
                title="去管理后台编辑"
              />
            </Space>
          }
        >
          {accounts.length === 0 ? (
            <Empty
              description="暂无直播账号，请在管理后台添加"
              image={Empty.PRESENTED_IMAGE_SIMPLE}
            />
          ) : (
            <Form layout="vertical" size="small">
              <Form.Item label="选择要连接的账号" style={{ marginBottom: 8 }}>
                <Select
                  value={accountId || undefined}
                  onChange={onAccountChange}
                  placeholder="请选择直播账号"
                  disabled={isConnected}
                >
                  {accounts.map((acc) => (
                    <Select.Option key={acc.id} value={acc.id}>
                      <Space>
                        <Tag
                          color={PLATFORM_COLOR[acc.platform] || "default"}
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
                      marginTop: 2,
                    }}
                  >
                    {currentAccount.live_url}
                  </div>
                </div>
              )}
            </Form>
          )}
        </Card>

        <Card
          size="small"
          title="礼物配置"
          extra={
            <Space size={0}>
              <Button
                size="small"
                type="text"
                icon={<ReloadOutlined />}
                onClick={onRefreshRules}
                loading={rulesLoading}
                disabled={!accountId}
              />
              <Button
                size="small"
                type="text"
                icon={<EditOutlined />}
                onClick={() => BrowserOpenURL(PRANK_CONFIG_URL)}
                title="去管理后台编辑"
              />
            </Space>
          }
        >
          <Spin spinning={rulesLoading}>
            {giftTriggers.length === 0 ? (
              <Empty
                description={accountId ? "暂无配置" : "请先选择直播账号"}
                image={Empty.PRESENTED_IMAGE_SIMPLE}
              />
            ) : (
              <Space direction="vertical" size={6} style={{ width: "100%" }}>
                {giftTriggers.map((g, i) => (
                  <div
                    key={i}
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: 8,
                      fontSize: 13,
                    }}
                  >
                    <Tag color="volcano" style={{ margin: 0 }}>
                      {g.gift_name}
                    </Tag>
                    <span style={{ color: "#999" }}>→</span>
                    <Text>{joinChoiceLabels(g.choices || [])}</Text>
                  </div>
                ))}
              </Space>
            )}
          </Spin>
        </Card>
      </Spin>
    </div>
  );
}
