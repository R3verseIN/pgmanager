import { useState } from "react";
import { Link } from "react-router-dom";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { Plus, Trash2, RefreshCw, Eye, EyeOff } from "lucide-react";
import { fetchDatabases, deleteDatabase } from "../api/client";
import type { Database } from "../lib/schemas";
import { useAuth } from "../contexts/AuthContext";

import { Button } from "./ui/button";
import { Input } from "./ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "./ui/table";
import CreateDatabaseDialog from "./dialogs/CreateDatabaseDialog";
import ConfirmDeleteDialog from "./dialogs/ConfirmDeleteDialog";

export default function DatabasesTable() {
  const [showSystem, setShowSystem] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [searchQuery, setSearchQuery] = useState("");

  const queryClient = useQueryClient();
  const { isAdmin } = useAuth();

  const {
    data: databases,
    isLoading,
    refetch,
  } = useQuery({
    queryKey: ["databases", showSystem],
    queryFn: () => fetchDatabases(showSystem),
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
                  {searchQuery
                    ? "No matching databases found."
                    : "No databases found."}
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
                    <Link
                      to={`/databases/${db.name}`}
                      className="text-foreground hover:underline"
                    >
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

      <CreateDatabaseDialog open={createOpen} onOpenChange={setCreateOpen} />
      <ConfirmDeleteDialog
        open={deleteTarget !== null}
        onOpenChange={(open) => {
          if (!open) setDeleteTarget(null);
        }}
        itemName={deleteTarget ?? ""}
        isPending={deleteMutation.isPending}
        onConfirm={() => {
          if (deleteTarget) {
            deleteMutation.mutate(deleteTarget);
            setDeleteTarget(null);
          }
        }}
      />
    </div>
  );
}
