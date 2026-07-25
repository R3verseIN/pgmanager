import { ErrorResponseSchema } from "../lib/schemas";

const BASE_URL = "/api";

export class ApiError extends Error {
  status: number;
  tables?: string[] | undefined;
  constructor(message: string, status?: number, tables?: string[]) {
    super(message);
    this.name = "ApiError";
    this.status = status ?? 0;
    this.tables = tables;
  }
}

export async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const response = await fetch(`${BASE_URL}${url}`, {
    headers: { "Content-Type": "application/json" },
    credentials: "include",
    ...options,
  });

  if (!response.ok) {
    const body: unknown = await response.json().catch(() => null);
    const parsed = ErrorResponseSchema.safeParse(body);
    if (parsed.success) {
      throw new ApiError(parsed.data.error, response.status);
    }
    throw new ApiError(`HTTP ${response.status}`, response.status);
  }

  if (response.status === 204) {
    return undefined as unknown as T;
  }

  const body: unknown = await response.json();
  return body as T;
}
