import { NavLink, Outlet } from "react-router-dom";
import { useAuth } from "../auth/auth";
import { apiBaseUrl } from "../api/client";

const NAV = [
  { to: "/", label: "Payments", end: true },
  { to: "/balances", label: "Balances", end: false },
  { to: "/webhooks", label: "Webhooks", end: false },
  { to: "/keys", label: "API keys", end: false },
];

export default function AppLayout() {
  const { merchant, logout } = useAuth();
  return (
    <div className="app">
      <aside className="sidebar">
        <div className="brand">Nem<span>Pay</span></div>
        <nav>
          {NAV.map((n) => (
            <NavLink key={n.to} to={n.to} end={n.end} className={({ isActive }) => (isActive ? "active" : "")}>
              {n.label}
            </NavLink>
          ))}
          <a href={`${apiBaseUrl}/docs`} target="_blank" rel="noreferrer">API docs ↗</a>
        </nav>
        <div className="sidebar__foot">
          <div className="muted">{merchant?.name}</div>
          <button className="link" onClick={logout}>Sign out</button>
        </div>
      </aside>
      <main className="content">
        <Outlet />
      </main>
    </div>
  );
}
