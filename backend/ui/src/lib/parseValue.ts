export function parseValue(raw: string, type: string): unknown {
  const t = type.toUpperCase();
  if (raw === "" || raw === "NULL") return null;
  if (
    t.includes("INT") ||
    t.includes("NUMERIC") ||
    t === "REAL" ||
    t === "DOUBLE PRECISION"
  ) {
    const n = Number(raw);
    return isNaN(n) ? raw : n;
  }
  if (t === "BOOLEAN") return raw === "true" || raw === "1";
  return raw;
}
