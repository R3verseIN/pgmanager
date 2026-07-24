import { Label } from "./ui/label";
import { RadioGroup, RadioGroupItem } from "./ui/radio-group";

const roles = [
  { value: "admin" as const, label: "Admin", description: "Full access" },
  { value: "dev" as const, label: "Dev", description: "Assigned DBs only" },
  { value: "viewer" as const, label: "Viewer", description: "Read-only" },
];

export default function RoleSelect({
  value,
  onValueChange,
  idPrefix = "role",
}: {
  value: "admin" | "dev" | "viewer";
  onValueChange: (value: "admin" | "dev" | "viewer") => void;
  idPrefix?: string;
}) {
  return (
    <div className="space-y-2">
      <Label>Role</Label>
      <RadioGroup
        value={value}
        onValueChange={(val: string) => onValueChange(val as "admin" | "dev" | "viewer")}
        className="grid grid-cols-3 gap-3 pt-2"
      >
        {roles.map((role) => (
          <RadioGroupItem key={role.value} value={role.value} id={`${idPrefix}-${role.value}`}>
            <div className="mb-1 flex items-center justify-between">
              <span className="text-sm font-bold tracking-wider uppercase group-data-[state=checked]:text-foreground">
                {role.label}
              </span>
              <div className="size-2 rounded-full bg-foreground opacity-0 transition-opacity group-data-[state=checked]:opacity-100" />
            </div>
            <span className="text-[10px] leading-snug text-ink-muted sm:text-xs">
              {role.description}
            </span>
          </RadioGroupItem>
        ))}
      </RadioGroup>
    </div>
  );
}
