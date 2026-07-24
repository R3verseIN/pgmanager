import { BrowserRouter, Routes, Route, Link, useLocation, Navigate } from "react-router-dom";
import { Database, Users as UsersIcon, Settings, Loader2 } from "lucide-react";
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

  return (
    <div className="flex min-h-screen bg-background">
      <aside className="flex w-56 flex-col border-r border-border bg-card">
        <div className="flex h-14 items-center gap-3 border-b border-border px-4">
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
          <span className="font-semibold text-foreground text-sm whitespace-nowrap">
            pgmanager
          </span>
        </div>

        <nav className="flex-1 space-y-1 p-2">
          <Link
            to="/"
            className={cn(
              "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
              selectedKey === "databases"
                ? "bg-accent text-accent-foreground"
                : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
            )}
          >
            <Database className="h-4 w-4" />
            Databases
          </Link>
          {user?.role === "admin" && (
            <Link
              to="/users"
              className={cn(
                "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                selectedKey === "users"
                  ? "bg-accent text-accent-foreground"
                  : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
              )}
            >
              <UsersIcon className="h-4 w-4" />
              Users
            </Link>
          )}
        </nav>

        <div className="border-t border-border p-2">
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <div className="flex cursor-not-allowed items-center gap-3 rounded-md px-3 py-2 text-sm font-medium text-muted-foreground opacity-50">
                  <Settings className="h-4 w-4" />
                  Settings
                </div>
              </TooltipTrigger>
              <TooltipContent side="right">Not implemented yet</TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </div>
      </aside>

      <main className="flex-1 p-6 overflow-auto">
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
