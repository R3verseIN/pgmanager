import { useState, useRef, useCallback } from "react";
import { useQuery, useMutation } from "@tanstack/react-query";
import {
  Upload,
  Loader2,
  CheckCircle2,
  AlertCircle,
  FileDown,
  Trash2,
} from "lucide-react";
import {
  fetchBackupDatabases,
  inspectBackup,
  restoreBackup,
} from "../../api/client";
import type { BackupDatabase, BackupInspectResult } from "../../api/client";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import { Switch } from "../ui/switch";
import {
  Select,
  SelectTrigger,
  SelectContent,
  SelectItem,
} from "../ui/select";
import { formatBytes } from "../../lib/format";

export function RestoreTab() {
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [inspectResult, setInspectResult] =
    useState<BackupInspectResult | null>(null);
  const [targetDB, setTargetDB] = useState("");
  const [dropFirst, setDropFirst] = useState(false);
  const [message, setMessage] = useState<{
    type: "success" | "error";
    text: string;
  } | null>(null);

  const { data: databases } = useQuery({
    queryKey: ["backup-databases"],
    queryFn: fetchBackupDatabases,
  });

  const inspectMutation = useMutation({
    mutationFn: (f: File) => inspectBackup(f),
    onSuccess: (data) => {
      setInspectResult(data);
      if (data.database && !targetDB) {
        setTargetDB(data.database);
      }
      setMessage(null);
    },
    onError: (err: Error) => {
      setInspectResult(null);
      setMessage({ type: "error", text: err.message });
    },
  });

  const restoreMutation = useMutation({
    mutationFn: ({
      file,
      database,
      dropFirst,
    }: {
      file: File;
      database: string;
      dropFirst: boolean;
    }) => restoreBackup(file, database, dropFirst),
    onSuccess: (data) => {
      setMessage({ type: "success", text: data.message });
    },
    onError: (err: unknown) => {
      const apiErr = err as { status?: number; message: string; tables?: string[] };
      if (apiErr.status === 409 && apiErr.tables) {
        setMessage({
          type: "error",
          text: `${apiErr.message}\n\nExisting tables: ${apiErr.tables.join(", ")}`,
        });
      } else {
        setMessage({ type: "error", text: apiErr.message || "Restore failed" });
      }
    },
  });

  const handleFileChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const f = e.target.files?.[0];
      if (f) {
        setFile(f);
        setMessage(null);
        setInspectResult(null);
        inspectMutation.mutate(f);
      }
    },
    [inspectMutation]
  );

  const handleRestore = () => {
    if (!file || !targetDB) return;
    restoreMutation.mutate({ file, database: targetDB, dropFirst });
  };

  const handleReset = () => {
    setFile(null);
    setInspectResult(null);
    setTargetDB("");
    setDropFirst(false);
    setMessage(null);
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }
  };

  return (
    <div className="rounded-lg border border-hairline bg-surface-1 p-4">
      <div className="mb-4 flex items-center gap-3">
        <Upload className="size-5 text-ink-muted" />
        <div>
          <h2 className="text-sm font-medium text-foreground">
            Restore Backup
          </h2>
          <p className="text-xs text-ink-muted">
            Upload a .dump file to restore a database
          </p>
        </div>
      </div>

      <div className="space-y-4">
        {/* File upload */}
        <div>
          <input
            ref={fileInputRef}
            type="file"
            accept=".dump,.backup"
            onChange={handleFileChange}
            className="hidden"
          />
          {!file ? (
            <button
              onClick={() => fileInputRef.current?.click()}
              className="flex w-full flex-col items-center gap-2 rounded-lg border-2 border-dashed border-hairline bg-surface-2 p-8 text-center transition-colors hover:border-accent-blue/30 hover:bg-surface-2/50"
            >
              <Upload className="size-8 text-ink-muted" />
              <div>
                <p className="text-sm text-foreground">
                  Click to upload a backup file
                </p>
                <p className="text-xs text-ink-muted">
                  Supports .dump files (PostgreSQL custom format)
                </p>
              </div>
            </button>
          ) : (
            <div className="flex items-center gap-3 rounded-lg border border-hairline bg-surface-2 p-3">
              <FileDown className="size-5 shrink-0 text-ink-muted" />
              <div className="flex-1 overflow-hidden">
                <p className="truncate text-sm text-foreground">{file.name}</p>
                <p className="text-xs text-ink-muted">
                  {formatBytes(file.size)}
                </p>
              </div>
              <button
                onClick={handleReset}
                className="shrink-0 rounded-full p-2 text-ink-muted transition-colors hover:bg-surface-1 hover:text-foreground"
              >
                <Trash2 className="size-4" />
              </button>
            </div>
          )}
        </div>

        {/* Inspect loading */}
        {inspectMutation.isPending && (
          <div className="flex items-center gap-2 text-sm text-ink-muted">
            <Loader2 className="size-4 animate-spin" />
            Inspecting backup file...
          </div>
        )}

        {/* Inspect results */}
        {inspectResult && (
          <InspectResult result={inspectResult} />
        )}

        {/* Target database */}
        {inspectResult && (
          <div className="space-y-3">
            <div className="space-y-2">
              <Label className="text-sm text-ink-muted">Target Database</Label>
              <Select value={targetDB} onValueChange={setTargetDB}>
                <SelectTrigger className="w-full border-hairline bg-surface-2">
                  <span>{targetDB || "Select target database"}</span>
                </SelectTrigger>
                <SelectContent>
                  {databases?.map((db: BackupDatabase) => (
                    <SelectItem key={db.name} value={db.name}>
                      {db.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label className="text-sm text-ink-muted">Or create new</Label>
              <Input
                value={
                  databases?.some((d) => d.name === targetDB) ? "" : targetDB
                }
                onChange={(e) => setTargetDB(e.target.value)}
                placeholder="new_database_name"
                className="border-hairline bg-surface-2"
              />
            </div>

            <div className="flex items-center justify-between rounded-md bg-surface-2 px-3 py-2">
              <div>
                <p className="text-sm text-foreground">Drop target first</p>
                <p className="text-xs text-ink-muted">
                  Drop and recreate the target database before restore
                </p>
              </div>
              <Switch
                checked={dropFirst}
                onCheckedChange={setDropFirst}
              />
            </div>

            <div className="flex justify-end">
              <Button
                disabled={!targetDB || restoreMutation.isPending}
                onClick={handleRestore}
                size="sm"
              >
                {restoreMutation.isPending ? (
                  <Loader2 className="mr-1 size-4 animate-spin" />
                ) : (
                  <Upload className="mr-1 size-4" />
                )}
                Restore
              </Button>
            </div>
          </div>
        )}

        {/* Messages */}
        {message && (
          <div
            className={`flex items-center gap-2 rounded-lg p-3 text-sm ${
              message.type === "success"
                ? "bg-green-500/10 text-green-400"
                : "bg-red-500/10 text-red-400"
            }`}
          >
            {message.type === "success" ? (
              <CheckCircle2 className="size-4 shrink-0" />
            ) : (
              <AlertCircle className="size-4 shrink-0" />
            )}
            {message.text}
          </div>
        )}
      </div>
    </div>
  );
}

function InspectResult({ result }: { result: BackupInspectResult }) {
  return (
    <div className="space-y-3 rounded-lg border border-hairline bg-surface-2 p-3">
      <div className="flex items-center gap-2">
        <CheckCircle2 className="size-4 text-green-400" />
        <span className="text-sm font-medium text-foreground">
          Valid backup file
        </span>
      </div>
      <div className="grid grid-cols-2 gap-2 text-xs text-ink-muted">
        <div>
          Database:{" "}
          <span className="text-foreground">
            {result.database || "unknown"}
          </span>
        </div>
        <div>
          Format:{" "}
          <span className="text-foreground">{result.format}</span>
        </div>
        <div>
          Tables:{" "}
          <span className="text-foreground">{result.tables.length}</span>
        </div>
        <div>
          Size:{" "}
          <span className="text-foreground">{formatBytes(result.size)}</span>
        </div>
      </div>
      {result.tables.length > 0 && (
        <div className="max-h-32 overflow-y-auto">
          <p className="mb-1 text-xs text-ink-muted">Tables:</p>
          <div className="space-y-0.5">
            {result.tables.map((t) => (
              <p
                key={`${t.schema}.${t.name}`}
                className="text-xs text-foreground"
              >
                {t.schema}.{t.name}
              </p>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
