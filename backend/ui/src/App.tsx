import { ConfigProvider, theme, App as AntApp, Layout, Menu } from "antd";
import { DatabaseOutlined, ThunderboltOutlined } from "@ant-design/icons";
import DatabasesTable from "./components/DatabasesTable";

export default function App() {
  return (
    <ConfigProvider
      theme={{
        algorithm: theme.darkAlgorithm,
        token: {
          fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
        },
      }}
    >
      <AntApp>
        <Layout style={{ minHeight: "100vh" }}>
          <Layout.Sider collapsible breakpoint="lg" collapsedWidth={48}>
            <div style={{ height: 32, margin: 16, color: "#fff", fontWeight: 600, fontSize: 16, whiteSpace: "nowrap", overflow: "hidden" }}>
              pgmanager
            </div>
            <Menu
              theme="dark"
              mode="inline"
              defaultSelectedKeys={["databases"]}
              items={[
                { key: "databases", icon: <DatabaseOutlined />, label: "Databases" },
                { key: "query", icon: <ThunderboltOutlined />, label: "Query", disabled: true },
              ]}
            />
          </Layout.Sider>
          <Layout.Content style={{ padding: 24 }}>
            <DatabasesTable />
          </Layout.Content>
        </Layout>
      </AntApp>
    </ConfigProvider>
  );
}
