import { Input } from "./ui/input";
import { Switch } from "./ui/switch";
import type { ColumnInfo } from "../lib/schemas";
import { getTypeCategory } from "../lib/pg-types";

interface TypeAwareInputProps {
  column: ColumnInfo;
  value: string;
  onChange: (value: string) => void;
}

export function TypeAwareInput({ column, value, onChange }: TypeAwareInputProps) {
  const category = getTypeCategory(column.type);

  if (category === "Boolean") {
    return (
      <div className="flex items-center gap-2">
        <Switch
          checked={value === "true"}
          onCheckedChange={(checked: boolean) => onChange(checked ? "true" : "false")}
        />
        <span className="text-xs text-ink-muted">
          {value === "true" ? "TRUE" : "FALSE"}
        </span>
      </div>
    );
  }

  if (category === "Date/Time") {
    const isDateOnly = column.type === "DATE";
    const isTimeOnly = column.type === "TIME";

    if (isTimeOnly) {
      return (
        <Input
          type="time"
          step="1"
          value={value}
          onChange={(e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
        />
      );
    }

    return (
      <Input
        type={isDateOnly ? "date" : "datetime-local"}
        step="1"
        value={value}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
      />
    );
  }

  if (category === "Numeric") {
    const isFloat =
      column.type === "REAL" ||
      column.type === "DOUBLE PRECISION" ||
      column.type === "NUMERIC";
    return (
      <Input
        type="number"
        step={isFloat ? "any" : "1"}
        value={value}
        onChange={(e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
        placeholder={column.default ?? ""}
      />
    );
  }

  if (category === "JSON") {
    return (
      <textarea
        className="w-full rounded-md border border-hairline bg-surface-1 px-3 py-2 font-mono text-sm text-foreground placeholder:text-ink-muted focus:border-accent-blue/30 focus:ring-2 focus:ring-accent-blue/15 focus:outline-none"
        rows={3}
        value={value}
        onChange={(e: React.ChangeEvent<HTMLTextAreaElement>) => onChange(e.target.value)}
        placeholder={column.default ?? ""}
        spellCheck={false}
      />
    );
  }

  return (
    <Input
      value={value}
      onChange={(e: React.ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
      placeholder={column.default ?? ""}
    />
  );
}
