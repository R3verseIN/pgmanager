import { useState } from "react";
import { useParams, useNavigate, Link } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  ArrowLeft,
  Plus,
  Trash2,
  RefreshCw,
  ChevronLeft,
  ChevronRight,
  Edit,
  ChevronUp,
  ChevronDown,
} from "lucide-react";
import {
  fetchColumns,
  fetchData,
  insertRow,
  updateRow,
  deleteRow,
  addColumn,
  dropColumn,
} from "../api/client";
import type { ColumnInfo, WhereCondition, ColumnDef } from "../lib/schemas";
import { useAuth } from "../contexts/AuthContext";

import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { Badge } from "../components/ui/badge";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "../components/ui/tabs";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../components/ui/select";

const PAGE_SIZE = 100;

export default function TableDetail() {
  const { name: dbName, table } = useParams<{ name: string; table: string }>();
  const navigate = useNavigate();
  const { isAdmin, isDev } = useAuth();
  const [tab, setTab] = useState("data");

  const [page, setPage] = useState(0);
  const [sortCol, setSortCol] = useState("");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("asc");
  const [insertOpen, setInsertOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [editRow, setEditRow] = useState<unknown[] | null>(null);
  const [deleteWhere, setDeleteWhere] = useState<WhereCondition[] | null>(null);
  const [addColOpen, setAddColOpen] = useState(false);

  if (!dbName || !table) return null;

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" onClick={() => navigate(`/databases/${dbName}`)}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div>
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Link to={`/databases/${dbName}`} className="hover:underline">{dbName}</Link>
            <span>/</span>
          </div>
          <h1 className="text-xl font-semibold">{table}</h1>
        </div>
      </div>

      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="data">Data</TabsTrigger>
          <TabsTrigger value="structure">Structure</TabsTrigger>
        </TabsList>

        <TabsContent value="data">
          <DataTab
            dbName={dbName}
            table={table}
            page={page}
            setPage={setPage}
            sortCol={sortCol}
            setSortCol={setSortCol}
            sortDir={sortDir}
            setSortDir={setSortDir}
            canWrite={isAdmin || isDev}
            onInsert={() => setInsertOpen(true)}
            onEdit={(row) => {
              setEditRow(row);
              setEditOpen(true);
            }}
            onDelete={(where) => {
              setDeleteWhere(where);
            }}
          />
        </TabsContent>

        <TabsContent value="structure">
          <StructureTab
            dbName={dbName}
            table={table}
            canWrite={isAdmin || isDev}
            onAddColumn={() => setAddColOpen(true)}
          />
        </TabsContent>
      </Tabs>

      <InsertRowDialog
        dbName={dbName}
        table={table}
        open={insertOpen}
        onOpenChange={setInsertOpen}
      />
      <EditRowDialog
        dbName={dbName}
        table={table}
        open={editOpen}
        onOpenChange={setEditOpen}
        row={editRow}
      />
      <DeleteRowConfirm
        dbName={dbName}
        table={table}
        where={deleteWhere}
        onOpenChange={(open) => { if (!open) setDeleteWhere(null); }}
      />
      <AddColumnDialog
        dbName={dbName}
        table={table}
        open={addColOpen}
        onOpenChange={setAddColOpen}
      />
    </div>
  );
}

function DataTab({
  dbName, table, page, setPage, sortCol, setSortCol, sortDir, setSortDir,
  canWrite, onInsert, onEdit, onDelete,
}: {
  dbName: string; table: string; page: number; setPage: (p: number) => void;
  sortCol: string; setSortCol: (c: string) => void;
  sortDir: string; setSortDir: (d: "asc" | "desc") => void;
  canWrite: boolean; onInsert: () => void;
  onEdit: (row: unknown[]) => void; onDelete: (where: WhereCondition[]) => void;
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
            <Plus className="mr-2 h-4 w-4" />
            Insert Row
          </Button>
        )}
        <Button variant="outline" onClick={() => refetch()}>
          <RefreshCw className="mr-2 h-4 w-4" />
          Refresh
        </Button>
      </div>

      {isLoading ? (
        <div className="text-center py-8 text-muted-foreground">Loading...</div>
      ) : !data?.columns?.length ? (
        <div className="text-center py-8 text-muted-foreground">No data.</div>
      ) : (
        <div className="rounded-md border border-border overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/50">
                {data.columns.map((col: string) => (
                  <th
                    key={col}
                    className="px-3 py-2 text-left font-medium text-muted-foreground cursor-pointer hover:text-foreground select-none"
                    onClick={() => handleSort(col)}
                  >
                    <div className="flex items-center gap-1">
                      {col}
                      {sortCol === col && (
                        sortDir === "asc" ? <ChevronUp className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />
                      )}
                    </div>
                  </th>
                ))}
                {canWrite && <th className="w-20"></th>}
              </tr>
            </thead>
            <tbody>
              {data.rows.map((row: unknown[], i: number) => (
                <tr key={i} className="border-b border-border last:border-0 hover:bg-muted/30">
                  {row.map((cell: unknown, j: number) => (
                    <td key={j} className="px-3 py-2 font-mono text-xs max-w-50 truncate">
                      {cell === null ? (
                        <span className="text-muted-foreground italic">NULL</span>
                      ) : (
                        String(cell)
                      )}
                    </td>
                  ))}
                  {canWrite && (
                    <td>
                      <div className="flex gap-1">
                        <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => onEdit(row)}>
                          <Edit className="h-3.5 w-3.5" />
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="h-7 w-7 text-destructive hover:bg-destructive/10"
                          onClick={() => onDelete(buildRowWhere(row, data.columns))}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
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
          <span className="text-sm text-muted-foreground">
            Page {page + 1} of {totalPages} ({data.total.toLocaleString()} total)
          </span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={page === 0}
              onClick={() => setPage(page - 1)}
            >
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages - 1}
              onClick={() => setPage(page + 1)}
            >
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

function StructureTab({
  dbName, table, canWrite, onAddColumn,
}: {
  dbName: string; table: string; canWrite: boolean; onAddColumn: () => void;
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
            <Plus className="mr-2 h-4 w-4" />
            Add Column
          </Button>
        </div>
      )}

      {isLoading ? (
        <div className="text-center py-8 text-muted-foreground">Loading...</div>
      ) : !columns?.length ? (
        <div className="text-center py-8 text-muted-foreground">No columns.</div>
      ) : (
        <div className="rounded-md border border-border overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border bg-muted/50">
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">Name</th>
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">Type</th>
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">Nullable</th>
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">Default</th>
                <th className="px-3 py-2 text-left font-medium text-muted-foreground">PK</th>
                {canWrite && <th className="w-12"></th>}
              </tr>
            </thead>
            <tbody>
              {columns.map((col: ColumnInfo) => (
                <tr key={col.name} className="border-b border-border last:border-0">
                  <td className="px-3 py-2 font-medium">{col.name}</td>
                  <td className="px-3 py-2">
                    <Badge variant="outline" className="text-xs">{col.type}</Badge>
                  </td>
                  <td className="px-3 py-2">{col.nullable ? "YES" : "NO"}</td>
                  <td className="px-3 py-2 font-mono text-xs">
                    {col.default ?? <span className="text-muted-foreground">—</span>}
                  </td>
                  <td className="px-3 py-2">
                    {col.isPrimaryKey && <Badge variant="default" className="text-xs">PK</Badge>}
                  </td>
                  {canWrite && (
                    <td className="px-3 py-2">
                      <Button
                        variant="ghost"
                        size="icon"
                        className="h-7 w-7 text-destructive hover:bg-destructive/10"
                        disabled={col.isPrimaryKey}
                        onClick={() => {
                          if (confirm(`Drop column "${col.name}"?`)) {
                            dropColMutation.mutate(col.name);
                          }
                        }}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function InsertRowDialog({
  dbName, table, open, onOpenChange,
}: {
  dbName: string; table: string; open: boolean; onOpenChange: (open: boolean) => void;
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
          {columns?.filter((c: ColumnInfo) => !c.type.includes("SERIAL")).map((col: ColumnInfo) => (
            <div key={col.name} className="space-y-1">
              <Label>
                {col.name}
                <span className="text-xs text-muted-foreground ml-1">({col.type})</span>
                {col.nullable && <span className="text-xs text-muted-foreground ml-1">nullable</span>}
              </Label>
              <Input
                placeholder={col.default ?? ""}
                value={values[col.name] ?? ""}
                onChange={(e) => setValues({ ...values, [col.name]: e.target.value })}
              />
            </div>
          ))}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button onClick={() => insertMutation.mutate()} disabled={insertMutation.isPending}>
            Insert
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function EditRowDialog({
  dbName, table, open, onOpenChange, row,
}: {
  dbName: string; table: string; open: boolean; onOpenChange: (open: boolean) => void;
  row: unknown[] | null;
}) {
  const queryClient = useQueryClient();
  const { data: columns } = useQuery({
    queryKey: ["columns", dbName, table],
    queryFn: () => fetchColumns(dbName, table),
    enabled: open,
  });

  const [values, setValues] = useState<Record<string, string>>({});

  // Initialize values when row changes
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
            Editing by primary key. WHERE conditions will match the original row values.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3 py-4">
          {columns?.map((col: ColumnInfo) => (
            <div key={col.name} className="space-y-1">
              <Label>
                {col.name}
                <span className="text-xs text-muted-foreground ml-1">({col.type})</span>
                {col.isPrimaryKey && <Badge variant="default" className="ml-1 text-xs">PK</Badge>}
              </Label>
              <Input
                value={values[col.name] ?? ""}
                onChange={(e) => setValues({ ...values, [col.name]: e.target.value })}
              />
            </div>
          ))}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
          <Button onClick={() => updateMutation.mutate()} disabled={updateMutation.isPending}>
            Save
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DeleteRowConfirm({
  dbName, table, where, onOpenChange,
}: {
  dbName: string; table: string; where: WhereCondition[] | null;
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
            <p className="text-sm text-muted-foreground">WHERE conditions:</p>
            <div className="rounded-md bg-muted p-2 font-mono text-xs space-y-1">
              {where.map((w, i) => (
                <div key={i}>
                  {w.column} {w.operator} {w.value !== undefined ? String(w.value) : ""}
                </div>
              ))}
            </div>
          </div>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
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

function AddColumnDialog({
  dbName, table, open, onOpenChange,
}: {
  dbName: string; table: string; open: boolean; onOpenChange: (open: boolean) => void;
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

  const TYPES = ["TEXT", "INTEGER", "BIGINT", "SMALLINT", "BOOLEAN", "DATE", "TIMESTAMP", "TIMESTAMPTZ", "NUMERIC", "REAL", "DOUBLE PRECISION", "UUID", "JSON", "JSONB", "BYTEA"];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add Column to {table}</DialogTitle>
        </DialogHeader>
        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label>Name</Label>
            <Input value={colName} onChange={(e) => setColName(e.target.value)} autoFocus />
          </div>
          <div className="space-y-2">
            <Label>Type</Label>
            <Select value={colType} onValueChange={setColType}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                {TYPES.map((t) => <SelectItem key={t} value={t}>{t}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-2">
            <Label>Default</Label>
            <Input placeholder="NULL" value={colDefault} onChange={(e) => setColDefault(e.target.value)} />
          </div>
          <div className="flex items-center gap-2">
            <input type="checkbox" id="nullable" checked={colNullable} onChange={(e) => setColNullable(e.target.checked)} />
            <Label htmlFor="nullable">Nullable</Label>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>Cancel</Button>
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

function parseValue(raw: string, type: string): unknown {
  const t = type.toUpperCase();
  if (raw === "" || raw === "NULL") return null;
  if (t.includes("INT") || t.includes("NUMERIC") || t === "REAL" || t === "DOUBLE PRECISION") {
    const n = Number(raw);
    return isNaN(n) ? raw : n;
  }
  if (t === "BOOLEAN") return raw === "true" || raw === "1";
  return raw;
}
