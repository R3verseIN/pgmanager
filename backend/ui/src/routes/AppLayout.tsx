import { useState, useEffect } from "react";
import { Routes, Route, useLocation, Navigate } from "react-router-dom";
import { Menu } from "lucide-react";
import { useAuth } from "../contexts/AuthContext";
import DesktopSidebar from "../components/layout/DesktopSidebar";
import MobileSidebar from "../components/layout/MobileSidebar";
import DatabasesTable from "../components/DatabasesTable";
import DatabaseDetail from "./DatabaseDetail";
import TableDetail from "./TableDetail";
import Users from "./Users";
import Logs from "./Logs";
import Profile from "./Profile";
import Settings from "./Settings";
import Backups from "./Backups";
import WalgBackups from "./WalgBackups";

export default function AppLayout() {
  const location = useLocation();
  const selectedKey =
    location.pathname === "/users"
      ? "users"
      : location.pathname === "/logs"
        ? "logs"
        : location.pathname === "/settings"
          ? "settings"
          : location.pathname === "/backups"
            ? "backups"
            : location.pathname === "/walg"
              ? "walg"
              : "databases";
  const { user } = useAuth();

  const [isMobileOpen, setIsMobileOpen] = useState(false);

  useEffect(() => {
    setIsMobileOpen(false);
  }, [location.pathname]);

  return (
    <div className="flex h-screen flex-col overflow-hidden bg-background md:flex-row">
      {/* Mobile Header */}
      <div className="flex h-14 shrink-0 items-center justify-between border-b border-hairline bg-background px-4 md:hidden">
        <div className="flex items-center gap-3">
          <img
            src="/1784864797625-019f923a-f479-741b-acd9-2e57c32ad86c.png"
            alt="pgmanager logo"
            className="size-8 shrink-0 object-contain"
          />
          <span className="text-sm font-(--font-display) text-foreground">pgmanager</span>
        </div>
        <button
          onClick={() => setIsMobileOpen(true)}
          className="-mr-2 p-2 text-ink-muted hover:text-foreground"
        >
          <Menu className="size-5" />
        </button>
      </div>

      <DesktopSidebar selectedKey={selectedKey} />
      <MobileSidebar
        open={isMobileOpen}
        onClose={() => setIsMobileOpen(false)}
        selectedKey={selectedKey}
      />

      <main className="h-full min-w-0 flex-1 overflow-y-auto p-4 sm:p-6">
        <Routes>
          <Route path="/" element={<DatabasesTable />} />
          <Route path="/databases/:name" element={<DatabaseDetail />} />
          <Route path="/databases/:name/tables/:table" element={<TableDetail />} />
          <Route
            path="/users"
            element={user?.role === "admin" ? <Users /> : <Navigate to="/" />}
          />
          <Route
            path="/logs"
            element={user?.role === "admin" ? <Logs /> : <Navigate to="/" />}
          />
          <Route path="/profile" element={<Profile />} />
          <Route
            path="/settings"
            element={user?.role === "admin" ? <Settings /> : <Navigate to="/" />}
          />
          <Route
            path="/backups"
            element={user?.role === "admin" || user?.role === "dev" ? <Backups /> : <Navigate to="/" />}
          />
          <Route
            path="/walg"
            element={user?.role === "admin" ? <WalgBackups /> : <Navigate to="/" />}
          />
        </Routes>
      </main>
    </div>
  );
}
