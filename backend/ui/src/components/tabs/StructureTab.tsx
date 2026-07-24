import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Plus, Trash2 } from "lucide-react";
import { fetchColumns, dropColumn } from "../../api/client";
import type { ColumnInfo } from "../../lib/schemas";
import { Button } from "../ui/button";
import { Badge } from "../ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../ui/table";

export default function StructureTab({
  dbName,
  table,
  canWrite,
  onAddColumn,
}: {
  dbName: string;
  table: string;
  canWrite: boolean;
  onAddColumn: () => void;
}) {
  const queryClient = useQueryClient();

  const { data: columns, isLoading } = useQuery({
    queryKey: ["columns", dbName, table],
    queryFn: () => fetchColumns(dbName, table),
  });

  const dropColMutation = useMutation({
    mutationFn: (col: string) => dropColumn(dbName, table, col),
    onSuccess: () => {
      toast.success("Column dropped");
      queryClient.invalidateQueries({ queryKey: ["columns", dbName, table] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <div className="space-y-4">
      {canWrite && (
        <div className="flex items-center gap-2">
          <Button onClick={onAddColumn}>
            <Plus className="mr-2 size-4" />
            Add Column
          </Button>
        </div>
      )}

      {isLoading ? (
        <div className="py-8 text-center text-ink-muted">Loading...</div>
      ) : !columns?.length ? (
        <div className="py-8 text-center text-ink-muted">No columns.</div>
      ) : (
        <div className="overflow-x-auto rounded-[10px] border border-hairline bg-surface-1">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Type</TableHead>
                <TableHead>Nullable</TableHead>
                <TableHead>Default</TableHead>
                <TableHead>PK</TableHead>
                {canWrite && <TableHead className="w-12"></TableHead>}
              </TableRow>
            </TableHeader>
            <TableBody>
              {columns.map((col: ColumnInfo) => (
                <TableRow key={col.name}>
                  <TableCell className="font-medium">{col.name}</TableCell>
                  <TableCell>
                    <Badge variant="outline" className="text-xs">
                      {col.type}
                    </Badge>
                  </TableCell>
                  <TableCell>{col.nullable ? "YES" : "NO"}</TableCell>
                  <TableCell className="font-mono text-xs">
                    {col.default ?? <span className="text-ink-muted">—</span>}
                  </TableCell>
                  <TableCell>
                    {col.isPrimaryKey && (
                      <Badge variant="default" className="text-xs">
                        PK
                      </Badge>
                    )}
                  </TableCell>
                  {canWrite && (
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="size-7 text-destructive hover:bg-destructive/10"
                        disabled={col.isPrimaryKey}
                        onClick={() => {
                          if (confirm(`Drop column "${col.name}"?`)) {
                            dropColMutation.mutate(col.name);
                          }
                        }}
                      >
                        <Trash2 className="size-3.5" />
                      </Button>
                    </TableCell>
                  )}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
