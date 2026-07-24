import { z } from "zod";
import {
  DatabasesResponseSchema,
  ErrorResponseSchema,
  UsersResponseSchema,
  CreateUserResponseSchema,
  DatabaseSchema,
  AuthUserListResponseSchema,
  ResetPasswordResponseSchema,
  TableInfoSchema,
  ColumnInfoSchema,
  DataResultSchema,
  QueryResultSchema,
  AuditLogResponseSchema,
} from "../lib/schemas";
import type { Database, User, AuthUserListItem, TableInfo, ColumnInfo, DataResult, WhereCondition, ColumnDef, QueryResult, AuditLogResponse } from "../lib/schemas";

const BASE_URL = "/api";

class ApiError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ApiError";
  }
}

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${BASE_URL}${url}`, {
    headers: { "Content-Type": "application/json" },
    credentials: "include",
    ...options,
  });

  if (!response.ok) {
    const body: unknown = await response.json().catch(() => null);
    const parsed = ErrorResponseSchema.safeParse(body);
    throw new ApiError(parsed.success ? parsed.data.error : `HTTP ${response.status}`);
  }

  if (response.status === 204) {
    return undefined as unknown as T;
  }

  const body: unknown = await response.json();
  return body as T;
}

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

export async function fetchUsers(): Promise<User[]> {
  const data = await request<unknown>("/users");
  return UsersResponseSchema.parse(data);
}

export interface CreateUserResult {
  username: string;
  password: string;
  databases: string[];
  connectionString: string;
  access: "read" | "write" | "ddl" | "full";
  createdAt: string;
}

export async function createUser(
  username: string,
  databases: string[],
  access: "read" | "write" | "ddl" | "full",
  password?: string,
): Promise<CreateUserResult> {
  const data = await request<unknown>("/users", {
    method: "POST",
    body: JSON.stringify({ username, databases, access, password: password || undefined }),
  });
  return CreateUserResponseSchema.parse(data);
}

export async function updateUser(
  username: string,
  opts: { password?: string; access?: "read" | "write" | "ddl" | "full" },
): Promise<void> {
  await request<void>(`/users/${encodeURIComponent(username)}`, {
    method: "PUT",
    body: JSON.stringify(opts),
  });
}

export async function addUserDatabase(username: string, database: string): Promise<void> {
  await request<void>(`/users/${encodeURIComponent(username)}/databases`, {
    method: "POST",
    body: JSON.stringify({ database }),
  });
}

export async function removeUserDatabase(username: string, database: string): Promise<void> {
  await request<void>(`/users/${encodeURIComponent(username)}/databases/${encodeURIComponent(database)}`, {
    method: "DELETE",
  });
}

export async function deleteUser(username: string): Promise<void> {
  await request<void>(`/users/${encodeURIComponent(username)}`, {
    method: "DELETE",
  });
}

export async function fetchMe(): Promise<{ username: string; role: "admin" | "dev" | "viewer" }> {
  const data = await request<unknown>("/auth/me");
  return data as { username: string; role: "admin" | "dev" | "viewer" };
}

export async function login(username: string, password: string): Promise<void> {
  await request<unknown>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ username, password }),
  });
}

export async function logout(): Promise<void> {
  await request<unknown>("/auth/logout", {
    method: "POST",
  });
}

export async function setup(username: string, password: string): Promise<void> {
  await request<unknown>("/auth/setup", {
    method: "POST",
    body: JSON.stringify({ username, password }),
  });
}

export async function fetchSetupCheck(): Promise<boolean> {
  const data = await request<{ needsSetup: boolean }>("/auth/setup-check");
  return data.needsSetup;
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  await request<unknown>("/auth/password", {
    method: "PUT",
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  });
}

export interface CreateAuthUserResult {
  status: string;
  username: string;
  password: string;
}

export async function createAuthUser(
  username: string,
  password: string,
  role: "admin" | "dev" | "viewer",
  databases?: string[]
): Promise<CreateAuthUserResult> {
  const data = await request<CreateAuthUserResult>("/auth/users", {
    method: "POST",
    body: JSON.stringify({ username, password: password || undefined, role, databases: databases || undefined }),
  });
  return data;
}

export async function updateAuthUser(username: string, role: "admin" | "dev" | "viewer", databases?: string[]): Promise<void> {
  await request<unknown>(`/auth/users/${encodeURIComponent(username)}`, {
    method: "PUT",
    body: JSON.stringify({ role, databases: databases || undefined }),
  });
}

export async function deleteAuthUser(username: string): Promise<void> {
  await request<void>(`/auth/users/${encodeURIComponent(username)}`, {
    method: "DELETE",
  });
}

export async function fetchAuthUsers(): Promise<AuthUserListItem[]> {
  const data = await request<unknown>("/auth/users");
  return AuthUserListResponseSchema.parse(data);
}

export async function resetAuthUserPassword(username: string, password?: string): Promise<string> {
  const init: RequestInit = { method: "POST" };
  if (password) {
    init.body = JSON.stringify({ password });
  }
  const data = await request<unknown>(`/auth/users/${encodeURIComponent(username)}/reset-password`, init);
  const parsed = ResetPasswordResponseSchema.parse(data);
  return parsed.password;
}

export async function fetchTables(dbName: string): Promise<TableInfo[]> {
  const data = await request<unknown>(`/databases/${encodeURIComponent(dbName)}/tables`);
  return z.array(TableInfoSchema).parse(data);
}

export async function fetchColumns(dbName: string, table: string): Promise<ColumnInfo[]> {
  const data = await request<unknown>(`/databases/${encodeURIComponent(dbName)}/columns/${encodeURIComponent(table)}`);
  return z.array(ColumnInfoSchema).parse(data);
}

export async function fetchData(dbName: string, table: string, params: { limit?: number; offset?: number; sort?: string; order?: string } = {}): Promise<DataResult> {
  const searchParams = new URLSearchParams();
  if (params.limit) searchParams.set("limit", String(params.limit));
  if (params.offset) searchParams.set("offset", String(params.offset));
  if (params.sort) searchParams.set("sort", params.sort);
  if (params.order) searchParams.set("order", params.order);
  const qs = searchParams.toString();
  const data = await request<unknown>(`/databases/${encodeURIComponent(dbName)}/data/${encodeURIComponent(table)}${qs ? "?" + qs : ""}`);
  return DataResultSchema.parse(data);
}

export async function insertRow(dbName: string, table: string, values: Record<string, unknown>): Promise<void> {
  await request<unknown>(`/databases/${encodeURIComponent(dbName)}/data/${encodeURIComponent(table)}`, {
    method: "POST",
    body: JSON.stringify({ values }),
  });
}

export async function updateRow(dbName: string, table: string, values: Record<string, unknown>, where: WhereCondition[]): Promise<void> {
  await request<unknown>(`/databases/${encodeURIComponent(dbName)}/data/${encodeURIComponent(table)}`, {
    method: "PUT",
    body: JSON.stringify({ values, where }),
  });
}

export async function deleteRow(dbName: string, table: string, where: WhereCondition[]): Promise<void> {
  await request<unknown>(`/databases/${encodeURIComponent(dbName)}/data/${encodeURIComponent(table)}`, {
    method: "DELETE",
    body: JSON.stringify({ where }),
  });
}

export async function createTable(dbName: string, name: string, columns: ColumnDef[]): Promise<void> {
  await request<unknown>(`/databases/${encodeURIComponent(dbName)}/tables`, {
    method: "POST",
    body: JSON.stringify({ name, columns }),
  });
}

export async function addColumn(dbName: string, table: string, column: ColumnDef): Promise<void> {
  await request<unknown>(`/databases/${encodeURIComponent(dbName)}/tables/${encodeURIComponent(table)}/columns`, {
    method: "POST",
    body: JSON.stringify(column),
  });
}

export async function dropColumn(dbName: string, table: string, column: string): Promise<void> {
  await request<void>(`/databases/${encodeURIComponent(dbName)}/tables/${encodeURIComponent(table)}/columns/${encodeURIComponent(column)}`, {
    method: "DELETE",
  });
}

export async function executeQuery(dbName: string, sql: string): Promise<QueryResult> {
  const data = await request<unknown>(`/databases/${encodeURIComponent(dbName)}/query`, {
    method: "POST",
    body: JSON.stringify({ sql }),
  });
  return QueryResultSchema.parse(data);
}

export async function fetchLogs(params: { username?: string; action?: string; database?: string; from?: string; to?: string; limit?: number; offset?: number } = {}): Promise<AuditLogResponse> {
  const searchParams = new URLSearchParams();
  if (params.username) searchParams.set("username", params.username);
  if (params.action) searchParams.set("action", params.action);
  if (params.database) searchParams.set("database", params.database);
  if (params.from) searchParams.set("from", params.from);
  if (params.to) searchParams.set("to", params.to);
  if (params.limit) searchParams.set("limit", String(params.limit));
  if (params.offset) searchParams.set("offset", String(params.offset));
  const qs = searchParams.toString();
  const data = await request<unknown>(`/logs${qs ? "?" + qs : ""}`);
  return AuditLogResponseSchema.parse(data);
}
