import { useQuery } from "@tanstack/react-query";
import { Plus, RefreshCw, ChevronLeft, ChevronRight, Edit, Trash2, ChevronUp, ChevronDown } from "lucide-react";
import { fetchData } from "../../api/client";
import type { WhereCondition } from "../../lib/schemas";
import { Button } from "../ui/button";

const PAGE_SIZE = 100;

export default function DataTab({
  dbName,
  table,
  page,
  setPage,
  sortCol,
  setSortCol,
  sortDir,
  setSortDir,
  canWrite,
  onInsert,
  onEdit,
  onDelete,
}: {
  dbName: string;
  table: string;
  page: number;
  setPage: (p: number) => void;
  sortCol: string;
  setSortCol: (c: string) => void;
  sortDir: string;
  setSortDir: (d: "asc" | "desc") => void;
  canWrite: boolean;
  onInsert: () => void;
  onEdit: (row: unknown[]) => void;
  onDelete: (where: WhereCondition[]) => void;
}) {
  const { data, isLoading, refetch } = useQuery({
    queryKey: ["data", dbName, table, page, sortCol, sortDir],
    queryFn: () =>
      fetchData(dbName, table, {
        limit: PAGE_SIZE,
        offset: page * PAGE_SIZE,
        ...(sortCol ? { sort: sortCol, order: sortDir } : { order: sortDir }),
      }),
  });

  const totalPages = data ? Math.ceil(data.total / PAGE_SIZE) : 0;

  function handleSort(col: string) {
    if (sortCol === col) {
      setSortDir(sortDir === "asc" ? "desc" : "asc");
    } else {
      setSortCol(col);
      setSortDir("asc");
    }
    setPage(0);
  }

  function buildRowWhere(row: unknown[], columns: string[]): WhereCondition[] {
    const where: WhereCondition[] = [];
    for (let i = 0; i < columns.length; i++) {
      const colName = columns[i];
      if (!colName) continue;
      const val = row[i];
      if (val === null || val === undefined) {
        where.push({ column: colName, operator: "IS NULL" });
      } else {
        where.push({ column: colName, operator: "=", value: val });
      }
    }
    return where;
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-2">
        {canWrite && (
          <Button onClick={onInsert}>
            <Plus className="mr-2 size-4" />
            Insert Row
          </Button>
        )}
        <Button variant="outline" onClick={() => refetch()}>
          <RefreshCw className="mr-2 size-4" />
          Refresh
        </Button>
      </div>

      {isLoading ? (
        <div className="py-8 text-center text-ink-muted">Loading...</div>
      ) : !data?.columns?.length ? (
        <div className="py-8 text-center text-ink-muted">No data.</div>
      ) : (
        <div className="overflow-x-auto rounded-[10px] border border-hairline bg-surface-1">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-hairline bg-surface-2/50">
                {data.columns.map((col: string) => (
                  <th
                    key={col}
                    className="cursor-pointer px-3 py-2 text-left font-medium text-ink-muted select-none hover:text-foreground"
                    onClick={() => handleSort(col)}
                  >
                    <div className="flex items-center gap-1">
                      {col}
                      {sortCol === col &&
                        (sortDir === "asc" ? (
                          <ChevronUp className="size-3" />
                        ) : (
                          <ChevronDown className="size-3" />
                        ))}
                    </div>
                  </th>
                ))}
                {canWrite && <th className="w-20"></th>}
              </tr>
            </thead>
            <tbody>
              {data.rows.map((row: unknown[], i: number) => (
                <tr
                  key={i}
                  className="border-b border-hairline last:border-0 hover:bg-surface-2/30"
                >
                  {row.map((cell: unknown, j: number) => (
                    <td key={j} className="max-w-50 truncate px-3 py-2 font-mono text-xs">
                      {cell === null ? (
                        <span className="text-ink-muted italic">NULL</span>
                      ) : (
                        String(cell)
                      )}
                    </td>
                  ))}
                  {canWrite && (
                    <td>
                      <div className="flex gap-1">
                        <Button
                          variant="ghost"
                          size="icon"
                          className="size-7"
                          onClick={() => onEdit(row)}
                        >
                          <Edit className="size-3.5" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="size-7 text-destructive hover:bg-destructive/10"
                          onClick={() => onDelete(buildRowWhere(row, data.columns))}
                        >
                          <Trash2 className="size-3.5" />
                        </Button>
                      </div>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {data && data.total > PAGE_SIZE && (
        <div className="flex items-center justify-between">
          <span className="text-sm text-ink-muted">
            Page {page + 1} of {totalPages} ({data.total.toLocaleString()} total)
          </span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={page === 0}
              onClick={() => setPage(page - 1)}
            >
              <ChevronLeft className="size-4" />
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages - 1}
              onClick={() => setPage(page + 1)}
            >
              <ChevronRight className="size-4" />
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
