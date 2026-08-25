import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { usePayments, PAGE_SIZE } from "../api/queries";
import { badgeClass, dt, money, shortId } from "../lib/format";

const STATUSES = [
  "", "created", "requires_confirmation", "authorized", "captured", "settled", "failed", "refunded", "partially_refunded",
];

export default function Payments() {
  const [status, setStatus] = useState("");
  const [page, setPage] = useState(0);
  const navigate = useNavigate();
  const { data, isLoading, isError } = usePayments(status, page);
  const rows = data?.data ?? [];

  return (
    <div>
      <div className="page-head">
        <h1>Payments</h1>
        <div className="toolbar">
          <select
            value={status}
            onChange={(e) => { setStatus(e.target.value); setPage(0); }}
            aria-label="Filter by status"
          >
            {STATUSES.map((s) => <option key={s} value={s}>{s === "" ? "All statuses" : s}</option>)}
          </select>
          <button disabled={page === 0} onClick={() => setPage((p) => Math.max(0, p - 1))}>← Prev</button>
          <button disabled={rows.length < PAGE_SIZE} onClick={() => setPage((p) => p + 1)}>Next →</button>
        </div>
      </div>

      {isLoading && <p className="loading">Loading…</p>}
      {isError && <p className="error">Could not load payments.</p>}
      {!isLoading && !isError && rows.length === 0 && <p className="empty">No payments yet.</p>}

      {rows.length > 0 && (
        <table>
          <thead>
            <tr><th>Intent</th><th>Amount</th><th>Status</th><th>Mode</th><th>Created</th></tr>
          </thead>
          <tbody>
            {rows.map((p) => (
              <tr key={p.id} className="clickable" onClick={() => navigate(`/payments/${p.id}`)}>
                <td className="mono">{shortId(p.id!)}</td>
                <td className="num">{money(p.amount!, p.currency!)}</td>
                <td><span className={badgeClass(p.status!)}>{p.status}</span></td>
                <td>{p.settlement_mode}</td>
                <td>{dt(p.created_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
