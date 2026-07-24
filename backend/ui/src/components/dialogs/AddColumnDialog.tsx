import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { addColumn } from "../../api/client";
import type { ColumnDef } from "../../lib/schemas";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../ui/select";

const TYPES = [
  "TEXT",
  "INTEGER",
  "BIGINT",
  "SMALLINT",
  "BOOLEAN",
  "DATE",
  "TIMESTAMP",
  "TIMESTAMPTZ",
  "NUMERIC",
  "REAL",
  "DOUBLE PRECISION",
  "UUID",
  "JSON",
  "JSONB",
  "BYTEA",
];

export default function AddColumnDialog({
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
  const [colName, setColName] = useState("");
  const [colType, setColType] = useState("TEXT");
  const [colNullable, setColNullable] = useState(true);
  const [colDefault, setColDefault] = useState("");

  const addColMutation = useMutation({
    mutationFn: () => {
      const col: ColumnDef = {
        name: colName,
        type: colType,
        nullable: colNullable,
        isPrimaryKey: false,
      };
      if (colDefault) col.default = colDefault;
      return addColumn(dbName, table, col);
    },
    onSuccess: () => {
      toast.success("Column added");
      onOpenChange(false);
      setColName("");
      setColType("TEXT");
      setColNullable(true);
      setColDefault("");
      queryClient.invalidateQueries({ queryKey: ["columns", dbName, table] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add Column to {table}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label>Name</Label>
            <Input
              value={colName}
              onChange={(e) => setColName(e.target.value)}
              autoFocus
            />
          </div>
          <div className="space-y-2">
            <Label>Type</Label>
            <Select value={colType} onValueChange={setColType}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {TYPES.map((t) => (
                  <SelectItem key={t} value={t}>
                    {t}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>Default</Label>
            <Input
              placeholder="NULL"
              value={colDefault}
              onChange={(e) => setColDefault(e.target.value)}
            />
          </div>
          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id="nullable"
              checked={colNullable}
              onChange={(e) => setColNullable(e.target.checked)}
            />
            <Label htmlFor="nullable">Nullable</Label>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            disabled={!colName || addColMutation.isPending}
            onClick={() => addColMutation.mutate()}
          >
            Add Column
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
