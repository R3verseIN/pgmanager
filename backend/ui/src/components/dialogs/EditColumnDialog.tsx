import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { executeQuery } from "../../api/client";
import type { ColumnInfo } from "../../lib/schemas";
import { POSTGRESQL_TYPES, buildTypeString, parseTypeParams } from "../../lib/pg-types";
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

function quoteIdent(name: string): string {
  return `"${name.replace(/"/g, '""')}"`;
}

export default function EditColumnDialog({
  dbName,
  table,
  column,
  open,
  onOpenChange,
}: {
  dbName: string;
  table: string;
  column: ColumnInfo | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const parsed = column ? parseTypeParams(column.type) : { base: "TEXT" };

  const [newName, setNewName] = useState(column?.name ?? "");
  const [newType, setNewType] = useState(parsed.base);
  const [newLength, setNewLength] = useState(
    parsed.length !== undefined ? String(parsed.length) : ""
  );
  const [newPrecision, setNewPrecision] = useState(
    parsed.precision !== undefined ? String(parsed.precision) : ""
  );
  const [newScale, setNewScale] = useState(
    parsed.scale !== undefined ? String(parsed.scale) : ""
  );
  const [newNullable, setNewNullable] = useState(column?.nullable ?? true);
  const [newDefault, setNewDefault] = useState(column?.default ?? "");

  const typeDef = POSTGRESQL_TYPES.find((t) => t.value === newType);
  const hasLength = typeDef != null && "hasLength" in typeDef && typeDef.hasLength === true;
  const hasPrecision = typeDef != null && "hasPrecision" in typeDef && typeDef.hasPrecision === true;

  const editMutation = useMutation({
    mutationFn: async () => {
      if (!column) return;
      const stmts: string[] = [];

      if (newName !== column.name) {
        stmts.push(
          `ALTER TABLE ${quoteIdent(table)} RENAME COLUMN ${quoteIdent(column.name)} TO ${quoteIdent(newName)}`
        );
      }

      const typeStr = buildTypeString(newType, {
        length: newLength ? parseInt(newLength, 10) : undefined,
        precision: newPrecision ? parseInt(newPrecision, 10) : undefined,
        scale: newScale ? parseInt(newScale, 10) : undefined,
      });
      if (typeStr !== column.type) {
        stmts.push(
          `ALTER TABLE ${quoteIdent(table)} ALTER COLUMN ${quoteIdent(newName)} TYPE ${typeStr}`
        );
      }

      if (newNullable && !column.nullable) {
        stmts.push(
          `ALTER TABLE ${quoteIdent(table)} ALTER COLUMN ${quoteIdent(newName)} DROP NOT NULL`
        );
      } else if (!newNullable && column.nullable) {
        stmts.push(
          `ALTER TABLE ${quoteIdent(table)} ALTER COLUMN ${quoteIdent(newName)} SET NOT NULL`
        );
      }

      if (newDefault !== (column.default ?? "")) {
        if (newDefault) {
          stmts.push(
            `ALTER TABLE ${quoteIdent(table)} ALTER COLUMN ${quoteIdent(newName)} SET DEFAULT ${newDefault}`
          );
        } else {
          stmts.push(
            `ALTER TABLE ${quoteIdent(table)} ALTER COLUMN ${quoteIdent(newName)} DROP DEFAULT`
          );
        }
      }

      for (const stmt of stmts) {
        const result = await executeQuery(dbName, stmt);
        if (result.error) throw new Error(result.error);
      }
    },
    onSuccess: () => {
      toast.success("Column updated");
      onOpenChange(false);
      queryClient.invalidateQueries({ queryKey: ["columns", dbName, table] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit Column {column?.name}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label>Name</Label>
            <Input
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              autoFocus
            />
          </div>
          <div className="space-y-2">
            <Label>Type</Label>
            <Select value={newType} onValueChange={setNewType}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {Object.entries(
                  POSTGRESQL_TYPES.reduce(
                    (acc, t) => ({
                      ...acc,
                      [t.category]: [...(acc[t.category] || []), t],
                    }),
                    {} as Record<string, typeof POSTGRESQL_TYPES[number][]>
                  )
                ).map(([category, types]) => (
                  <div key={category}>
                    <div className="px-2 py-1 text-xs font-medium text-ink-muted">
                      {category}
                    </div>
                    {types.map((t) => (
                      <SelectItem key={t.value} value={t.value}>
                        {t.label}
                      </SelectItem>
                    ))}
                  </div>
                ))}
              </SelectContent>
            </Select>
          </div>
          {hasLength && (
            <div className="space-y-2">
              <Label>Length</Label>
              <Input
                placeholder="e.g. 255"
                value={newLength}
                onChange={(e) => setNewLength(e.target.value)}
                type="number"
                min="1"
              />
            </div>
          )}
          {hasPrecision && (
            <div className="flex gap-2">
              <div className="flex-1 space-y-2">
                <Label>Precision</Label>
                <Input
                  placeholder="e.g. 10"
                  value={newPrecision}
                  onChange={(e) => setNewPrecision(e.target.value)}
                  type="number"
                  min="1"
                />
              </div>
              <div className="flex-1 space-y-2">
                <Label>Scale</Label>
                <Input
                  placeholder="e.g. 2"
                  value={newScale}
                  onChange={(e) => setNewScale(e.target.value)}
                  type="number"
                  min="0"
                />
              </div>
            </div>
          )}
          <div className="space-y-2">
            <Label>Default</Label>
            <Input
              placeholder="NULL"
              value={newDefault}
              onChange={(e) => setNewDefault(e.target.value)}
            />
          </div>
          <div className="flex items-center gap-2">
            <input
              type="checkbox"
              id="edit-nullable"
              checked={newNullable}
              onChange={(e) => setNewNullable(e.target.checked)}
              className="size-4 rounded border-hairline"
            />
            <Label htmlFor="edit-nullable">Nullable</Label>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            disabled={!newName || editMutation.isPending}
            onClick={() => editMutation.mutate()}
          >
            Save Changes
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
