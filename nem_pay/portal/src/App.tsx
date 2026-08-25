import { Routes, Route } from "react-router-dom";
import Login from "./pages/Login";
import AppLayout from "./components/AppLayout";
import { RequireAuth } from "./auth/auth";

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        path="/"
        element={
          <RequireAuth>
            <AppLayout />
          </RequireAuth>
        }
      >
        <Route
          index
          element={
            <div className="placeholder">
              <h1>Signed in</h1>
              <p className="muted">Dashboard pages arrive in the next task.</p>
            </div>
          }
        />
      </Route>
    </Routes>
  );
}
