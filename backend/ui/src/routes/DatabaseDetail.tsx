import { useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  ArrowLeft,
  Plus,
  RefreshCw,
  Table,
  Terminal,
  Trash2,
  GripVertical,
} from "lucide-react";
import { fetchTables, createTable } from "../api/client";
import { CreateDatabaseSchema } from "../lib/schemas";
import type { TableInfo } from "../lib/schemas";
import { useAuth } from "../contexts/AuthContext";
import {
  POSTGRESQL_TYPES,
  buildTypeString,
} from "../lib/pg-types";

import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../components/ui/select";
import SqlConsole from "../components/SqlConsole";

interface ColumnDraft {
  id: string;
  name: string;
  type: string;
  length: string;
  precision: string;
  scale: string;
  nullable: boolean;
  isPrimaryKey: boolean;
  default: string;
}

function newColumnDraft(): ColumnDraft {
  return {
    id: crypto.randomUUID(),
    name: "",
    type: "TEXT",
    length: "",
    precision: "",
    scale: "",
    nullable: true,
    isPrimaryKey: false,
    default: "",
  };
}

function serialColumnDraft(): ColumnDraft {
  return {
    id: crypto.randomUUID(),
    name: "id",
    type: "SERIAL",
    length: "",
    precision: "",
    scale: "",
    nullable: false,
    isPrimaryKey: true,
    default: "",
  };
}

export default function DatabaseDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { isAdmin, isDev } = useAuth();
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [newTableName, setNewTableName] = useState("");
  const [tableNameError, setTableNameError] = useState<string | null>(null);
  const [columns, setColumns] = useState<ColumnDraft[]>([serialColumnDraft()]);
  const [tab, setTab] = useState("tables");

  const {
    data: tables,
    isLoading: tablesLoading,
    refetch: refetchTables,
  } = useQuery({
    queryKey: ["tables", name],
    queryFn: () => fetchTables(name!),
    enabled: !!name,
  });

  const createTableMutation = useMutation({
    mutationFn: (tableName: string) =>
      createTable(
        name!,
        tableName,
        columns.map((col) => {
          const typeStr = buildTypeString(col.type, {
            length: col.length ? parseInt(col.length, 10) : undefined,
            precision: col.precision ? parseInt(col.precision, 10) : undefined,
            scale: col.scale ? parseInt(col.scale, 10) : undefined,
          });
          return {
            name: col.name,
            type: typeStr,
            nullable: col.nullable,
            isPrimaryKey: col.isPrimaryKey,
          };
        })
      ),
    onSuccess: () => {
      toast.success("Table created");
      setCreateOpen(false);
      setNewTableName("");
      setTableNameError(null);
      setColumns([serialColumnDraft()]);
      queryClient.invalidateQueries({ queryKey: ["tables", name] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  function handleCreateTable(e?: React.FormEvent) {
    e?.preventDefault();
    const result = CreateDatabaseSchema.safeParse({ name: newTableName });
    if (!result.success) {
      const firstError = result.error.errors[0];
      setTableNameError(firstError?.message ?? "Invalid name");
      return;
    }

    const validColumns = columns.filter((c) => c.name.trim());
    if (validColumns.length === 0) {
      toast.error("Add at least one column");
      return;
    }

    for (const col of validColumns) {
      if (!col.name.trim()) {
        toast.error("All columns must have a name");
        return;
      }
    }

    setTableNameError(null);
    setColumns(validColumns);
    createTableMutation.mutate(result.data.name);
  }

  function addColumn() {
    setColumns([...columns, newColumnDraft()]);
  }

  function removeColumn(id: string) {
    if (columns.length <= 1) return;
    setColumns(columns.filter((c) => c.id !== id));
  }

  function updateColumn(id: string, updates: Partial<ColumnDraft>) {
    setColumns(
      columns.map((c) => (c.id === id ? { ...c, ...updates } : c))
    );
  }

  if (!name) return null;

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" onClick={() => navigate("/")}>
          <ArrowLeft className="size-4" />
        </Button>
        <div>
          <h1 className="text-xl font-(--font-display) tracking-tight">{name}</h1>
          <p className="text-sm text-ink-muted">Database browser</p>
        </div>
      </div>

      <Tabs value={tab} onValueChange={setTab}>
        <div className="flex items-center justify-between">
          <TabsList>
            <TabsTrigger value="tables">
              <Table className="mr-2 size-4" />
              Tables
            </TabsTrigger>
            {(isAdmin || isDev) && (
              <TabsTrigger value="query">
                <Terminal className="mr-2 size-4" />
                SQL Console
              </TabsTrigger>
            )}
          </TabsList>
        </div>

        <TabsContent value="tables" className="space-y-4">
          <div className="flex items-center gap-2">
            {(isAdmin || isDev) && (
              <Button onClick={() => setCreateOpen(true)}>
                <Plus className="mr-2 size-4" />
                Create Table
              </Button>
            )}
            <Button variant="outline" onClick={() => refetchTables()}>
              <RefreshCw className="mr-2 size-4" />
              Refresh
            </Button>
          </div>

          {tablesLoading ? (
            <div className="py-8 text-center text-ink-muted">
              Loading...
            </div>
          ) : !tables?.length ? (
            <div className="py-8 text-center text-ink-muted">
              No tables found.
            </div>
          ) : (
            <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
              {tables.map((t: TableInfo, i: number) => (
                <button
                  key={t.name}
                  onClick={() =>
                    navigate(`/databases/${name}/tables/${t.name}`)
                  }
                  className="flex animate-in items-center justify-between rounded-[10px] border border-hairline bg-surface-1 p-4 text-left transition-all duration-200 fill-mode-both fade-in slide-in-from-bottom-2 hover:border-border hover:bg-surface-2"
                  style={{ animationDelay: `${i * 30}ms` }}
                >
                  <div className="flex items-center gap-3">
                    <Table className="size-5 text-ink-muted" />
                    <div>
                      <div className="font-medium">{t.name}</div>
                      <div className="text-xs text-ink-muted">
                        ~{t.rowCount.toLocaleString()} rows
                      </div>
                    </div>
                  </div>
                </button>
              ))}
            </div>
          )}
        </TabsContent>

        <TabsContent value="query">
          <SqlConsole dbName={name} />
        </TabsContent>
      </Tabs>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent className="max-h-[85vh] max-w-2xl overflow-y-auto">
          <DialogHeader>
            <DialogTitle>Create Table</DialogTitle>
            <DialogDescription>
              Create a new table in {name}. Define columns with their types and
              constraints.
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleCreateTable}>
            <div className="space-y-6 py-4">
              <div className="space-y-2">
                <Label>Table Name</Label>
                <Input
                  placeholder="e.g. users"
                  value={newTableName}
                  onChange={(e) => {
                    setNewTableName(e.target.value);
                    setTableNameError(null);
                  }}
                  autoFocus
                />
                {tableNameError && (
                  <p className="text-sm text-destructive">{tableNameError}</p>
                )}
              </div>

              <div className="space-y-3">
                <div className="flex items-center justify-between">
                  <Label>Columns</Label>
                  <Button
                    type="button"
                    variant="outline"
                    size="sm"
                    onClick={addColumn}
                  >
                    <Plus className="mr-1 size-3" />
                    Add Column
                  </Button>
                </div>

                <div className="space-y-2">
                  {columns.map((col) => {
                    const typeDef = POSTGRESQL_TYPES.find(
                      (t) => t.value === col.type
                    );
                    const hasLength = typeDef != null && "hasLength" in typeDef && typeDef.hasLength === true;
                    const hasPrecision = typeDef != null && "hasPrecision" in typeDef && typeDef.hasPrecision === true;

                    return (
                      <div
                        key={col.id}
                        className="flex items-start gap-2 rounded-[10px] border border-hairline bg-surface-2/50 p-3"
                      >
                        <GripVertical className="mt-2 size-4 shrink-0 text-ink-muted" />
                        <div className="flex-1 space-y-2">
                          <div className="flex gap-2">
                            <Input
                              placeholder="column_name"
                              value={col.name}
                              onChange={(e) =>
                                updateColumn(col.id, { name: e.target.value })
                              }
                              className="flex-1"
                            />
                            <Select
                              value={col.type}
                              onValueChange={(val) =>
                                updateColumn(col.id, { type: val })
                              }
                            >
                              <SelectTrigger className="w-40">
                                <SelectValue />
                              </SelectTrigger>
                              <SelectContent>
                                {Object.entries(
                                  POSTGRESQL_TYPES.reduce(
                                    (acc, t) => ({
                                      ...acc,
                                      [t.category]: [
                                        ...(acc[t.category] || []),
                                        t,
                                      ],
                                    }),
                                    {} as Record<string, typeof POSTGRESQL_TYPES[number][]>
                                  )
                                ).map(([category, types]) => (
                                  <div key={category}>
                                    <div className="px-2 py-1 text-xs font-medium text-ink-muted">
                                      {category}
                                    </div>
                                    {types.map((t) => (
                                      <SelectItem
                                        key={t.value}
                                        value={t.value}
                                      >
                                        {t.label}
                                      </SelectItem>
                                    ))}
                                  </div>
                                ))}
                              </SelectContent>
                            </Select>
                            {hasLength && (
                              <Input
                                placeholder="length"
                                value={col.length}
                                onChange={(e) =>
                                  updateColumn(col.id, {
                                    length: e.target.value,
                                  })
                                }
                                className="w-20"
                                type="number"
                                min="1"
                              />
                            )}
                            {hasPrecision && (
                              <div className="flex gap-1">
                                <Input
                                  placeholder="prec"
                                  value={col.precision}
                                  onChange={(e) =>
                                    updateColumn(col.id, {
                                      precision: e.target.value,
                                    })
                                  }
                                  className="w-16"
                                  type="number"
                                  min="1"
                                />
                                <Input
                                  placeholder="scale"
                                  value={col.scale}
                                  onChange={(e) =>
                                    updateColumn(col.id, {
                                      scale: e.target.value,
                                    })
                                  }
                                  className="w-16"
                                  type="number"
                                  min="0"
                                />
                              </div>
                            )}
                          </div>
                          <div className="flex items-center gap-4">
                            <div className="flex items-center gap-1.5">
                              <input
                                type="checkbox"
                                id={`pk-${col.id}`}
                                checked={col.isPrimaryKey}
                                onChange={(e) =>
                                  updateColumn(col.id, {
                                    isPrimaryKey: e.target.checked,
                                    nullable: e.target.checked
                                      ? false
                                      : col.nullable,
                                  })
                                }
                                className="size-3.5 rounded border-hairline"
                              />
                              <Label
                                htmlFor={`pk-${col.id}`}
                                className="text-xs"
                              >
                                PK
                              </Label>
                            </div>
                            {!col.isPrimaryKey && (
                              <div className="flex items-center gap-1.5">
                                <input
                                  type="checkbox"
                                  id={`null-${col.id}`}
                                  checked={col.nullable}
                                  onChange={(e) =>
                                    updateColumn(col.id, {
                                      nullable: e.target.checked,
                                    })
                                  }
                                  className="size-3.5 rounded border-hairline"
                                />
                                <Label
                                  htmlFor={`null-${col.id}`}
                                  className="text-xs"
                                >
                                  Nullable
                                </Label>
                              </div>
                            )}
                            <Input
                              placeholder="DEFAULT"
                              value={col.default}
                              onChange={(e) =>
                                updateColumn(col.id, { default: e.target.value })
                              }
                              className="h-7 flex-1 text-xs"
                            />
                            {columns.length > 1 && (
                              <Button
                                type="button"
                                variant="ghost"
                                size="icon"
                                className="size-7 text-destructive hover:bg-destructive/10"
                                onClick={() => removeColumn(col.id)}
                              >
                                <Trash2 className="size-3.5" />
                              </Button>
                            )}
                          </div>
                        </div>
                      </div>
                    );
                  })}
                </div>
              </div>
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setCreateOpen(false)}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={createTableMutation.isPending}>
                Create Table
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
