import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Plus, RefreshCw } from "lucide-react";
import {
  fetchUsers,
  fetchDatabases,
  deleteUser,
  fetchAuthUsers,
  deleteAuthUser,
} from "../api/client";
import type { User, AuthUserListItem } from "../lib/schemas";

import { Button } from "../components/ui/button";
import PostgresUsersTable from "../components/PostgresUsersTable";
import AuthUsersTable from "../components/AuthUsersTable";
import CreatePostgresUserDialog from "../components/dialogs/CreatePostgresUserDialog";
import EditPostgresUserDialog from "../components/dialogs/EditPostgresUserDialog";
import ResetPasswordDialog from "../components/dialogs/ResetPasswordDialog";
import CredentialsDialog from "../components/dialogs/CredentialsDialog";
import CreateAuthUserDialog from "../components/dialogs/CreateAuthUserDialog";
import EditAuthUserDialog from "../components/dialogs/EditAuthUserDialog";
import ResetAuthPasswordDialog from "../components/dialogs/ResetAuthPasswordDialog";
import ConfirmDeleteDialog from "../components/dialogs/ConfirmDeleteDialog";

export default function Users() {
  const queryClient = useQueryClient();

  // Postgres Users
  const [createOpen, setCreateOpen] = useState(false);
  const [editOpen, setEditOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<User | null>(null);
  const [resetOpen, setResetOpen] = useState(false);
  const [resetTarget, setResetTarget] = useState<User | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<User | null>(null);
  const [showCreds, setShowCreds] = useState<{
    username: string;
    password: string;
    databases: string[];
    connectionString: string;
  } | null>(null);

  // Auth Users
  const [authCreateOpen, setAuthCreateOpen] = useState(false);
  const [authEditOpen, setAuthEditOpen] = useState(false);
  const [authEditTarget, setAuthEditTarget] = useState<AuthUserListItem | null>(null);
  const [authResetOpen, setAuthResetOpen] = useState(false);
  const [authResetTarget, setAuthResetTarget] = useState<AuthUserListItem | null>(null);
  const [authDeleteTarget, setAuthDeleteTarget] = useState<AuthUserListItem | null>(null);
  const [authShowCreds, setAuthShowCreds] = useState<{
    username: string;
    password: string;
  } | null>(null);

  const { data: users, isLoading } = useQuery({ queryKey: ["users"], queryFn: fetchUsers });
  const { data: databases } = useQuery({
    queryKey: ["databases"],
    queryFn: () => fetchDatabases(false),
  });
  const { data: authUsers, isLoading: authLoading } = useQuery({
    queryKey: ["authUsers"],
    queryFn: fetchAuthUsers,
  });

  // Postgres User Mutations
  const deleteMutation = useMutation({
    mutationFn: (username: string) => deleteUser(username),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const deleteAuthMutation = useMutation({
    mutationFn: (username: string) => deleteAuthUser(username),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["authUsers"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  return (
    <div className="space-y-12">
      {/* Postgres Users Section */}
      <div className="space-y-4">
        <div>
          <h2 className="text-xl font-semibold">Postgres Users</h2>
          <p className="text-sm text-muted-foreground">
            Manage database users and their access controls.
          </p>
        </div>
        <div className="flex flex-col sm:flex-row sm:items-center gap-4 sm:gap-2">
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="mr-2 h-4 w-4" /> Create User
          </Button>
          <Button
            variant="outline"
            onClick={() => queryClient.invalidateQueries({ queryKey: ["users"] })}
          >
            <RefreshCw className="mr-2 h-4 w-4" /> Refresh
          </Button>
        </div>
        <PostgresUsersTable
          users={users ?? []}
          isLoading={isLoading}
          onResetPassword={(user) => {
            setResetTarget(user);
            setResetOpen(true);
          }}
          onEdit={(user) => {
            setEditTarget(user);
            setEditOpen(true);
          }}
          onDelete={(user) => {
            setDeleteTarget(user);
          }}
        />
      </div>

      {/* Auth Users Section */}
      <div className="space-y-4">
        <div>
          <h2 className="text-xl font-semibold">Auth Users</h2>
          <p className="text-sm text-muted-foreground">
            Manage users who can log into this dashboard.
          </p>
        </div>
        <div className="flex flex-col sm:flex-row sm:items-center gap-4 sm:gap-2">
          <Button onClick={() => setAuthCreateOpen(true)}>
            <Plus className="mr-2 h-4 w-4" /> Create Auth User
          </Button>
          <Button
            variant="outline"
            onClick={() => queryClient.invalidateQueries({ queryKey: ["authUsers"] })}
          >
            <RefreshCw className="mr-2 h-4 w-4" /> Refresh
          </Button>
        </div>
        <AuthUsersTable
          users={authUsers ?? []}
          isLoading={authLoading}
          onEdit={(user) => {
            setAuthEditTarget(user);
            setAuthEditOpen(true);
          }}
          onResetPassword={(user) => {
            setAuthResetTarget(user);
            setAuthResetOpen(true);
          }}
          onDelete={(user) => {
            setAuthDeleteTarget(user);
          }}
        />
      </div>

      {/* DIALOGS */}
      <CreatePostgresUserDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        databases={databases ?? []}
        onCreated={(creds) => setShowCreds(creds)}
      />
      <CredentialsDialog
        open={showCreds !== null}
        onOpenChange={() => setShowCreds(null)}
        title="User Created"
        credentials={
          showCreds
            ? [
                { label: "USERNAME", value: showCreds.username },
                { label: "PASSWORD", value: showCreds.password, isSecret: true },
                { label: "CONNECTION STRING", value: showCreds.connectionString },
              ]
            : []
        }
      />
      <EditPostgresUserDialog
        open={editOpen}
        onOpenChange={setEditOpen}
        user={editTarget}
        databases={databases ?? []}
      />
      <ResetPasswordDialog
        open={resetOpen}
        onOpenChange={setResetOpen}
        user={resetTarget}
      />

      {/* Auth User Dialogs */}
      <CreateAuthUserDialog
        open={authCreateOpen}
        onOpenChange={setAuthCreateOpen}
        databases={databases ?? []}
        onCreated={(creds) => setAuthShowCreds(creds)}
      />
      <CredentialsDialog
        open={authShowCreds !== null}
        onOpenChange={() => setAuthShowCreds(null)}
        title="Auth User Created"
        credentials={
          authShowCreds
            ? [
                { label: "USERNAME", value: authShowCreds.username },
                { label: "PASSWORD", value: authShowCreds.password, isSecret: true },
              ]
            : []
        }
      />
      <EditAuthUserDialog
        open={authEditOpen}
        onOpenChange={setAuthEditOpen}
        user={authEditTarget}
        databases={databases ?? []}
      />
      <ResetAuthPasswordDialog
        open={authResetOpen}
        onOpenChange={setAuthResetOpen}
        user={authResetTarget}
      />

      {/* Delete Confirmations */}
      <ConfirmDeleteDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
        itemName={deleteTarget?.username ?? ""}
        isPending={deleteMutation.isPending}
        onConfirm={() => {
          if (deleteTarget) {
            deleteMutation.mutate(deleteTarget.username);
            setDeleteTarget(null);
          }
        }}
      />
      <ConfirmDeleteDialog
        open={authDeleteTarget !== null}
        onOpenChange={(open) => !open && setAuthDeleteTarget(null)}
        itemName={authDeleteTarget?.username ?? ""}
        isPending={deleteAuthMutation.isPending}
        onConfirm={() => {
          if (authDeleteTarget) {
            deleteAuthMutation.mutate(authDeleteTarget.username);
            setAuthDeleteTarget(null);
          }
        }}
      />
    </div>
  );
}
