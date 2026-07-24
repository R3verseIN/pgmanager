import { Copy } from "lucide-react";
import { toast } from "sonner";
import { Button } from "../ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../ui/dialog";

function copyText(text: string) {
  navigator.clipboard.writeText(text).then(() => toast.success("Copied to clipboard"));
}

export default function CredentialsDialog({
  open,
  onOpenChange,
  title,
  description = "Save these credentials — the password cannot be shown again.",
  credentials,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  credentials: { label: string; value: string; isSecret?: boolean }[];
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4 font-mono text-sm">
          {credentials.map((cred) => (
            <div key={cred.label}>
              <div className="text-xs text-muted-foreground">{cred.label}</div>
              <div
                className={`flex items-center gap-2 ${cred.isSecret ? "text-destructive" : ""}`}
              >
                <span className={cred.label === "CONNECTION STRING" ? "break-all" : ""}>
                  {cred.value}
                </span>
                <Copy
                  className="h-4 w-4 cursor-pointer text-muted-foreground"
                  onClick={() => copyText(cred.value)}
                />
              </div>
            </div>
          ))}
        </div>
        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>Done</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
