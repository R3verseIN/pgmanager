import { useState } from "react";
import { Link } from "react-router-dom";
import {
  Database,
  Users as UsersIcon,
  ChevronLeft,
  ChevronRight,
  LogOut,
  User,
  ScrollText,
  Shield,
} from "lucide-react";
import { useAuth } from "../../contexts/AuthContext";
import NavItem from "./NavItem";
import { cn } from "../../lib/utils";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "../ui/tooltip";

export default function DesktopSidebar({
  selectedKey,
}: {
  selectedKey: string;
}) {
  const { user, logout } = useAuth();
  const [isCollapsed, setIsCollapsed] = useState(false);

  return (
    <aside
      className={cn(
        "hidden shrink-0 flex-col border-r border-hairline bg-background transition-all duration-300 md:flex",
        isCollapsed ? "w-20" : "w-56"
      )}
    >
      <div
        className={cn(
          "flex h-14 items-center gap-3 overflow-hidden border-b border-hairline transition-all duration-300",
          isCollapsed ? "justify-center px-0" : "px-4"
        )}
      >
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <img
                src="/1784864797625-019f923a-f479-741b-acd9-2e57c32ad86c.png"
                alt="pgmanager logo"
                className="size-8 shrink-0 object-contain"
              />
            </TooltipTrigger>
            <TooltipContent side="right">pgmanager</TooltipContent>
          </Tooltip>
        </TooltipProvider>
        {!isCollapsed && (
          <span className="animate-in text-sm font-(--font-display) whitespace-nowrap text-foreground fade-in">
            pgmanager
          </span>
        )}
      </div>

      <nav className="flex-1 space-y-1 overflow-y-auto p-2">
        <NavItem
          to="/"
          icon={Database}
          label="Databases"
          isActive={selectedKey === "databases"}
          isCollapsed={isCollapsed}
        />
        {user?.role === "admin" && (
          <>
            <NavItem
              to="/users"
              icon={UsersIcon}
              label="Users"
              isActive={selectedKey === "users"}
              isCollapsed={isCollapsed}
              className="mt-1"
            />
            <NavItem
              to="/logs"
              icon={ScrollText}
              label="Logs"
              isActive={selectedKey === "logs"}
              isCollapsed={isCollapsed}
              className="mt-1"
            />
            <NavItem
              to="/settings"
              icon={Shield}
              label="Settings"
              isActive={selectedKey === "settings"}
              isCollapsed={isCollapsed}
              className="mt-1"
            />
          </>
        )}
      </nav>

      <div className="space-y-2 border-t border-hairline p-2">
        <div
          className={cn(
            "flex items-center rounded-md p-2",
            isCollapsed ? "justify-center" : "justify-between"
          )}
        >
          {!isCollapsed ? (
            <Link
              to="/profile"
              className="flex items-center gap-3 overflow-hidden rounded-md transition-colors hover:bg-surface-1"
            >
              <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-surface-2 text-foreground">
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
          ) : (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Link to="/profile">
                    <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-surface-2 text-foreground">
                      <User className="size-4" />
                    </div>
                  </Link>
                </TooltipTrigger>
                <TooltipContent side="right">
                  <p className="font-medium">{user?.username}</p>
                  <p className="text-xs text-ink-muted capitalize">
                    {user?.role}
                  </p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )}

          {!isCollapsed && (
            <button
              onClick={logout}
              className="shrink-0 rounded-full p-2 text-ink-muted transition-colors hover:bg-surface-1 hover:text-foreground"
              title="Logout"
            >
              <LogOut className="size-4" />
            </button>
          )}
        </div>

        {isCollapsed && (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  onClick={logout}
                  className="flex w-full items-center justify-center rounded-full p-2 text-ink-muted transition-colors hover:bg-surface-1 hover:text-foreground"
                >
                  <LogOut className="size-4" />
                </button>
              </TooltipTrigger>
              <TooltipContent side="right">Logout</TooltipContent>
            </Tooltip>
          </TooltipProvider>
        )}

        <button
          onClick={() => setIsCollapsed(!isCollapsed)}
          className={cn(
            "flex w-full items-center rounded-md px-3 py-2 text-sm font-medium text-ink-muted transition-all duration-200 hover:bg-surface-1 hover:text-foreground active:scale-95",
            isCollapsed ? "justify-center" : "gap-3 hover:translate-x-1"
          )}
        >
          {isCollapsed ? (
            <ChevronRight className="size-4 shrink-0" />
          ) : (
            <ChevronLeft className="size-4 shrink-0" />
          )}
          {!isCollapsed && <span>Collapse Sidebar</span>}
        </button>
      </div>
    </aside>
  );
}
