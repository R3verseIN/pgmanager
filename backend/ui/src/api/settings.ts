import { request } from "./request";

export async function fetchSettings(): Promise<Record<string, string>> {
  return request<Record<string, string>>("/settings");
}

export async function updateSettings(
  key: string,
  value: string,
): Promise<void> {
  await request("/settings", {
    method: "PUT",
    body: JSON.stringify({ [key]: value }),
  });
}
