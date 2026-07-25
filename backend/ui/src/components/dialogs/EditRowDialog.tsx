import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { fetchColumns, updateRow } from "../../api/client";
import { parseValue } from "../../lib/parseValue";
import type { ColumnInfo, WhereCondition } from "../../lib/schemas";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import { Badge } from "../ui/badge";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../ui/dialog";

export default function EditRowDialog({
  dbName,
  table,
  open,
  onOpenChange,
  row,
}: {
  dbName: string;
  table: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  row: unknown[] | null;
}) {
  const queryClient = useQueryClient();
  const { data: columns } = useQuery({
    queryKey: ["columns", dbName, table],
    queryFn: () => fetchColumns(dbName, table),
    enabled: open,
  });

  const [values, setValues] = useState<Record<string, string>>({});

  const initializedKey = row ? JSON.stringify(row) : "";
  const [lastKey, setLastKey] = useState("");
  if (initializedKey !== lastKey && row && columns) {
    const newValues: Record<string, string> = {};
    columns.forEach((col: ColumnInfo, i: number) => {
      newValues[col.name] = row[i] === null ? "" : String(row[i]);
    });
    setValues(newValues);
    setLastKey(initializedKey);
  }

  const updateMutation = useMutation({
    mutationFn: () => {
      if (!row || !columns) throw new Error("No row selected");
      const parsed: Record<string, unknown> = {};
      for (const col of columns) {
        const raw = values[col.name] ?? "";
        parsed[col.name] = raw === "" ? null : parseValue(raw, col.type);
      }
      const where: WhereCondition[] = columns.map((col: ColumnInfo, i: number) => {
        const val = row[i];
        if (val === null) return { column: col.name, operator: "IS NULL" as const };
        return { column: col.name, operator: "=" as const, value: val };
      });
      return updateRow(dbName, table, parsed, where);
    },
    onSuccess: () => {
      toast.success("Row updated");
      onOpenChange(false);
      queryClient.invalidateQueries({ queryKey: ["data", dbName, table] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[80vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Edit Row in {table}</DialogTitle>
          <DialogDescription>
            Editing by primary key. WHERE conditions will match the original row
            values.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3 py-4">
          {columns?.map((col: ColumnInfo) => (
            <div key={col.name} className="space-y-1">
              <Label>
                {col.name}
                <span className="ml-1 text-xs text-ink-muted">
                  ({col.type})
                </span>
                {col.isPrimaryKey && (
                  <Badge variant="default" className="ml-1 text-xs">
                    PK
                  </Badge>
                )}
              </Label>
              <Input
                value={values[col.name] ?? ""}
                onChange={(e) =>
                  setValues({ ...values, [col.name]: e.target.value })
                }
              />
            </div>
          ))}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={() => updateMutation.mutate()}
            disabled={updateMutation.isPending}
          >
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
