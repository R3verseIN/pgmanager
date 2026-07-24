import { useState, useEffect } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { updateUser } from "../../api/client";
import { UpdateUserRequestSchema } from "../../lib/schemas";
import type { User, Database } from "../../lib/schemas";
import DbMultiSelect from "../DbMultiSelect";
import IpInput from "../IpInput";
import AccessLevelSelect from "../AccessLevelSelect";
import { Button } from "../ui/button";
import { Label } from "../ui/label";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../ui/dialog";

export default function EditPostgresUserDialog({
  open,
  onOpenChange,
  user,
  databases,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  user: User | null;
  databases: Database[];
}) {
  const queryClient = useQueryClient();
  const [access, setAccess] = useState<"read" | "write" | "ddl" | "full">(
    user?.access ?? "write"
  );
  const [selectedDbs, setSelectedDbs] = useState<string[]>(user?.databases ?? []);
  const [allowedIps, setAllowedIps] = useState<string[]>(user?.allowedIps ?? []);

  useEffect(() => {
    if (user) {
      setAccess(user.access);
      setSelectedDbs(user.databases ?? []);
      setAllowedIps(user.allowedIps ?? []);
    }
  }, [user]);

  const updateMutation = useMutation({
    mutationFn: (vars: {
      username: string;
      access?: "read" | "write" | "ddl" | "full";
      allowedIps?: string[];
      databases?: string[];
    }) => updateUser(vars.username, vars),
    onSuccess: () => {
      toast.success("User updated");
      onOpenChange(false);
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!user) return;
    const result = UpdateUserRequestSchema.safeParse({ access, databases: selectedDbs });
    if (!result.success) {
      toast.error(result.error.errors[0]?.message ?? "Invalid input");
      return;
    }
    const vars: {
      username: string;
      access?: "read" | "write" | "ddl" | "full";
      allowedIps?: string[];
      databases?: string[];
    } = { username: user.username };
    if (result.data.access) vars.access = result.data.access;
    if (result.data.databases) vars.databases = result.data.databases;
    vars.allowedIps = allowedIps.length > 0 ? allowedIps : ["0.0.0.0/0"];
    updateMutation.mutate(vars);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit User — {user?.username}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <div className="space-y-4 py-4">
            <DbMultiSelect
              databases={databases}
              selected={selectedDbs}
              onChange={setSelectedDbs}
            />
            <AccessLevelSelect
              value={access}
              onValueChange={setAccess}
              idPrefix="edit-access"
            />
            <div className="space-y-2">
              <Label>Allowed IPs</Label>
              <IpInput ips={allowedIps} onChange={setAllowedIps} />
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button type="submit" disabled={updateMutation.isPending}>
              Save
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
