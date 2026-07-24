import { useState } from "react";
import { Badge } from "./ui/badge";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import type { Database } from "../lib/schemas";

export default function DbMultiSelect({
  databases,
  selected,
  onChange,
}: {
  databases: Database[];
  selected: string[];
  onChange: (selected: string[]) => void;
}) {
  const [search, setSearch] = useState("");

  const filtered = databases.filter((d) =>
    d.name.toLowerCase().includes(search.toLowerCase())
  );

  const allSelected = databases.length > 0 && selected.length === databases.length;

  const toggleAll = () => {
    if (allSelected) {
      onChange([]);
    } else {
      onChange(databases.map((d) => d.name));
    }
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <Label>
          Databases ({selected.length} selected)
        </Label>
        {databases.length > 0 && (
          <button
            type="button"
            onClick={toggleAll}
            className="text-xs text-primary hover:underline font-medium"
          >
            {allSelected ? "Deselect All" : "Select All"}
          </button>
        )}
      </div>

      {databases.length > 5 && (
        <Input
          placeholder="Search database..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="h-8 text-xs"
        />
      )}

      <div className="max-h-40 overflow-y-auto border border-border rounded-md p-2 flex flex-wrap gap-1.5 bg-muted/20">
        {filtered.length === 0 ? (
          <span className="text-xs text-muted-foreground p-1">
            {search ? "No matching databases" : "No databases available"}
          </span>
        ) : (
          filtered.map((d) => {
            const isSelected = selected.includes(d.name);
            return (
              <Badge
                key={d.name}
                variant={isSelected ? "default" : "outline"}
                className="cursor-pointer text-xs select-none transition-colors"
                onClick={() => {
                  if (isSelected) {
                    onChange(selected.filter((x) => x !== d.name));
                  } else {
                    onChange([...selected, d.name]);
                  }
                }}
              >
                {d.name}
              </Badge>
            );
          })
        )}
      </div>
    </div>
  );
}
