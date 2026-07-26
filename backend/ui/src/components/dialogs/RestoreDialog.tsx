import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { fetchDatabases, restoreWalgBackup } from "../../api/client";
import { Button } from "../ui/button";
import { Label } from "../ui/label";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../ui/select";
import { toast } from "sonner";

export default function RestoreDialog({
  open,
  onOpenChange,
  backupName,
  onRestored,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  backupName: string;
  onRestored: () => void;
}) {
  const [targetDb, setTargetDb] = useState("");
  const [restoring, setRestoring] = useState(false);

  const { data: databases, isLoading: dbsLoading } = useQuery({
    queryKey: ["databases"],
    queryFn: () => fetchDatabases(false),
    enabled: open,
  });

  async function handleRestore() {
    if (!targetDb || !backupName) return;
    setRestoring(true);
    try {
      await restoreWalgBackup(backupName, targetDb);
      toast.success(`Restored ${backupName} to ${targetDb}`);
      onOpenChange(false);
      setTargetDb("");
      onRestored();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Restore failed");
    } finally {
      setRestoring(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Restore Backup</DialogTitle>
          <DialogDescription>
            Restore <span className="font-mono text-foreground">{backupName}</span> to a target database.
            The target database will be overwritten.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          <div className="space-y-2">
            <Label>Target Database</Label>
            {dbsLoading ? (
              <div className="flex items-center gap-2 text-sm text-ink-muted">
                <Loader2 className="size-4 animate-spin" />
                Loading databases...
              </div>
            ) : (
              <Select value={targetDb} onValueChange={setTargetDb}>
                <SelectTrigger className="border-hairline bg-surface-2">
                  <SelectValue placeholder="Select a database" />
                </SelectTrigger>
                <SelectContent>
                  {databases?.map((db) => (
                    <SelectItem key={db.name} value={db.name}>
                      {db.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            )}
          </div>
        </div>

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={restoring}
          >
            Cancel
          </Button>
          <Button
            onClick={handleRestore}
            disabled={!targetDb || restoring}
          >
            {restoring && <Loader2 className="mr-2 size-4 animate-spin" />}
            Restore
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
