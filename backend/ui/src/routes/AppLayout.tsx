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

export default function AppLayout() {
  const location = useLocation();
  const selectedKey =
    location.pathname === "/users"
      ? "users"
      : location.pathname === "/logs"
        ? "logs"
        : "databases";
  const { user } = useAuth();

  const [isMobileOpen, setIsMobileOpen] = useState(false);

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
        <button
          onClick={() => setIsMobileOpen(true)}
          className="p-2 -mr-2 text-muted-foreground hover:text-foreground"
        >
          <Menu className="h-5 w-5" />
        </button>
      </div>

      <DesktopSidebar selectedKey={selectedKey} />
      <MobileSidebar
        open={isMobileOpen}
        onClose={() => setIsMobileOpen(false)}
        selectedKey={selectedKey}
      />

      <main className="flex-1 p-4 sm:p-6 overflow-y-auto min-w-0 h-full">
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
        </Routes>
      </main>
    </div>
  );
}
