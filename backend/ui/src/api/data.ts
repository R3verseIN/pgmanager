import { z } from "zod";
import {
  ColumnInfoSchema,
  DataResultSchema,
  QueryResultSchema,
} from "../lib/schemas";
import type {
  ColumnInfo,
  DataResult,
  WhereCondition,
  ColumnDef,
  QueryResult,
} from "../lib/schemas";
import { request } from "./request";

export async function fetchColumns(
  dbName: string,
  table: string
): Promise<ColumnInfo[]> {
  const data = await request<unknown>(
    `/databases/${encodeURIComponent(dbName)}/columns/${encodeURIComponent(table)}`
  );
  return z.array(ColumnInfoSchema).parse(data);
}

export async function fetchData(
  dbName: string,
  table: string,
  params: {
    limit?: number;
    offset?: number;
    sort?: string;
    order?: string;
  } = {}
): Promise<DataResult> {
  const searchParams = new URLSearchParams();
  if (params.limit) searchParams.set("limit", String(params.limit));
  if (params.offset) searchParams.set("offset", String(params.offset));
  if (params.sort) searchParams.set("sort", params.sort);
  if (params.order) searchParams.set("order", params.order);
  const qs = searchParams.toString();
  const data = await request<unknown>(
    `/databases/${encodeURIComponent(dbName)}/data/${encodeURIComponent(table)}${qs ? "?" + qs : ""}`
  );
  return DataResultSchema.parse(data);
}

export async function insertRow(
  dbName: string,
  table: string,
  values: Record<string, unknown>
): Promise<void> {
  await request<unknown>(
    `/databases/${encodeURIComponent(dbName)}/data/${encodeURIComponent(table)}`,
    {
      method: "POST",
      body: JSON.stringify({ values }),
    }
  );
}

export async function updateRow(
  dbName: string,
  table: string,
  values: Record<string, unknown>,
  where: WhereCondition[]
): Promise<void> {
  await request<unknown>(
    `/databases/${encodeURIComponent(dbName)}/data/${encodeURIComponent(table)}`,
    {
      method: "PUT",
      body: JSON.stringify({ values, where }),
    }
  );
}

export async function deleteRow(
  dbName: string,
  table: string,
  where: WhereCondition[]
): Promise<void> {
  await request<unknown>(
    `/databases/${encodeURIComponent(dbName)}/data/${encodeURIComponent(table)}`,
    {
      method: "DELETE",
      body: JSON.stringify({ where }),
    }
  );
}

export async function addColumn(
  dbName: string,
  table: string,
  column: ColumnDef
): Promise<void> {
  await request<unknown>(
    `/databases/${encodeURIComponent(dbName)}/tables/${encodeURIComponent(table)}/columns`,
    {
      method: "POST",
      body: JSON.stringify(column),
    }
  );
}

export async function dropColumn(
  dbName: string,
  table: string,
  column: string
): Promise<void> {
  await request<void>(
    `/databases/${encodeURIComponent(dbName)}/tables/${encodeURIComponent(table)}/columns/${encodeURIComponent(column)}`,
    {
      method: "DELETE",
    }
  );
}

export async function executeQuery(
  dbName: string,
  sql: string
): Promise<QueryResult> {
  const data = await request<unknown>(
    `/databases/${encodeURIComponent(dbName)}/query`,
    {
      method: "POST",
      body: JSON.stringify({ sql }),
    }
  );
  return QueryResultSchema.parse(data);
}
