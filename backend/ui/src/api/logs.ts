import { AuditLogResponseSchema } from "../lib/schemas";
import type { AuditLogResponse } from "../lib/schemas";
import { request } from "./request";

export async function fetchLogs(
  params: {
    username?: string;
    action?: string;
    database?: string;
    from?: string;
    to?: string;
    limit?: number;
    offset?: number;
  } = {}
): Promise<AuditLogResponse> {
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
