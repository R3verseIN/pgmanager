import { UsersResponseSchema, CreateUserResponseSchema } from "../lib/schemas";
import type { User } from "../lib/schemas";
import { request } from "./request";

export interface CreateUserResult {
  username: string;
  password: string;
  databases: string[];
  connectionString: string;
  access: "read" | "write" | "ddl" | "full";
  createdAt: string;
}

export async function fetchUsers(): Promise<User[]> {
  const data = await request<unknown>("/users");
  return UsersResponseSchema.parse(data);
}

export async function createUser(
  username: string,
  databases: string[],
  access: "read" | "write" | "ddl" | "full",
  password?: string,
  allowedIps?: string[]
): Promise<CreateUserResult> {
  const data = await request<unknown>("/users", {
    method: "POST",
    body: JSON.stringify({
      username,
      databases,
      access,
      password: password || undefined,
      allowedIps: allowedIps && allowedIps.length > 0 ? allowedIps : undefined,
    }),
  });
  return CreateUserResponseSchema.parse(data);
}

export async function updateUser(
  username: string,
  opts: {
    password?: string;
    access?: "read" | "write" | "ddl" | "full";
    generatePassword?: boolean;
    allowedIps?: string[];
    databases?: string[];
  }
): Promise<{ password?: string }> {
  const data = await request<{ password?: string }>(
    `/users/${encodeURIComponent(username)}`,
    {
      method: "PUT",
      body: JSON.stringify(opts),
    }
  );
  return data;
}

export async function addUserDatabase(
  username: string,
  database: string
): Promise<void> {
  await request<void>(
    `/users/${encodeURIComponent(username)}/databases`,
    {
      method: "POST",
      body: JSON.stringify({ database }),
    }
  );
}

export async function removeUserDatabase(
  username: string,
  database: string
): Promise<void> {
  await request<void>(
    `/users/${encodeURIComponent(username)}/databases/${encodeURIComponent(database)}`,
    {
      method: "DELETE",
    }
  );
}

export async function deleteUser(username: string): Promise<void> {
  await request<void>(`/users/${encodeURIComponent(username)}`, {
    method: "DELETE",
  });
}
