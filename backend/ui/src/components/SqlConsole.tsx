import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { toast } from "sonner";
import { executeQuery } from "../api/client";
import type { QueryResult } from "../lib/schemas";
import { Button } from "./ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "./ui/table";

export default function SqlConsole({ dbName }: { dbName: string }) {
  const [sql, setSql] = useState("");
  const [result, setResult] = useState<QueryResult | null>(null);

  const queryMutation = useMutation<QueryResult, Error, string>({
    mutationFn: (query: string) => executeQuery(dbName, query),
    onSuccess: (data) => {
      setResult(data);
      if (data.error) {
        toast.error(data.error);
      }
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <textarea
          className="w-full h-40 rounded-md border border-border bg-card px-3 py-2 text-sm font-mono text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring resize-y"
          placeholder="SELECT * FROM table_name;"
          value={sql}
          onChange={(e) => setSql(e.target.value)}
          onKeyDown={(e) => {
            if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
              if (sql.trim()) queryMutation.mutate(sql.trim());
            }
          }}
          spellCheck={false}
        />
        <div className="flex items-center gap-2">
          <Button
            onClick={() => {
              if (sql.trim()) queryMutation.mutate(sql.trim());
            }}
            disabled={!sql.trim() || queryMutation.isPending}
          >
            Execute
          </Button>
          {result && (
            <span className="text-sm text-muted-foreground">
              {result.duration}ms | {result.rowCount} row
              {result.rowCount !== 1 ? "s" : ""}
            </span>
          )}
        </div>
      </div>

      {result?.error && (
        <div className="rounded-md bg-destructive/10 border border-destructive/20 p-3 text-sm text-destructive">
          {result.error}
        </div>
      )}

      {result && !result.error && result.columns.length > 0 && (
        <div className="rounded-md border border-border overflow-x-auto">
          <Table>
            <TableHeader>
              <TableRow>
                {result.columns.map((col: string) => (
                  <TableHead key={col}>{col}</TableHead>
                ))}
              </TableRow>
            </TableHeader>
            <TableBody>
              {result.rows.map((row: unknown[], i: number) => (
                <TableRow key={i}>
                  {row.map((cell: unknown, j: number) => (
                    <TableCell key={j} className="font-mono text-xs">
                      {cell === null ? (
                        <span className="text-muted-foreground italic">NULL</span>
                      ) : (
                        String(cell)
                      )}
                    </TableCell>
                  ))}
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  );
}
