import { useState } from "react";
import { useWebhookEvents, PAGE_SIZE } from "../api/queries";
import { badgeClass, dt, shortId, webhookStatus } from "../lib/format";

export default function Webhooks() {
  const [page, setPage] = useState(0);
  const { data, isLoading, isError } = useWebhookEvents(page);
  const rows = data?.webhook_events ?? [];

  return (
    <div>
      <div className="page-head">
        <h1>Webhooks</h1>
        <div className="toolbar">
          <button disabled={page === 0} onClick={() => setPage((p) => Math.max(0, p - 1))}>← Prev</button>
          <button disabled={rows.length < PAGE_SIZE} onClick={() => setPage((p) => p + 1)}>Next →</button>
        </div>
      </div>

      {isLoading && <p className="loading">Loading…</p>}
      {isError && <p className="error">Could not load webhook events.</p>}
      {!isLoading && !isError && rows.length === 0 && <p className="empty">No webhook events yet.</p>}

      {rows.length > 0 && (
        <table>
          <thead>
            <tr><th>Event</th><th>Type</th><th>Delivery</th><th className="num">Attempts</th><th>Created</th></tr>
          </thead>
          <tbody>
            {rows.map((e) => {
              const s = webhookStatus(e.status!, e.attempts ?? 0);
              return (
                <tr key={e.event_id}>
                  <td className="mono">{shortId(e.event_id!)}</td>
                  <td>{e.event_type}</td>
                  <td><span className={badgeClass(s)}>{s}</span></td>
                  <td className="num">{e.attempts}</td>
                  <td>{dt(e.created_at)}</td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
    </div>
  );
}
