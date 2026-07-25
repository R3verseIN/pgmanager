import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Shield, Loader2, Save } from "lucide-react";
import {
  fetchPgBouncerDatabases,
  togglePgBouncerDatabase,
  fetchPgBouncerConfig,
  updatePgBouncerConfig,
} from "../api/client";
import type { PgBouncerDatabase, PgBouncerConfig } from "../api/client";
import { DatabaseAccessRow } from "../components/DatabaseAccessRow";
import { Select, SelectTrigger, SelectContent, SelectItem } from "../components/ui/select";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { Button } from "../components/ui/button";

export default function Settings() {
  const queryClient = useQueryClient();

  // --- Database Access ---
  const { data: databases, isLoading } = useQuery({
    queryKey: ["pgbouncer-databases"],
    queryFn: fetchPgBouncerDatabases,
  });

  const toggleMutation = useMutation({
    mutationFn: ({
      databaseName,
      allowed,
    }: {
      databaseName: string;
      allowed: boolean;
    }) => togglePgBouncerDatabase(databaseName, allowed),
    onMutate: async ({ databaseName, allowed }) => {
      await queryClient.cancelQueries({ queryKey: ["pgbouncer-databases"] });
      const previous = queryClient.getQueryData<PgBouncerDatabase[]>([
        "pgbouncer-databases",
      ]);
      queryClient.setQueryData<PgBouncerDatabase[]>(
        ["pgbouncer-databases"],
        (old) =>
          old?.map((db) =>
            db.databaseName === databaseName ? { ...db, allowed } : db
          )
      );
      return { previous };
    },
    onError: (_err, _variables, context) => {
      if (context?.previous) {
        queryClient.setQueryData(["pgbouncer-databases"], context.previous);
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ["pgbouncer-databases"] });
    },
  });

  // --- Connection Pool Config ---
  const { data: config, isLoading: configLoading } = useQuery({
    queryKey: ["pgbouncer-config"],
    queryFn: fetchPgBouncerConfig,
  });

  const [localConfig, setLocalConfig] = useState<PgBouncerConfig>({
    poolMode: "transaction",
    defaultPoolSize: 20,
    maxClientConn: 100,
  });

  useEffect(() => {
    if (config) {
      setLocalConfig(config);
    }
  }, [config]);

  const configMutation = useMutation({
    mutationFn: (newConfig: PgBouncerConfig) =>
      updatePgBouncerConfig(newConfig),
    onSuccess: (data) => {
      queryClient.setQueryData<PgBouncerConfig>(["pgbouncer-config"], data);
    },
  });

  const hasChanges =
    config &&
    (localConfig.poolMode !== config.poolMode ||
      localConfig.defaultPoolSize !== config.defaultPoolSize ||
      localConfig.maxClientConn !== config.maxClientConn);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-(--font-display) text-foreground">
          Settings
        </h1>
        <p className="text-sm text-ink-muted">
          Manage PgBouncer configuration
        </p>
      </div>

      {/* Connection Pool */}
      <div className="rounded-lg border border-hairline bg-surface-1 p-4">
        <div className="flex items-center gap-3 mb-4">
          <Loader2 className="size-5 text-ink-muted" />
          <div>
            <h2 className="text-sm font-medium text-foreground">
              Connection Pool
            </h2>
            <p className="text-xs text-ink-muted">
              Configure PgBouncer connection pooling behavior
            </p>
          </div>
        </div>

        {configLoading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="size-5 animate-spin text-ink-muted" />
          </div>
        ) : (
          <div className="space-y-4">
            <div className="space-y-2">
              <Label className="text-sm text-ink-muted">Pool Mode</Label>
              <Select
                value={localConfig.poolMode}
                onValueChange={(value) =>
                  setLocalConfig({ ...localConfig, poolMode: value })
                }
              >
                <SelectTrigger className="w-full bg-surface-2 border-hairline">
                  <span>{localConfig.poolMode}</span>
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="session">session</SelectItem>
                  <SelectItem value="transaction">transaction</SelectItem>
                  <SelectItem value="statement">statement</SelectItem>
                </SelectContent>
              </Select>
              <p className="text-xs text-ink-muted">
                {localConfig.poolMode === "transaction"
                  ? "Best for most applications — connections released after transaction completes"
                  : localConfig.poolMode === "session"
                    ? "Connections held until client disconnects — safe but slower"
                    : "Connections released after each statement — fastest but breaks stateful features"}
              </p>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label className="text-sm text-ink-muted">Default Pool Size</Label>
                <Input
                  type="number"
                  min={1}
                  max={10000}
                  value={localConfig.defaultPoolSize}
                  onChange={(e) =>
                    setLocalConfig({
                      ...localConfig,
                      defaultPoolSize: parseInt(e.target.value) || 20,
                    })
                  }
                  className="bg-surface-2 border-hairline"
                />
                <p className="text-xs text-ink-muted">
                  Max server connections per user/database pair
                </p>
              </div>

              <div className="space-y-2">
                <Label className="text-sm text-ink-muted">Max Client Connections</Label>
                <Input
                  type="number"
                  min={1}
                  max={100000}
                  value={localConfig.maxClientConn}
                  onChange={(e) =>
                    setLocalConfig({
                      ...localConfig,
                      maxClientConn: parseInt(e.target.value) || 100,
                    })
                  }
                  className="bg-surface-2 border-hairline"
                />
                <p className="text-xs text-ink-muted">
                  Total max connections PgBouncer accepts from clients
                </p>
              </div>
            </div>

            <div className="flex justify-end pt-2">
              <Button
                disabled={!hasChanges || configMutation.isPending}
                onClick={() => configMutation.mutate(localConfig)}
                size="sm"
              >
                {configMutation.isPending ? (
                  <Loader2 className="size-4 animate-spin mr-1" />
                ) : (
                  <Save className="size-4 mr-1" />
                )}
                Save
              </Button>
            </div>
          </div>
        )}
      </div>

      {/* Database Access */}
      <div className="rounded-lg border border-hairline bg-surface-1 p-4">
        <div className="flex items-center gap-3 mb-4">
          <Shield className="size-5 text-ink-muted" />
          <div>
            <h2 className="text-sm font-medium text-foreground">
              PgBouncer Database Access
            </h2>
            <p className="text-xs text-ink-muted">
              Control which databases are accessible through PgBouncer (external
              connections)
            </p>
          </div>
        </div>

        {isLoading ? (
          <div className="flex items-center justify-center py-8">
            <Loader2 className="size-5 animate-spin text-ink-muted" />
          </div>
        ) : !databases || databases.length === 0 ? (
          <p className="text-sm text-ink-muted py-4">No databases found</p>
        ) : (
          <div className="space-y-1">
            {databases.map((db: PgBouncerDatabase) => (
              <DatabaseAccessRow
                key={db.databaseName}
                databaseName={db.databaseName}
                allowed={db.allowed}
                disabled={toggleMutation.isPending}
                onToggle={(checked) =>
                  toggleMutation.mutate({
                    databaseName: db.databaseName,
                    allowed: checked,
                  })
                }
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
