import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { fetchColumns, insertRow } from "../../api/client";
import { parseValue } from "../../lib/parseValue";
import type { ColumnInfo } from "../../lib/schemas";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../ui/dialog";

export default function InsertRowDialog({
  dbName,
  table,
  open,
  onOpenChange,
}: {
  dbName: string;
  table: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const { data: columns } = useQuery({
    queryKey: ["columns", dbName, table],
    queryFn: () => fetchColumns(dbName, table),
    enabled: open,
  });

  const [values, setValues] = useState<Record<string, string>>({});

  const insertMutation = useMutation({
    mutationFn: () => {
      const parsed: Record<string, unknown> = {};
      for (const col of columns || []) {
        const raw = values[col.name];
        if (raw === undefined || raw === "") {
          if (!col.nullable && !col.type.includes("SERIAL")) {
            throw new Error(`${col.name} is required`);
          }
          continue;
        }
        parsed[col.name] = parseValue(raw, col.type);
      }
      return insertRow(dbName, table, parsed);
    },
    onSuccess: () => {
      toast.success("Row inserted");
      onOpenChange(false);
      setValues({});
      queryClient.invalidateQueries({ queryKey: ["data", dbName, table] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[80vh] overflow-y-auto">
        <DialogHeader>
          <DialogTitle>Insert Row into {table}</DialogTitle>
        </DialogHeader>
        <div className="space-y-3 py-4">
          {columns
            ?.filter((c: ColumnInfo) => !c.type.includes("SERIAL"))
            .map((col: ColumnInfo) => (
              <div key={col.name} className="space-y-1">
                <Label>
                  {col.name}
                  <span className="ml-1 text-xs text-ink-muted">
                    ({col.type})
                  </span>
                  {col.nullable && (
                    <span className="ml-1 text-xs text-ink-muted">
                      nullable
                    </span>
                  )}
                </Label>
                <Input
                  placeholder={col.default ?? ""}
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
            onClick={() => insertMutation.mutate()}
            disabled={insertMutation.isPending}
          >
            Insert
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
