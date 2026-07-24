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
    <div className="space-y-6 max-w-lg">
      <div>
        <button
          onClick={() => navigate(-1)}
          className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors mb-4"
        >
          <ArrowLeft className="h-4 w-4" />
          Back
        </button>
        <h1 className="text-xl font-semibold">Profile</h1>
        <p className="text-sm text-muted-foreground">Manage your account settings</p>
      </div>

      {/* User Info */}
      <div className="rounded-md border border-border p-6 space-y-4">
        <div className="flex items-center gap-4">
          <div className="flex h-12 w-12 shrink-0 items-center justify-center rounded-full bg-accent text-accent-foreground">
            <User className="h-6 w-6" />
          </div>
          <div>
            <div className="font-medium">{user?.username}</div>
            <div className="text-sm text-muted-foreground capitalize">{user?.role}</div>
          </div>
        </div>
      </div>

      {/* Change Password */}
      <div className="rounded-md border border-border p-6 space-y-4">
        <div className="flex items-center gap-2">
          <Key className="h-4 w-4" />
          <h2 className="font-medium">Change Password</h2>
        </div>
        <p className="text-sm text-muted-foreground">
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
