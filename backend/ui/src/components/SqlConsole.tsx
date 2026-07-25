import { useState, useCallback } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import {
  Play,
  Code,
  Clock,
  Download,
  ChevronDown,
  ChevronRight,
  Trash2,
} from "lucide-react";
import CodeMirror from "@uiw/react-codemirror";
import { sql as sqlLang, PostgreSQL } from "@codemirror/lang-sql";
import { EditorView } from "@codemirror/view";
import { createTheme } from "@uiw/codemirror-themes";
import { tags } from "@lezer/highlight";
import { format } from "sql-formatter";
import { executeQuery, fetchTables, fetchColumns } from "../api/client";
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

const framerTheme = createTheme({
  theme: "dark",
  settings: {
    background: "#141414",
    foreground: "#ffffff",
    caret: "#0099ff",
    selection: "rgba(0, 153, 255, 0.15)",
    gutterBackground: "#141414",
    lineHighlight: "#1c1c1c",
    fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace",
    fontSize: "13px",
  },
  styles: [
    { tag: tags.keyword, color: "#c586c0" },
    { tag: tags.operator, color: "#d4d4d4" },
    { tag: tags.special(tags.variableName), color: "#9cdcfe" },
    { tag: tags.typeName, color: "#4ec9b0" },
    { tag: tags.atom, color: "#569cd6" },
    { tag: tags.number, color: "#b5cea8" },
    { tag: tags.definition(tags.variableName), color: "#4fc1ff" },
    { tag: tags.string, color: "#ce9178" },
    { tag: tags.comment, color: "#6a9955", fontStyle: "italic" },
    { tag: tags.variableName, color: "#9cdcfe" },
    { tag: tags.tagName, color: "#569cd6" },
    { tag: tags.bool, color: "#569cd6" },
    { tag: tags.null, color: "#569cd6" },
    { tag: tags.className, color: "#4ec9b0" },
    { tag: tags.propertyName, color: "#9cdcfe" },
    { tag: tags.function(tags.variableName), color: "#dcdcaa" },
    { tag: tags.regexp, color: "#d16969" },
  ],
});

interface QueryHistoryEntry {
  sql: string;
  duration: number;
  rowCount: number;
  timestamp: number;
  error?: string | undefined;
}

export default function SqlConsole({ dbName }: { dbName: string }) {
  const [sql, setSql] = useState("");
  const [result, setResult] = useState<QueryResult | null>(null);
  const [history, setHistory] = useState<QueryHistoryEntry[]>([]);
  const [showHistory, setShowHistory] = useState(false);

  const { data: tables } = useQuery({
    queryKey: ["tables", dbName],
    queryFn: () => fetchTables(dbName),
  });

  const { data: tableColumns } = useQuery({
    queryKey: ["allColumns", dbName],
    queryFn: async () => {
      if (!tables) return {};
      const result: Record<string, string[]> = {};
      for (const table of tables) {
        const columns = await fetchColumns(dbName, table.name);
        result[table.name] = columns.map((c) => c.name);
      }
      return result;
    },
    enabled: !!tables && tables.length > 0,
  });

  const schema =
    tables && tableColumns
      ? Object.fromEntries(
          tables.map((t) => [t.name, tableColumns[t.name] || []])
        )
      : undefined;

  const extensions = [
    sqlLang({ dialect: PostgreSQL, ...(schema ? { schema } : {}) }),
    EditorView.lineWrapping,
    EditorView.theme({
      "&": { height: "200px" },
      ".cm-scroller": { overflow: "auto" },
    }),
  ];

  const queryMutation = useMutation<QueryResult, Error, string>({
    mutationFn: (query: string) => executeQuery(dbName, query),
    onSuccess: (data) => {
      setResult(data);
      setHistory((prev) => [
        {
          sql,
          duration: data.duration,
          rowCount: data.rowCount,
          timestamp: Date.now(),
          error: data.error,
        },
        ...prev.slice(0, 49),
      ]);
      if (data.error) {
        toast.error(data.error);
      }
    },
    onError: (error: Error) => toast.error(error.message),
  });

  function handleExecute() {
    if (sql.trim()) queryMutation.mutate(sql.trim());
  }

  function handleFormat() {
    try {
      const formatted = format(sql, {
        language: "postgresql",
        keywordCase: "upper",
      });
      setSql(formatted);
    } catch {
      toast.error("Failed to format SQL");
    }
  }

  function exportCsv() {
    if (!result || result.columns.length === 0) return;
    const headers = result.columns.join(",");
    const rows = result.rows.map((row) =>
      row
        .map((cell) => {
          if (cell === null) return "NULL";
          const str = String(cell);
          if (str.includes(",") || str.includes('"') || str.includes("\n")) {
            return `"${str.replace(/"/g, '""')}"`;
          }
          return str;
        })
        .join(",")
    );
    const csv = [headers, ...rows].join("\n");
    const blob = new Blob([csv], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `query-result-${Date.now()}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "Enter") {
        e.preventDefault();
        handleExecute();
      }
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key === "f") {
        e.preventDefault();
        handleFormat();
      }
    },
    [sql]
  );

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <div
          className="overflow-hidden rounded-[10px] border border-hairline bg-surface-1"
          onKeyDown={handleKeyDown}
        >
          <CodeMirror
            value={sql}
            onChange={setSql}
            theme={framerTheme}
            extensions={extensions}
            basicSetup={{
              lineNumbers: true,
              highlightActiveLineGutter: true,
              highlightSpecialChars: true,
              foldGutter: true,
              drawSelection: true,
              dropCursor: true,
              allowMultipleSelections: true,
              indentOnInput: true,
              bracketMatching: true,
              closeBrackets: true,
              autocompletion: true,
              rectangularSelection: true,
              crosshairCursor: false,
              highlightActiveLine: true,
              highlightSelectionMatches: true,
              closeBracketsKeymap: true,
              searchKeymap: true,
              foldKeymap: true,
              completionKeymap: true,
              lintKeymap: true,
            }}
          />
        </div>

        <div className="flex items-center gap-2">
          <Button
            onClick={handleExecute}
            disabled={!sql.trim() || queryMutation.isPending}
          >
            <Play className="mr-2 size-4" />
            Execute
          </Button>
          <Button
            variant="outline"
            onClick={handleFormat}
            disabled={!sql.trim()}
          >
            <Code className="mr-2 size-4" />
            Format
          </Button>
          {result && !result.error && result.columns.length > 0 && (
            <Button variant="outline" onClick={exportCsv}>
              <Download className="mr-2 size-4" />
              CSV
            </Button>
          )}
          <Button
            variant="outline"
            onClick={() => setShowHistory(!showHistory)}
          >
            <Clock className="mr-2 size-4" />
            History
            {showHistory ? (
              <ChevronDown className="ml-1 size-3" />
            ) : (
              <ChevronRight className="ml-1 size-3" />
            )}
          </Button>
          {result && (
            <span className="text-sm text-ink-muted">
              {result.duration}ms | {result.rowCount} row
              {result.rowCount !== 1 ? "s" : ""}
            </span>
          )}
        </div>
      </div>

      {showHistory && history.length > 0 && (
        <div className="rounded-[10px] border border-hairline bg-surface-1 p-3">
          <div className="mb-2 flex items-center justify-between">
            <span className="text-sm font-medium">Query History</span>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setHistory([])}
              className="h-6 text-xs text-ink-muted"
            >
              <Trash2 className="mr-1 size-3" />
              Clear
            </Button>
          </div>
          <div className="max-h-48 space-y-1 overflow-y-auto">
            {history.map((entry, i) => (
              <button
                key={`${entry.timestamp}-${i}`}
                className="w-full rounded-md px-2 py-1.5 text-left text-xs transition-colors hover:bg-surface-2"
                onClick={() => setSql(entry.sql)}
              >
                <div className="flex items-center gap-2">
                  <span className="truncate font-mono text-foreground">
                    {entry.sql}
                  </span>
                  <span className="shrink-0 text-ink-muted">
                    {entry.duration}ms
                  </span>
                  {entry.error && (
                    <span className="shrink-0 text-destructive">error</span>
                  )}
                </div>
              </button>
            ))}
          </div>
        </div>
      )}

      {result?.error && (
        <div className="rounded-[10px] border border-destructive/20 bg-destructive/10 p-3 text-sm text-destructive">
          {result.error}
        </div>
      )}

      {result && !result.error && result.columns.length > 0 && (
        <div className="overflow-x-auto rounded-[10px] border border-hairline bg-surface-1">
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
                        <span className="text-ink-muted italic">NULL</span>
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
