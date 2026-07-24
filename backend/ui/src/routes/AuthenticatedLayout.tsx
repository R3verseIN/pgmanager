import { Routes, Route, Navigate } from "react-router-dom";
import { Loader2 } from "lucide-react";
import { useAuth } from "../contexts/AuthContext";
import Login from "./Login";
import Setup from "./Setup";
import AppLayout from "./AppLayout";

export default function AuthenticatedLayout() {
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
        <Route
          path="/setup"
          element={needsSetup ? <Setup /> : <Navigate to="/login" />}
        />
        <Route
          path="/login"
          element={needsSetup ? <Navigate to="/setup" /> : <Login />}
        />
        <Route
          path="*"
          element={needsSetup ? <Navigate to="/setup" /> : <Login />}
        />
      </Routes>
    );
  }

  return <AppLayout />;
}
