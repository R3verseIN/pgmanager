import { createContext, useContext, useState, useEffect, type ReactNode } from "react";
import { fetchMe, login as apiLogin, logout as apiLogout, fetchSetupCheck } from "../api/client";

interface User {
  username: string;
  role: "admin" | "dev" | "viewer";
}

interface AuthContextType {
  user: User | null;
  loading: boolean;
  needsSetup: boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  isAdmin: boolean;
  isDev: boolean;
  isViewer: boolean;
}

const AuthContext = createContext<AuthContextType | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);
  const [needsSetup, setNeedsSetup] = useState(false);

  useEffect(() => {
    fetchMe()
      .then((data) => {
        setUser(data);
      })
      .catch(() => {
        fetchSetupCheck()
          .then((needed) => setNeedsSetup(needed))
          .catch(() => {});
      })
      .finally(() => {
        setLoading(false);
      });
  }, []);

  async function login(username: string, password: string) {
    await apiLogin(username, password);
    const data = await fetchMe();
    setUser(data);
    setNeedsSetup(false);
  }

  async function logout() {
    await apiLogout();
    setUser(null);
  }

  return (
    <AuthContext.Provider
      value={{
        user,
        loading,
        needsSetup,
        login,
        logout,
        isAdmin: user?.role === "admin",
        isDev: user?.role === "dev",
        isViewer: user?.role === "viewer",
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}
