import { useState, type ReactNode } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Table,
  Button,
  Modal,
  Input,
  App,
  Radio,
  Select,
  Tooltip,
  Typography,
} from "antd";
import {
  PlusOutlined,
  DeleteOutlined,
  ReloadOutlined,
  CopyOutlined,
} from "@ant-design/icons";
import {
  fetchUsers,
  fetchDatabases,
  createUser,
  deleteUser,
} from "../api/client";
import { CreateUserRequestSchema } from "../lib/schemas";
import type { User } from "../lib/schemas";

export default function Users() {
  const [createOpen, setCreateOpen] = useState(false);
  const [formDb, setFormDb] = useState("");
  const [formUsername, setFormUsername] = useState("");
  const [formAccess, setFormAccess] = useState<"read" | "write" | "ddl" | "full">("write");
  const [formError, setFormError] = useState<string | null>(null);
  const [showCreds, setShowCreds] = useState<{ username: string; password: string; database: string } | null>(null);

  const queryClient = useQueryClient();
  const { message } = App.useApp();

  const { data: users, isLoading } = useQuery({
    queryKey: ["users"],
    queryFn: fetchUsers,
  });

  const { data: databases } = useQuery({
    queryKey: ["databases", true],
    queryFn: () => fetchDatabases(true),
  });

  const createMutation = useMutation({
    mutationFn: (vars: { username: string; database: string; access: "read" | "write" | "ddl" | "full" }) =>
      createUser(vars.username, vars.database, vars.access),
    onSuccess: (data) => {
      message.success("user created successfully");
      setCreateOpen(false);
      resetForm();
      setShowCreds({ username: data.username, password: data.password, database: data.database });
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (error: Error) => {
      message.error(error.message);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (username: string) => deleteUser(username),
    onSuccess: () => {
      message.success("user deleted");
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (error: Error) => {
      message.error(error.message);
    },
  });

  function resetForm() {
    setFormDb("");
    setFormUsername("");
    setFormAccess("write");
    setFormError(null);
  }

  function handleCreate() {
    const result = CreateUserRequestSchema.safeParse({
      database: formDb,
      username: formUsername,
      access: formAccess,
    });
    if (!result.success) {
      const firstError = result.error.errors[0];
      setFormError(firstError?.message ?? "invalid input");
      return;
    }
    setFormError(null);
    createMutation.mutate({
      username: result.data.username,
      database: result.data.database,
      access: result.data.access,
    });
  }

  function handleDelete(username: string) {
    Modal.confirm({
      title: "Delete user",
      content: `Are you sure you want to delete "${username}"? This will drop the PostgreSQL role.`,
      okText: "Delete",
      okType: "danger",
      onOk: () => deleteMutation.mutate(username),
    });
  }

  function copyText(text: string) {
    navigator.clipboard.writeText(text).then(() => {
      message.success("copied");
    });
  }

  const accessColors: Record<string, string> = {
    read: "#49aa19",
    write: "#1677ff",
    ddl: "#faad14",
    full: "#ff4d4f",
  };

  const accessLabels: Record<string, string> = {
    read: "SELECT",
    write: "SELECT, INSERT, UPDATE, DELETE",
    ddl: "Write + CREATE, ALTER, DROP",
    full: "ALL PRIVILEGES",
  };

  const columns = [
    {
      title: "Username",
      dataIndex: "username",
      key: "username",
    },
    {
      title: "Database",
      dataIndex: "database",
      key: "database",
    },
    {
      title: "Access",
      dataIndex: "access",
      key: "access",
      render: (access: User["access"]): ReactNode => (
        <span style={{ color: accessColors[access] ?? "#fff", textTransform: "uppercase", fontWeight: 600, fontSize: 12 }}>
          {access}
        </span>
      ),
    },
    {
      title: "Created",
      dataIndex: "createdAt",
      key: "createdAt",
      render: (v: string): ReactNode => new Date(v).toLocaleDateString(),
    },
    {
      title: "",
      key: "actions",
      width: 48,
      render: (_: unknown, record: User): ReactNode => (
        <Button
          type="text"
          danger
          icon={<DeleteOutlined />}
          onClick={() => handleDelete(record.username)}
        />
      ),
    },
  ];

  return (
    <div>
      <div style={{ marginBottom: 16, display: "flex", gap: 8 }}>
        <Button
          type="primary"
          icon={<PlusOutlined />}
          onClick={() => setCreateOpen(true)}
        >
          Create User
        </Button>
        <Button
          icon={<ReloadOutlined />}
          onClick={() => queryClient.invalidateQueries({ queryKey: ["users"] })}
        >
          Refresh
        </Button>
      </div>

      <Table
        dataSource={users}
        columns={columns}
        rowKey="username"
        loading={isLoading}
        pagination={false}
        size="small"
        bordered={false}
        rowClassName={(_, index) => (index % 2 === 0 ? "row-even" : "row-odd")}
        style={{ fontSize: 13 }}
      />

      <Modal
        title="Create Database User"
        open={createOpen}
        onOk={handleCreate}
        onCancel={() => {
          setCreateOpen(false);
          resetForm();
        }}
        confirmLoading={createMutation.isPending}
      >
        <div style={{ marginTop: 8 }}>
          <div style={{ marginBottom: 16 }}>
            <label style={{ display: "block", marginBottom: 4, fontSize: 13, color: "#ccc" }}>
              Database
            </label>
            <Select
              placeholder="select database"
              value={formDb || undefined}
              onChange={(v) => { setFormDb(v ?? ""); setFormError(null); }}
              style={{ width: "100%" }}
              options={(databases ?? []).map((d) => ({
                value: d.name,
                label: d.name,
              }))}
            />
          </div>
          <div style={{ marginBottom: 16 }}>
            <label style={{ display: "block", marginBottom: 4, fontSize: 13, color: "#ccc" }}>
              Username
            </label>
            <Input
              placeholder="e.g. myapp_user"
              value={formUsername}
              onChange={(e) => { setFormUsername(e.target.value); setFormError(null); }}
              status={formError !== null ? "error" : ""}
            />
          </div>
          <div style={{ marginBottom: 16 }}>
            <label style={{ display: "block", marginBottom: 4, fontSize: 13, color: "#ccc" }}>
              Access
            </label>
            <Radio.Group
              value={formAccess}
              onChange={(e) => setFormAccess(e.target.value)}
              style={{ display: "flex", flexDirection: "column", gap: 8 }}
            >
              {(["read", "write", "ddl", "full"] as const).map((level) => (
                <Radio key={level} value={level}>
                  <Tooltip title={accessLabels[level]}>
                    <span style={{ textTransform: "uppercase", fontWeight: 600 }}>
                      {level}
                    </span>
                    <span style={{ marginLeft: 8, fontSize: 12, color: "#888" }}>
                      — {accessLabels[level]}
                    </span>
                  </Tooltip>
                </Radio>
              ))}
            </Radio.Group>
          </div>
          {formError !== null && (
            <div style={{ color: "#ff4d4f", fontSize: 12, marginTop: 4 }}>
              {formError}
            </div>
          )}
        </div>
      </Modal>

      <Modal
        title="User Created"
        open={showCreds !== null}
        onOk={() => setShowCreds(null)}
        onCancel={() => setShowCreds(null)}
        footer={[
          <Button key="close" onClick={() => setShowCreds(null)}>
            Done
          </Button>,
        ]}
      >
        <div style={{ marginTop: 16 }}>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            Save these credentials — the password cannot be shown again.
          </Typography.Text>
          <div
            style={{
              marginTop: 12,
              padding: "12px 16px",
              background: "#1a1a1a",
              border: "1px solid #333",
              borderRadius: 6,
              fontFamily: "monospace",
              fontSize: 13,
            }}
          >
            <div style={{ marginBottom: 12 }}>
              <div style={{ color: "#888", fontSize: 11, marginBottom: 2 }}>USERNAME</div>
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <span style={{ userSelect: "all" }}>{showCreds?.username}</span>
                <CopyOutlined style={{ color: "#888", cursor: "pointer" }} onClick={() => copyText(showCreds?.username ?? "")} />
              </div>
            </div>
            <div style={{ marginBottom: 12 }}>
              <div style={{ color: "#888", fontSize: 11, marginBottom: 2 }}>PASSWORD</div>
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <span style={{ userSelect: "all", color: "#ff4d4f" }}>{showCreds?.password}</span>
                <CopyOutlined style={{ color: "#888", cursor: "pointer" }} onClick={() => copyText(showCreds?.password ?? "")} />
              </div>
            </div>
            <div>
              <div style={{ color: "#888", fontSize: 11, marginBottom: 2 }}>DATABASE</div>
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <span style={{ userSelect: "all" }}>{showCreds?.database}</span>
                <CopyOutlined style={{ color: "#888", cursor: "pointer" }} onClick={() => copyText(showCreds?.database ?? "")} />
              </div>
            </div>
          </div>
        </div>
      </Modal>
    </div>
  );
}
