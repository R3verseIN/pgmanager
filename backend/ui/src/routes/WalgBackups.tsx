import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Cloud,
  Loader2,
  Trash2,
  Download,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  Archive,
  Terminal,
} from "lucide-react";
import {
  fetchWalgStatus,
  fetchWalgBackups,
  triggerWalgBackup,
  deleteWalgBackup,
  verifyWalgIntegrity,
  cleanWalgGarbage,
  testWalgConnection,
} from "../api/client";
import type { WalgBackup } from "../api/client";
import RestoreDialog from "../components/dialogs/RestoreDialog";
import { Button } from "../components/ui/button";
import { toast } from "sonner";

export default function WalgBackups() {
  const queryClient = useQueryClient();

  const { data: status, isLoading: statusLoading } = useQuery({
    queryKey: ["walg-status"],
    queryFn: fetchWalgStatus,
  });

  const { data: backups, isLoading: backupsLoading } = useQuery({
    queryKey: ["walg-backups"],
    queryFn: fetchWalgBackups,
    enabled: status?.enabled ?? false,
    refetchInterval: 30000,
  });

  const backupMutation = useMutation({
    mutationFn: triggerWalgBackup,
    onSuccess: () => {
      toast.success("Base backup started");
      queryClient.invalidateQueries({ queryKey: ["walg-backups"] });
      queryClient.invalidateQueries({ queryKey: ["walg-status"] });
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });

  const verifyMutation = useMutation({
    mutationFn: verifyWalgIntegrity,
    onSuccess: (data) => {
      if (data.status === "OK") {
        toast.success("WAL integrity check passed");
      } else {
        toast.warning(`WAL integrity: ${data.status}`);
      }
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });

  const garbageMutation = useMutation({
    mutationFn: cleanWalgGarbage,
    onSuccess: () => {
      toast.success("Garbage cleanup completed");
      queryClient.invalidateQueries({ queryKey: ["walg-backups"] });
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });

  const testConnectionMutation = useMutation({
    mutationFn: testWalgConnection,
    onSuccess: (data) => {
      toast.success(data.message || "Connection successful");
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });

  // Restore dialog state
  const [restoreDialogOpen, setRestoreDialogOpen] = useState(false);
  const [restoreBackupName, setRestoreBackupName] = useState("");

  const handleDelete = async (name: string) => {
    if (!confirm(`Delete backup ${name}? This cannot be undone.`)) return;
    try {
      await deleteWalgBackup(name);
      toast.success(`Deleted backup ${name}`);
      queryClient.invalidateQueries({ queryKey: ["walg-backups"] });
      queryClient.invalidateQueries({ queryKey: ["walg-status"] });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Delete failed");
    }
  };

  function formatBytes(bytes: number): string {
    if (bytes === 0) return "0 B";
    const k = 1024;
    const sizes = ["B", "KB", "MB", "GB", "TB"];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
  }

  if (statusLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="size-5 animate-spin text-ink-muted" />
      </div>
    );
  }

  const errors = status?.errors || [];
  const warnings = status?.warnings || [];
  const hasErrors = errors.length > 0;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-(--font-display) text-foreground">
          S3 Backups
        </h1>
        <p className="text-sm text-ink-muted">
          Continuous WAL archiving and base backups to S3-compatible storage
        </p>
      </div>

      {/* Diagnostics Panel — shown when env vars are missing/misconfigured */}
      {hasErrors && (
        <div className="rounded-lg border border-red-200 bg-red-50 p-4 dark:border-red-900/50 dark:bg-red-950/30">
          <div className="flex items-start gap-3">
            <AlertTriangle className="mt-0.5 size-5 shrink-0 text-red-500" />
            <div className="flex-1">
              <h2 className="text-sm font-medium text-red-700 dark:text-red-400">
                WAL-G Not Configured
              </h2>
              <p className="mt-1 text-xs text-red-600/80 dark:text-red-400/70">
                Set the following environment variables to enable S3 backups:
              </p>
              <div className="mt-3 rounded-md bg-red-100/50 p-3 dark:bg-red-900/30">
                <code className="block whitespace-pre text-xs text-red-700 dark:text-red-300">
{`environment:
  WALG_S3_PREFIX: "s3://your-bucket/path"
  AWS_ACCESS_KEY_ID: "your-key"
  AWS_SECRET_ACCESS_KEY: "your-secret"
${warnings.includes("AWS_ENDPOINT is not set") ? `  AWS_ENDPOINT: "https://<account-id>.r2.cloudflarestorage.com  # Cloudflare R2
  AWS_REGION: "auto"  # Required for R2
  AWS_S3_FORCE_PATH_STYLE: "true"  # Required for R2"` : warnings.includes("AWS_S3_FORCE_PATH_STYLE is not set") ? `  AWS_S3_FORCE_PATH_STYLE: "true"` : ""}`}
                </code>
              </div>
              <ul className="mt-2 space-y-1 text-xs text-red-600/80 dark:text-red-400/70">
                {errors.map((e, i) => (
                  <li key={i} className="flex items-center gap-1">
                    <XCircle className="size-3 shrink-0" />
                    {e}
                  </li>
                ))}
              </ul>
              <a
                href="https://github.com/R3verseIN/pgmanager/blob/main/docs/walg-s3-setup.md"
                target="_blank"
                rel="noopener noreferrer"
                className="mt-2 inline-flex items-center gap-1 text-xs text-red-600 underline hover:text-red-700 dark:text-red-400"
              >
                <Terminal className="size-3" />
                Setup Guide
              </a>
            </div>
          </div>
        </div>
      )}

      {/* Status Banner */}
      {status?.enabled && (
        <div className="rounded-lg border border-hairline bg-surface-1 p-4">
          <div className="flex items-center gap-3">
            <Cloud className="size-5 text-accent-blue" />
            <div className="flex-1">
              <h2 className="text-sm font-medium text-foreground">
                WAL-G Status
              </h2>
              <div className="mt-1 flex flex-wrap gap-3 text-xs text-ink-muted">
                <span className="flex items-center gap-1">
                  {status.archiving ? (
                    <CheckCircle2 className="size-3 text-green-500" />
                  ) : (
                    <XCircle className="size-3 text-red-500" />
                  )}
                  Archiving: {status.archiving ? "Active" : "Inactive"}
                </span>
                <span>Backups: {status.backupCount}</span>
                {status.totalSize > 0 && (
                  <span>Storage: {formatBytes(status.totalSize)}</span>
                )}
                {status.lastBackup && (
                  <span>
                    Last: {new Date(status.lastBackup).toLocaleString()}
                  </span>
                )}
                <span>Interval: {status.intervalSec}s</span>
                <span>Retention: {status.retentionDays}d</span>
              </div>
            </div>
            <div className="flex gap-2">
              <Button
                size="sm"
                variant="outline"
                disabled={backupMutation.isPending}
                onClick={() => backupMutation.mutate()}
              >
                {backupMutation.isPending ? (
                  <Loader2 className="mr-1 size-4 animate-spin" />
                ) : (
                  <Download className="mr-1 size-4" />
                )}
                Backup Now
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={verifyMutation.isPending}
                onClick={() => verifyMutation.mutate()}
              >
                {verifyMutation.isPending ? (
                  <Loader2 className="mr-1 size-4 animate-spin" />
                ) : (
                  <CheckCircle2 className="mr-1 size-4" />
                )}
                Verify
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={garbageMutation.isPending}
                onClick={() => garbageMutation.mutate()}
              >
                {garbageMutation.isPending ? (
                  <Loader2 className="mr-1 size-4 animate-spin" />
                ) : (
                  <Trash2 className="mr-1 size-4" />
                )}
                Clean Garbage
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={testConnectionMutation.isPending}
                onClick={() => testConnectionMutation.mutate()}
              >
                {testConnectionMutation.isPending ? (
                  <Loader2 className="mr-1 size-4 animate-spin" />
                ) : (
                  <CheckCircle2 className="mr-1 size-4" />
                )}
                Test Connection
              </Button>
            </div>
          </div>
          {warnings.length > 0 && (
            <div className="mt-3 rounded-md bg-surface-2 p-2">
              {warnings.map((w, i) => (
                <p key={i} className="flex items-center gap-1 text-xs text-amber-600">
                  <AlertTriangle className="size-3 shrink-0" />
                  {w}
                </p>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Backup List */}
      {status?.enabled && (
        <div className="rounded-lg border border-hairline bg-surface-1 p-4">
          <div className="mb-4 flex items-center gap-3">
            <Archive className="size-5 text-ink-muted" />
            <div>
              <h2 className="text-sm font-medium text-foreground">
                S3 Backups
              </h2>
              <p className="text-xs text-ink-muted">
                Backups stored in S3 — restore or delete as needed
              </p>
            </div>
          </div>

          {backupsLoading ? (
            <div className="flex items-center justify-center py-8">
              <Loader2 className="size-5 animate-spin text-ink-muted" />
            </div>
          ) : !backups || backups.length === 0 ? (
            <div className="py-8 text-center">
              <Cloud className="mx-auto mb-2 size-8 text-ink-muted" />
              <p className="text-sm text-ink-muted">
                No backups found in S3 storage
              </p>
              <p className="mt-1 text-xs text-ink-muted">
                Click "Backup Now" to create your first base backup
              </p>
            </div>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-hairline text-left text-xs text-ink-muted">
                    <th className="pb-2 font-medium">Name</th>
                    <th className="pb-2 font-medium">Time</th>
                    <th className="pb-2 font-medium">WAL Segment</th>
                    <th className="pb-2 font-medium text-right">Size</th>
                    <th className="pb-2 font-medium text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-hairline">
                  {backups.map((backup: WalgBackup) => (
                    <tr key={backup.name} className="group">
                      <td className="py-2 font-mono text-xs">{backup.name}</td>
                      <td className="py-2 text-xs text-ink-muted">
                        {backup.time
                          ? new Date(backup.time).toLocaleString()
                          : "—"}
                      </td>
                      <td className="py-2 font-mono text-xs text-ink-muted">
                        {backup.walSegment || "—"}
                      </td>
                      <td className="py-2 text-right text-xs text-ink-muted">
                        {backup.size > 0 ? formatBytes(backup.size) : "—"}
                      </td>
                      <td className="py-2 text-right">
                        <div className="flex justify-end gap-1 opacity-0 transition-opacity group-hover:opacity-100">
                          <Button
                            size="sm"
                            variant="ghost"
                            className="h-7 px-2 text-xs"
                            onClick={() => {
                              setRestoreBackupName(backup.name);
                              setRestoreDialogOpen(true);
                            }}
                          >
                            <Download className="mr-1 size-3" />
                            Restore
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            className="h-7 px-2 text-xs text-red-500 hover:text-red-600"
                            onClick={() => handleDelete(backup.name)}
                          >
                            <Trash2 className="mr-1 size-3" />
                            Delete
                          </Button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      <RestoreDialog
        open={restoreDialogOpen}
        onOpenChange={setRestoreDialogOpen}
        backupName={restoreBackupName}
        onRestored={() => {
          queryClient.invalidateQueries({ queryKey: ["walg-backups"] });
          queryClient.invalidateQueries({ queryKey: ["walg-status"] });
        }}
      />
    </div>
  );
}
