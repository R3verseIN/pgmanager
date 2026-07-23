import { DatabaseSchema, DatabasesResponseSchema, ErrorResponseSchema } from "../lib/schemas";
import type { Database } from "../lib/schemas";

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
