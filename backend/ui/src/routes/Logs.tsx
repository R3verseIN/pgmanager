import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { RefreshCw, FileJson } from "lucide-react";
import { fetchLogs } from "../api/client";
import type { AuditLogEntry } from "../lib/schemas";

import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { Badge } from "../components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "../components/ui/dialog";

function LogDetail({ detail }: { detail: any }) {
  const [open, setOpen] = useState(false);
  
  if (!detail) return <span>—</span>;
  
  const isObj = typeof detail === "object";
  const rawStr = isObj ? JSON.stringify(detail) : String(detail);
  const prettyStr = isObj ? JSON.stringify(detail, null, 2) : String(detail);
  const isLong = rawStr.length > 40;
  
  if (!isLong) {
    return <span className="font-mono text-xs text-ink-muted">{rawStr}</span>;
  }
  
  return (
    <>
      <button 
        onClick={() => setOpen(true)}
        className="inline-flex items-center gap-1.5 rounded-sm border border-hairline bg-surface-2 px-2 py-1 text-xs font-medium text-ink-muted transition-colors hover:bg-surface-2/80"
      >
        <FileJson className="size-3.5" />
        View Details
      </button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-2xl">
          <DialogHeader>
            <DialogTitle>Audit Log Details</DialogTitle>
          </DialogHeader>
          <div className="mt-4 overflow-x-auto rounded-md border border-hairline bg-surface-2 p-4">
            <pre className="font-mono text-xs whitespace-pre-wrap text-foreground">
              {prettyStr}
            </pre>
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}

export default function Logs() {
  const [page, setPage] = useState(0);
  const [username, setUsername] = useState("");
  const [action, setAction] = useState("");
  const [database, setDatabase] = useState("");
  const LIMIT = 50;

  const { data, isLoading, refetch } = useQuery({
    queryKey: ["logs", page, username, action, database],
    queryFn: () => {
      const params: { limit: number; offset: number; username?: string; action?: string; database?: string } = {
        limit: LIMIT,
        offset: page * LIMIT,
      };
      if (username) params.username = username;
      if (action) params.action = action;
      if (database) params.database = database;
      return fetchLogs(params);
    },
  });

  const totalPages = data ? Math.ceil(data.total / LIMIT) : 0;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-(--font-display) tracking-tight">Audit Logs</h1>
          <p className="text-sm text-ink-muted">Track all database operations and user actions</p>
        </div>
        <Button variant="outline" onClick={() => refetch()}>
          <RefreshCw className="mr-2 size-4" />
          Refresh
        </Button>
      </div>

      <div className="flex flex-wrap gap-3">
        <div className="space-y-1">
          <Label className="text-xs">User</Label>
          <Input
            placeholder="Filter by user..."
            value={username}
            onChange={(e) => { setUsername(e.target.value); setPage(0); }}
            className="w-40"
          />
        </div>
        <div className="space-y-1">
          <Label className="text-xs">Action</Label>
          <Input
            placeholder="e.g. create_table"
            value={action}
            onChange={(e) => { setAction(e.target.value); setPage(0); }}
            className="w-40"
          />
        </div>
        <div className="space-y-1">
          <Label className="text-xs">Database</Label>
          <Input
            placeholder="Filter by DB..."
            value={database}
            onChange={(e) => { setDatabase(e.target.value); setPage(0); }}
            className="w-40"
          />
        </div>
      </div>

      {isLoading ? (
        <div className="py-8 text-center text-ink-muted">Loading...</div>
      ) : !data?.entries?.length ? (
        <div className="py-8 text-center text-ink-muted">No log entries found.</div>
      ) : (
        <div className="overflow-x-auto rounded-md border border-hairline bg-surface-1">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-hairline bg-surface-2/50">
                <th className="px-3 py-2 text-left font-medium text-ink-muted">Time</th>
                <th className="px-3 py-2 text-left font-medium text-ink-muted">User</th>
                <th className="px-3 py-2 text-left font-medium text-ink-muted">Action</th>
                <th className="px-3 py-2 text-left font-medium text-ink-muted">Database</th>
                <th className="px-3 py-2 text-left font-medium text-ink-muted">Table</th>
                <th className="px-3 py-2 text-left font-medium text-ink-muted">Detail</th>
                <th className="px-3 py-2 text-left font-medium text-ink-muted">IP</th>
              </tr>
            </thead>
            <tbody>
              {data.entries.map((entry: AuditLogEntry) => (
                <tr key={entry.id} className="border-b border-hairline last:border-0 hover:bg-surface-2/30">
                  <td className="px-3 py-2 font-mono text-xs whitespace-nowrap">
                    {new Date(entry.createdAt).toLocaleString()}
                  </td>
                  <td className="px-3 py-2 font-medium">{entry.username}</td>
                  <td className="px-3 py-2">
                    <Badge variant="outline" className="text-xs">{entry.action}</Badge>
                  </td>
                  <td className="px-3 py-2">{entry.database ?? "—"}</td>
                  <td className="px-3 py-2">{entry.tableName ?? "—"}</td>
                  <td className="px-3 py-2">
                    <LogDetail detail={entry.detail} />
                  </td>
                  <td className="px-3 py-2 font-mono text-xs">{entry.ipAddress ?? "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {data && data.total > LIMIT && (
        <div className="flex items-center justify-between">
          <span className="text-sm text-ink-muted">
            Page {page + 1} of {totalPages} ({data.total.toLocaleString()} entries)
          </span>
          <div className="flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={page === 0}
              onClick={() => setPage(page - 1)}
            >
              Previous
            </Button>
            <Button
              variant="outline"
              size="sm"
              disabled={page >= totalPages - 1}
              onClick={() => setPage(page + 1)}
            >
              Next
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
