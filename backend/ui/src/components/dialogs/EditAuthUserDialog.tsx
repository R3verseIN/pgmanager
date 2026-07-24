import { useState, useEffect } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { updateAuthUser } from "../../api/client";
import type { AuthUserListItem, Database } from "../../lib/schemas";
import DbMultiSelect from "../DbMultiSelect";
import RoleSelect from "../RoleSelect";
import { Button } from "../ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../ui/dialog";

export default function EditAuthUserDialog({
  open,
  onOpenChange,
  user,
  databases,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  user: AuthUserListItem | null;
  databases: Database[];
}) {
  const queryClient = useQueryClient();
  const [role, setRole] = useState<"admin" | "dev" | "viewer">(user?.role ?? "viewer");
  const [selectedDbs, setSelectedDbs] = useState<string[]>(user?.databases ?? []);

  useEffect(() => {
    if (user) {
      setRole(user.role);
      setSelectedDbs(user.databases ?? []);
    }
  }, [user]);

  const updateMutation = useMutation({
    mutationFn: (vars: {
      username: string;
      role: "admin" | "dev" | "viewer";
      databases?: string[];
    }) => updateAuthUser(vars.username, vars.role, vars.databases),
    onSuccess: () => {
      toast.success("Auth user updated");
      onOpenChange(false);
      queryClient.invalidateQueries({ queryKey: ["authUsers"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  function handleRoleChange(val: "admin" | "dev" | "viewer") {
    setRole(val);
    if (val !== "dev") setSelectedDbs([]);
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!user) return;
    const vars: {
      username: string;
      role: "admin" | "dev" | "viewer";
      databases?: string[];
    } = {
      username: user.username,
      role,
    };
    if (role === "dev" && selectedDbs.length > 0) vars.databases = selectedDbs;
    updateMutation.mutate(vars);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit Auth User — {user?.username}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <div className="space-y-4 py-4">
            <RoleSelect value={role} onValueChange={handleRoleChange} idPrefix="edit-role" />
            {role === "dev" && (
              <DbMultiSelect
                databases={databases}
                selected={selectedDbs}
                onChange={setSelectedDbs}
              />
            )}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={updateMutation.isPending || (role === "dev" && selectedDbs.length === 0)}
            >
              Save
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
