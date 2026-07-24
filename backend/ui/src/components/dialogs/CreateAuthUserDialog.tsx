import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { createAuthUser } from "../../api/client";
import { CreateAuthUserRequestSchema } from "../../lib/schemas";
import type { Database } from "../../lib/schemas";
import DbMultiSelect from "../DbMultiSelect";
import RoleSelect from "../RoleSelect";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../ui/dialog";

export default function CreateAuthUserDialog({
  open,
  onOpenChange,
  databases,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  databases: Database[];
  onCreated: (creds: { username: string; password: string }) => void;
}) {
  const queryClient = useQueryClient();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<"admin" | "dev" | "viewer">("viewer");
  const [selectedDbs, setSelectedDbs] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);

  const createMutation = useMutation({
    mutationFn: (vars: {
      username: string;
      password?: string;
      role: "admin" | "dev" | "viewer";
      databases?: string[];
    }) => createAuthUser(vars.username, vars.password || "", vars.role, vars.databases),
    onSuccess: (data) => {
      toast.success("Auth user created");
      onOpenChange(false);
      resetForm();
      queryClient.invalidateQueries({ queryKey: ["authUsers"] });
      onCreated({ username: data.username, password: data.password });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  function resetForm() {
    setUsername("");
    setPassword("");
    setRole("viewer");
    setSelectedDbs([]);
    setError(null);
  }

  function handleRoleChange(val: "admin" | "dev" | "viewer") {
    setRole(val);
    if (val !== "dev") setSelectedDbs([]);
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const vars: {
      username: string;
      password?: string;
      role: "admin" | "dev" | "viewer";
      databases?: string[];
    } = {
      username,
      role,
    };
    if (password) vars.password = password;
    if (role === "dev" && selectedDbs.length > 0) vars.databases = selectedDbs;

    const result = CreateAuthUserRequestSchema.safeParse({
      username: vars.username,
      password: vars.password || undefined,
      role: vars.role,
      databases: vars.databases,
    });
    if (!result.success) {
      setError(result.error.errors[0]?.message ?? "Invalid input");
      return;
    }
    setError(null);
    createMutation.mutate(vars);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create Auth User</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label>Username</Label>
              <Input
                placeholder="e.g. admin_user"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <Label>Password (Optional)</Label>
              <Input
                type="password"
                placeholder="Leave blank to auto-generate password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
            <RoleSelect value={role} onValueChange={handleRoleChange} />
            {role === "dev" && (
              <DbMultiSelect
                databases={databases}
                selected={selectedDbs}
                onChange={setSelectedDbs}
              />
            )}
            {error && <div className="text-sm text-destructive">{error}</div>}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={createMutation.isPending || (role === "dev" && selectedDbs.length === 0)}
            >
              Create
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
