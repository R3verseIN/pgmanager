import { z } from "zod";
import { DatabasesResponseSchema, DatabaseSchema, TableInfoSchema } from "../lib/schemas";
import type { Database, TableInfo } from "../lib/schemas";
import { request } from "./request";

export async function fetchDatabases(showSystem?: boolean): Promise<Database[]> {
  const params = showSystem ? "?showSystem=true" : "";
  const data = await request<unknown>(`/databases${params}`);
  return DatabasesResponseSchema.parse(data);
}

export async function createDatabase(name: string): Promise<Database> {
  const data = await request<unknown>("/databases", {
    method: "POST",
    body: JSON.stringify({ name }),
  });
  return DatabaseSchema.parse(data);
}

export async function deleteDatabase(name: string): Promise<void> {
  await request<void>(`/databases/${encodeURIComponent(name)}`, {
    method: "DELETE",
  });
}

export async function fetchTables(dbName: string): Promise<TableInfo[]> {
  const data = await request<unknown>(
    `/databases/${encodeURIComponent(dbName)}/tables`
  );
  return z.array(TableInfoSchema).parse(data);
}

export async function createTable(
  dbName: string,
  name: string,
  columns: { name: string; type: string; nullable: boolean; isPrimaryKey: boolean }[]
): Promise<void> {
  await request<unknown>(`/databases/${encodeURIComponent(dbName)}/tables`, {
    method: "POST",
    body: JSON.stringify({ name, columns }),
  });
}
