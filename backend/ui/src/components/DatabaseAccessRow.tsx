import { Database } from "lucide-react";
import { Switch } from "./ui/switch";

interface DatabaseAccessRowProps {
  databaseName: string;
  allowed: boolean;
  disabled?: boolean;
  onToggle: (allowed: boolean) => void;
}

export function DatabaseAccessRow({
  databaseName,
  allowed,
  disabled,
  onToggle,
}: DatabaseAccessRowProps) {
  return (
    <div className="flex items-center justify-between rounded-md px-3 py-2 hover:bg-surface-2">
      <div className="flex items-center gap-3">
        <Database className="size-4 text-ink-muted" />
        <span className="text-sm text-foreground font-mono">{databaseName}</span>
      </div>
      <Switch
        checked={allowed}
        disabled={disabled}
        onCheckedChange={onToggle}
      />
    </div>
  );
}
