import { Link, useNavigate } from "react-router-dom";
import { useBalances, usePayments, useWebhookEvents } from "../api/queries";
import { badgeClass, dt, money, shortId, webhookStatus } from "../lib/format";

export default function Overview() {
  const navigate = useNavigate();
  const balances = useBalances();
  const payments = usePayments("", 0);
  const webhooks = useWebhookEvents(0);

  const bal = balances.data?.balances ?? [];
  const recentPayments = (payments.data?.data ?? []).slice(0, 5);
  const recentWebhooks = (webhooks.data?.webhook_events ?? []).slice(0, 5);

  return (
    <div>
      <div className="page-head"><h1>Overview</h1></div>

      {balances.isLoading && <p className="loading">Loading…</p>}
      {bal.length > 0 && (
        <div className="cards">
          {bal.map((b, i) => (
            <div className="card" key={i}>
              <div className="card__label">{b.kind} · {b.currency}</div>
              <div className="card__value">{money(b.balance ?? 0, b.currency ?? "")}</div>
              <div className="muted" style={{ fontSize: "0.72rem", marginTop: "0.4rem" }}>{b.type}</div>
            </div>
          ))}
        </div>
      )}

      <div className="overview-grid">
        <section>
          <div className="section-title">
            <h2>Recent payments</h2>
            <Link to="/payments">View all →</Link>
          </div>
          {recentPayments.length === 0 ? (
            <p className="empty">No payments yet.</p>
          ) : (
            <table>
              <thead><tr><th>Intent</th><th className="num">Amount</th><th>Status</th></tr></thead>
              <tbody>
                {recentPayments.map((p) => (
                  <tr key={p.id} className="clickable" onClick={() => navigate(`/payments/${p.id}`)}>
                    <td className="mono">{shortId(p.id!)}</td>
                    <td className="num">{money(p.amount!, p.currency!)}</td>
                    <td><span className={badgeClass(p.status!)}>{p.status}</span></td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </section>

        <section>
          <div className="section-title">
            <h2>Recent webhooks</h2>
            <Link to="/webhooks">View all →</Link>
          </div>
          {recentWebhooks.length === 0 ? (
            <p className="empty">No webhook events yet.</p>
          ) : (
            <table>
              <thead><tr><th>Type</th><th>Delivery</th><th>Created</th></tr></thead>
              <tbody>
                {recentWebhooks.map((e) => {
                  const s = webhookStatus(e.status!, e.attempts ?? 0);
                  return (
                    <tr key={e.event_id}>
                      <td>{e.event_type}</td>
                      <td><span className={badgeClass(s)}>{s}</span></td>
                      <td>{dt(e.created_at)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </section>
      </div>
    </div>
  );
}
