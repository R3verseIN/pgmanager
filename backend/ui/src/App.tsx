import { useState, useEffect } from "react";
import { BrowserRouter, Routes, Route, Link, useLocation, Navigate } from "react-router-dom";
import { Database, Users as UsersIcon, Loader2, ChevronLeft, ChevronRight, Menu, X, LogOut, User, ScrollText } from "lucide-react";
import { Toaster } from "sonner";
import { AuthProvider, useAuth } from "./contexts/AuthContext";
import DatabasesTable from "./components/DatabasesTable";
import DatabaseDetail from "./routes/DatabaseDetail";
import TableDetail from "./routes/TableDetail";
import Users from "./routes/Users";
import Logs from "./routes/Logs";
import Login from "./routes/Login";
import Setup from "./routes/Setup";
import Profile from "./routes/Profile";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "./components/ui/tooltip";
import { cn } from "./lib/utils";

function AppLayout() {
  const location = useLocation();
  const selectedKey = location.pathname === "/users"
    ? "users"
    : location.pathname === "/logs"
      ? "logs"
      : "databases";
  const { user, logout } = useAuth();

  const [isCollapsed, setIsCollapsed] = useState(false);
  const [isMobileOpen, setIsMobileOpen] = useState(false);

  // Close mobile menu when route changes
  useEffect(() => {
    setIsMobileOpen(false);
  }, [location.pathname]);

  return (
    <div className="flex h-screen overflow-hidden bg-background flex-col md:flex-row">
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
        <div className={cn("flex h-14 items-center gap-3 border-b border-border overflow-hidden transition-all duration-300", isCollapsed ? "justify-center px-0" : "px-4")}>
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
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Link
                  to="/"
                  className={cn(
                    "flex items-center rounded-md px-3 py-2 text-sm font-medium transition-all duration-200 active:scale-95",
                    isCollapsed ? "justify-center" : "gap-3",
                    selectedKey === "databases"
                      ? "bg-accent text-accent-foreground"
                      : "text-muted-foreground hover:bg-accent/50 hover:text-foreground hover:translate-x-1"
                  )}
                >
                  <Database className="h-4 w-4 shrink-0" />
                  {!isCollapsed && <span>Databases</span>}
                </Link>
              </TooltipTrigger>
              {isCollapsed && <TooltipContent side="right">Databases</TooltipContent>}
            </Tooltip>

            {user?.role === "admin" && (
              <>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Link
                      to="/users"
                      className={cn(
                        "flex items-center rounded-md px-3 py-2 text-sm font-medium transition-all duration-200 active:scale-95 mt-1",
                        isCollapsed ? "justify-center" : "gap-3",
                        selectedKey === "users"
                          ? "bg-accent text-accent-foreground"
                          : "text-muted-foreground hover:bg-accent/50 hover:text-foreground hover:translate-x-1"
                      )}
                    >
                      <UsersIcon className="h-4 w-4 shrink-0" />
                      {!isCollapsed && <span>Users</span>}
                    </Link>
                  </TooltipTrigger>
                  {isCollapsed && <TooltipContent side="right">Users</TooltipContent>}
                </Tooltip>

                <Tooltip>
                  <TooltipTrigger asChild>
                    <Link
                      to="/logs"
                      className={cn(
                        "flex items-center rounded-md px-3 py-2 text-sm font-medium transition-all duration-200 active:scale-95 mt-1",
                        isCollapsed ? "justify-center" : "gap-3",
                        selectedKey === "logs"
                          ? "bg-accent text-accent-foreground"
                          : "text-muted-foreground hover:bg-accent/50 hover:text-foreground hover:translate-x-1"
                      )}
                    >
                      <ScrollText className="h-4 w-4 shrink-0" />
                      {!isCollapsed && <span>Logs</span>}
                    </Link>
                  </TooltipTrigger>
                  {isCollapsed && <TooltipContent side="right">Logs</TooltipContent>}
                </Tooltip>
              </>
            )}
          </TooltipProvider>
        </nav>

        <div className="border-t border-border p-2 space-y-2">
          {/* User Profile / Logout */}
          <div className={cn(
            "flex items-center rounded-md p-2",
            isCollapsed ? "justify-center" : "justify-between"
          )}>
            {!isCollapsed ? (
              <Link to="/profile" className="flex items-center gap-3 overflow-hidden hover:bg-accent/50 rounded-md transition-colors">
                <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-accent text-accent-foreground">
                  <User className="h-4 w-4" />
                </div>
                <div className="flex flex-col overflow-hidden">
                  <span className="truncate text-sm font-medium text-foreground">{user?.username}</span>
                  <span className="truncate text-xs text-muted-foreground capitalize">{user?.role}</span>
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
                    <p className="text-xs text-muted-foreground capitalize">{user?.role}</p>
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
                <>
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
                  <Link
                    to="/logs"
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
                    <span className="truncate text-sm font-medium text-foreground">{user?.username}</span>
                    <span className="truncate text-xs text-muted-foreground capitalize">{user?.role}</span>
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
      )}

      <main className="flex-1 p-4 sm:p-6 overflow-y-auto min-w-0 h-full">
        <Routes>
          <Route path="/" element={<DatabasesTable />} />
          <Route path="/databases/:name" element={<DatabaseDetail />} />
          <Route path="/databases/:name/tables/:table" element={<TableDetail />} />
          <Route path="/users" element={user?.role === "admin" ? <Users /> : <Navigate to="/" />} />
          <Route path="/logs" element={user?.role === "admin" ? <Logs /> : <Navigate to="/" />} />
          <Route path="/profile" element={<Profile />} />
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
