import { Key, Edit, Trash2 } from "lucide-react";
import type { User } from "../lib/schemas";
import { Button } from "./ui/button";
import { Badge } from "./ui/badge";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "./ui/table";

const accessColors: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  read: "default",
  write: "secondary",
  ddl: "outline",
  full: "destructive",
};

export default function PostgresUsersTable({
  users,
  isLoading,
  onResetPassword,
  onEdit,
  onDelete,
}: {
  users: User[];
  isLoading: boolean;
  onResetPassword: (user: User) => void;
  onEdit: (user: User) => void;
  onDelete: (user: User) => void;
}) {
  return (
    <div className="rounded-md border border-hairline bg-surface-1">
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
              <TableCell colSpan={6} className="h-24 text-center text-ink-muted">
                Loading...
              </TableCell>
            </TableRow>
          ) : !users.length ? (
            <TableRow>
              <TableCell colSpan={6} className="h-24 text-center text-ink-muted">
                No users found.
              </TableCell>
            </TableRow>
          ) : (
            users.map((user, i) => (
              <TableRow
                key={user.username}
                className="animate-in duration-300 fill-mode-both fade-in slide-in-from-bottom-2"
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
                      <Badge
                        key={ip}
                        variant={ip === "0.0.0.0/0" ? "outline" : "secondary"}
                        className="font-mono text-xs"
                      >
                        {ip === "0.0.0.0/0" ? "any" : ip}
                      </Badge>
                    ))}
                  </div>
                </TableCell>
                <TableCell className="hidden md:table-cell">
                  {new Date(user.createdAt).toLocaleDateString()}
                </TableCell>
                <TableCell>
                  <div className="flex gap-2">
                    <Button
                      variant="ghost"
                      size="icon"
                      title="Reset Password"
                      onClick={() => onResetPassword(user)}
                    >
                      <Key className="size-4" />
                    </Button>
                    <Button variant="ghost" size="icon" onClick={() => onEdit(user)}>
                      <Edit className="size-4" />
                    </Button>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="text-destructive"
                      onClick={() => onDelete(user)}
                    >
                      <Trash2 className="size-4" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  );
}
