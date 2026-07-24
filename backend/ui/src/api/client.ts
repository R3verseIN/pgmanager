import {
  DatabasesResponseSchema,
  ErrorResponseSchema,
  UsersResponseSchema,
  CreateUserResponseSchema,
  DatabaseSchema,
} from "../lib/schemas";
import type { Database, User } from "../lib/schemas";

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

export async function fetchMe(): Promise<{ username: string; role: "admin" | "viewer" }> {
  const data = await request<unknown>("/auth/me");
  return data as { username: string; role: "admin" | "viewer" };
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

export async function createAuthUser(username: string, password: string, role: "admin" | "viewer"): Promise<void> {
  await request<unknown>("/auth/users", {
    method: "POST",
    body: JSON.stringify({ username, password, role }),
  });
}

export async function updateAuthUser(username: string, role: "admin" | "viewer"): Promise<void> {
  await request<unknown>(`/auth/users/${encodeURIComponent(username)}`, {
    method: "PUT",
    body: JSON.stringify({ role }),
  });
}

export async function deleteAuthUser(username: string): Promise<void> {
  await request<void>(`/auth/users/${encodeURIComponent(username)}`, {
    method: "DELETE",
  });
}
