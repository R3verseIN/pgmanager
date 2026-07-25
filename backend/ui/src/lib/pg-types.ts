export type PostgresTypeItem = {
  label: string;
  value: string;
  category: string;
  hasLength?: boolean;
  hasPrecision?: boolean;
};

export const POSTGRESQL_TYPES: PostgresTypeItem[] = [
  { label: "SERIAL", value: "SERIAL", category: "Auto Increment" },
  { label: "BIGSERIAL", value: "BIGSERIAL", category: "Auto Increment" },
  { label: "SMALLSERIAL", value: "SMALLSERIAL", category: "Auto Increment" },
  { label: "INTEGER", value: "INTEGER", category: "Numeric" },
  { label: "BIGINT", value: "BIGINT", category: "Numeric" },
  { label: "SMALLINT", value: "SMALLINT", category: "Numeric" },
  { label: "NUMERIC", value: "NUMERIC", category: "Numeric", hasPrecision: true },
  { label: "REAL", value: "REAL", category: "Numeric" },
  { label: "DOUBLE PRECISION", value: "DOUBLE PRECISION", category: "Numeric" },
  { label: "MONEY", value: "MONEY", category: "Numeric" },
  { label: "TEXT", value: "TEXT", category: "String" },
  { label: "VARCHAR", value: "VARCHAR", category: "String", hasLength: true },
  { label: "CHAR", value: "CHAR", category: "String", hasLength: true },
  { label: "BOOLEAN", value: "BOOLEAN", category: "Boolean" },
  { label: "DATE", value: "DATE", category: "Date/Time" },
  { label: "TIME", value: "TIME", category: "Date/Time" },
  { label: "TIMESTAMP", value: "TIMESTAMP", category: "Date/Time" },
  { label: "TIMESTAMPTZ", value: "TIMESTAMPTZ", category: "Date/Time" },
  { label: "UUID", value: "UUID", category: "Special" },
  { label: "JSON", value: "JSON", category: "JSON" },
  { label: "JSONB", value: "JSONB", category: "JSON" },
  { label: "BYTEA", value: "BYTEA", category: "Binary" },
  { label: "INET", value: "INET", category: "Network" },
  { label: "CIDR", value: "CIDR", category: "Network" },
  { label: "MACADDR", value: "MACADDR", category: "Network" },
  { label: "XML", value: "XML", category: "Special" },
  { label: "ARRAY", value: "ARRAY", category: "Special" },
];

export type TypeParams = {
  length?: number | undefined;
  precision?: number | undefined;
  scale?: number | undefined;
};

export function buildTypeString(baseType: string, params?: TypeParams): string {
  if (params?.length != null && (baseType === "VARCHAR" || baseType === "CHAR")) {
    return `${baseType}(${params.length})`;
  }
  if (params?.precision != null && baseType === "NUMERIC") {
    if (params.scale != null && params.scale > 0) {
      return `NUMERIC(${params.precision},${params.scale})`;
    }
    return `NUMERIC(${params.precision})`;
  }
  return baseType;
}

export function parseTypeParams(type: string): {
  base: string;
  length?: number | undefined;
  precision?: number | undefined;
  scale?: number | undefined;
} {
  const match = type.match(/^(\w+)(?:\((\d+)(?:,(\d+))?\))?$/);
  if (!match) return { base: type };

  const base = match[1] ?? type;
  const param1 = match[2] != null ? parseInt(match[2], 10) : undefined;
  const param2 = match[3] != null ? parseInt(match[3], 10) : undefined;

  if (base === "VARCHAR" || base === "CHAR") {
    return { base, length: param1 };
  }
  if (base === "NUMERIC") {
    return { base, precision: param1, scale: param2 };
  }
  return { base: type };
}

export function getTypeCategory(type: string): string {
  const upper = type.toUpperCase();
  for (const t of POSTGRESQL_TYPES) {
    if (upper === t.value || upper.startsWith(t.value + "(")) {
      return t.category;
    }
  }
  return "Other";
}
