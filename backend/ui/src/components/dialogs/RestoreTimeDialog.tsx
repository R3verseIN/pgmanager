import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { fetchDatabases } from "../../api/client";
import { restoreS3Backup } from "../../api/pgbackrest";
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

export default function RestoreTimeDialog({
  open,
  onOpenChange,
  targetTime,
  onRestored,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  targetTime: string;
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
    if (!targetDb || !targetTime) return;
    setRestoring(true);
    // Convert targetTime (YYYY-MM-DDThh:mm) to the format expected by pgbackrest (YYYY-MM-DD hh:mm:ss)
    const formattedTime = targetTime.replace("T", " ") + ":00";
    try {
      await restoreS3Backup(targetDb, undefined, formattedTime);
      toast.success(`Restored state to ${formattedTime} into ${targetDb}`);
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
          <DialogTitle>Point-In-Time Recovery</DialogTitle>
          <DialogDescription>
            Restore the database to exactly <span className="font-mono text-foreground">{targetTime.replace("T", " ")}</span>.
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
            Restore to Time
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
