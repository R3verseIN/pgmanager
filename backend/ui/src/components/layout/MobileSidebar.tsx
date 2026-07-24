import { Link } from "react-router-dom";
import {
  Database,
  Users as UsersIcon,
  X,
  LogOut,
  User,
  ScrollText,
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
      <div className="fixed inset-y-0 left-0 w-64 bg-card border-r border-border shadow-lg flex flex-col p-4 animate-in slide-in-from-left">
        <div className="flex items-center justify-between mb-6">
          <div className="flex items-center gap-3">
            <img
              src="/1784864797625-019f923a-f479-741b-acd9-2e57c32ad86c.png"
              alt="pgmanager logo"
              className="h-8 w-8 object-contain shrink-0"
            />
            <span className="font-semibold text-foreground text-sm">
              pgmanager
            </span>
          </div>
          <button
            onClick={onClose}
            className="p-2 -mr-2 text-muted-foreground hover:text-foreground"
          >
            <X className="h-5 w-5" />
          </button>
        </div>
        <nav className="flex-1 space-y-1">
          <Link
            to="/"
            onClick={onClose}
            className={cn(
              "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
              selectedKey === "databases"
                ? "bg-accent text-accent-foreground"
                : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
            )}
          >
            <Database className="h-4 w-4 shrink-0" />
            Databases
          </Link>
          {user?.role === "admin" && (
            <>
              <Link
                to="/users"
                onClick={onClose}
                className={cn(
                  "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors mt-1",
                  selectedKey === "users"
                    ? "bg-accent text-accent-foreground"
                    : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
                )}
              >
                <UsersIcon className="h-4 w-4 shrink-0" />
                Users
              </Link>
              <Link
                to="/logs"
                onClick={onClose}
                className={cn(
                  "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors mt-1",
                  selectedKey === "logs"
                    ? "bg-accent text-accent-foreground"
                    : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
                )}
              >
                <ScrollText className="h-4 w-4 shrink-0" />
                Logs
              </Link>
            </>
          )}
        </nav>
        <div className="border-t border-border pt-4 mt-auto">
          <div className="flex items-center justify-between rounded-md p-2 bg-accent/30">
            <Link to="/profile" className="flex items-center gap-3 overflow-hidden">
              <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-accent text-accent-foreground">
                <User className="h-4 w-4" />
              </div>
              <div className="flex flex-col overflow-hidden">
                <span className="truncate text-sm font-medium text-foreground">
                  {user?.username}
                </span>
                <span className="truncate text-xs text-muted-foreground capitalize">
                  {user?.role}
                </span>
              </div>
            </Link>
            <button
              onClick={logout}
              className="rounded-md p-2 text-muted-foreground hover:bg-destructive hover:text-destructive-foreground transition-colors shrink-0"
              title="Logout"
            >
              <LogOut className="h-4 w-4" />
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
