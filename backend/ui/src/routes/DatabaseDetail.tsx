import { useState } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ArrowLeft, Plus, RefreshCw, Table, Terminal } from "lucide-react";
import { fetchTables, createTable } from "../api/client";
import { CreateDatabaseSchema } from "../lib/schemas";
import type { TableInfo } from "../lib/schemas";
import { useAuth } from "../contexts/AuthContext";

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
import SqlConsole from "../components/SqlConsole";

export default function DatabaseDetail() {
  const { name } = useParams<{ name: string }>();
  const navigate = useNavigate();
  const { isAdmin, isDev } = useAuth();
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [newTableName, setNewTableName] = useState("");
  const [tableNameError, setTableNameError] = useState<string | null>(null);
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
      createTable(name!, tableName, [
        { name: "id", type: "SERIAL", nullable: false, isPrimaryKey: true },
      ]),
    onSuccess: () => {
      toast.success("Table created");
      setCreateOpen(false);
      setNewTableName("");
      setTableNameError(null);
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
    setTableNameError(null);
    createTableMutation.mutate(result.data.name);
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
                  className="flex animate-in items-center justify-between rounded-md border border-hairline bg-surface-1 p-4 text-left transition-all duration-200 fill-mode-both fade-in slide-in-from-bottom-2 hover:border-border hover:bg-surface-2"
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
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create Table</DialogTitle>
            <DialogDescription>
              Create a new table in {name} with a default id column.
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleCreateTable}>
            <div className="space-y-4 py-4">
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
                Create
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
