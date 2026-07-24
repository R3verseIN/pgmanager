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
            <div className="flex items-center justify-between mb-1">
              <span className="font-bold text-sm tracking-wider uppercase group-data-[state=checked]:text-primary">
                {role.label}
              </span>
              <div className="h-2 w-2 rounded-full bg-primary opacity-0 group-data-[state=checked]:opacity-100 transition-opacity" />
            </div>
            <span className="text-[10px] sm:text-xs text-muted-foreground leading-snug">
              {role.description}
            </span>
          </RadioGroupItem>
        ))}
      </RadioGroup>
    </div>
  );
}
