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
            <div className="flex items-center justify-between mb-1">
              <span className="font-bold text-sm tracking-wider uppercase group-data-[state=checked]:text-primary">
                {level}
              </span>
              <div className="h-2 w-2 rounded-full bg-primary opacity-0 group-data-[state=checked]:opacity-100 transition-opacity" />
            </div>
            <span className="text-[10px] sm:text-xs text-muted-foreground leading-snug">
              {accessLabels[level]}
            </span>
          </RadioGroupItem>
        ))}
      </RadioGroup>
    </div>
  );
}
