import { Link } from "react-router-dom";
import {
  Database,
  Users as UsersIcon,
  X,
  LogOut,
  User,
  ScrollText,
  Shield,
  DatabaseBackup,
} from "lucide-react";
import { useAuth } from "../../contexts/AuthContext";
import { cn } from "../../lib/utils";

export default function MobileSidebar({
  open,
  onClose,
  selectedKey,
}: {
  open: boolean;
  onClose: () => void;
  selectedKey: string;
}) {
  const { user, logout } = useAuth();

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex md:hidden">
      <div
        className="fixed inset-0 bg-background/80 backdrop-blur-sm"
        onClick={onClose}
      />
      <div className="fixed inset-y-0 left-0 flex w-64 animate-in flex-col border-r border-hairline bg-surface-1 p-4 shadow-lg slide-in-from-left">
        <div className="mb-6 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <img
              src="/1784864797625-019f923a-f479-741b-acd9-2e57c32ad86c.png"
              alt="pgmanager logo"
              className="size-8 shrink-0 object-contain"
            />
            <span className="text-sm font-(--font-display) text-foreground">
              pgmanager
            </span>
          </div>
          <button
            onClick={onClose}
            className="-mr-2 p-2 text-ink-muted hover:text-foreground"
          >
            <X className="size-5" />
          </button>
        </div>
        <nav className="flex-1 space-y-1">
          <Link
            to="/"
            onClick={onClose}
            className={cn(
              "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
              selectedKey === "databases"
                ? "bg-surface-2 text-foreground"
                : "text-ink-muted hover:bg-surface-1 hover:text-foreground"
            )}
          >
            <Database className="size-4 shrink-0" />
            Databases
          </Link>
          {user?.role === "admin" && (
            <>
              <Link
                to="/users"
                onClick={onClose}
                className={cn(
                  "mt-1 flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                  selectedKey === "users"
                    ? "bg-surface-2 text-foreground"
                    : "text-ink-muted hover:bg-surface-1 hover:text-foreground"
                )}
              >
                <UsersIcon className="size-4 shrink-0" />
                Users
              </Link>
              <Link
                to="/logs"
                onClick={onClose}
                className={cn(
                  "mt-1 flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                  selectedKey === "logs"
                    ? "bg-surface-2 text-foreground"
                    : "text-ink-muted hover:bg-surface-1 hover:text-foreground"
                )}
              >
                <ScrollText className="size-4 shrink-0" />
                Logs
              </Link>
              <Link
                to="/backups"
                onClick={onClose}
                className={cn(
                  "mt-1 flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                  selectedKey === "backups"
                    ? "bg-surface-2 text-foreground"
                    : "text-ink-muted hover:bg-surface-1 hover:text-foreground"
                )}
              >
                <DatabaseBackup className="size-4 shrink-0" />
                Backups
              </Link>
              <Link
                to="/settings"
                onClick={onClose}
                className={cn(
                  "mt-1 flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                  selectedKey === "settings"
                    ? "bg-surface-2 text-foreground"
                    : "text-ink-muted hover:bg-surface-1 hover:text-foreground"
                )}
              >
                <Shield className="size-4 shrink-0" />
                Settings
              </Link>
            </>
          )}
        </nav>
        <div className="mt-auto border-t border-hairline pt-4">
          <div className="flex items-center justify-between rounded-md bg-surface-1 p-2">
            <Link to="/profile" className="flex items-center gap-3 overflow-hidden">
              <div className="flex size-9 shrink-0 items-center justify-center rounded-full bg-surface-2 text-foreground">
                <User className="size-4" />
              </div>
              <div className="flex flex-col overflow-hidden">
                <span className="truncate text-sm font-medium text-foreground">
                  {user?.username}
                </span>
                <span className="truncate text-xs text-ink-muted capitalize">
                  {user?.role}
                </span>
              </div>
            </Link>
            <button
              onClick={logout}
              className="shrink-0 rounded-full p-2 text-ink-muted transition-colors hover:bg-destructive hover:text-destructive-foreground"
              title="Logout"
            >
              <LogOut className="size-4" />
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
