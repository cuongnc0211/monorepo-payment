import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { Navigate, useNavigate } from "react-router-dom";
import { api, setUnauthorizedHandler } from "../api/client";
import { clearToken, getMerchant, setSession } from "./token";

type Merchant = { id: string; name: string };

type AuthState = {
  merchant: Merchant | null;
  login: (email: string, password: string) => Promise<string | null>; // returns an error message or null
  logout: () => void;
};

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [merchant, setMerchant] = useState<Merchant | null>(getMerchant());
  const navigate = useNavigate();

  // A 401 from any call drops the session and returns to login (re-login on expiry).
  useEffect(() => {
    setUnauthorizedHandler(() => {
      clearToken();
      setMerchant(null);
      navigate("/login");
    });
  }, [navigate]);

  async function login(email: string, password: string): Promise<string | null> {
    const { data, error } = await api.POST("/v1/portal/login", { body: { email, password } });
    if (error || !data?.token || !data.merchant) {
      return "The email or password is incorrect.";
    }
    const m = data.merchant as Merchant;
    setSession(data.token, m);
    setMerchant(m);
    return null;
  }

  function logout() {
    clearToken();
    setMerchant(null);
    navigate("/login");
  }

  return <AuthContext.Provider value={{ merchant, login, logout }}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within an AuthProvider");
  return ctx;
}

// Guards the authenticated area: no session → login.
export function RequireAuth({ children }: { children: ReactNode }) {
  const { merchant } = useAuth();
  if (!merchant) return <Navigate to="/login" replace />;
  return <>{children}</>;
}
