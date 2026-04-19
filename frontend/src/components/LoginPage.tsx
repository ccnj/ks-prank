import { LoginOutlined, UserOutlined } from "@ant-design/icons";
import {
  Button,
  Card,
  Form,
  Input,
  Layout,
  Space,
  Typography,
  message,
} from "antd";
import { useState } from "react";

const { Text } = Typography;

interface LoginPageProps {
  onLogin: (username: string, password: string) => Promise<void>;
}

export function LoginPage({ onLogin }: LoginPageProps) {
  const [form] = Form.useForm();
  const [loading, setLoading] = useState(false);

  const handleSubmit = async (values: { username: string; password: string }) => {
    setLoading(true);
    try {
      await onLogin(values.username, values.password);
    } catch (err: any) {
      message.error(err?.message || String(err));
    } finally {
      setLoading(false);
    }
  };

  return (
    <Layout
      style={{
        height: "100vh",
        alignItems: "center",
        justifyContent: "center",
        background: "linear-gradient(135deg, #f0f2f5 0%, #e6e9ef 100%)",
      }}
    >
      <Card style={{ width: 380 }} styles={{ body: { padding: 32 } }}>
        <Space
          direction="vertical"
          size="middle"
          style={{ width: "100%", textAlign: "center" }}
        >
          <div>
            <div style={{ fontSize: 24, fontWeight: 600, marginBottom: 4 }}>
              萌物·整蛊助手
            </div>
            <Text type="secondary" style={{ fontSize: 13 }}>
              请使用加盟商账号登录
            </Text>
          </div>
          <Form
            form={form}
            layout="vertical"
            onFinish={handleSubmit}
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
                autoComplete="username"
              />
            </Form.Item>
            <Form.Item
              name="password"
              rules={[{ required: true, message: "请输入密码" }]}
            >
              <Input.Password
                size="large"
                placeholder="密码"
                autoComplete="current-password"
              />
            </Form.Item>
            <Form.Item style={{ marginBottom: 0 }}>
              <Button
                type="primary"
                htmlType="submit"
                size="large"
                block
                icon={<LoginOutlined />}
                loading={loading}
              >
                登录
              </Button>
            </Form.Item>
          </Form>
        </Space>
      </Card>
    </Layout>
  );
}
