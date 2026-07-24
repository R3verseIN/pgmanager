import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { deleteRow } from "../../api/client";
import type { WhereCondition } from "../../lib/schemas";
import { Button } from "../ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../ui/dialog";

export default function DeleteRowConfirm({
  dbName,
  table,
  where,
  onOpenChange,
}: {
  dbName: string;
  table: string;
  where: WhereCondition[] | null;
  onOpenChange: (open: boolean) => void;
}) {
  const queryClient = useQueryClient();

  const deleteMutation = useMutation({
    mutationFn: () => {
      if (!where) throw new Error("No row selected");
      return deleteRow(dbName, table, where);
    },
    onSuccess: () => {
      toast.success("Row deleted");
      onOpenChange(false);
      queryClient.invalidateQueries({ queryKey: ["data", dbName, table] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <Dialog open={where !== null} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete Row</DialogTitle>
          <DialogDescription>
            Are you sure you want to delete this row from {table}?
          </DialogDescription>
        </DialogHeader>
        {where && (
          <div className="py-4 space-y-2">
            <p className="text-sm text-ink-muted">WHERE conditions:</p>
            <div className="rounded-[10px] bg-surface-2 p-2 font-mono text-xs space-y-1">
              {where.map((w, i) => (
                <div key={i}>
                  {w.column} {w.operator}{" "}
                  {w.value !== undefined ? String(w.value) : ""}
                </div>
              ))}
            </div>
          </div>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            disabled={deleteMutation.isPending}
            onClick={() => deleteMutation.mutate()}
          >
            Delete
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
