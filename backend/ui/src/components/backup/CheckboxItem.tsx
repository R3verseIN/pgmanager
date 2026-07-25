import { CheckCircle2, type LucideIcon } from "lucide-react";
import { cn } from "../../lib/utils";

export function CheckboxItem({
  checked,
  onClick,
  icon: Icon,
  label,
  sublabel,
}: {
  checked: boolean;
  onClick: () => void;
  icon?: LucideIcon;
  label: string;
  sublabel?: string;
}) {
  return (
    <button
      onClick={onClick}
      className="flex w-full items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors hover:bg-surface-2"
    >
      <div
        className={cn(
          "flex size-4 shrink-0 items-center justify-center rounded border",
          checked
            ? "border-accent-blue bg-accent-blue text-white"
            : "border-hairline bg-surface-2"
        )}
      >
        {checked && <CheckCircle2 className="size-3" />}
      </div>
      {Icon && <Icon className="size-4 shrink-0 text-ink-muted" />}
      {sublabel && (
        <span className="text-xs text-ink-muted">{sublabel}</span>
      )}
      <span className="text-foreground">{label}</span>
    </button>
  );
}
