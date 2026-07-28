import { request } from "./request";

export interface BackupSettings {
  enabled: boolean;
  archive_timeout: number;
  retention_days: number;
  full_backup_day: number;
  backup_hour: number;
}

export interface BackupStatus {
  settings: BackupSettings;
  info: any;
  configured: boolean;
}

export async function fetchBackupStatus(): Promise<BackupStatus> {
  return request<BackupStatus>("/pgbackrest/status");
}

export async function fetchBackups(): Promise<any[]> {
  return request<any[]>("/pgbackrest/list");
}

export async function triggerBackup(type: string): Promise<{ message: string }> {
  return request("/pgbackrest/trigger", {
    method: "POST",
    body: JSON.stringify({ type }),
  });
}

export async function restoreS3Backup(database: string, backup_name?: string, target_time?: string): Promise<{ message: string }> {
  return request("/pgbackrest/restore", {
    method: "POST",
    body: JSON.stringify({ database, backup_name, target_time }),
  });
}

export async function updateBackupSettings(settings: BackupSettings): Promise<{ message: string }> {
  return request("/pgbackrest/settings", {
    method: "POST",
    body: JSON.stringify(settings),
  });
}

export async function testBackupConnection(): Promise<{ message: string }> {
  return request("/pgbackrest/test-connection", { method: "POST" });
}
