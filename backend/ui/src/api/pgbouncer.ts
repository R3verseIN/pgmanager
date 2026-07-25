import { request } from "./request";

export interface PgBouncerDatabase {
  databaseName: string;
  allowed: boolean;
  createdAt?: string;
  updatedAt?: string;
}

export interface PgBouncerConfig {
  poolMode: string;
  defaultPoolSize: number;
  maxClientConn: number;
}

export async function fetchPgBouncerDatabases(): Promise<PgBouncerDatabase[]> {
  return request<PgBouncerDatabase[]>("/pgbouncer/databases");
}

export async function togglePgBouncerDatabase(
  databaseName: string,
  allowed: boolean
): Promise<PgBouncerDatabase> {
  return request<PgBouncerDatabase>(`/pgbouncer/databases/${databaseName}`, {
    method: "PUT",
    body: JSON.stringify({ allowed }),
  });
}

export async function fetchPgBouncerConfig(): Promise<PgBouncerConfig> {
  return request<PgBouncerConfig>("/pgbouncer/config");
}

export async function updatePgBouncerConfig(
  config: PgBouncerConfig
): Promise<PgBouncerConfig> {
  return request<PgBouncerConfig>("/pgbouncer/config", {
    method: "PUT",
    body: JSON.stringify(config),
  });
}
