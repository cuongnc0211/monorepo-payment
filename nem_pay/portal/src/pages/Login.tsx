import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth/auth";

export default function Login() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const [email, setEmail] = useState("owner@merchant-a.test");
  const [password, setPassword] = useState("portal-dev-password");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const err = await login(email.trim(), password);
    setBusy(false);
    if (err) setError(err);
    else navigate("/");
  }

  return (
    <div className="login">
      <div className="login__card">
        <div className="brand">Nem<span>Pay</span> <em>Portal</em></div>
        <p className="muted">Sign in to your merchant dashboard.</p>
        <form onSubmit={onSubmit}>
          <label>
            Email
            <input type="email" value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="username" />
          </label>
          <label>
            Password
            <input type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
          </label>
          {error && <p className="error" role="alert">{error}</p>}
          <button type="submit" disabled={busy}>{busy ? "Signing in…" : "Sign in"}</button>
        </form>
        <p className="hint">Dev users: <code>owner@merchant-a.test</code> / <code>owner@merchant-b.test</code> · password <code>portal-dev-password</code></p>
      </div>
    </div>
  );
}
