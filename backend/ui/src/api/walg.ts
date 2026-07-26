import { request } from "./request";

export interface WalgStatus {
  enabled: boolean;
  archiving: boolean;
  configured: boolean;
  s3Prefix: string;
  lastBackup: string;
  backupCount: number;
  totalSize: number;
  intervalSec: number;
  retentionDays: number;
}

export interface WalgBackup {
  name: string;
  time: string;
  walSegment: string;
  size: number;
  status: string;
}

export interface WalgConfig {
  s3Prefix: string;
  endpoint: string;
  region: string;
  forcePathStyle: string;
  interval: string;
  retentionDays: string;
}

export interface WalgVerifyResult {
  status: string;
  details: string;
}

export async function fetchWalgStatus(): Promise<WalgStatus> {
  return request<WalgStatus>("/walg/status");
}

export async function fetchWalgConfig(): Promise<WalgConfig> {
  return request<WalgConfig>("/walg/config");
}

export async function updateWalgConfig(config: {
  s3Prefix: string;
  endpoint?: string;
  region?: string;
  forcePathStyle?: boolean;
  interval?: number;
  retentionDays?: number;
}): Promise<{ status: string; message: string }> {
  return request("/walg/config", {
    method: "PUT",
    body: JSON.stringify(config),
  });
}

export async function fetchWalgBackups(): Promise<WalgBackup[]> {
  return request<WalgBackup[]>("/walg/backups");
}

export async function triggerWalgBackup(): Promise<{
  status: string;
  message: string;
  output: string;
}> {
  return request("/walg/backup", { method: "POST" });
}

export async function restoreWalgBackup(
  backupName: string,
  database: string
): Promise<{ status: string; database: string; backup: string }> {
  return request("/walg/restore", {
    method: "POST",
    body: JSON.stringify({ backupName, database }),
  });
}

export async function deleteWalgBackup(
  name: string
): Promise<{ status: string; backup: string }> {
  return request(`/walg/backup/${encodeURIComponent(name)}`, {
    method: "DELETE",
  });
}

export async function verifyWalgIntegrity(): Promise<WalgVerifyResult> {
  return request<WalgVerifyResult>("/walg/verify", { method: "POST" });
}

export async function cleanWalgGarbage(): Promise<{
  status: string;
  output: string;
}> {
  return request("/walg/garbage", { method: "DELETE" });
}

export async function testWalgConnection(): Promise<{
  status: string;
  message: string;
}> {
  return request("/walg/test-connection", { method: "POST" });
}
