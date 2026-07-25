import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { DatabaseBackup, Loader2, Download, CheckCircle2 } from "lucide-react";
import {
  fetchBackupDatabases,
  backupDatabase,
  downloadBlob,
} from "../../api/client";
import type { BackupDatabase } from "../../api/client";
import { Button } from "../ui/button";
import { CheckboxItem } from "./CheckboxItem";

export function BackupDatabaseTab() {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [downloading, setDownloading] = useState(false);

  const { data: databases, isLoading } = useQuery({
    queryKey: ["backup-databases"],
    queryFn: fetchBackupDatabases,
  });

  const toggle = (name: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  };

  const selectAll = () => {
    if (!databases) return;
    if (selected.size === databases.length) {
      setSelected(new Set());
    } else {
      setSelected(new Set(databases.map((d) => d.name)));
    }
  };

  const handleBackup = async () => {
    if (selected.size === 0) return;
    setDownloading(true);
    try {
      for (const dbName of selected) {
        const blob = await backupDatabase(dbName);
        const timestamp = new Date()
          .toISOString()
          .slice(0, 19)
          .replace(/[:.]/g, "-");
        downloadBlob(blob, `${dbName}_${timestamp}.dump`);
      }
    } catch {
      // error thrown by backupDatabase
    } finally {
      setDownloading(false);
    }
  };

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="size-5 animate-spin text-ink-muted" />
      </div>
    );
  }

  return (
    <div className="rounded-lg border border-hairline bg-surface-1 p-4">
      <div className="mb-4 flex items-center gap-3">
        <DatabaseBackup className="size-5 text-ink-muted" />
        <div>
          <h2 className="text-sm font-medium text-foreground">
            Backup Databases
          </h2>
          <p className="text-xs text-ink-muted">
            Select databases to download as individual .dump files
          </p>
        </div>
      </div>

      {!databases || databases.length === 0 ? (
        <p className="py-4 text-sm text-ink-muted">No databases found</p>
      ) : (
        <>
          <div className="space-y-1">
            <button
              onClick={selectAll}
              className="flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm text-ink-muted transition-colors hover:bg-surface-2 hover:text-foreground"
            >
              <div
                className={`flex size-4 shrink-0 items-center justify-center rounded border ${
                  selected.size === databases.length
                    ? "border-accent-blue bg-accent-blue text-white"
                    : "border-hairline bg-surface-2"
                }`}
              >
                {selected.size === databases.length && (
                  <CheckCircle2 className="size-3" />
                )}
              </div>
              Select all ({databases.length})
            </button>

            {databases.map((db: BackupDatabase) => (
              <CheckboxItem
                key={db.name}
                checked={selected.has(db.name)}
                onClick={() => toggle(db.name)}
                icon={DatabaseBackup}
                label={db.name}
              />
            ))}
          </div>

          <div className="flex justify-end pt-4">
            <Button
              disabled={selected.size === 0 || downloading}
              onClick={handleBackup}
              size="sm"
            >
              {downloading ? (
                <Loader2 className="mr-1 size-4 animate-spin" />
              ) : (
                <Download className="mr-1 size-4" />
              )}
              Backup {selected.size > 0 ? `(${selected.size})` : ""}
            </Button>
          </div>
        </>
      )}
    </div>
  );
}
