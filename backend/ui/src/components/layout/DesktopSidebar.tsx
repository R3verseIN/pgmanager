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
        "hidden md:flex flex-col border-r border-border bg-card transition-all duration-300 shrink-0",
        isCollapsed ? "w-20" : "w-56"
      )}
    >
      <div
        className={cn(
          "flex h-14 items-center gap-3 border-b border-border overflow-hidden transition-all duration-300",
          isCollapsed ? "justify-center px-0" : "px-4"
        )}
      >
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <img
                src="/1784864797625-019f923a-f479-741b-acd9-2e57c32ad86c.png"
                alt="pgmanager logo"
                className="h-8 w-8 object-contain shrink-0"
              />
            </TooltipTrigger>
            <TooltipContent side="right">pgmanager</TooltipContent>
          </Tooltip>
        </TooltipProvider>
        {!isCollapsed && (
          <span className="font-semibold text-foreground text-sm whitespace-nowrap animate-in fade-in">
            pgmanager
          </span>
        )}
      </div>

      <nav className="flex-1 space-y-1 p-2 overflow-y-auto">
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
          </>
        )}
      </nav>

      <div className="border-t border-border p-2 space-y-2">
        <div
          className={cn(
            "flex items-center rounded-md p-2",
            isCollapsed ? "justify-center" : "justify-between"
          )}
        >
          {!isCollapsed ? (
            <Link
              to="/profile"
              className="flex items-center gap-3 overflow-hidden hover:bg-accent/50 rounded-md transition-colors"
            >
              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-accent text-accent-foreground">
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
          ) : (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Link to="/profile">
                    <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-accent text-accent-foreground">
                      <User className="h-4 w-4" />
                    </div>
                  </Link>
                </TooltipTrigger>
                <TooltipContent side="right">
                  <p className="font-medium">{user?.username}</p>
                  <p className="text-xs text-muted-foreground capitalize">
                    {user?.role}
                  </p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )}

          {!isCollapsed && (
            <button
              onClick={logout}
              className="rounded-md p-2 text-muted-foreground hover:bg-accent hover:text-foreground transition-colors shrink-0"
              title="Logout"
            >
              <LogOut className="h-4 w-4" />
            </button>
          )}
        </div>

        {isCollapsed && (
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  onClick={logout}
                  className="flex w-full items-center justify-center rounded-md p-2 text-muted-foreground hover:bg-accent hover:text-foreground transition-colors"
                >
                  <LogOut className="h-4 w-4" />
                </button>
              </TooltipTrigger>
              <TooltipContent side="right">Logout</TooltipContent>
            </Tooltip>
          </TooltipProvider>
        )}

        <button
          onClick={() => setIsCollapsed(!isCollapsed)}
          className={cn(
            "flex w-full items-center rounded-md px-3 py-2 text-sm font-medium text-muted-foreground hover:bg-accent/50 hover:text-foreground transition-all duration-200 active:scale-95",
            isCollapsed ? "justify-center" : "gap-3 hover:translate-x-1"
          )}
        >
          {isCollapsed ? (
            <ChevronRight className="h-4 w-4 shrink-0" />
          ) : (
            <ChevronLeft className="h-4 w-4 shrink-0" />
          )}
          {!isCollapsed && <span>Collapse Sidebar</span>}
        </button>
      </div>
    </aside>
  );
}
