import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Form, Input, Button, App } from "antd";
import { useAuth } from "../contexts/AuthContext";

export default function Login() {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const { login } = useAuth();
  const navigate = useNavigate();
  const { message } = App.useApp();

  async function handleSubmit(values: { username: string; password: string }) {
    setLoading(true);
    setError(null);
    try {
      await login(values.username, values.password);
      message.success("logged in");
      navigate("/");
    } catch (err) {
      setError(err instanceof Error ? err.message : "login failed");
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
            pgmanager
          </h1>
        </div>

        <Form onFinish={handleSubmit} layout="vertical">
          <Form.Item
            name="username"
            rules={[{ required: true, message: "username is required" }]}
          >
            <Input
              placeholder="username"
              size="large"
              autoFocus
            />
          </Form.Item>

          <Form.Item
            name="password"
            rules={[{ required: true, message: "password is required" }]}
          >
            <Input.Password
              placeholder="password"
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
              Login
            </Button>
          </Form.Item>
        </Form>
      </div>
    </div>
  );
}
