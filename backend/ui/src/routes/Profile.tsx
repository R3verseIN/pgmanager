import { useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { toast } from "sonner";
import { User, Key, ArrowLeft } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../contexts/AuthContext";
import { changePassword } from "../api/client";

import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";

export default function Profile() {
  const { user } = useAuth();
  const navigate = useNavigate();

  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [error, setError] = useState<string | null>(null);

  const mutation = useMutation({
    mutationFn: () => changePassword(currentPassword, newPassword),
    onSuccess: () => {
      toast.success("Password changed successfully. You have been logged out.");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      setError(null);
    },
    onError: (error: Error) => {
      setError(error.message);
    },
  });

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);

    if (!currentPassword || !newPassword) {
      setError("All fields are required");
      return;
    }
    if (newPassword.length < 8) {
      setError("New password must be at least 8 characters");
      return;
    }
    if (newPassword.length > 72) {
      setError("New password must be at most 72 characters");
      return;
    }
    if (newPassword !== confirmPassword) {
      setError("New passwords do not match");
      return;
    }
    if (currentPassword === newPassword) {
      setError("New password must be different from current password");
      return;
    }

    mutation.mutate();
  }

  return (
    <div className="max-w-lg space-y-6">
      <div>
        <button
          onClick={() => navigate(-1)}
          className="mb-4 flex items-center gap-2 text-sm text-ink-muted transition-colors hover:text-foreground"
        >
          <ArrowLeft className="size-4" />
          Back
        </button>
        <h1 className="text-xl font-(--font-display) tracking-tight">Profile</h1>
        <p className="text-sm text-ink-muted">Manage your account settings</p>
      </div>

      {/* User Info */}
      <div className="space-y-4 rounded-md border border-hairline bg-surface-1 p-6">
        <div className="flex items-center gap-4">
          <div className="flex size-12 shrink-0 items-center justify-center rounded-full bg-surface-2 text-foreground">
            <User className="size-6" />
          </div>
          <div>
            <div className="font-medium">{user?.username}</div>
            <div className="text-sm text-ink-muted capitalize">{user?.role}</div>
          </div>
        </div>
      </div>

      {/* Change Password */}
      <div className="space-y-4 rounded-md border border-hairline bg-surface-1 p-6">
        <div className="flex items-center gap-2">
          <Key className="size-4" />
          <h2 className="font-medium">Change Password</h2>
        </div>
        <p className="text-sm text-ink-muted">
          After changing your password, you will be logged out and need to sign in again.
        </p>
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="current-password">Current Password</Label>
            <Input
              id="current-password"
              type="password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              placeholder="Enter current password"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="new-password">New Password</Label>
            <Input
              id="new-password"
              type="password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              placeholder="8-72 characters"
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="confirm-password">Confirm New Password</Label>
            <Input
              id="confirm-password"
              type="password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              placeholder="Re-enter new password"
            />
          </div>
          {error && <div className="text-sm text-destructive">{error}</div>}
          <Button type="submit" disabled={mutation.isPending}>
            {mutation.isPending ? "Changing..." : "Change Password"}
          </Button>
        </form>
      </div>
    </div>
  );
}
