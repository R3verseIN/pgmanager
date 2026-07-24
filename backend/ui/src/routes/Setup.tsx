import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Form, Input, Button, App } from "antd";
import { useAuth } from "../contexts/AuthContext";
import { setup } from "../api/client";

export default function Setup() {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const { login } = useAuth();
  const navigate = useNavigate();
  const { message } = App.useApp();

  async function handleSubmit(values: { username: string; password: string; confirmPassword: string }) {
    if (values.password !== values.confirmPassword) {
      setError("passwords do not match");
      return;
    }

    setLoading(true);
    setError(null);
    try {
      await setup(values.username, values.password);
      await login(values.username, values.password);
      message.success("admin account created");
      navigate("/");
    } catch (err) {
      setError(err instanceof Error ? err.message : "setup failed");
    } finally {
      setLoading(false);
    }
  }

  return (
    <div
      style={{
        minHeight: "100vh",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        background: "#0a0a0a",
      }}
    >
      <div
        style={{
          width: 380,
          padding: 40,
          background: "#111",
          border: "1px solid #1e1e1e",
          borderRadius: 12,
        }}
      >
        <div style={{ textAlign: "center", marginBottom: 32 }}>
          <div
            style={{
              width: 48,
              height: 48,
              borderRadius: 12,
              background: "linear-gradient(135deg, #1668dc 0%, #41a0ff 100%)",
              display: "inline-flex",
              alignItems: "center",
              justifyContent: "center",
              fontWeight: 700,
              fontSize: 18,
              color: "#fff",
              marginBottom: 16,
            }}
          >
            pg
          </div>
          <h1 style={{ color: "#eee", fontSize: 20, fontWeight: 600, margin: 0 }}>
            Welcome to pgmanager
          </h1>
          <p style={{ color: "#888", fontSize: 13, marginTop: 8 }}>
            Set up your admin account to get started
          </p>
        </div>

        <Form onFinish={handleSubmit} layout="vertical">
          <Form.Item
            name="username"
            rules={[
              { required: true, message: "username is required" },
              { min: 3, message: "username must be at least 3 characters" },
            ]}
          >
            <Input
              placeholder="username"
              size="large"
              autoFocus
            />
          </Form.Item>

          <Form.Item
            name="password"
            rules={[
              { required: true, message: "password is required" },
              { min: 8, message: "password must be at least 8 characters" },
            ]}
          >
            <Input.Password
              placeholder="password"
              size="large"
            />
          </Form.Item>

          <Form.Item
            name="confirmPassword"
            rules={[
              { required: true, message: "please confirm your password" },
              { min: 8, message: "password must be at least 8 characters" },
            ]}
          >
            <Input.Password
              placeholder="confirm password"
              size="large"
            />
          </Form.Item>

          {error && (
            <div
              style={{
                color: "#ff4d4f",
                fontSize: 13,
                marginBottom: 16,
                textAlign: "center",
              }}
            >
              {error}
            </div>
          )}

          <Form.Item>
            <Button
              type="primary"
              htmlType="submit"
              loading={loading}
              block
              size="large"
            >
              Get Started
            </Button>
          </Form.Item>
        </Form>
      </div>
    </div>
  );
}
