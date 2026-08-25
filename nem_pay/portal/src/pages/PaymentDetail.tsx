import { Link, useParams } from "react-router-dom";
import { usePayment, usePaymentLedger } from "../api/queries";
import { badgeClass, dt, money } from "../lib/format";

export default function PaymentDetail() {
  const { id = "" } = useParams();
  const { data: pi, isLoading, isError } = usePayment(id);
  const { data: ledger } = usePaymentLedger(id);
  const txns = ledger?.transactions ?? [];

  return (
    <div>
      <Link className="back" to="/">← Payments</Link>
      {isLoading && <p className="loading">Loading…</p>}
      {isError && <p className="error">Could not load this payment.</p>}

      {pi && (
        <>
          <div className="page-head">
            <h1 className="mono">{pi.id}</h1>
            <span className={badgeClass(pi.status!)}>{pi.status}</span>
          </div>

          <div className="cards">
            <div className="card"><div className="card__label">Amount</div><div className="card__value">{money(pi.amount!, pi.currency!)}</div></div>
            <div className="card"><div className="card__label">Settlement</div><div className="card__value" style={{ fontSize: "1.1rem" }}>{pi.settlement_mode}</div></div>
            <div className="card"><div className="card__label">Created</div><div className="card__value" style={{ fontSize: "1rem" }}>{dt(pi.created_at)}</div></div>
            <div className="card"><div className="card__label">Updated</div><div className="card__value" style={{ fontSize: "1rem" }}>{dt(pi.updated_at)}</div></div>
          </div>

          {pi.metadata && Object.keys(pi.metadata).length > 0 && (
            <>
              <h1 style={{ fontSize: "1.05rem", marginTop: "2rem" }}>Metadata</h1>
              <pre className="card mono" style={{ margin: 0, overflow: "auto" }}>{JSON.stringify(pi.metadata, null, 2)}</pre>
            </>
          )}

          <h1 style={{ fontSize: "1.05rem", marginTop: "2rem" }}>Ledger</h1>
          {txns.length === 0 && <p className="empty">No ledger entries yet (the payment has not been captured).</p>}
          {txns.map((t) => (
            <div key={t.id} style={{ marginBottom: "1.4rem" }}>
              <div className="muted" style={{ marginBottom: "0.4rem", fontSize: "0.82rem" }}>
                <span className="badge">{t.kind}</span> · {dt(t.created_at)} · <span className="mono">{t.id?.slice(0, 8)}</span>
              </div>
              <table>
                <thead><tr><th>Account</th><th className="num">Debit</th><th className="num">Credit</th></tr></thead>
                <tbody>
                  {(t.entries ?? []).map((e, i) => (
                    <tr key={i}>
                      <td><span className="muted">{e.account_type}</span> {e.account_kind}</td>
                      <td className="num">{e.debit ? money(e.debit, e.currency!) : "—"}</td>
                      <td className="num">{e.credit ? money(e.credit, e.currency!) : "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ))}
        </>
      )}
    </div>
  );
}
