import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Plus, Trash2, RefreshCw, Copy, Edit, Key, Dices, X } from "lucide-react";
import {
  fetchUsers,
  fetchDatabases,
  createUser,
  deleteUser,
  updateUser,
  fetchAuthUsers,
  createAuthUser,
  updateAuthUser,
  deleteAuthUser,
  resetAuthUserPassword,
} from "../api/client";
import {
  CreateUserRequestSchema,
  UpdateUserRequestSchema,
  CreateAuthUserRequestSchema,
} from "../lib/schemas";
import type { User, AuthUserListItem, Database } from "../lib/schemas";

import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";
import { Label } from "../components/ui/label";
import { RadioGroup, RadioGroupItem } from "../components/ui/radio-group";

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

function IpInput({ ips, onChange }: { ips: string[], onChange: (ips: string[]) => void }) {
  const [input, setInput] = useState("");

  const addIp = () => {
    const val = input.trim();
    if (val && !ips.includes(val)) {
      onChange([...ips, val]);
    }
    setInput("");
  };

  const removeIp = (ipToRemove: string) => {
    onChange(ips.filter(ip => ip !== ipToRemove));
  };

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap gap-2 mb-2 min-h-7">
        {ips.length === 0 && <span className="text-xs text-muted-foreground pt-1">Any IP (0.0.0.0/0)</span>}
        {ips.map(ip => (
          <Badge key={ip} variant="secondary" className="pl-2 pr-1 py-1 flex items-center gap-1 font-mono text-[10px]">
            {ip}
            <button
              type="button"
              onClick={() => removeIp(ip)}
              className="rounded-full hover:bg-muted p-0.5 ml-1"
            >
              <X className="h-3 w-3" />
            </button>
          </Badge>
        ))}
      </div>
      <div className="flex gap-2">
        <Input 
          value={input} 
          onChange={e => setInput(e.target.value)}
          onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); addIp(); } }}
          placeholder="e.g. 192.168.0.10 or 10.0.0.0/24"
          className="font-mono text-sm h-9"
        />
        <Button type="button" variant="outline" className="h-9" onClick={addIp}>Add</Button>
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
  const [editDatabases, setEditDatabases] = useState<string[]>([]);
  const [editAllowedIps, setEditAllowedIps] = useState<string[]>([]);

  const [resetOpen, setResetOpen] = useState(false);
  const [resetTarget, setResetTarget] = useState<User | null>(null);
  const [resetPassword, setResetPassword] = useState("");
  const [resetGenerate, setResetGenerate] = useState(false);
  const [resetResult, setResetResult] = useState<string | null>(null);

  const [deleteTarget, setDeleteTarget] = useState<User | null>(null);
  const [deleteConfirmText, setDeleteConfirmText] = useState("");

  const [formAllowedIps, setFormAllowedIps] = useState<string[]>([]);

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
    mutationFn: (vars: { username: string; databases: string[]; access: "read" | "write" | "ddl" | "full"; password?: string; allowedIps?: string[] }) =>
      createUser(vars.username, vars.databases, vars.access, vars.password, vars.allowedIps),
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
    mutationFn: (vars: { username: string; password?: string; access?: "read" | "write" | "ddl" | "full"; generatePassword?: boolean; allowedIps?: string[]; databases?: string[] }) => {
      const opts: { password?: string; access?: "read" | "write" | "ddl" | "full"; generatePassword?: boolean; allowedIps?: string[]; databases?: string[] } = {};
      if (vars.password) opts.password = vars.password;
      if (vars.access) opts.access = vars.access;
      if (vars.generatePassword) opts.generatePassword = vars.generatePassword;
      if (vars.allowedIps) opts.allowedIps = vars.allowedIps;
      if (vars.databases) opts.databases = vars.databases;
      return updateUser(vars.username, opts);
    },
    onSuccess: (data, vars) => {
      toast.success("User updated");
      setEditOpen(false);
      setResetOpen(false);
      
      const finalPassword = data?.password || vars.password;
      if (finalPassword) {
        setResetResult(finalPassword);
      }
      setEditTarget(null);
      setResetTarget(null);
      setResetPassword("");
      setResetGenerate(false);
      queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (error: Error) => toast.error(error.message),
  });

  // Auth User Mutations
  const createAuthMutation = useMutation({
    mutationFn: (vars: { username: string; password?: string; role: "admin" | "dev" | "viewer"; databases?: string[] }) =>
      createAuthUser(vars.username, vars.password || "", vars.role, vars.databases),
    onSuccess: (data) => {
      toast.success("Auth user created");
      setAuthCreateOpen(false);
      setAuthCreateUsername("");
      setAuthCreatePassword("");
      setAuthCreateRole("viewer");
      setAuthCreateDatabases([]);
      setAuthCreateError(null);
      queryClient.invalidateQueries({ queryKey: ["authUsers"] });
      setAuthShowCreds({ username: data.username, password: data.password });
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
    setFormAllowedIps([]);
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
    const allowedIps = formAllowedIps.length > 0 ? formAllowedIps : undefined;

    const vars: { username: string; databases: string[]; access: "read" | "write" | "ddl" | "full"; allowedIps?: string[] } = {
      username: result.data.username,
      databases: result.data.databases,
      access: result.data.access,
    };
    if (allowedIps) {
      vars.allowedIps = allowedIps;
    }
    createMutation.mutate(vars);
  }

  function handleEdit(e: React.FormEvent) {
    e.preventDefault();
    if (!editTarget) return;
    const result = UpdateUserRequestSchema.safeParse({ access: editAccess, databases: editDatabases });
    if (!result.success) {
      toast.error(result.error.errors[0]?.message ?? "Invalid input");
      return;
    }
    const vars: { username: string; access?: "read" | "write" | "ddl" | "full"; allowedIps?: string[]; databases?: string[] } = { username: editTarget.username };
    if (result.data.access) vars.access = result.data.access;
    if (result.data.databases) vars.databases = result.data.databases;
    
    vars.allowedIps = editAllowedIps.length > 0 ? editAllowedIps : ["0.0.0.0/0"];
    
    updateMutation.mutate(vars);
  }

  function handleReset(e: React.FormEvent) {
    e.preventDefault();
    if (!resetTarget) return;
    const result = UpdateUserRequestSchema.safeParse({ password: resetPassword || undefined, generatePassword: resetGenerate });
    if (!result.success) {
      toast.error(result.error.errors[0]?.message ?? "Invalid input");
      return;
    }
    const vars: { username: string; password?: string; generatePassword?: boolean } = { username: resetTarget.username };
    if (result.data.password) vars.password = result.data.password;
    if (result.data.generatePassword) vars.generatePassword = result.data.generatePassword;
    
    updateMutation.mutate(vars);
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
                <TableHead className="hidden lg:table-cell">Allowed IPs</TableHead>
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
                          <Badge key={db} variant="secondary" className="cursor-default">
                            {db}
                          </Badge>
                        ))}
                      </div>
                    </TableCell>
                    <TableCell className="hidden sm:table-cell">
                      <Badge variant={accessColors[user.access]}>{user.access.toUpperCase()}</Badge>
                    </TableCell>
                    <TableCell className="hidden lg:table-cell">
                      <div className="flex flex-wrap gap-1">
                        {(user.allowedIps ?? ["0.0.0.0/0"]).map((ip) => (
                          <Badge key={ip} variant={ip === "0.0.0.0/0" ? "outline" : "secondary"} className="text-xs font-mono">
                            {ip === "0.0.0.0/0" ? "any" : ip}
                          </Badge>
                        ))}
                      </div>
                    </TableCell>
                    <TableCell className="hidden md:table-cell">{new Date(user.createdAt).toLocaleDateString()}</TableCell>
                    <TableCell>
                      <div className="flex gap-2">
                        <Button variant="ghost" size="icon" title="Reset Password" onClick={() => {
                          setResetTarget(user);
                          setResetPassword("");
                          setResetGenerate(false);
                          setResetOpen(true);
                        }}>
                          <Key className="h-4 w-4" />
                        </Button>
                        <Button variant="ghost" size="icon" onClick={() => {
                          setEditTarget(user);
                          setEditAccess(user.access);
                          setEditDatabases(user.databases || []);
                          setEditAllowedIps(user.allowedIps ?? []);
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
              <div className="space-y-2">
                <Label>Allowed IPs</Label>
                <IpInput ips={formAllowedIps} onChange={setFormAllowedIps} />
              </div>
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
              <DbMultiSelect
                databases={databases || []}
                selected={editDatabases}
                onChange={setEditDatabases}
              />
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
              <div className="space-y-2">
                <Label>Allowed IPs</Label>
                <IpInput ips={editAllowedIps} onChange={setEditAllowedIps} />
              </div>
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setEditOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={updateMutation.isPending}>Save</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      {/* Reset Password */}
      <Dialog open={resetOpen} onOpenChange={setResetOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Reset Password — {resetTarget?.username}</DialogTitle>
          </DialogHeader>
          {resetResult ? (
            <div className="space-y-4 py-4">
              <div className="p-4 border rounded-lg bg-muted/50 space-y-3">
                <div className="space-y-1">
                  <Label className="text-muted-foreground text-xs uppercase tracking-wider">New Password</Label>
                  <div className="flex gap-2">
                    <Input readOnly value={resetResult} className="font-mono bg-background" />
                    <Button variant="outline" size="icon" onClick={() => copyText(resetResult)}>
                      <Copy className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
                <p className="text-sm text-destructive font-medium">Please copy this password now. You won't be able to see it again!</p>
              </div>
              <DialogFooter>
                <Button onClick={() => setResetOpen(false)}>Done</Button>
              </DialogFooter>
            </div>
          ) : (
            <form onSubmit={handleReset}>
              <div className="space-y-4 py-4">
                <div className="space-y-2">
                  <Label>New Password</Label>
                  <div className="flex gap-2">
                    <Input
                      type="password"
                      placeholder={resetGenerate ? "Will be auto-generated" : "Enter new password"}
                      value={resetPassword}
                      onChange={(e) => setResetPassword(e.target.value)}
                      disabled={resetGenerate}
                    />
                    <Button
                      type="button"
                      variant={resetGenerate ? "default" : "outline"}
                      size="icon"
                      onClick={() => {
                        setResetGenerate(!resetGenerate);
                        if (!resetGenerate) setResetPassword("");
                      }}
                      title="Auto-generate password"
                    >
                      <Dices className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              </div>
              <DialogFooter>
                <Button type="button" variant="outline" onClick={() => setResetOpen(false)}>Cancel</Button>
                <Button type="submit" disabled={updateMutation.isPending}>Reset Password</Button>
              </DialogFooter>
            </form>
          )}
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
                <Label>Password (Optional)</Label>
                <Input type="password" placeholder="Leave blank to auto-generate password" value={authCreatePassword} onChange={(e) => setAuthCreatePassword(e.target.value)} />
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
                <Input
                  id="auth-reset-password"
                  placeholder="Leave blank to auto-generate password"
                  type="password"
                  value={authResetInput}
                  onChange={(e) => setAuthResetInput(e.target.value)}
                />
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
