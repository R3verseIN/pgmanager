import { useState, type ReactNode } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Table, Button, Modal, Input, App } from "antd";
import { PlusOutlined, DeleteOutlined, ReloadOutlined, EyeOutlined, EyeInvisibleOutlined } from "@ant-design/icons";
import { fetchDatabases, createDatabase, deleteDatabase } from "../api/client";
import { CreateDatabaseSchema } from "../lib/schemas";
import type { Database } from "../lib/schemas";

export default function DatabasesTable() {
  const [showSystem, setShowSystem] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [newName, setNewName] = useState("");
  const [nameError, setNameError] = useState<string | null>(null);

  const queryClient = useQueryClient();
  const { message } = App.useApp();

  const { data: databases, isLoading, refetch } = useQuery({
    queryKey: ["databases", showSystem],
    queryFn: () => fetchDatabases(showSystem),
  });

  const createMutation = useMutation({
    mutationFn: (name: string) => createDatabase(name),
    onSuccess: () => {
      message.success("database created");
      setCreateOpen(false);
      setNewName("");
      setNameError(null);
      queryClient.invalidateQueries({ queryKey: ["databases"] });
    },
    onError: (error: Error) => {
      message.error(error.message);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (name: string) => deleteDatabase(name),
    onSuccess: () => {
      message.success("database deleted");
      queryClient.invalidateQueries({ queryKey: ["databases"] });
    },
    onError: (error: Error) => {
      message.error(error.message);
    },
  });

  function handleCreate() {
    const result = CreateDatabaseSchema.safeParse({ name: newName });
    if (!result.success) {
      const firstError = result.error.errors[0];
      setNameError(firstError?.message ?? "invalid name");
      return;
    }
    setNameError(null);
    createMutation.mutate(result.data.name);
  }

  function handleDelete(name: string) {
    Modal.confirm({
      title: "Delete database",
      content: `Are you sure you want to delete "${name}"? This cannot be undone.`,
      okText: "Delete",
      okType: "danger",
      onOk: () => deleteMutation.mutate(name),
    });
  }

  const columns = [
    {
      title: "Name",
      dataIndex: "name",
      key: "name",
      sorter: (a: Database, b: Database) => a.name.localeCompare(b.name),
    },
    {
      title: "",
      key: "actions",
      width: 48,
      render: (_: unknown, record: Database): ReactNode => (
        <Button
          type="text"
          danger
          icon={<DeleteOutlined />}
          disabled={record.protected}
          onClick={() => handleDelete(record.name)}
        />
      ),
    },
  ];

  return (
    <div>
      <div style={{ marginBottom: 16, display: "flex", gap: 8 }}>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateOpen(true)}>
          Create Database
        </Button>
        <Button
          icon={showSystem ? <EyeInvisibleOutlined /> : <EyeOutlined />}
          onClick={() => setShowSystem((prev) => !prev)}
        >
          {showSystem ? "Hide System" : "Show System"}
        </Button>
        <Button icon={<ReloadOutlined />} onClick={() => refetch()}>
          Refresh
        </Button>
      </div>

      <Table
        dataSource={databases}
        columns={columns}
        rowKey="name"
        loading={isLoading}
        pagination={false}
        size="small"
        bordered={false}
        rowClassName={(_, index) => (index % 2 === 0 ? "row-even" : "row-odd")}
        style={{ fontSize: 13 }}
      />

      <Modal
        title="Create Database"
        open={createOpen}
        onOk={handleCreate}
        onCancel={() => {
          setCreateOpen(false);
          setNewName("");
          setNameError(null);
        }}
        confirmLoading={createMutation.isPending}
      >
        <div style={{ marginTop: 8 }}>
          <Input
            placeholder="database name"
            value={newName}
            onChange={(e) => {
              setNewName(e.target.value);
              setNameError(null);
            }}
            onPressEnter={handleCreate}
            status={nameError !== null ? "error" : ""}
          />
          {nameError !== null && (
            <div style={{ color: "#ff4d4f", fontSize: 12, marginTop: 4 }}>
              {nameError}
            </div>
          )}
        </div>
      </Modal>
    </div>
  );
}
