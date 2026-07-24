import { useState, useEffect } from "react";
import { BrowserRouter, Routes, Route, Link, useLocation, Navigate } from "react-router-dom";
import { Database, Users as UsersIcon, Settings, Loader2, ChevronLeft, ChevronRight, Menu, X } from "lucide-react";
import { Toaster } from "sonner";
import { AuthProvider, useAuth } from "./contexts/AuthContext";
import DatabasesTable from "./components/DatabasesTable";
import Users from "./routes/Users";
import Login from "./routes/Login";
import Setup from "./routes/Setup";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "./components/ui/tooltip";
import { cn } from "./lib/utils";

function AppLayout() {
  const location = useLocation();
  const selectedKey = location.pathname === "/users" ? "users" : "databases";
  const { user } = useAuth();

  const [isCollapsed, setIsCollapsed] = useState(false);
  const [isMobileOpen, setIsMobileOpen] = useState(false);

  // Close mobile menu when route changes
  useEffect(() => {
    setIsMobileOpen(false);
  }, [location.pathname]);

  return (
    <div className="flex min-h-screen bg-background flex-col md:flex-row">
      {/* Mobile Header */}
      <div className="md:hidden flex items-center justify-between border-b border-border bg-card px-4 h-14 shrink-0">
        <div className="flex items-center gap-3">
          <img 
            src="/1784864797625-019f923a-f479-741b-acd9-2e57c32ad86c.png" 
            alt="pgmanager logo" 
            className="h-8 w-8 object-contain shrink-0" 
          />
          <span className="font-semibold text-foreground text-sm">pgmanager</span>
        </div>
        <button onClick={() => setIsMobileOpen(true)} className="p-2 -mr-2 text-muted-foreground hover:text-foreground">
          <Menu className="h-5 w-5" />
        </button>
      </div>

      {/* Desktop Sidebar */}
      <aside 
        className={cn(
          "hidden md:flex flex-col border-r border-border bg-card transition-all duration-300 shrink-0",
          isCollapsed ? "w-20" : "w-56"
        )}
      >
        <div className="flex h-14 items-center gap-3 border-b border-border px-4 overflow-hidden">
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

        <nav className="flex-1 space-y-1 p-2">
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Link
                  to="/"
                  className={cn(
                    "flex items-center rounded-md px-3 py-2 text-sm font-medium transition-colors",
                    isCollapsed ? "justify-center" : "gap-3",
                    selectedKey === "databases"
                      ? "bg-accent text-accent-foreground"
                      : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
                  )}
                >
                  <Database className="h-4 w-4 shrink-0" />
                  {!isCollapsed && <span>Databases</span>}
                </Link>
              </TooltipTrigger>
              {isCollapsed && <TooltipContent side="right">Databases</TooltipContent>}
            </Tooltip>

            {user?.role === "admin" && (
              <Tooltip>
                <TooltipTrigger asChild>
                  <Link
                    to="/users"
                    className={cn(
                      "flex items-center rounded-md px-3 py-2 text-sm font-medium transition-colors mt-1",
                      isCollapsed ? "justify-center" : "gap-3",
                      selectedKey === "users"
                        ? "bg-accent text-accent-foreground"
                        : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
                    )}
                  >
                    <UsersIcon className="h-4 w-4 shrink-0" />
                    {!isCollapsed && <span>Users</span>}
                  </Link>
                </TooltipTrigger>
                {isCollapsed && <TooltipContent side="right">Users</TooltipContent>}
              </Tooltip>
            )}
          </TooltipProvider>
        </nav>

        <div className="border-t border-border p-2 space-y-1">
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <div className={cn(
                  "flex cursor-not-allowed items-center rounded-md px-3 py-2 text-sm font-medium text-muted-foreground opacity-50",
                  isCollapsed ? "justify-center" : "gap-3"
                )}>
                  <Settings className="h-4 w-4 shrink-0" />
                  {!isCollapsed && <span>Settings</span>}
                </div>
              </TooltipTrigger>
              <TooltipContent side="right">Not implemented yet</TooltipContent>
            </Tooltip>
          </TooltipProvider>

          <button
            onClick={() => setIsCollapsed(!isCollapsed)}
            className={cn(
              "flex w-full items-center rounded-md px-3 py-2 text-sm font-medium text-muted-foreground hover:bg-accent/50 hover:text-foreground transition-colors",
              isCollapsed ? "justify-center" : "gap-3"
            )}
          >
            {isCollapsed ? <ChevronRight className="h-4 w-4 shrink-0" /> : <ChevronLeft className="h-4 w-4 shrink-0" />}
            {!isCollapsed && <span>Collapse Sidebar</span>}
          </button>
        </div>
      </aside>

      {/* Mobile Sidebar Overlay */}
      {isMobileOpen && (
        <div className="fixed inset-0 z-50 flex md:hidden">
          <div className="fixed inset-0 bg-background/80 backdrop-blur-sm" onClick={() => setIsMobileOpen(false)} />
          <div className="fixed inset-y-0 left-0 w-64 bg-card border-r border-border shadow-lg flex flex-col p-4 animate-in slide-in-from-left">
            <div className="flex items-center justify-between mb-6">
              <div className="flex items-center gap-3">
                <img 
                  src="/1784864797625-019f923a-f479-741b-acd9-2e57c32ad86c.png" 
                  alt="pgmanager logo" 
                  className="h-8 w-8 object-contain shrink-0" 
                />
                <span className="font-semibold text-foreground text-sm">pgmanager</span>
              </div>
              <button onClick={() => setIsMobileOpen(false)} className="p-2 -mr-2 text-muted-foreground hover:text-foreground">
                <X className="h-5 w-5" />
              </button>
            </div>
            <nav className="flex-1 space-y-1">
              <Link
                to="/"
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
                <Link
                  to="/users"
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
              )}
            </nav>
            <div className="border-t border-border pt-4">
              <div className="flex cursor-not-allowed items-center gap-3 rounded-md px-3 py-2 text-sm font-medium text-muted-foreground opacity-50">
                <Settings className="h-4 w-4 shrink-0" />
                Settings
              </div>
            </div>
          </div>
        </div>
      )}

      <main className="flex-1 p-4 sm:p-6 overflow-auto min-w-0">
        <Routes>
          <Route path="/" element={<DatabasesTable />} />
          <Route path="/users" element={user?.role === "admin" ? <Users /> : <Navigate to="/" />} />
        </Routes>
      </main>
    </div>
  );
}

function AuthenticatedLayout() {
  const { user, loading, needsSetup } = useAuth();

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-background">
        <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (!user) {
    return (
      <Routes>
        <Route path="/setup" element={needsSetup ? <Setup /> : <Navigate to="/login" />} />
        <Route path="/login" element={needsSetup ? <Navigate to="/setup" /> : <Login />} />
        <Route path="*" element={needsSetup ? <Navigate to="/setup" /> : <Login />} />
      </Routes>
    );
  }

  return <AppLayout />;
}

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <AuthenticatedLayout />
      </AuthProvider>
      <Toaster theme="dark" position="bottom-right" />
    </BrowserRouter>
  );
}
