import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { FileDown, Loader2, Download, CheckCircle2 } from "lucide-react";
import {
  fetchBackupDatabases,
  fetchBackupTables,
  backupDatabase,
  downloadBlob,
} from "../../api/client";
import type { BackupDatabase, BackupTable } from "../../api/client";
import { Button } from "../ui/button";
import { Label } from "../ui/label";
import {
  Select,
  SelectTrigger,
  SelectContent,
  SelectItem,
} from "../ui/select";
import { CheckboxItem } from "./CheckboxItem";

export function BackupTablesTab() {
  const [selectedDB, setSelectedDB] = useState<string>("");
  const [selectedTables, setSelectedTables] = useState<Set<string>>(new Set());
  const [downloading, setDownloading] = useState(false);

  const { data: databases } = useQuery({
    queryKey: ["backup-databases"],
    queryFn: fetchBackupDatabases,
  });

  const { data: tableData, isLoading: tablesLoading } = useQuery({
    queryKey: ["backup-tables", selectedDB],
    queryFn: () => fetchBackupTables(selectedDB),
    enabled: !!selectedDB,
  });

  const toggleTable = (name: string) => {
    setSelectedTables((prev) => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  };

  const selectAllTables = () => {
    if (!tableData) return;
    if (selectedTables.size === tableData.tables.length) {
      setSelectedTables(new Set());
    } else {
      setSelectedTables(new Set(tableData.tables.map((t) => t.name)));
    }
  };

  const handleBackup = async () => {
    if (!selectedDB || selectedTables.size === 0) return;
    setDownloading(true);
    try {
      const blob = await backupDatabase(selectedDB, Array.from(selectedTables));
      const timestamp = new Date()
        .toISOString()
        .slice(0, 19)
        .replace(/[:.]/g, "-");
      downloadBlob(blob, `${selectedDB}_tables_${timestamp}.dump`);
    } catch {
      // error thrown by backupDatabase
    } finally {
      setDownloading(false);
    }
  };

  return (
    <div className="rounded-lg border border-hairline bg-surface-1 p-4">
      <div className="mb-4 flex items-center gap-3">
        <FileDown className="size-5 text-ink-muted" />
        <div>
          <h2 className="text-sm font-medium text-foreground">
            Backup Tables
          </h2>
          <p className="text-xs text-ink-muted">
            Select a database, then choose specific tables to back up
          </p>
        </div>
      </div>

      <div className="space-y-4">
        <div className="space-y-2">
          <Label className="text-sm text-ink-muted">Database</Label>
          <Select
            value={selectedDB}
            onValueChange={(value) => {
              setSelectedDB(value);
              setSelectedTables(new Set());
            }}
          >
            <SelectTrigger className="w-full border-hairline bg-surface-2">
              <span>{selectedDB || "Select a database"}</span>
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

        {selectedDB && (
          <>
            {tablesLoading ? (
              <div className="flex items-center justify-center py-8">
                <Loader2 className="size-5 animate-spin text-ink-muted" />
              </div>
            ) : !tableData || tableData.tables.length === 0 ? (
              <p className="py-4 text-sm text-ink-muted">
                No tables found in {selectedDB}
              </p>
            ) : (
              <>
                <div className="space-y-1">
                  <button
                    onClick={selectAllTables}
                    className="flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm text-ink-muted transition-colors hover:bg-surface-2 hover:text-foreground"
                  >
                    <div
                      className={`flex size-4 shrink-0 items-center justify-center rounded border ${
                        selectedTables.size === tableData.tables.length
                          ? "border-accent-blue bg-accent-blue text-white"
                          : "border-hairline bg-surface-2"
                      }`}
                    >
                      {selectedTables.size === tableData.tables.length && (
                        <CheckCircle2 className="size-3" />
                      )}
                    </div>
                    Select all ({tableData.tables.length})
                  </button>

                  {tableData.tables.map((table: BackupTable) => (
                    <CheckboxItem
                      key={table.name}
                      checked={selectedTables.has(table.name)}
                      onClick={() => toggleTable(table.name)}
                      sublabel={`${table.schema}.`}
                      label={table.name}
                    />
                  ))}
                </div>

                <div className="flex justify-end pt-2">
                  <Button
                    disabled={selectedTables.size === 0 || downloading}
                    onClick={handleBackup}
                    size="sm"
                  >
                    {downloading ? (
                      <Loader2 className="mr-1 size-4 animate-spin" />
                    ) : (
                      <Download className="mr-1 size-4" />
                    )}
                    Backup {selectedTables.size > 0 ? `(${selectedTables.size})` : ""}
                  </Button>
                </div>
              </>
            )}
          </>
        )}
      </div>
    </div>
  );
}
