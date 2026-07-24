import {
  AuthUserListResponseSchema,
  ResetPasswordResponseSchema,
} from "../lib/schemas";
import type { AuthUserListItem } from "../lib/schemas";
import { request } from "./request";

export async function fetchMe(): Promise<{
  username: string;
  role: "admin" | "dev" | "viewer";
}> {
  const data = await request<unknown>("/auth/me");
  return data as { username: string; role: "admin" | "dev" | "viewer" };
}

export async function login(
  username: string,
  password: string
): Promise<void> {
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

export async function setup(
  username: string,
  password: string
): Promise<void> {
  await request<unknown>("/auth/setup", {
    method: "POST",
    body: JSON.stringify({ username, password }),
  });
}

export async function fetchSetupCheck(): Promise<boolean> {
  const data = await request<{ needsSetup: boolean }>("/auth/setup-check");
  return data.needsSetup;
}

export async function changePassword(
  currentPassword: string,
  newPassword: string
): Promise<void> {
  await request<unknown>("/auth/password", {
    method: "PUT",
    body: JSON.stringify({
      current_password: currentPassword,
      new_password: newPassword,
    }),
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
    body: JSON.stringify({
      username,
      password: password || undefined,
      role,
      databases: databases || undefined,
    }),
  });
  return data;
}

export async function updateAuthUser(
  username: string,
  role: "admin" | "dev" | "viewer",
  databases?: string[]
): Promise<void> {
  await request<unknown>(
    `/auth/users/${encodeURIComponent(username)}`,
    {
      method: "PUT",
      body: JSON.stringify({
        role,
        databases: databases || undefined,
      }),
    }
  );
}

export async function deleteAuthUser(username: string): Promise<void> {
  await request<void>(
    `/auth/users/${encodeURIComponent(username)}`,
    {
      method: "DELETE",
    }
  );
}

export async function fetchAuthUsers(): Promise<AuthUserListItem[]> {
  const data = await request<unknown>("/auth/users");
  return AuthUserListResponseSchema.parse(data);
}

export async function resetAuthUserPassword(
  username: string,
  password?: string
): Promise<string> {
  const init: RequestInit = { method: "POST" };
  if (password) {
    init.body = JSON.stringify({ password });
  }
  const data = await request<unknown>(
    `/auth/users/${encodeURIComponent(username)}/reset-password`,
    init
  );
  const parsed = ResetPasswordResponseSchema.parse(data);
  return parsed.password;
}
