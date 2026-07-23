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
  Tag,
} from "antd";
import {
  PlusOutlined,
  DeleteOutlined,
  ReloadOutlined,
  CopyOutlined,
  EditOutlined,
} from "@ant-design/icons";
import {
  fetchUsers,
  fetchDatabases,
  createUser,
  deleteUser,
  updateUser,
  addUserDatabase,
  removeUserDatabase,
} from "../api/client";
import { CreateUserRequestSchema, UpdateUserRequestSchema, AddDatabaseRequestSchema } from "../lib/schemas";
import type { User } from "../lib/schemas";

export default function Users() {
  const [createOpen, setCreateOpen] = useState(false);
  const [formDbs, setFormDbs] = useState<string[]>([]);
  const [formUsername, setFormUsername] = useState("");
  const [formAccess, setFormAccess] = useState<"read" | "write" | "ddl" | "full">("write");
  const [formError, setFormError] = useState<string | null>(null);
  const [showCreds, setShowCreds] = useState<{ username: string; password: string; databases: string[]; connectionString: string } | null>(null);
  const [editOpen, setEditOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<User | null>(null);
  const [editAccess, setEditAccess] = useState<"read" | "write" | "ddl" | "full">("write");
  const [editPassword, setEditPassword] = useState("");
  const [addDbOpen, setAddDbOpen] = useState(false);
  const [addDbTarget, setAddDbTarget] = useState<User | null>(null);
  const [addDbName, setAddDbName] = useState("");
  const [addDbError, setAddDbError] = useState<string | null>(null);

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
    mutationFn: (vars: { username: string; databases: string[]; access: "read" | "write" | "ddl" | "full"; password?: string }) =>
      createUser(vars.username, vars.databases, vars.access, vars.password),
    onSuccess: (data) => {
      message.success("user created successfully");
      setCreateOpen(false);
      resetForm();
      setShowCreds({ username: data.username, password: data.password, databases: data.databases, connectionString: data.connectionString });
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

  const updateMutation = useMutation({
    mutationFn: (vars: { username: string; password?: string; access?: "read" | "write" | "ddl" | "full" }) => {
      const opts: { password?: string; access?: "read" | "write" | "ddl" | "full" } = {};
      if (vars.password !== undefined) opts.password = vars.password;
      if (vars.access !== undefined) opts.access = vars.access;
      return updateUser(vars.username, opts);
    },
    onSuccess: () => {
      message.success("user updated");
      setEditOpen(false);
      setEditTarget(null);
      setEditPassword("");
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (error: Error) => {
      message.error(error.message);
    },
  });

  const addDbMutation = useMutation({
    mutationFn: (vars: { username: string; database: string }) =>
      addUserDatabase(vars.username, vars.database),
    onSuccess: () => {
      message.success("database granted");
      setAddDbOpen(false);
      setAddDbTarget(null);
      setAddDbName("");
      setAddDbError(null);
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (error: Error) => {
      message.error(error.message);
    },
  });

  const removeDbMutation = useMutation({
    mutationFn: (vars: { username: string; database: string }) =>
      removeUserDatabase(vars.username, vars.database),
    onSuccess: () => {
      message.success("database removed");
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (error: Error) => {
      message.error(error.message);
    },
  });

  function resetForm() {
    setFormDbs([]);
    setFormUsername("");
    setFormAccess("write");
    setFormError(null);
  }

  function handleCreate() {
    const result = CreateUserRequestSchema.safeParse({
      databases: formDbs,
      username: formUsername,
      access: formAccess,
    });
    if (!result.success) {
      const firstError = result.error.errors[0];
      setFormError(firstError?.message ?? "invalid input");
      return;
    }
    setFormError(null);
    const vars: { username: string; databases: string[]; access: "read" | "write" | "ddl" | "full"; password?: string } = {
      username: result.data.username,
      databases: result.data.databases,
      access: result.data.access,
    };
    if (result.data.password) vars.password = result.data.password;
    createMutation.mutate(vars);
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

  function openEdit(user: User) {
    setEditTarget(user);
    setEditAccess(user.access);
    setEditPassword("");
    setEditOpen(true);
  }

  function handleEdit() {
    if (!editTarget) return;
    const result = UpdateUserRequestSchema.safeParse({
      password: editPassword || undefined,
      access: editAccess,
    });
    if (!result.success) {
      message.error(result.error.errors[0]?.message ?? "invalid input");
      return;
    }
    const vars: { username: string; password?: string; access?: "read" | "write" | "ddl" | "full" } = { username: editTarget.username };
    if (result.data.password) vars.password = result.data.password;
    if (result.data.access) vars.access = result.data.access;
    updateMutation.mutate(vars);
  }

  function openAddDb(user: User) {
    setAddDbTarget(user);
    setAddDbName("");
    setAddDbError(null);
    setAddDbOpen(true);
  }

  function handleAddDb() {
    if (!addDbTarget) return;
    const result = AddDatabaseRequestSchema.safeParse({ database: addDbName });
    if (!result.success) {
      setAddDbError(result.error.errors[0]?.message ?? "invalid input");
      return;
    }
    setAddDbError(null);
    addDbMutation.mutate({ username: addDbTarget.username, database: result.data.database });
  }

  function handleRemoveDb(username: string, database: string) {
    Modal.confirm({
      title: "Remove database access",
      content: `Remove access to "${database}" from "${username}"?`,
      okText: "Remove",
      okType: "danger",
      onOk: () => removeDbMutation.mutate({ username, database }),
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
      title: "Databases",
      dataIndex: "databases",
      key: "databases",
      render: (dbs: string[], record: User): ReactNode => (
        <div style={{ display: "flex", flexWrap: "wrap", gap: 4 }}>
          {dbs.map((db) => (
            <Tag
              key={db}
              closable
              onClose={(e) => { e.preventDefault(); handleRemoveDb(record.username, db); }}
              style={{ margin: 0 }}
            >
              {db}
            </Tag>
          ))}
          <Tag
            style={{ borderStyle: "dashed", cursor: "pointer", margin: 0 }}
            onClick={() => openAddDb(record)}
          >
            <PlusOutlined /> add
          </Tag>
        </div>
      ),
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
      width: 80,
      render: (_: unknown, record: User): ReactNode => (
        <div style={{ display: "flex", gap: 4 }}>
          <Button
            type="text"
            icon={<EditOutlined />}
            onClick={() => openEdit(record)}
          />
          <Button
            type="text"
            danger
            icon={<DeleteOutlined />}
            onClick={() => handleDelete(record.username)}
          />
        </div>
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
        onCancel={() => { setCreateOpen(false); resetForm(); }}
        confirmLoading={createMutation.isPending}
      >
        <div style={{ marginTop: 8 }}>
          <div style={{ marginBottom: 16 }}>
            <label style={{ display: "block", marginBottom: 4, fontSize: 13, color: "#ccc" }}>
              Databases
            </label>
            <Select
              mode="multiple"
              placeholder="select databases"
              value={formDbs}
              onChange={(v) => { setFormDbs(v); setFormError(null); }}
              style={{ width: "100%" }}
              options={(databases ?? []).map((d) => ({ value: d.name, label: d.name }))}
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
                    <span style={{ textTransform: "uppercase", fontWeight: 600 }}>{level}</span>
                    <span style={{ marginLeft: 8, fontSize: 12, color: "#888" }}>— {accessLabels[level]}</span>
                  </Tooltip>
                </Radio>
              ))}
            </Radio.Group>
          </div>
          {formError !== null && (
            <div style={{ color: "#ff4d4f", fontSize: 12, marginTop: 4 }}>{formError}</div>
          )}
        </div>
      </Modal>

      <Modal
        title="User Created"
        open={showCreds !== null}
        onOk={() => setShowCreds(null)}
        onCancel={() => setShowCreds(null)}
        footer={[<Button key="close" onClick={() => setShowCreds(null)}>Done</Button>]}
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
            <div style={{ marginBottom: 12 }}>
              <div style={{ color: "#888", fontSize: 11, marginBottom: 2 }}>DATABASES</div>
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <span style={{ userSelect: "all" }}>{showCreds?.databases.join(", ")}</span>
                <CopyOutlined style={{ color: "#888", cursor: "pointer" }} onClick={() => copyText(showCreds?.databases.join(", ") ?? "")} />
              </div>
            </div>
            <div>
              <div style={{ color: "#888", fontSize: 11, marginBottom: 2 }}>CONNECTION STRING</div>
              <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                <span style={{ userSelect: "all", wordBreak: "break-all" }}>{showCreds?.connectionString}</span>
                <CopyOutlined style={{ color: "#888", cursor: "pointer", flexShrink: 0 }} onClick={() => copyText(showCreds?.connectionString ?? "")} />
              </div>
            </div>
          </div>
        </div>
      </Modal>

      <Modal
        title={`Edit User — ${editTarget?.username ?? ""}`}
        open={editOpen}
        onOk={handleEdit}
        onCancel={() => { setEditOpen(false); setEditTarget(null); setEditPassword(""); }}
        confirmLoading={updateMutation.isPending}
      >
        <div style={{ marginTop: 8 }}>
          <div style={{ marginBottom: 16 }}>
            <label style={{ display: "block", marginBottom: 4, fontSize: 13, color: "#ccc" }}>
              New Password (leave blank to keep current)
            </label>
            <Input.Password
              placeholder="new password (8-128 chars)"
              value={editPassword}
              onChange={(e) => setEditPassword(e.target.value)}
            />
          </div>
          <div style={{ marginBottom: 16 }}>
            <label style={{ display: "block", marginBottom: 4, fontSize: 13, color: "#ccc" }}>
              Access Level
            </label>
            <Radio.Group
              value={editAccess}
              onChange={(e) => setEditAccess(e.target.value)}
              style={{ display: "flex", flexDirection: "column", gap: 8 }}
            >
              {(["read", "write", "ddl", "full"] as const).map((level) => (
                <Radio key={level} value={level}>
                  <Tooltip title={accessLabels[level]}>
                    <span style={{ textTransform: "uppercase", fontWeight: 600 }}>{level}</span>
                    <span style={{ marginLeft: 8, fontSize: 12, color: "#888" }}>— {accessLabels[level]}</span>
                  </Tooltip>
                </Radio>
              ))}
            </Radio.Group>
          </div>
        </div>
      </Modal>

      <Modal
        title={`Add Database — ${addDbTarget?.username ?? ""}`}
        open={addDbOpen}
        onOk={handleAddDb}
        onCancel={() => { setAddDbOpen(false); setAddDbTarget(null); setAddDbError(null); }}
        confirmLoading={addDbMutation.isPending}
      >
        <div style={{ marginTop: 8 }}>
          <label style={{ display: "block", marginBottom: 4, fontSize: 13, color: "#ccc" }}>
            Database
          </label>
          <Select
            placeholder="select database"
            value={addDbName || undefined}
            onChange={(v) => { setAddDbName(v ?? ""); setAddDbError(null); }}
            style={{ width: "100%" }}
            options={(databases ?? []).map((d) => ({ value: d.name, label: d.name }))}
          />
          {addDbError !== null && (
            <div style={{ color: "#ff4d4f", fontSize: 12, marginTop: 4 }}>{addDbError}</div>
          )}
        </div>
      </Modal>
    </div>
  );
}
