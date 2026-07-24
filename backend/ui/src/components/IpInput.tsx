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
      <div className="flex flex-wrap gap-2 mb-2 min-h-7">
        {ips.length === 0 && (
          <span className="text-xs text-muted-foreground pt-1">
            Any IP (0.0.0.0/0)
          </span>
        )}
        {ips.map((ip) => (
          <Badge
            key={ip}
            variant="secondary"
            className="pl-2 pr-1 py-1 flex items-center gap-1 font-mono text-[10px]"
          >
            {ip}
            <button
              type="button"
              onClick={() => removeIp(ip)}
              className="rounded-full hover:bg-muted p-0.5 ml-1"
            >
              <X className="h-3 w-3" />
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
          className="font-mono text-sm h-9"
        />
        <Button type="button" variant="outline" className="h-9" onClick={addIp}>
          Add
        </Button>
      </div>
    </div>
  );
}
