import { Card, Empty, Form, Select, Space, Spin, Tag, Typography } from "antd";
import type { types } from "../../wailsjs/go/models";
import { PLATFORM_COLOR, PLATFORM_LABEL } from "../types";
import { ConnectionSteps } from "./ConnectionSteps";

const { Text } = Typography;

interface SidePanelProps {
  profile: types.Profile | null;
  profileLoading: boolean;
  accountId: string;
  onAccountChange: (id: string) => void;
  isConnected: boolean;
  status: string;
}

export function SidePanel({
  profile,
  profileLoading,
  accountId,
  onAccountChange,
  isConnected,
  status,
}: SidePanelProps) {
  const site = profile?.site;
  const accounts = (profile?.live_accounts || []).filter((a) => a.enabled);
  const arBoxes = profile?.ar_boxes || [];
  const monsterBox = arBoxes.find((b) => b.type === "MONSTER");
  const currentAccount = accounts.find((a) => a.id === accountId);
  const prankDeviceSn = profile?.prank_device_sn || "";

  const showSteps = status !== "disconnected";

  return (
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
                  <Tag color="default">未绑定（不影响连接）</Tag>
                )}
              </div>
              <div>
                <Text type="secondary">整蛊设备：</Text>
                {prankDeviceSn ? (
                  <Tag
                    color="orange"
                    style={{ fontFamily: "monospace" }}
                  >
                    {prankDeviceSn}
                  </Tag>
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

        <Card size="small" title="直播账号" style={{ marginBottom: 12 }}>
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

        {showSteps && (
          <Card size="small" title="连接进度">
            <ConnectionSteps status={status} />
          </Card>
        )}
      </Spin>
    </div>
  );
}
