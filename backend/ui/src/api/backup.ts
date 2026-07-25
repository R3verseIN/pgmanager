import { request, ApiError } from "./request";

const BASE_URL = "/api";

export interface BackupDatabase {
  name: string;
}

export interface BackupTable {
  schema: string;
  name: string;
}

export interface BackupTableList {
  database: string;
  tables: BackupTable[];
}

export interface BackupInspectResult {
  database: string;
  format: string;
  tables: BackupTable[];
  size: number;
}

export interface BackupRestoreResult {
  success: boolean;
  database: string;
  message: string;
}

export async function fetchBackupDatabases(): Promise<BackupDatabase[]> {
  return request<BackupDatabase[]>("/backup/databases");
}

export async function fetchBackupTables(
  database: string
): Promise<BackupTableList> {
  return request<BackupTableList>(`/backup/tables?db=${encodeURIComponent(database)}`);
}

export async function backupDatabase(
  database: string,
  tables?: string[]
): Promise<Blob> {
  const response = await fetch(`${BASE_URL}/backup/create`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    credentials: "include",
    body: JSON.stringify({ database, tables }),
  });

  if (!response.ok) {
    const body: unknown = await response.json().catch(() => null);
    const parsed = body && typeof body === "object" && "error" in body
      ? body as { error: string }
      : null;
    throw new ApiError(parsed?.error || `HTTP ${response.status}`);
  }

  return response.blob();
}

export async function inspectBackup(file: File): Promise<BackupInspectResult> {
  const formData = new FormData();
  formData.append("file", file);

  const response = await fetch(`${BASE_URL}/backup/inspect`, {
    method: "POST",
    credentials: "include",
    body: formData,
  });

  if (!response.ok) {
    const body: unknown = await response.json().catch(() => null);
    const parsed = body && typeof body === "object" && "error" in body
      ? body as { error: string }
      : null;
    throw new ApiError(parsed?.error || `HTTP ${response.status}`);
  }

  return response.json();
}

export async function restoreBackup(
  file: File,
  database: string,
  dropFirst: boolean
): Promise<BackupRestoreResult> {
  const formData = new FormData();
  formData.append("file", file);
  formData.append("database", database);
  formData.append("dropFirst", String(dropFirst));

  const response = await fetch(`${BASE_URL}/backup/restore`, {
    method: "POST",
    credentials: "include",
    body: formData,
  });

  if (!response.ok) {
    const body: unknown = await response.json().catch(() => null);
    const parsed = body && typeof body === "object" && "error" in body
      ? body as { error: string }
      : null;
    throw new ApiError(parsed?.error || `HTTP ${response.status}`);
  }

  return response.json();
}

export function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}
