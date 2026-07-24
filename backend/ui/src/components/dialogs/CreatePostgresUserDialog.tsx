import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { createUser } from "../../api/client";
import { CreateUserRequestSchema } from "../../lib/schemas";
import type { Database } from "../../lib/schemas";
import DbMultiSelect from "../DbMultiSelect";
import IpInput from "../IpInput";
import AccessLevelSelect from "../AccessLevelSelect";
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

export default function CreatePostgresUserDialog({
  open,
  onOpenChange,
  databases,
  onCreated,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  databases: Database[];
  onCreated: (creds: {
    username: string;
    password: string;
    databases: string[];
    connectionString: string;
  }) => void;
}) {
  const queryClient = useQueryClient();
  const [username, setUsername] = useState("");
  const [access, setAccess] = useState<"read" | "write" | "ddl" | "full">("write");
  const [selectedDbs, setSelectedDbs] = useState<string[]>([]);
  const [allowedIps, setAllowedIps] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);

  const createMutation = useMutation({
    mutationFn: (vars: {
      username: string;
      databases: string[];
      access: "read" | "write" | "ddl" | "full";
      allowedIps?: string[];
    }) => createUser(vars.username, vars.databases, vars.access, undefined, vars.allowedIps),
    onSuccess: (data) => {
      toast.success("User created successfully");
      onOpenChange(false);
      resetForm();
      onCreated({
        username: data.username,
        password: data.password,
        databases: data.databases,
        connectionString: data.connectionString,
      });
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  function resetForm() {
    setUsername("");
    setAccess("write");
    setSelectedDbs([]);
    setAllowedIps([]);
    setError(null);
  }

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    const result = CreateUserRequestSchema.safeParse({
      databases: selectedDbs,
      username,
      access,
    });
    if (!result.success) {
      setError(result.error.errors[0]?.message ?? "Invalid input");
      return;
    }
    setError(null);
    const vars: {
      username: string;
      databases: string[];
      access: "read" | "write" | "ddl" | "full";
      allowedIps?: string[];
    } = {
      username: result.data.username,
      databases: result.data.databases,
      access: result.data.access,
    };
    if (allowedIps.length > 0) vars.allowedIps = allowedIps;
    createMutation.mutate(vars);
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create Database User</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <div className="space-y-4 py-4">
            <div className="space-y-2">
              <Label>Username</Label>
              <Input
                placeholder="e.g. app_user"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                required
              />
            </div>
            <AccessLevelSelect value={access} onValueChange={setAccess} />
            <DbMultiSelect
              databases={databases}
              selected={selectedDbs}
              onChange={setSelectedDbs}
            />
            <div className="space-y-2">
              <Label>Allowed IPs</Label>
              <IpInput ips={allowedIps} onChange={setAllowedIps} />
            </div>
            {error && <div className="text-sm text-destructive">{error}</div>}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={createMutation.isPending || selectedDbs.length === 0}
            >
              Create
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
