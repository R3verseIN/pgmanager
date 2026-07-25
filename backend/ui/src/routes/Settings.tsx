import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Shield, Loader2 } from "lucide-react";
import {
  fetchPgBouncerDatabases,
  togglePgBouncerDatabase,
} from "../api/client";
import type { PgBouncerDatabase } from "../api/client";
import { DatabaseAccessRow } from "../components/DatabaseAccessRow";

export default function Settings() {
  const queryClient = useQueryClient();

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

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-(--font-display) text-foreground">
          Settings
        </h1>
        <p className="text-sm text-ink-muted">
          Manage PgBouncer database access
        </p>
      </div>

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
