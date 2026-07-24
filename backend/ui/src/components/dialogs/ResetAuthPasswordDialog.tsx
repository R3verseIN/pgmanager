import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Copy } from "lucide-react";
import { resetAuthUserPassword } from "../../api/client";
import type { AuthUserListItem } from "../../lib/schemas";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import { Label } from "../ui/label";
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

export default function ResetAuthPasswordDialog({
  open,
  onOpenChange,
  user,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  user: AuthUserListItem | null;
}) {
  const queryClient = useQueryClient();
  const [input, setInput] = useState("");
  const [resultPassword, setResultPassword] = useState<string | null>(null);

  const resetMutation = useMutation({
    mutationFn: (vars: { username: string; password?: string }) =>
      resetAuthUserPassword(vars.username, vars.password),
    onSuccess: (password) => {
      toast.success("Password reset");
      setResultPassword(password);
      queryClient.invalidateQueries({ queryKey: ["authUsers"] });
    },
    onError: (err: Error) => toast.error(err.message),
  });

  const handleClose = (open: boolean) => {
    if (!open) {
      setInput("");
      setResultPassword(null);
    }
    onOpenChange(open);
  };

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Reset Password — {user?.username}</DialogTitle>
          <DialogDescription>
            {resultPassword === null
              ? "Set a new password or leave blank to generate a highly secure random one. They will be logged out immediately."
              : "Save this password — it cannot be shown again."}
          </DialogDescription>
        </DialogHeader>

        {resultPassword === null && (
          <div className="grid gap-4 py-4">
            <div className="grid gap-2">
              <Label htmlFor="auth-reset-password">New Password (Optional)</Label>
              <Input
                id="auth-reset-password"
                placeholder="Leave blank to auto-generate password"
                type="password"
                value={input}
                onChange={(e) => setInput(e.target.value)}
              />
            </div>
          </div>
        )}

        {resultPassword !== null && (
          <div className="font-mono text-sm">
            <div className="text-xs text-muted-foreground">PASSWORD</div>
            <div className="flex items-center gap-2 text-destructive">
              <span>{resultPassword}</span>
              <Copy
                className="h-4 w-4 cursor-pointer text-muted-foreground"
                onClick={() => copyText(resultPassword)}
              />
            </div>
          </div>
        )}
        <DialogFooter>
          {resultPassword === null ? (
            <>
              <Button variant="outline" onClick={() => handleClose(false)}>
                Cancel
              </Button>
              <Button
                disabled={resetMutation.isPending}
                onClick={() => {
                  if (!user) return;
                  const payload: { username: string; password?: string } = {
                    username: user.username,
                  };
                  if (input) payload.password = input;
                  resetMutation.mutate(payload);
                }}
              >
                Reset Password
              </Button>
            </>
          ) : (
            <Button onClick={() => handleClose(false)}>Done</Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
