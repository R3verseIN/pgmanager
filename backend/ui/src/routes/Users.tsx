import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Plus, Trash2, RefreshCw, Copy, Edit, Key } from "lucide-react";
import {
  fetchUsers,
  fetchDatabases,
  createUser,
  deleteUser,
  updateUser,
  addUserDatabase,
  removeUserDatabase,
  fetchAuthUsers,
  createAuthUser,
  updateAuthUser,
  deleteAuthUser,
  resetAuthUserPassword,
} from "../api/client";
import {
  CreateUserRequestSchema,
  UpdateUserRequestSchema,
  AddDatabaseRequestSchema,
  CreateAuthUserRequestSchema,
} from "../lib/schemas";
import type { User, AuthUserListItem, Database } from "../lib/schemas";

import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { RadioGroup, RadioGroupItem } from "../components/ui/radio-group";

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../components/ui/select";
import { Badge } from "../components/ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "../components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "../components/ui/dialog";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "../components/ui/tooltip";

function DbMultiSelect({
  databases,
  selected,
  onChange,
}: {
  databases: Database[];
  selected: string[];
  onChange: (selected: string[]) => void;
}) {
  const [search, setSearch] = useState("");

  const filtered = databases.filter((d) =>
    d.name.toLowerCase().includes(search.toLowerCase())
  );

  const allSelected = databases.length > 0 && selected.length === databases.length;

  const toggleAll = () => {
    if (allSelected) {
      onChange([]);
    } else {
      onChange(databases.map((d) => d.name));
    }
  };

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <Label>
          Databases ({selected.length} selected)
        </Label>
        {databases.length > 0 && (
          <button
            type="button"
            onClick={toggleAll}
            className="text-xs text-primary hover:underline font-medium"
          >
            {allSelected ? "Deselect All" : "Select All"}
          </button>
        )}
      </div>

      {databases.length > 5 && (
        <Input
          placeholder="Search database..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="h-8 text-xs"
        />
      )}

      <div className="max-h-40 overflow-y-auto border border-border rounded-md p-2 flex flex-wrap gap-1.5 bg-muted/20">
        {filtered.length === 0 ? (
          <span className="text-xs text-muted-foreground p-1">
            {search ? "No matching databases" : "No databases available"}
          </span>
        ) : (
          filtered.map((d) => {
            const isSelected = selected.includes(d.name);
            return (
              <Badge
                key={d.name}
                variant={isSelected ? "default" : "outline"}
                className="cursor-pointer text-xs select-none transition-colors"
                onClick={() => {
                  if (isSelected) {
                    onChange(selected.filter((x) => x !== d.name));
                  } else {
                    onChange([...selected, d.name]);
                  }
                }}
              >
                {d.name}
              </Badge>
            );
          })
        )}
      </div>
    </div>
  );
}

export default function Users() {
  const queryClient = useQueryClient();

  // Postgres Users State
  const [createOpen, setCreateOpen] = useState(false);
  const [formDbs, setFormDbs] = useState<string[]>([]);
  const [formUsername, setFormUsername] = useState("");
  const [formAccess, setFormAccess] = useState<"read" | "write" | "ddl" | "full">("write");
  const [formError, setFormError] = useState<string | null>(null);
  const [showCreds, setShowCreds] = useState<{ username: string; password: string; databases: string[]; connectionString: string } | null>(null);
  const [editOpen, setEditOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<User | null>(null);
  const [editAccess, setEditAccess] = useState<"read" | "write" | "ddl" | "full">("write");
  const [editPassword, setEditPassword] = useState("");
  const [addDbOpen, setAddDbOpen] = useState(false);
  const [addDbTarget, setAddDbTarget] = useState<User | null>(null);
  const [addDbName, setAddDbName] = useState("");
  const [addDbError, setAddDbError] = useState<string | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<User | null>(null);
  const [deleteConfirmText, setDeleteConfirmText] = useState("");

  // Auth Users State
  const [authCreateOpen, setAuthCreateOpen] = useState(false);
  const [authCreateUsername, setAuthCreateUsername] = useState("");
  const [authCreatePassword, setAuthCreatePassword] = useState("");
  const [authCreateRole, setAuthCreateRole] = useState<"admin" | "dev" | "viewer">("viewer");
  const [authCreateDatabases, setAuthCreateDatabases] = useState<string[]>([]);
  const [authCreateError, setAuthCreateError] = useState<string | null>(null);
  const [authShowCreds, setAuthShowCreds] = useState<{ username: string; password: string } | null>(null);
  const [authEditOpen, setAuthEditOpen] = useState(false);
  const [authEditTarget, setAuthEditTarget] = useState<AuthUserListItem | null>(null);
  const [authEditRole, setAuthEditRole] = useState<"admin" | "dev" | "viewer">("viewer");
  const [authEditDatabases, setAuthEditDatabases] = useState<string[]>([]);
  const [authResetOpen, setAuthResetOpen] = useState(false);
  const [authResetTarget, setAuthResetTarget] = useState<AuthUserListItem | null>(null);
  const [authResetPassword, setAuthResetPassword] = useState<string | null>(null);
  const [authResetInput, setAuthResetInput] = useState("");
  const [authDeleteTarget, setAuthDeleteTarget] = useState<AuthUserListItem | null>(null);
  const [authDeleteConfirmText, setAuthDeleteConfirmText] = useState("");

  const { data: users, isLoading } = useQuery({ queryKey: ["users"], queryFn: fetchUsers });
  const { data: databases } = useQuery({ queryKey: ["databases"], queryFn: () => fetchDatabases(false) });
  const { data: authUsers, isLoading: authLoading } = useQuery({ queryKey: ["authUsers"], queryFn: fetchAuthUsers });

  // Postgres User Mutations
  const createMutation = useMutation({
    mutationFn: (vars: { username: string; databases: string[]; access: "read" | "write" | "ddl" | "full"; password?: string }) =>
      createUser(vars.username, vars.databases, vars.access, vars.password),
    onSuccess: (data) => {
      toast.success("User created successfully");
      setCreateOpen(false);
      resetForm();
      setShowCreds({ username: data.username, password: data.password, databases: data.databases, connectionString: data.connectionString });
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const deleteMutation = useMutation({
    mutationFn: (username: string) => deleteUser(username),
    onSuccess: () => {
      toast.success("User deleted");
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const updateMutation = useMutation({
    mutationFn: (vars: { username: string; password?: string; access?: "read" | "write" | "ddl" | "full" }) => {
      const opts: { password?: string; access?: "read" | "write" | "ddl" | "full" } = {};
      if (vars.password) opts.password = vars.password;
      if (vars.access) opts.access = vars.access;
      return updateUser(vars.username, opts);
    },
    onSuccess: (_, vars) => {
      toast.success("User updated");
      setEditOpen(false);
      if (vars.password && editTarget) {
        const firstDb = editTarget.databases?.[0] || "postgres";
        const connStr = `postgres://${vars.username}:${vars.password}@localhost:5432/${firstDb}`;
        setShowCreds({
          username: vars.username,
          password: vars.password,
          databases: editTarget.databases || [],
          connectionString: connStr,
        });
      }
      setEditTarget(null);
      setEditPassword("");
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const addDbMutation = useMutation({
    mutationFn: (vars: { username: string; database: string }) => addUserDatabase(vars.username, vars.database),
    onSuccess: () => {
      toast.success("Database granted");
      setAddDbOpen(false);
      setAddDbTarget(null);
      setAddDbName("");
      setAddDbError(null);
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const removeDbMutation = useMutation({
    mutationFn: (vars: { username: string; database: string }) => removeUserDatabase(vars.username, vars.database),
    onSuccess: () => {
      toast.success("Database removed");
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  // Auth User Mutations
  const createAuthMutation = useMutation({
    mutationFn: (vars: { username: string; password?: string; role: "admin" | "dev" | "viewer"; databases?: string[] }) =>
      createAuthUser(vars.username, vars.password || "", vars.role, vars.databases),
    onSuccess: (_data, vars) => {
      toast.success("Auth user created");
      setAuthCreateOpen(false);
      setAuthCreateUsername("");
      setAuthCreatePassword("");
      setAuthCreateRole("viewer");
      setAuthCreateDatabases([]);
      setAuthCreateError(null);
      queryClient.invalidateQueries({ queryKey: ["authUsers"] });
      if (vars.password) {
        setAuthShowCreds({ username: vars.username, password: vars.password });
      }
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const updateAuthMutation = useMutation({
    mutationFn: (vars: { username: string; role: "admin" | "dev" | "viewer"; databases?: string[] }) => updateAuthUser(vars.username, vars.role, vars.databases),
    onSuccess: () => {
      toast.success("Auth user updated");
      setAuthEditOpen(false);
      setAuthEditTarget(null);
      queryClient.invalidateQueries({ queryKey: ["authUsers"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const deleteAuthMutation = useMutation({
    mutationFn: (username: string) => deleteAuthUser(username),
    onSuccess: () => {
      toast.success("Auth user deleted");
      queryClient.invalidateQueries({ queryKey: ["authUsers"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  const resetAuthMutation = useMutation({
    mutationFn: (vars: { username: string; password?: string }) => resetAuthUserPassword(vars.username, vars.password),
    onSuccess: (password) => {
      toast.success("Password reset");
      setAuthResetPassword(password);
      queryClient.invalidateQueries({ queryKey: ["authUsers"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  function resetForm() {
    setFormDbs([]);
    setFormUsername("");
    setFormAccess("write");
    setFormError(null);
  }

  function handleCreate(e: React.FormEvent) {
    e.preventDefault();
    const result = CreateUserRequestSchema.safeParse({ databases: formDbs, username: formUsername, access: formAccess });
    if (!result.success) {
      setFormError(result.error.errors[0]?.message ?? "Invalid input");
      return;
    }
    setFormError(null);
    createMutation.mutate({ username: result.data.username, databases: result.data.databases, access: result.data.access });
  }

  function handleEdit(e: React.FormEvent) {
    e.preventDefault();
    if (!editTarget) return;
    const result = UpdateUserRequestSchema.safeParse({ password: editPassword || undefined, access: editAccess });
    if (!result.success) {
      toast.error(result.error.errors[0]?.message ?? "Invalid input");
      return;
    }
    const vars: { username: string; password?: string; access?: "read" | "write" | "ddl" | "full" } = { username: editTarget.username };
    if (result.data.password) vars.password = result.data.password;
    if (result.data.access) vars.access = result.data.access;
    updateMutation.mutate(vars);
  }

  function handleAddDb(e: React.FormEvent) {
    e.preventDefault();
    if (!addDbTarget) return;
    const result = AddDatabaseRequestSchema.safeParse({ database: addDbName });
    if (!result.success) {
      setAddDbError(result.error.errors[0]?.message ?? "Invalid input");
      return;
    }
    setAddDbError(null);
    addDbMutation.mutate({ username: addDbTarget.username, database: result.data.database });
  }

  function handleAuthCreate(e: React.FormEvent) {
    e.preventDefault();
    const vars: { username: string; password?: string; role: "admin" | "dev" | "viewer"; databases?: string[] } = {
      username: authCreateUsername,
      role: authCreateRole,
    };
    if (authCreatePassword) vars.password = authCreatePassword;
    if (authCreateRole === "dev" && authCreateDatabases.length > 0) vars.databases = authCreateDatabases;

    const result = CreateAuthUserRequestSchema.safeParse({
      username: vars.username,
      password: vars.password || undefined,
      role: vars.role,
      databases: vars.databases,
    });
    if (!result.success) {
      setAuthCreateError(result.error.errors[0]?.message ?? "Invalid input");
      return;
    }
    setAuthCreateError(null);
    createAuthMutation.mutate(vars);
  }

  function handleAuthEdit(e: React.FormEvent) {
    e.preventDefault();
    if (!authEditTarget) return;
    const vars: { username: string; role: "admin" | "dev" | "viewer"; databases?: string[] } = {
      username: authEditTarget.username,
      role: authEditRole,
    };
    if (authEditRole === "dev" && authEditDatabases.length > 0) vars.databases = authEditDatabases;
    updateAuthMutation.mutate(vars);
  }

  function copyText(text: string) {
    navigator.clipboard.writeText(text).then(() => toast.success("Copied to clipboard"));
  }

  const accessColors: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
    read: "default",
    write: "secondary",
    ddl: "outline",
    full: "destructive",
  };

  const accessLabels: Record<string, string> = {
    read: "SELECT",
    write: "SELECT, INSERT, UPDATE, DELETE",
    ddl: "Write + CREATE, ALTER, DROP",
    full: "ALL PRIVILEGES",
  };

  return (
    <div className="space-y-12">
      {/* Postgres Users Section */}
      <div className="space-y-4">
        <div>
          <h2 className="text-xl font-semibold">Postgres Users</h2>
          <p className="text-sm text-muted-foreground">Manage database users and their access controls.</p>
        </div>
        <div className="flex flex-col sm:flex-row sm:items-center gap-4 sm:gap-2">
          <Button onClick={() => setCreateOpen(true)}>
            <Plus className="mr-2 h-4 w-4" /> Create User
          </Button>
          <Button variant="outline" onClick={() => queryClient.invalidateQueries({ queryKey: ["users"] })}>
            <RefreshCw className="mr-2 h-4 w-4" /> Refresh
          </Button>
        </div>

        <div className="rounded-md border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Username</TableHead>
                <TableHead>Databases</TableHead>
                <TableHead className="hidden sm:table-cell">Access</TableHead>
                <TableHead className="hidden md:table-cell">Created</TableHead>
                <TableHead className="w-25"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {isLoading ? (
                <TableRow>
                  <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">Loading...</TableCell>
                </TableRow>
              ) : !users?.length ? (
                <TableRow>
                  <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">No users found.</TableCell>
                </TableRow>
              ) : (
                users.map((user: User, i: number) => (
                  <TableRow 
                    key={user.username}
                    className="animate-in fade-in slide-in-from-bottom-2 duration-300 fill-mode-both"
                    style={{ animationDelay: `${i * 50}ms` }}
                  >
                    <TableCell className="font-medium">{user.username}</TableCell>
                    <TableCell>
                      <div className="flex flex-wrap gap-2">
                        {user.databases.map((db) => (
                          <Badge key={db} variant="secondary" className="pr-1 cursor-default">
                            {db}
                            <button
                              onClick={() => removeDbMutation.mutate({ username: user.username, database: db })}
                              className="ml-1 rounded-full p-0.5 hover:bg-muted focus:outline-none"
                            >
                              <Trash2 className="h-3 w-3" />
                            </button>
                          </Badge>
                        ))}
                        <Badge
                          variant="outline"
                          className="cursor-pointer border-dashed hover:bg-accent"
                          onClick={() => {
                            setAddDbTarget(user);
                            setAddDbName("");
                            setAddDbOpen(true);
                          }}
                        >
                          <Plus className="mr-1 h-3 w-3" /> Add
                        </Badge>
                      </div>
                    </TableCell>
                    <TableCell className="hidden sm:table-cell">
                      <Badge variant={accessColors[user.access]}>{user.access.toUpperCase()}</Badge>
                    </TableCell>
                    <TableCell className="hidden md:table-cell">{new Date(user.createdAt).toLocaleDateString()}</TableCell>
                    <TableCell>
                      <div className="flex gap-2">
                        <Button variant="ghost" size="icon" onClick={() => {
                          setEditTarget(user);
                          setEditAccess(user.access);
                          setEditPassword("");
                          setEditOpen(true);
                        }}>
                          <Edit className="h-4 w-4" />
                        </Button>
                        <Button variant="ghost" size="icon" className="text-destructive" onClick={() => {
                          setDeleteTarget(user);
                          setDeleteConfirmText("");
                        }}>
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </div>

      {/* Auth Users Section */}
      <div className="space-y-4">
        <div>
          <h2 className="text-xl font-semibold">Auth Users</h2>
          <p className="text-sm text-muted-foreground">Manage users who can log into this dashboard.</p>
        </div>
        <div className="flex flex-col sm:flex-row sm:items-center gap-4 sm:gap-2">
          <Button onClick={() => setAuthCreateOpen(true)}>
            <Plus className="mr-2 h-4 w-4" /> Create Auth User
          </Button>
          <Button variant="outline" onClick={() => queryClient.invalidateQueries({ queryKey: ["authUsers"] })}>
            <RefreshCw className="mr-2 h-4 w-4" /> Refresh
          </Button>
        </div>

        <div className="rounded-md border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Username</TableHead>
                <TableHead>Role</TableHead>
                <TableHead className="hidden sm:table-cell">Databases</TableHead>
                <TableHead className="hidden sm:table-cell">Created</TableHead>
                <TableHead className="w-30"></TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {authLoading ? (
                <TableRow>
                  <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">Loading...</TableCell>
                </TableRow>
              ) : !authUsers?.length ? (
                <TableRow>
                  <TableCell colSpan={5} className="h-24 text-center text-muted-foreground">No auth users found.</TableCell>
                </TableRow>
              ) : (
                authUsers.map((authUser: AuthUserListItem, i: number) => (
                  <TableRow 
                    key={authUser.id}
                    className="animate-in fade-in slide-in-from-bottom-2 duration-300 fill-mode-both"
                    style={{ animationDelay: `${i * 50}ms` }}
                  >
                    <TableCell className="font-medium">{authUser.username}</TableCell>
                    <TableCell>
                      <Badge variant={authUser.role === "admin" ? "destructive" : authUser.role === "dev" ? "outline" : "secondary"}>
                        {authUser.role.toUpperCase()}
                      </Badge>
                    </TableCell>
                    <TableCell className="hidden sm:table-cell">
                      {authUser.role === "dev" && authUser.databases && authUser.databases.length > 0 ? (
                        <div className="flex flex-wrap gap-1">
                          {authUser.databases.map((db) => (
                            <Badge key={db} variant="secondary" className="text-xs">{db}</Badge>
                          ))}
                        </div>
                      ) : (
                        <span className="text-muted-foreground text-sm">—</span>
                      )}
                    </TableCell>
                    <TableCell className="hidden sm:table-cell">{new Date(authUser.createdAt).toLocaleDateString()}</TableCell>
                    <TableCell>
                      <div className="flex gap-2">
                        <Button variant="ghost" size="icon" onClick={() => {
                          setAuthEditTarget(authUser);
                          setAuthEditRole(authUser.role);
                          setAuthEditDatabases(authUser.databases || []);
                          setAuthEditOpen(true);
                        }}>
                          <Edit className="h-4 w-4" />
                        </Button>
                        <TooltipProvider>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button variant="ghost" size="icon" onClick={() => {
                                setAuthResetTarget(authUser);
                                setAuthResetPassword(null);
                                setAuthResetInput("");
                                setAuthResetOpen(true);
                              }}>
                                <Key className="h-4 w-4" />
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent>Reset password</TooltipContent>
                          </Tooltip>
                        </TooltipProvider>
                        <Button variant="ghost" size="icon" className="text-destructive" onClick={() => {
                          setAuthDeleteTarget(authUser);
                          setAuthDeleteConfirmText("");
                        }}>
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
        </div>
      </div>

      {/* DIALOGS */}
      
      {/* Create Postgres User */}
      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create Database User</DialogTitle>
          </DialogHeader>
          <form onSubmit={handleCreate}>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label>Username</Label>
                <Input placeholder="e.g. app_user" value={formUsername} onChange={(e) => setFormUsername(e.target.value)} required />
              </div>
              <div className="space-y-2">
                <Label>Access Level</Label>
                <RadioGroup value={formAccess} onValueChange={(val: any) => setFormAccess(val)} className="grid grid-cols-2 gap-3 pt-2">
                  {(["read", "write", "ddl", "full"] as const).map((level) => (
                    <RadioGroupItem key={level} value={level} id={`access-${level}`}>
                      <div className="flex items-center justify-between mb-1">
                        <span className="font-bold text-sm tracking-wider uppercase group-data-[state=checked]:text-primary">{level}</span>
                        <div className="h-2 w-2 rounded-full bg-primary opacity-0 group-data-[state=checked]:opacity-100 transition-opacity" />
                      </div>
                      <span className="text-[10px] sm:text-xs text-muted-foreground leading-snug">{accessLabels[level]}</span>
                    </RadioGroupItem>
                  ))}
                </RadioGroup>
              </div>
              <DbMultiSelect
                databases={databases || []}
                selected={formDbs}
                onChange={setFormDbs}
              />
              {formError && <div className="text-sm text-destructive">{formError}</div>}
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setCreateOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={createMutation.isPending || formDbs.length === 0}>Create</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Show Credentials */}
      <Dialog open={showCreds !== null} onOpenChange={() => setShowCreds(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>User Created</DialogTitle>
            <DialogDescription>Save these credentials — the password cannot be shown again.</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 font-mono text-sm">
            <div>
              <div className="text-xs text-muted-foreground">USERNAME</div>
              <div className="flex items-center gap-2">
                <span>{showCreds?.username}</span>
                <Copy className="h-4 w-4 cursor-pointer text-muted-foreground" onClick={() => copyText(showCreds?.username ?? "")} />
              </div>
            </div>
            <div>
              <div className="text-xs text-muted-foreground">PASSWORD</div>
              <div className="flex items-center gap-2 text-destructive">
                <span>{showCreds?.password}</span>
                <Copy className="h-4 w-4 cursor-pointer text-muted-foreground" onClick={() => copyText(showCreds?.password ?? "")} />
              </div>
            </div>
            <div>
              <div className="text-xs text-muted-foreground">CONNECTION STRING</div>
              <div className="flex items-center gap-2">
                <span className="break-all">{showCreds?.connectionString}</span>
                <Copy className="h-4 w-4 cursor-pointer text-muted-foreground" onClick={() => copyText(showCreds?.connectionString ?? "")} />
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button onClick={() => setShowCreds(null)}>Done</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Edit Postgres User */}
      <Dialog open={editOpen} onOpenChange={setEditOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit User — {editTarget?.username}</DialogTitle>
          </DialogHeader>
          <form onSubmit={handleEdit}>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label>New Password (leave blank to keep current)</Label>
                <div className="flex gap-2">
                  <Input type="text" placeholder="8-128 chars" value={editPassword} onChange={(e) => setEditPassword(e.target.value)} />
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => {
                      const rand = Array.from(crypto.getRandomValues(new Uint8Array(12)))
                        .map((b) => "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"[b % 62])
                        .join("");
                      setEditPassword(rand);
                    }}
                  >
                    Generate
                  </Button>
                </div>
              </div>
              <div className="space-y-2">
                <Label>Access Level</Label>
                <RadioGroup value={editAccess} onValueChange={(val: any) => setEditAccess(val)} className="grid grid-cols-2 gap-3 pt-2">
                  {(["read", "write", "ddl", "full"] as const).map((level) => (
                    <RadioGroupItem key={level} value={level} id={`edit-access-${level}`}>
                      <div className="flex items-center justify-between mb-1">
                        <span className="font-bold text-sm tracking-wider uppercase group-data-[state=checked]:text-primary">{level}</span>
                        <div className="h-2 w-2 rounded-full bg-primary opacity-0 group-data-[state=checked]:opacity-100 transition-opacity" />
                      </div>
                      <span className="text-[10px] sm:text-xs text-muted-foreground leading-snug">{accessLabels[level]}</span>
                    </RadioGroupItem>
                  ))}
                </RadioGroup>
              </div>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setEditOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={updateMutation.isPending}>Save</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Add Database Access */}
      <Dialog open={addDbOpen} onOpenChange={setAddDbOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add Database — {addDbTarget?.username}</DialogTitle>
          </DialogHeader>
          <form onSubmit={handleAddDb}>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label>Database</Label>
                <Select value={addDbName} onValueChange={setAddDbName}>
                  <SelectTrigger>
                    <SelectValue placeholder="Select database" />
                  </SelectTrigger>
                  <SelectContent>
                    {databases?.map((d: Database) => (
                      <SelectItem key={d.name} value={d.name}>{d.name}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {addDbError && <div className="text-sm text-destructive">{addDbError}</div>}
              </div>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setAddDbOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={addDbMutation.isPending}>Grant Access</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Create Auth User */}
      <Dialog open={authCreateOpen} onOpenChange={setAuthCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create Auth User</DialogTitle>
          </DialogHeader>
          <form onSubmit={handleAuthCreate}>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label>Username</Label>
                <Input placeholder="e.g. admin_user" value={authCreateUsername} onChange={(e) => setAuthCreateUsername(e.target.value)} required />
              </div>
              <div className="space-y-2">
                <Label>Password (leave empty to auto-generate)</Label>
                <Input type="password" placeholder="password" value={authCreatePassword} onChange={(e) => setAuthCreatePassword(e.target.value)} />
              </div>
              <div className="space-y-2">
                <Label>Role</Label>
                <RadioGroup value={authCreateRole} onValueChange={(val: any) => {
                  setAuthCreateRole(val);
                  if (val !== "dev") setAuthCreateDatabases([]);
                }} className="grid grid-cols-3 gap-3 pt-2">
                  <RadioGroupItem value="admin" id="role-admin">
                    <div className="flex items-center justify-between mb-1">
                      <span className="font-bold text-sm tracking-wider uppercase group-data-[state=checked]:text-primary">Admin</span>
                      <div className="h-2 w-2 rounded-full bg-primary opacity-0 group-data-[state=checked]:opacity-100 transition-opacity" />
                    </div>
                    <span className="text-[10px] sm:text-xs text-muted-foreground leading-snug">Full access</span>
                  </RadioGroupItem>
                  <RadioGroupItem value="dev" id="role-dev">
                    <div className="flex items-center justify-between mb-1">
                      <span className="font-bold text-sm tracking-wider uppercase group-data-[state=checked]:text-primary">Dev</span>
                      <div className="h-2 w-2 rounded-full bg-primary opacity-0 group-data-[state=checked]:opacity-100 transition-opacity" />
                    </div>
                    <span className="text-[10px] sm:text-xs text-muted-foreground leading-snug">Assigned DBs only</span>
                  </RadioGroupItem>
                  <RadioGroupItem value="viewer" id="role-viewer">
                    <div className="flex items-center justify-between mb-1">
                      <span className="font-bold text-sm tracking-wider uppercase group-data-[state=checked]:text-primary">Viewer</span>
                      <div className="h-2 w-2 rounded-full bg-primary opacity-0 group-data-[state=checked]:opacity-100 transition-opacity" />
                    </div>
                    <span className="text-[10px] sm:text-xs text-muted-foreground leading-snug">Read-only</span>
                  </RadioGroupItem>
                </RadioGroup>
              </div>
              {authCreateRole === "dev" && (
                <DbMultiSelect
                  databases={databases || []}
                  selected={authCreateDatabases}
                  onChange={setAuthCreateDatabases}
                />
              )}
              {authCreateError && <div className="text-sm text-destructive">{authCreateError}</div>}
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setAuthCreateOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={createAuthMutation.isPending || (authCreateRole === "dev" && authCreateDatabases.length === 0)}>Create</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Edit Auth User */}
      <Dialog open={authEditOpen} onOpenChange={setAuthEditOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Edit Auth User — {authEditTarget?.username}</DialogTitle>
          </DialogHeader>
          <form onSubmit={handleAuthEdit}>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label>Role</Label>
                <RadioGroup value={authEditRole} onValueChange={(val: any) => {
                  setAuthEditRole(val);
                  if (val !== "dev") setAuthEditDatabases([]);
                }} className="grid grid-cols-3 gap-3 pt-2">
                  <RadioGroupItem value="admin" id="edit-role-admin">
                    <div className="flex items-center justify-between mb-1">
                      <span className="font-bold text-sm tracking-wider uppercase group-data-[state=checked]:text-primary">Admin</span>
                      <div className="h-2 w-2 rounded-full bg-primary opacity-0 group-data-[state=checked]:opacity-100 transition-opacity" />
                    </div>
                    <span className="text-[10px] sm:text-xs text-muted-foreground leading-snug">Full access</span>
                  </RadioGroupItem>
                  <RadioGroupItem value="dev" id="edit-role-dev">
                    <div className="flex items-center justify-between mb-1">
                      <span className="font-bold text-sm tracking-wider uppercase group-data-[state=checked]:text-primary">Dev</span>
                      <div className="h-2 w-2 rounded-full bg-primary opacity-0 group-data-[state=checked]:opacity-100 transition-opacity" />
                    </div>
                    <span className="text-[10px] sm:text-xs text-muted-foreground leading-snug">Assigned DBs only</span>
                  </RadioGroupItem>
                  <RadioGroupItem value="viewer" id="edit-role-viewer">
                    <div className="flex items-center justify-between mb-1">
                      <span className="font-bold text-sm tracking-wider uppercase group-data-[state=checked]:text-primary">Viewer</span>
                      <div className="h-2 w-2 rounded-full bg-primary opacity-0 group-data-[state=checked]:opacity-100 transition-opacity" />
                    </div>
                    <span className="text-[10px] sm:text-xs text-muted-foreground leading-snug">Read-only</span>
                  </RadioGroupItem>
                </RadioGroup>
              </div>
              {authEditRole === "dev" && (
                <DbMultiSelect
                  databases={databases || []}
                  selected={authEditDatabases}
                  onChange={setAuthEditDatabases}
                />
              )}
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setAuthEditOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={updateAuthMutation.isPending || (authEditRole === "dev" && authEditDatabases.length === 0)}>Save</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Show Auth Credentials */}
      <Dialog open={authShowCreds !== null} onOpenChange={() => setAuthShowCreds(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Auth User Created</DialogTitle>
            <DialogDescription>Save these credentials — the password cannot be shown again.</DialogDescription>
          </DialogHeader>
          <div className="space-y-4 font-mono text-sm">
            <div>
              <div className="text-xs text-muted-foreground">USERNAME</div>
              <div className="flex items-center gap-2">
                <span>{authShowCreds?.username}</span>
                <Copy className="h-4 w-4 cursor-pointer text-muted-foreground" onClick={() => copyText(authShowCreds?.username ?? "")} />
              </div>
            </div>
            <div>
              <div className="text-xs text-muted-foreground">PASSWORD</div>
              <div className="flex items-center gap-2 text-destructive">
                <span>{authShowCreds?.password}</span>
                <Copy className="h-4 w-4 cursor-pointer text-muted-foreground" onClick={() => copyText(authShowCreds?.password ?? "")} />
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button onClick={() => setAuthShowCreds(null)}>Done</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Reset Auth Password */}
      <Dialog open={authResetOpen} onOpenChange={setAuthResetOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Reset Password — {authResetTarget?.username}</DialogTitle>
            <DialogDescription>
              {authResetPassword === null 
                ? "Set a new password or leave blank to generate a highly secure random one. They will be logged out immediately." 
                : "Save this password — it cannot be shown again."}
            </DialogDescription>
          </DialogHeader>
          
          {authResetPassword === null && (
            <div className="grid gap-4 py-4">
              <div className="grid gap-2">
                <Label htmlFor="auth-reset-password">New Password (Optional)</Label>
                <div className="flex gap-2">
                  <Input
                    id="auth-reset-password"
                    placeholder="Leave blank to generate randomly"
                    type="text"
                    value={authResetInput}
                    onChange={(e) => setAuthResetInput(e.target.value)}
                  />
                  <Button
                    type="button"
                    variant="outline"
                    onClick={() => {
                      const rand = Array.from(crypto.getRandomValues(new Uint8Array(16)))
                        .map((b) => "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"[b % 62])
                        .join("");
                      setAuthResetInput(rand);
                    }}
                  >
                    Generate
                  </Button>
                </div>
              </div>
            </div>
          )}

          {authResetPassword !== null && (
            <div className="font-mono text-sm">
              <div className="text-xs text-muted-foreground">PASSWORD</div>
              <div className="flex items-center gap-2 text-destructive">
                <span>{authResetPassword}</span>
                <Copy className="h-4 w-4 cursor-pointer text-muted-foreground" onClick={() => copyText(authResetPassword)} />
              </div>
            </div>
          )}
          <DialogFooter>
            {authResetPassword === null ? (
              <>
                <Button variant="outline" onClick={() => setAuthResetOpen(false)}>Cancel</Button>
                <Button disabled={resetAuthMutation.isPending} onClick={() => {
                  if (!authResetTarget) return;
                  const payload: { username: string; password?: string } = { username: authResetTarget.username };
                  if (authResetInput) payload.password = authResetInput;
                  resetAuthMutation.mutate(payload);
                }}>
                  Reset Password
                </Button>
              </>
            ) : (
              <Button onClick={() => setAuthResetOpen(false)}>Done</Button>
            )}
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Postgres User */}
      <Dialog open={!!deleteTarget} onOpenChange={(open) => !open && setDeleteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete User</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete the user "{deleteTarget?.username}"? This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="py-4 space-y-4">
            <div className="space-y-2">
              <Label>Type DELETE to confirm</Label>
              <Input 
                value={deleteConfirmText} 
                onChange={(e) => setDeleteConfirmText(e.target.value)}
                placeholder="DELETE"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTarget(null)}>Cancel</Button>
            <Button 
              variant="destructive" 
              disabled={deleteMutation.isPending || deleteConfirmText !== "DELETE"}
              onClick={() => {
                if (deleteTarget) {
                  deleteMutation.mutate(deleteTarget.username);
                  setDeleteTarget(null);
                }
              }}
            >
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Delete Auth User */}
      <Dialog open={!!authDeleteTarget} onOpenChange={(open) => !open && setAuthDeleteTarget(null)}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Auth User</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete auth user "{authDeleteTarget?.username}"? This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <div className="py-4 space-y-4">
            <div className="space-y-2">
              <Label>Type DELETE to confirm</Label>
              <Input 
                value={authDeleteConfirmText} 
                onChange={(e) => setAuthDeleteConfirmText(e.target.value)}
                placeholder="DELETE"
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setAuthDeleteTarget(null)}>Cancel</Button>
            <Button 
              variant="destructive" 
              disabled={deleteAuthMutation.isPending || authDeleteConfirmText !== "DELETE"}
              onClick={() => {
                if (authDeleteTarget) {
                  deleteAuthMutation.mutate(authDeleteTarget.username);
                  setAuthDeleteTarget(null);
                }
              }}
            >
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
