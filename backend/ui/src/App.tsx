import { ConfigProvider, theme, App as AntApp, Layout, Menu, Tooltip, Spin } from "antd";
import { DatabaseOutlined, TeamOutlined, SettingOutlined } from "@ant-design/icons";
import { BrowserRouter, Routes, Route, Link, useLocation, Navigate } from "react-router-dom";
import { AuthProvider, useAuth } from "./contexts/AuthContext";
import DatabasesTable from "./components/DatabasesTable";
import Users from "./routes/Users";
import Login from "./routes/Login";
import Setup from "./routes/Setup";

function AppLayout() {
  const location = useLocation();
  const selectedKey = location.pathname === "/users" ? "users" : "databases";
  const { user } = useAuth();

  return (
    <Layout style={{ minHeight: "100vh" }}>
      <Layout.Sider
        collapsible
        breakpoint="lg"
        collapsedWidth={56}
        width={200}
        style={{
          background: "linear-gradient(180deg, #0a0a0a 0%, #111 100%)",
          borderRight: "1px solid #1e1e1e",
          overflow: "auto",
        }}
      >
        <div
          style={{
            height: 56,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            borderBottom: "1px solid #1e1e1e",
            cursor: "default",
          }}
        >
          <div
            style={{
              display: "flex",
              alignItems: "center",
              gap: 10,
              overflow: "hidden",
            }}
          >
            <Tooltip title="pgmanager" placement="right">
              <div
                style={{
                  width: 32,
                  height: 32,
                  borderRadius: 8,
                  background: "linear-gradient(135deg, #1668dc 0%, #41a0ff 100%)",
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  fontWeight: 700,
                  fontSize: 13,
                  color: "#fff",
                  flexShrink: 0,
                }}
              >
                pg
              </div>
            </Tooltip>
            <span
              style={{
                color: "#eee",
                fontWeight: 600,
                fontSize: 14,
                whiteSpace: "nowrap",
              }}
            >
              pgmanager
            </span>
          </div>
        </div>

        <div style={{ flex: 1, display: "flex", flexDirection: "column", padding: "8px 0" }}>
          <Menu
            mode="inline"
            selectedKeys={[selectedKey]}
            style={{
              background: "transparent",
              border: "none",
            }}
            items={[
              {
                key: "databases",
                icon: <DatabaseOutlined style={{ fontSize: 16 }} />,
                label: <Link to="/">Databases</Link>,
              },
              ...(user?.role === "admin"
                ? [
                    {
                      key: "users",
                      icon: <TeamOutlined style={{ fontSize: 16 }} />,
                      label: <Link to="/users">Users</Link>,
                    },
                  ]
                : []),
            ]}
          />
        </div>

        <div
          style={{
            borderTop: "1px solid #1e1e1e",
            padding: "8px 0",
          }}
        >
          <Tooltip title="Settings" placement="right">
            <div
              style={{
                display: "flex",
                alignItems: "center",
                gap: 10,
                padding: "8px 20px",
                color: "#555",
                fontSize: 16,
                cursor: "not-allowed",
              }}
            >
              <SettingOutlined />
              <span style={{ fontSize: 14 }}>Settings</span>
            </div>
          </Tooltip>
        </div>
      </Layout.Sider>

      <Layout.Content style={{ padding: 24 }}>
        <Routes>
          <Route path="/" element={<DatabasesTable />} />
          <Route path="/users" element={user?.role === "admin" ? <Users /> : <Navigate to="/" />} />
        </Routes>
      </Layout.Content>
    </Layout>
  );
}

function AuthenticatedLayout() {
  const { user, loading, needsSetup } = useAuth();

  if (loading) {
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
        <Spin size="large" />
      </div>
    );
  }

  if (!user) {
    return (
      <Routes>
        <Route path="/setup" element={needsSetup ? <Setup /> : <Navigate to="/login" />} />
        <Route path="/login" element={needsSetup ? <Navigate to="/setup" /> : <Login />} />
        <Route path="*" element={needsSetup ? <Navigate to="/setup" /> : <Login />} />
      </Routes>
    );
  }

  return <AppLayout />;
}

export default function App() {
  return (
    <ConfigProvider
      theme={{
        algorithm: theme.darkAlgorithm,
        token: {
          fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
          colorPrimary: "#1668dc",
          borderRadius: 6,
        },
        components: {
          Button: {
            dangerColor: "#ff4d4f",
          },
          Menu: {
            darkItemBg: "transparent",
            darkItemColor: "#888",
            darkItemSelectedBg: "rgba(22, 104, 220, 0.15)",
            darkItemSelectedColor: "#fff",
            darkItemHoverColor: "#ccc",
            darkItemHoverBg: "rgba(255, 255, 255, 0.04)",
            itemMarginBlock: 4,
            itemMarginInline: 8,
            itemBorderRadius: 6,
            iconSize: 16,
          },
        },
      }}
    >
      <AntApp>
        <BrowserRouter>
          <AuthProvider>
            <AuthenticatedLayout />
          </AuthProvider>
        </BrowserRouter>
      </AntApp>
    </ConfigProvider>
  );
}
