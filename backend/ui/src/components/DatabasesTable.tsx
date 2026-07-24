import { useState } from "react";
import { Link } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Plus, Trash2, RefreshCw, Eye, EyeOff } from "lucide-react";
import { fetchDatabases, createDatabase, deleteDatabase } from "../api/client";
import { CreateDatabaseSchema } from "../lib/schemas";
import type { Database } from "../lib/schemas";
import { useAuth } from "../contexts/AuthContext";

import { Button } from "./ui/button";
import { Input } from "./ui/input";
import { Label } from "./ui/label";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "./ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";

export default function DatabasesTable() {
  const [showSystem, setShowSystem] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [deleteConfirmText, setDeleteConfirmText] = useState("");
  const [newName, setNewName] = useState("");
  const [nameError, setNameError] = useState<string | null>(null);

  const queryClient = useQueryClient();
  const { isAdmin } = useAuth();

  const { data: databases, isLoading, refetch } = useQuery({
    queryKey: ["databases", showSystem],
    queryFn: () => fetchDatabases(showSystem),
  });

  const createMutation = useMutation({
    mutationFn: (name: string) => createDatabase(name),
    onSuccess: () => {
      toast.success("Database created");
      setCreateOpen(false);
      setNewName("");
      setNameError(null);
      queryClient.invalidateQueries({ queryKey: ["databases"] });
    },
    onError: (error: Error) => {
      toast.error(error.message);
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (name: string) => deleteDatabase(name),
    onSuccess: () => {
      toast.success("Database deleted");
      setDeleteTarget(null);
      queryClient.invalidateQueries({ queryKey: ["databases"] });
    },
    onError: (error: Error) => {
      toast.error(error.message);
      setDeleteTarget(null);
    },
  });

  function handleCreate(e?: React.FormEvent) {
    e?.preventDefault();
    const result = CreateDatabaseSchema.safeParse({ name: newName });
    if (!result.success) {
      const firstError = result.error.errors[0];
      setNameError(firstError?.message ?? "Invalid name");
      return;
    }
    setNameError(null);
    createMutation.mutate(result.data.name);
  }

  const [searchQuery, setSearchQuery] = useState("");

  const filteredDatabases = databases?.filter((db: Database) =>
    db.name.toLowerCase().includes(searchQuery.toLowerCase())
  );

  return (
    <div className="space-y-4">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div className="flex items-center gap-3">
          {isAdmin && (
            <Button onClick={() => setCreateOpen(true)}>
              <Plus className="mr-2 h-4 w-4" />
              Create Database
            </Button>
          )}
          <Input
            placeholder="Search databases..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="w-48 sm:w-64"
          />
        </div>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            onClick={() => setShowSystem((prev) => !prev)}
          >
            {showSystem ? (
              <EyeOff className="mr-2 h-4 w-4" />
            ) : (
              <Eye className="mr-2 h-4 w-4" />
            )}
            {showSystem ? "Hide System" : "Show System"}
          </Button>
          <Button variant="outline" onClick={() => refetch()}>
            <RefreshCw className="mr-2 h-4 w-4" />
            Refresh
          </Button>
        </div>
      </div>

      <div className="rounded-md border border-border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Name</TableHead>
              {isAdmin && <TableHead className="w-20"></TableHead>}
            </TableRow>
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <TableRow>
                <TableCell
                  colSpan={isAdmin ? 2 : 1}
                  className="h-24 text-center text-muted-foreground"
                >
                  Loading...
                </TableCell>
              </TableRow>
            ) : !filteredDatabases?.length ? (
              <TableRow>
                <TableCell
                  colSpan={isAdmin ? 2 : 1}
                  className="h-24 text-center text-muted-foreground"
                >
                  {searchQuery ? "No matching databases found." : "No databases found."}
                </TableCell>
              </TableRow>
            ) : (
              filteredDatabases.map((db: Database, i: number) => (
                <TableRow 
                  key={db.name} 
                  className="animate-in fade-in slide-in-from-bottom-2 duration-300 fill-mode-both"
                  style={{ animationDelay: `${i * 50}ms` }}
                >
                  <TableCell className="font-medium">
                    <Link to={`/databases/${db.name}`} className="text-foreground hover:underline">
                      {db.name}
                    </Link>
                  </TableCell>
                  {isAdmin && (
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="text-destructive hover:bg-destructive/10 hover:text-destructive"
                        disabled={db.protected}
                        onClick={() => setDeleteTarget(db.name)}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </TableCell>
                  )}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create Database</DialogTitle>
            <DialogDescription>
              Create a new PostgreSQL database instance.
            </DialogDescription>
          </DialogHeader>
          <form onSubmit={handleCreate}>
            <div className="space-y-4 py-4">
              <div className="space-y-2">
                <Label htmlFor="dbname">Database Name</Label>
                <Input
                  id="dbname"
                  placeholder="e.g. staging_db"
                  value={newName}
                  onChange={(e) => {
                    setNewName(e.target.value);
                    setNameError(null);
                  }}
                  autoFocus
                />
                {nameError && (
                  <p className="text-sm text-destructive">{nameError}</p>
                )}
              </div>
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => setCreateOpen(false)}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={createMutation.isPending}>
                Create
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog
        open={!!deleteTarget}
        onOpenChange={(open) => {
          if (!open) {
            setDeleteTarget(null);
            setDeleteConfirmText("");
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Database</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete "{deleteTarget}"? This cannot be
              undone.
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
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setDeleteTarget(null);
                setDeleteConfirmText("");
              }}
            >
              Cancel
            </Button>
            <Button
              type="button"
              variant="destructive"
              disabled={deleteMutation.isPending || deleteConfirmText !== "DELETE"}
              onClick={() => {
                if (deleteTarget) {
                  deleteMutation.mutate(deleteTarget);
                  setDeleteConfirmText("");
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
