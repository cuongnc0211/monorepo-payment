import { useState, type FormEvent } from "react";
import {
  useCreateEndpoint,
  useDisableEndpoint,
  useWebhookEndpoints,
  useWebhookEvents,
  PAGE_SIZE,
} from "../api/queries";
import { badgeClass, dt, shortId, webhookStatus } from "../lib/format";

function Endpoints() {
  const { data, isLoading } = useWebhookEndpoints();
  const create = useCreateEndpoint();
  const disable = useDisableEndpoint();
  const endpoints = data?.webhook_endpoints ?? [];

  const [url, setUrl] = useState("");
  const [secret, setSecret] = useState("");
  const [error, setError] = useState<string | null>(null);

  async function onAdd(e: FormEvent) {
    e.preventDefault();
    setError(null);
    try {
      await create.mutateAsync({ url: url.trim(), secret });
      setUrl("");
      setSecret("");
    } catch {
      setError("Could not create the endpoint — check the URL and secret.");
    }
  }

  return (
    <section style={{ marginBottom: "2.4rem" }}>
      <div className="section-title"><h2>Endpoints</h2></div>

      <form className="inline-form" onSubmit={onAdd}>
        <input
          type="url"
          placeholder="https://your-app.example/webhooks"
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          required
        />
        <input
          type="password"
          placeholder="Signing secret"
          value={secret}
          onChange={(e) => setSecret(e.target.value)}
          autoComplete="new-password"
          required
        />
        <button type="submit" disabled={create.isPending}>{create.isPending ? "Adding…" : "Add endpoint"}</button>
      </form>
      {error && <p className="error" role="alert">{error}</p>}

      {isLoading && <p className="loading">Loading…</p>}
      {!isLoading && endpoints.length === 0 && <p className="empty">No endpoints yet — add one above.</p>}
      {endpoints.length > 0 && (
        <table>
          <thead><tr><th>URL</th><th>Status</th><th>Created</th><th></th></tr></thead>
          <tbody>
            {endpoints.map((e) => (
              <tr key={e.id}>
                <td className="mono">{e.url}</td>
                <td>
                  <span className={e.active ? "badge badge--ok" : "badge"}>{e.active ? "active" : "disabled"}</span>
                </td>
                <td>{dt(e.created_at)}</td>
                <td className="num">
                  {e.active && (
                    <button className="link" disabled={disable.isPending} onClick={() => disable.mutate(e.id!)}>
                      Disable
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}

export default function Webhooks() {
  const [page, setPage] = useState(0);
  const { data, isLoading, isError } = useWebhookEvents(page);
  const rows = data?.webhook_events ?? [];

  return (
    <div>
      <div className="page-head"><h1>Webhooks</h1></div>

      <Endpoints />

      <div className="section-title">
        <h2>Recent deliveries</h2>
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
