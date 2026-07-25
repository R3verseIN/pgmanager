import { useState } from "react";
import { DatabaseBackup, FileDown, Upload } from "lucide-react";
import { BackupDatabaseTab } from "../components/backup/BackupDatabaseTab";
import { BackupTablesTab } from "../components/backup/BackupTablesTab";
import { RestoreTab } from "../components/backup/RestoreTab";
import { useAuth } from "../contexts/AuthContext";

type Tab = "database" | "tables" | "restore";

const allTabs = [
  { key: "database" as Tab, label: "Backup Database", icon: DatabaseBackup },
  { key: "tables" as Tab, label: "Backup Tables", icon: FileDown },
  { key: "restore" as Tab, label: "Restore", icon: Upload, adminOnly: true },
];

export default function Backups() {
  const { isAdmin } = useAuth();
  const [tab, setTab] = useState<Tab>("database");

  const tabs = allTabs.filter((t) => !t.adminOnly || isAdmin);

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-(--font-display) text-foreground">
          Backups
        </h1>
        <p className="text-sm text-ink-muted">
          Backup and restore your databases
        </p>
      </div>

      <div className="flex gap-1 rounded-lg border border-hairline bg-surface-1 p-1">
        {tabs.map((t) => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            className={`flex flex-1 items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors ${
              tab === t.key
                ? "bg-surface-2 text-foreground"
                : "text-ink-muted hover:text-foreground"
            }`}
          >
            <t.icon className="size-4" />
            {t.label}
          </button>
        ))}
      </div>

      {tab === "database" && <BackupDatabaseTab />}
      {tab === "tables" && <BackupTablesTab />}
      {tab === "restore" && <RestoreTab />}
    </div>
  );
}
