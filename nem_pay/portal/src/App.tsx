import { Routes, Route } from "react-router-dom";
import Login from "./pages/Login";
import Overview from "./pages/Overview";
import Payments from "./pages/Payments";
import PaymentDetail from "./pages/PaymentDetail";
import Balances from "./pages/Balances";
import Webhooks from "./pages/Webhooks";
import ApiKeys from "./pages/ApiKeys";
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
        <Route index element={<Overview />} />
        <Route path="payments" element={<Payments />} />
        <Route path="payments/:id" element={<PaymentDetail />} />
        <Route path="balances" element={<Balances />} />
        <Route path="webhooks" element={<Webhooks />} />
        <Route path="keys" element={<ApiKeys />} />
      </Route>
    </Routes>
  );
}
