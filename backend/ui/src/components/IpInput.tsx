import { useState } from "react";
import { X } from "lucide-react";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import { Input } from "./ui/input";

export default function IpInput({
  ips,
  onChange,
}: {
  ips: string[];
  onChange: (ips: string[]) => void;
}) {
  const [input, setInput] = useState("");

  const addIp = () => {
    const val = input.trim();
    if (val && !ips.includes(val)) {
      onChange([...ips, val]);
    }
    setInput("");
  };

  const removeIp = (ipToRemove: string) => {
    onChange(ips.filter((ip) => ip !== ipToRemove));
  };

  return (
    <div className="space-y-2">
      <div className="mb-2 flex min-h-7 flex-wrap gap-2">
        {ips.length === 0 && (
          <span className="pt-1 text-xs text-ink-muted">
            Any IP (0.0.0.0/0)
          </span>
        )}
        {ips.map((ip) => (
          <Badge
            key={ip}
            variant="secondary"
            className="flex items-center gap-1 py-1 pr-1 pl-2 font-mono text-[10px]"
          >
            {ip}
            <button
              type="button"
              onClick={() => removeIp(ip)}
              className="ml-1 rounded-full p-0.5 hover:bg-surface-2"
            >
              <X className="size-3" />
            </button>
          </Badge>
        ))}
      </div>
      <div className="flex gap-2">
        <Input
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              addIp();
            }
          }}
          placeholder="e.g. 192.168.0.10 or 10.0.0.0/24"
          className="h-9 font-mono text-sm"
        />
        <Button type="button" variant="outline" className="h-9" onClick={addIp}>
          Add
        </Button>
      </div>
    </div>
  );
}
