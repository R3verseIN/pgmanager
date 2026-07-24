import { Label } from "./ui/label";
import { RadioGroup, RadioGroupItem } from "./ui/radio-group";

const accessLabels: Record<string, string> = {
  read: "SELECT",
  write: "SELECT, INSERT, UPDATE, DELETE",
  ddl: "Write + CREATE, ALTER, DROP",
  full: "ALL PRIVILEGES",
};

export default function AccessLevelSelect({
  value,
  onValueChange,
  idPrefix = "access",
}: {
  value: "read" | "write" | "ddl" | "full";
  onValueChange: (value: "read" | "write" | "ddl" | "full") => void;
  idPrefix?: string;
}) {
  return (
    <div className="space-y-2">
      <Label>Access Level</Label>
      <RadioGroup
        value={value}
        onValueChange={(val: string) => onValueChange(val as "read" | "write" | "ddl" | "full")}
        className="grid grid-cols-2 gap-3 pt-2"
      >
        {(["read", "write", "ddl", "full"] as const).map((level) => (
          <RadioGroupItem key={level} value={level} id={`${idPrefix}-${level}`}>
            <div className="mb-1 flex items-center justify-between">
              <span className="text-sm font-bold tracking-wider uppercase group-data-[state=checked]:text-foreground">
                {level}
              </span>
              <div className="size-2 rounded-full bg-foreground opacity-0 transition-opacity group-data-[state=checked]:opacity-100" />
            </div>
            <span className="text-[10px] leading-snug text-ink-muted sm:text-xs">
              {accessLabels[level]}
            </span>
          </RadioGroupItem>
        ))}
      </RadioGroup>
    </div>
  );
}
