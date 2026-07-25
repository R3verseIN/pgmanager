import { Edit, Key, Trash2 } from "lucide-react";
import type { AuthUserListItem } from "../lib/schemas";
import { Button } from "./ui/button";
import { Badge } from "./ui/badge";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "./ui/tooltip";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "./ui/table";

export default function AuthUsersTable({
  users,
  isLoading,
  onEdit,
  onResetPassword,
  onDelete,
}: {
  users: AuthUserListItem[];
  isLoading: boolean;
  onEdit: (user: AuthUserListItem) => void;
  onResetPassword: (user: AuthUserListItem) => void;
  onDelete: (user: AuthUserListItem) => void;
}) {
  return (
    <div className="rounded-md border border-hairline bg-surface-1">
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
          {isLoading ? (
            <TableRow>
              <TableCell colSpan={5} className="h-24 text-center text-ink-muted">
                Loading...
              </TableCell>
            </TableRow>
          ) : !users.length ? (
            <TableRow>
              <TableCell colSpan={5} className="h-24 text-center text-ink-muted">
                No auth users found.
              </TableCell>
            </TableRow>
          ) : (
            users.map((authUser, i) => (
              <TableRow
                key={authUser.id}
                className="animate-in duration-300 fill-mode-both fade-in slide-in-from-bottom-2"
                style={{ animationDelay: `${i * 50}ms` }}
              >
                <TableCell className="font-medium">{authUser.username}</TableCell>
                <TableCell>
                  <Badge
                    variant={
                      authUser.role === "admin"
                        ? "destructive"
                        : authUser.role === "dev"
                          ? "outline"
                          : "secondary"
                    }
                  >
                    {authUser.role.toUpperCase()}
                  </Badge>
                </TableCell>
                <TableCell className="hidden sm:table-cell">
                  {authUser.role === "dev" &&
                  authUser.databases &&
                  authUser.databases.length > 0 ? (
                    <div className="flex flex-wrap gap-1">
                      {authUser.databases.map((db) => (
                        <Badge key={db} variant="secondary" className="text-xs">
                          {db}
                        </Badge>
                      ))}
                    </div>
                  ) : (
                    <span className="text-sm text-ink-muted">—</span>
                  )}
                </TableCell>
                <TableCell className="hidden sm:table-cell">
                  {new Date(authUser.createdAt).toLocaleDateString()}
                </TableCell>
                <TableCell>
                  <div className="flex gap-2">
                    <Button variant="ghost" size="icon" onClick={() => onEdit(authUser)}>
                      <Edit className="size-4" />
                    </Button>
                    <TooltipProvider>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => onResetPassword(authUser)}
                          >
                            <Key className="size-4" />
                          </Button>
                        </TooltipTrigger>
                        <TooltipContent>Reset password</TooltipContent>
                      </Tooltip>
                    </TooltipProvider>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="text-destructive"
                      onClick={() => onDelete(authUser)}
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
