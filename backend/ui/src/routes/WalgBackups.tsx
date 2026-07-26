import { useState, useEffect } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Cloud,
  Loader2,
  Save,
  Trash2,
  Download,
  CheckCircle2,
  XCircle,
  Settings,
  Archive,
} from "lucide-react";
import {
  fetchWalgStatus,
  fetchWalgConfig,
  updateWalgConfig,
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
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { Switch } from "../components/ui/switch";
import { toast } from "sonner";

export default function WalgBackups() {
  const queryClient = useQueryClient();

  const { data: status, isLoading: statusLoading } = useQuery({
    queryKey: ["walg-status"],
    queryFn: fetchWalgStatus,
  });

  const { data: config, isLoading: configLoading } = useQuery({
    queryKey: ["walg-config"],
    queryFn: fetchWalgConfig,
    enabled: status?.enabled ?? false,
  });

  const { data: backups, isLoading: backupsLoading } = useQuery({
    queryKey: ["walg-backups"],
    queryFn: fetchWalgBackups,
    enabled: status?.enabled ?? false,
    refetchInterval: 30000,
  });

  // Config form state
  const [s3Prefix, setS3Prefix] = useState("");
  const [endpoint, setEndpoint] = useState("");
  const [region, setRegion] = useState("us-east-1");
  const [forcePathStyle, setForcePathStyle] = useState(false);
  const [interval, setInterval] = useState(3600);
  const [retentionDays, setRetentionDays] = useState(7);

  useEffect(() => {
    if (config) {
      setS3Prefix(config.s3Prefix || "");
      setEndpoint(config.endpoint || "");
      setRegion(config.region || "us-east-1");
      setForcePathStyle(config.forcePathStyle === "true");
      if (config.interval) setInterval(parseInt(config.interval) || 3600);
      if (config.retentionDays)
        setRetentionDays(parseInt(config.retentionDays) || 7);
    }
  }, [config]);

  const configMutation = useMutation({
    mutationFn: () =>
      updateWalgConfig({
        s3Prefix,
        ...((endpoint || undefined) && { endpoint }),
        region,
        forcePathStyle,
        interval,
        retentionDays,
      }),
    onSuccess: (data) => {
      toast.success(data.message || "Configuration saved");
      queryClient.invalidateQueries({ queryKey: ["walg-config"] });
      queryClient.invalidateQueries({ queryKey: ["walg-status"] });
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
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
                <span>
                  Backups: {status.backupCount}
                </span>
                {status.totalSize > 0 && (
                  <span>
                    Storage: {formatBytes(status.totalSize)}
                  </span>
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
            </div>
          </div>
        </div>
      )}

      {/* S3 Configuration */}
      <div className="rounded-lg border border-hairline bg-surface-1 p-4">
        <div className="mb-4 flex items-center gap-3">
          <Settings className="size-5 text-ink-muted" />
          <div>
            <h2 className="text-sm font-medium text-foreground">
              S3 Configuration
            </h2>
            <p className="text-xs text-ink-muted">
              Configure S3-compatible storage for backups. Leave empty to disable
              WAL-G.
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
              <Label className="text-sm text-ink-muted">
                S3 Bucket Path <span className="text-red-500">*</span>
              </Label>
              <Input
                value={s3Prefix}
                onChange={(e) => setS3Prefix(e.target.value)}
                placeholder="s3://my-bucket/pgmanager"
                className="border-hairline bg-surface-2"
              />
              <p className="text-xs text-ink-muted">
                Full S3 URI including bucket and optional path prefix
              </p>
            </div>

            <div className="rounded-md bg-surface-2 p-3 text-xs text-ink-muted">
              <p>
                S3 credentials (<code className="text-foreground">AWS_ACCESS_KEY_ID</code>,{" "}
                <code className="text-foreground">AWS_SECRET_ACCESS_KEY</code>) are
                configured via environment variables. See the{" "}
                <a
                  href="https://github.com/R3verseIN/pgmanager/blob/main/docs/walg-s3-setup.md"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-accent-blue underline"
                >
                  setup guide
                </a>{" "}
                for provider-specific instructions.
              </p>
            </div>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label className="text-sm text-ink-muted">
                  Endpoint URL
                </Label>
                <Input
                  value={endpoint}
                  onChange={(e) => setEndpoint(e.target.value)}
                  placeholder="http://minio:9000 (leave empty for AWS S3)"
                  className="border-hairline bg-surface-2"
                />
                <p className="text-xs text-ink-muted">
                  Required for MinIO, SeaweedFS, or other S3-compatible storage
                </p>
              </div>
              <div className="space-y-2">
                <Label className="text-sm text-ink-muted">Region</Label>
                <Input
                  value={region}
                  onChange={(e) => setRegion(e.target.value)}
                  placeholder="us-east-1"
                  className="border-hairline bg-surface-2"
                />
              </div>
            </div>

            <div className="flex items-center gap-3">
              <Switch
                checked={forcePathStyle}
                onCheckedChange={setForcePathStyle}
              />
              <div>
                <Label className="text-sm text-ink-muted">
                  Force Path Style
                </Label>
                <p className="text-xs text-ink-muted">
                  Required for MinIO and most S3-compatible storage
                </p>
              </div>
            </div>

            <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
              <div className="space-y-2">
                <Label className="text-sm text-ink-muted">
                  Backup Interval (seconds)
                </Label>
                <Input
                  type="number"
                  min={60}
                  value={interval}
                  onChange={(e) => setInterval(parseInt(e.target.value) || 3600)}
                  className="border-hairline bg-surface-2"
                />
                <p className="text-xs text-ink-muted">
                  How often to create base backups (default: 3600 = 1 hour)
                </p>
              </div>
              <div className="space-y-2">
                <Label className="text-sm text-ink-muted">
                  Retention (days)
                </Label>
                <Input
                  type="number"
                  min={1}
                  value={retentionDays}
                  onChange={(e) =>
                    setRetentionDays(parseInt(e.target.value) || 7)
                  }
                  className="border-hairline bg-surface-2"
                />
                <p className="text-xs text-ink-muted">
                  Number of days to keep backups before automatic cleanup
                </p>
              </div>
            </div>

            <div className="flex justify-end gap-2 pt-2">
              <Button
                variant="outline"
                size="sm"
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
                disabled={testConnectionMutation.isPending || !s3Prefix}
                onClick={() => testConnectionMutation.mutate()}
              >
                {testConnectionMutation.isPending ? (
                  <Loader2 className="mr-1 size-4 animate-spin" />
                ) : (
                  <CheckCircle2 className="mr-1 size-4" />
                )}
                Test Connection
              </Button>
              <Button
                size="sm"
                disabled={configMutation.isPending || !s3Prefix}
                onClick={() => configMutation.mutate()}
              >
                {configMutation.isPending ? (
                  <Loader2 className="mr-1 size-4 animate-spin" />
                ) : (
                  <Save className="mr-1 size-4" />
                )}
                Save Configuration
              </Button>
            </div>
          </div>
        )}
      </div>

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
                      <td className="py-2 font-mono text-xs">
                        {backup.name}
                      </td>
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

      {/* Not Configured State */}
      {!status?.enabled && (
        <div className="rounded-lg border border-hairline bg-surface-1 p-8 text-center">
          <Cloud className="mx-auto mb-3 size-10 text-ink-muted" />
          <h2 className="text-sm font-medium text-foreground">
            WAL-G Not Configured
          </h2>
          <p className="mt-1 text-xs text-ink-muted">
            Configure S3 storage above to enable continuous WAL archiving and
            automated base backups with point-in-time recovery.
          </p>
          <div className="mt-4 mx-auto max-w-md text-left rounded-md bg-surface-2 p-3 text-xs text-ink-muted">
            <p className="font-medium text-foreground mb-1">How it works:</p>
            <ul className="space-y-1 list-disc list-inside">
              <li>
                PostgreSQL continuously archives WAL segments to S3 (every 60s)
              </li>
              <li>Base backups are created at the configured interval</li>
              <li>
                Restore to any point in time using WAL-G point-in-time recovery
              </li>
              <li>
                Works with AWS S3, MinIO, and any S3-compatible storage
              </li>
            </ul>
          </div>
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
