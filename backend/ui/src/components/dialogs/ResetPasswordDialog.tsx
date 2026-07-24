import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Copy, Dices } from "lucide-react";
import { updateUser } from "../../api/client";
import { UpdateUserRequestSchema } from "../../lib/schemas";
import type { User } from "../../lib/schemas";
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

function copyText(text: string) {
  navigator.clipboard.writeText(text).then(() => toast.success("Copied to clipboard"));
}

export default function ResetPasswordDialog({
  open,
  onOpenChange,
  user,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  user: User | null;
}) {
  const queryClient = useQueryClient();
  const [password, setPassword] = useState("");
  const [generate, setGenerate] = useState(false);
  const [result, setResult] = useState<string | null>(null);

  const updateMutation = useMutation({
    mutationFn: (vars: {
      username: string;
      password?: string;
      generatePassword?: boolean;
    }) => updateUser(vars.username, vars),
    onSuccess: (data, vars) => {
      toast.success("User updated");
      const finalPassword = data?.password || vars.password;
      if (finalPassword) {
        setResult(finalPassword);
      }
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const handleClose = (open: boolean) => {
    if (!open) {
      setPassword("");
      setGenerate(false);
      setResult(null);
    }
    onOpenChange(open);
  };

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!user) return;
    const result = UpdateUserRequestSchema.safeParse({
      password: password || undefined,
      generatePassword: generate,
    });
    if (!result.success) {
      toast.error(result.error.errors[0]?.message ?? "Invalid input");
      return;
    }
    const vars: { username: string; password?: string; generatePassword?: boolean } = {
      username: user.username,
    };
    if (result.data.password) vars.password = result.data.password;
    if (result.data.generatePassword) vars.generatePassword = result.data.generatePassword;
    updateMutation.mutate(vars);
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Reset Password — {user?.username}</DialogTitle>
        </DialogHeader>
        {result ? (
          <div className="space-y-4 py-4">
            <div className="p-4 border rounded-lg bg-muted/50 space-y-3">
              <div className="space-y-1">
                <Label className="text-muted-foreground text-xs uppercase tracking-wider">
                  New Password
                </Label>
                <div className="flex gap-2">
                  <Input readOnly value={result} className="font-mono bg-background" />
                  <Button variant="outline" size="icon" onClick={() => copyText(result)}>
                    <Copy className="h-4 w-4" />
                  </Button>
                </div>
              </div>
              <p className="text-sm text-destructive font-medium">
                Please copy this password now. You won't be able to see it again!
              </p>
            </div>
            <DialogFooter>
              <Button onClick={() => handleClose(false)}>Done</Button>
            </DialogFooter>
          </div>
        ) : (
          <form onSubmit={handleSubmit}>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label>New Password</Label>
                <div className="flex gap-2">
                  <Input
                    type="password"
                    placeholder={generate ? "Will be auto-generated" : "Enter new password"}
                    value={password}
                    onChange={(e) => setPassword(e.target.value)}
                    disabled={generate}
                  />
                  <Button
                    type="button"
                    variant={generate ? "default" : "outline"}
                    size="icon"
                    onClick={() => {
                      setGenerate(!generate);
                      if (!generate) setPassword("");
                    }}
                    title="Auto-generate password"
                  >
                    <Dices className="h-4 w-4" />
                  </Button>
                </div>
              </div>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => handleClose(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={updateMutation.isPending}>
                Reset Password
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
