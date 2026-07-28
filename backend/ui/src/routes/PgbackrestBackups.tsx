import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  Cloud,
  Loader2,
  CheckCircle2,
  XCircle,
  AlertTriangle,
} from "lucide-react";
import {
  fetchBackupStatus,
  fetchBackups,
  triggerBackup,
  updateBackupSettings,
  testBackupConnection,
} from "../api/pgbackrest";
import type { BackupSettings } from "../api/pgbackrest";
import RestoreBackupDialog from "../components/dialogs/RestoreBackupDialog";
import RestoreTimeDialog from "../components/dialogs/RestoreTimeDialog";
import { Button } from "../components/ui/button";
import { toast } from "sonner";
import { Switch } from "../components/ui/switch";

export default function PgbackrestBackups() {
  const queryClient = useQueryClient();

  const { data: status, isLoading: statusLoading } = useQuery({
    queryKey: ["backup-status"],
    queryFn: fetchBackupStatus,
  });

  const { data: backups, isLoading: backupsLoading } = useQuery({
    queryKey: ["backup-list"],
    queryFn: fetchBackups,
    enabled: status?.settings.enabled ?? false,
    refetchInterval: 30000,
  });

  const backupMutation = useMutation({
    mutationFn: (type: string) => triggerBackup(type),
    onSuccess: () => {
      toast.success("Backup started");
      queryClient.invalidateQueries({ queryKey: ["backup-list"] });
      queryClient.invalidateQueries({ queryKey: ["backup-status"] });
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });

  const settingsMutation = useMutation({
    mutationFn: updateBackupSettings,
    onSuccess: () => {
      toast.success("Settings applied");
      queryClient.invalidateQueries({ queryKey: ["backup-status"] });
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });

  const testConnectionMutation = useMutation({
    mutationFn: testBackupConnection,
    onSuccess: (data) => {
      toast.success(data.message || "Connection successful");
    },
    onError: (err: Error) => {
      toast.error(err.message);
    },
  });

  const [restoreBackupDialogOpen, setRestoreBackupDialogOpen] = useState(false);
  const [restoreBackupName, setRestoreBackupName] = useState("");

  const [restoreTimeDialogOpen, setRestoreTimeDialogOpen] = useState(false);
  const [targetTime, setTargetTime] = useState("");

  const [editSettings, setEditSettings] = useState<BackupSettings | null>(null);

  if (statusLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="size-5 animate-spin text-ink-muted" />
      </div>
    );
  }

  const isConfigured = status?.configured;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-(--font-display) text-foreground">
          S3 Backups (pgBackRest)
        </h1>
        <p className="text-sm text-ink-muted">
          Continuous WAL archiving and Full/Incremental base backups to S3-compatible storage
        </p>
      </div>

      {!isConfigured && (
        <div className="rounded-lg border border-red-200 bg-red-50 p-4 dark:border-red-900/50 dark:bg-red-950/30">
          <div className="flex items-start gap-3">
            <AlertTriangle className="mt-0.5 size-5 shrink-0 text-red-500" />
            <div className="flex-1">
              <h2 className="text-sm font-medium text-red-700 dark:text-red-400">
                pgBackRest Not Configured
              </h2>
              <p className="mt-1 text-xs text-red-600/80 dark:text-red-400/70">
                Set the following environment variables in .env to enable S3 backups:
              </p>
              <div className="mt-3 rounded-md bg-red-100/50 p-3 dark:bg-red-900/30">
                <code className="block whitespace-pre text-xs text-red-700 dark:text-red-300">
{`PGBACKREST_REPO1_TYPE=s3
PGBACKREST_REPO1_S3_BUCKET=my-backup-bucket
PGBACKREST_REPO1_S3_ENDPOINT=s3.us-east-1.amazonaws.com
PGBACKREST_REPO1_S3_REGION=us-east-1
PGBACKREST_REPO1_S3_KEY=your_access_key
PGBACKREST_REPO1_S3_KEY_SECRET=your_secret_key
PGBACKREST_REPO1_PATH=/backups`}
                </code>
              </div>
            </div>
          </div>
        </div>
      )}

      {isConfigured && status && (
        <div className="space-y-6">
          <div className="rounded-lg border border-hairline bg-surface-1 p-4">
            <div className="flex items-center gap-3">
              <Cloud className="size-5 text-accent-blue" />
              <div className="flex-1">
                <h2 className="text-sm font-medium text-foreground">
                  Backup Configuration
                </h2>
                <div className="mt-1 flex flex-wrap gap-3 text-xs text-ink-muted">
                  <span className="flex items-center gap-1">
                    {status.settings.enabled ? (
                      <CheckCircle2 className="size-3 text-green-500" />
                    ) : (
                      <XCircle className="size-3 text-red-500" />
                    )}
                    Archiving: {status.settings.enabled ? "Enabled" : "Disabled"}
                  </span>
                  <span>Retention: {status.settings.retention_days} days</span>
                  <span>Timeout: {status.settings.archive_timeout}s</span>
                  <span>Full Day: {["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"][status.settings.full_backup_day]}</span>
                  <span>Time: {status.settings.backup_hour}:00</span>
                </div>
              </div>
              <div className="flex gap-2">
                <Button
                  size="sm"
                  variant="outline"
                  disabled={settingsMutation.isPending}
                  onClick={() => setEditSettings(status.settings)}
                >
                  Configure
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
                  Test S3
                </Button>
              </div>
            </div>
          </div>

          {editSettings && (
            <div className="rounded-lg border border-hairline bg-surface-0 p-4">
              <h3 className="mb-4 text-sm font-medium">Edit Settings</h3>
              <div className="grid gap-4 sm:grid-cols-2">
                <div className="flex items-center justify-between col-span-2">
                  <span className="text-sm">Enable Backups & Archiving</span>
                  <Switch 
                    checked={editSettings.enabled} 
                    onCheckedChange={(c) => setEditSettings({ ...editSettings, enabled: c })} 
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs text-ink-muted">Archive Timeout (s)</label>
                  <input
                    type="number"
                    className="w-full rounded-md border border-hairline bg-surface-1 p-2 text-sm"
                    value={editSettings.archive_timeout}
                    onChange={(e) => setEditSettings({ ...editSettings, archive_timeout: parseInt(e.target.value) })}
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs text-ink-muted">Retention (Days)</label>
                  <input
                    type="number"
                    className="w-full rounded-md border border-hairline bg-surface-1 p-2 text-sm"
                    value={editSettings.retention_days}
                    onChange={(e) => setEditSettings({ ...editSettings, retention_days: parseInt(e.target.value) })}
                  />
                </div>
                <div>
                  <label className="mb-1 block text-xs text-ink-muted">Full Backup Day</label>
                  <select
                    className="w-full rounded-md border border-hairline bg-surface-1 p-2 text-sm"
                    value={editSettings.full_backup_day}
                    onChange={(e) => setEditSettings({ ...editSettings, full_backup_day: parseInt(e.target.value) })}
                  >
                    {["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"].map((d, i) => (
                      <option key={i} value={i}>{d}</option>
                    ))}
                  </select>
                </div>
                <div>
                  <label className="mb-1 block text-xs text-ink-muted">Backup Hour (0-23)</label>
                  <input
                    type="number"
                    min="0"
                    max="23"
                    className="w-full rounded-md border border-hairline bg-surface-1 p-2 text-sm"
                    value={editSettings.backup_hour}
                    onChange={(e) => setEditSettings({ ...editSettings, backup_hour: parseInt(e.target.value) })}
                  />
                </div>
              </div>
              <div className="mt-4 flex justify-end gap-2">
                <Button variant="ghost" onClick={() => setEditSettings(null)}>Cancel</Button>
                <Button onClick={() => {
                  settingsMutation.mutate(editSettings);
                  setEditSettings(null);
                }}>Save & Apply</Button>
              </div>
            </div>
          )}

          {status.settings.enabled && (
            <div className="rounded-lg border border-hairline bg-surface-0 shadow-xs">
              <div className="flex items-center justify-between border-b border-hairline px-4 py-3">
                <h2 className="text-sm font-medium text-foreground">
                  Available Backups
                </h2>
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => backupMutation.mutate("full")}
                    disabled={backupMutation.isPending}
                  >
                    Run Full Backup
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => backupMutation.mutate("incr")}
                    disabled={backupMutation.isPending}
                  >
                    Run Incremental
                  </Button>
                </div>
              </div>
              <div className="p-0">
                {backupsLoading ? (
                  <div className="flex justify-center p-8">
                    <Loader2 className="size-5 animate-spin text-ink-muted" />
                  </div>
                ) : !backups || (backups as any[]).length === 0 ? (
                  <div className="p-8 text-center text-sm text-ink-muted">
                    No backups found in S3 yet. Run a full backup to start.
                  </div>
                ) : (
                  <div className="overflow-x-auto">
                    <table className="w-full text-left text-sm">
                      <thead className="bg-surface-1 text-ink-muted">
                        <tr>
                          <th className="px-4 py-3 font-medium">Backup Name</th>
                          <th className="px-4 py-3 font-medium">Time</th>
                          <th className="px-4 py-3 font-medium">Type</th>
                          <th className="px-4 py-3 text-right font-medium">Actions</th>
                        </tr>
                      </thead>
                      <tbody className="divide-y divide-hairline">
                        {(backups as any[])[0]?.backup?.map((b: any) => (
                          <tr key={b.label}>
                            <td className="px-4 py-3 text-ink-muted">
                              {b.label}
                            </td>
                            <td className="px-4 py-3 text-foreground">
                              {new Date(b.timestamp.stop * 1000).toLocaleString()}
                            </td>
                            <td className="px-4 py-3 text-ink-muted">
                              {b.type === "incr" ? "Incremental" : "Full"}
                            </td>
                            <td className="px-4 py-3 text-right">
                              <Button
                                size="sm"
                                variant="outline"
                                onClick={() => {
                                  setRestoreBackupName(b.label);
                                  setRestoreBackupDialogOpen(true);
                                }}
                              >
                                Restore
                              </Button>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            </div>
          )}

          {isConfigured && status && status.settings.enabled && (
            <div className="rounded-lg border border-hairline bg-surface-0 shadow-xs mt-6">
              <div className="border-b border-hairline px-4 py-3">
                <h2 className="text-sm font-medium text-foreground">
                  Point-In-Time Recovery
                </h2>
                <p className="mt-1 text-xs text-ink-muted">
                  Restore the database to any specific point in time using continuous WAL archives.
                </p>
              </div>
              <div className="p-4 flex flex-col sm:flex-row items-end gap-4">
                <div className="flex-1 w-full space-y-1">
                  <label className="text-xs text-ink-muted">Target Date & Time</label>
                  <input 
                    type="datetime-local" 
                    className="w-full rounded-md border border-hairline bg-surface-1 p-2 text-sm"
                    value={targetTime}
                    onChange={(e) => setTargetTime(e.target.value)}
                  />
                </div>
                <Button 
                  onClick={() => setRestoreTimeDialogOpen(true)}
                  disabled={!targetTime}
                >
                  Restore to Time
                </Button>
              </div>
            </div>
          )}
        </div>
      )}

      <RestoreBackupDialog
        open={restoreBackupDialogOpen}
        onOpenChange={setRestoreBackupDialogOpen}
        backupName={restoreBackupName}
        onRestored={() => {
          queryClient.invalidateQueries({ queryKey: ["backup-list"] });
          queryClient.invalidateQueries({ queryKey: ["backup-status"] });
        }}
      />

      <RestoreTimeDialog
        open={restoreTimeDialogOpen}
        onOpenChange={setRestoreTimeDialogOpen}
        targetTime={targetTime}
        onRestored={() => {
          queryClient.invalidateQueries({ queryKey: ["backup-list"] });
          queryClient.invalidateQueries({ queryKey: ["backup-status"] });
        }}
      />
    </div>
  );
}
