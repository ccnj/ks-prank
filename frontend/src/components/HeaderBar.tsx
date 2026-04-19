import {
  DisconnectOutlined,
  LinkOutlined,
  LogoutOutlined,
  ReloadOutlined,
  UserOutlined,
} from "@ant-design/icons";
import { Avatar, Badge, Button, Dropdown, Layout, Space } from "antd";
import { STATUS_COLOR, STATUS_TEXT } from "../types";

const { Header } = Layout;

interface HeaderBarProps {
  status: string;
  isConnected: boolean;
  connectLoading: boolean;
  canConnect: boolean;
  displayName: string;
  onConnect: () => void;
  onDisconnect: () => void;
  onRefresh: () => void;
  onLogout: () => void;
}

export function HeaderBar({
  status,
  isConnected,
  connectLoading,
  canConnect,
  displayName,
  onConnect,
  onDisconnect,
  onRefresh,
  onLogout,
}: HeaderBarProps) {
  const userMenuItems = [
    {
      key: "refresh",
      label: "刷新 profile",
      icon: <ReloadOutlined />,
      onClick: onRefresh,
    },
    {
      key: "logout",
      label: "退出登录",
      icon: <LogoutOutlined />,
      onClick: onLogout,
      danger: true,
    },
  ];

  return (
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
        <span style={{ fontSize: 18, fontWeight: 600 }}>萌物·整蛊助手</span>
        <Badge
          color={STATUS_COLOR[status] || "default"}
          text={STATUS_TEXT[status] || status}
        />
      </Space>
      <Space>
        {isConnected ? (
          <Button icon={<DisconnectOutlined />} danger onClick={onDisconnect}>
            断开
          </Button>
        ) : (
          <Button
            type="primary"
            icon={<LinkOutlined />}
            loading={connectLoading}
            onClick={onConnect}
            disabled={!canConnect}
          >
            连接
          </Button>
        )}
        <Dropdown menu={{ items: userMenuItems }} placement="bottomRight">
          <Space style={{ cursor: "pointer" }}>
            <Avatar size="small" icon={<UserOutlined />} />
            <span>{displayName}</span>
          </Space>
        </Dropdown>
      </Space>
    </Header>
  );
}
