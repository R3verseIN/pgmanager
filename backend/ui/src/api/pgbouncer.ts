import { request } from "./request";

export interface PgBouncerDatabase {
  databaseName: string;
  allowed: boolean;
  createdAt?: string;
  updatedAt?: string;
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
