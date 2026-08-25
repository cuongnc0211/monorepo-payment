import { useApiKeys } from "../api/queries";
import { dt } from "../lib/format";

export default function ApiKeys() {
  const { data, isLoading, isError } = useApiKeys();
  const rows = data?.api_keys ?? [];

  return (
    <div>
      <div className="page-head"><h1>API keys</h1></div>
      {isLoading && <p className="loading">Loading…</p>}
      {isError && <p className="error">Could not load API keys.</p>}
      {!isLoading && !isError && rows.length === 0 && <p className="empty">No API keys.</p>}

      {rows.length > 0 && (
        <table>
          <thead>
            <tr><th>Kind</th><th>Key</th><th>Created</th><th>Status</th></tr>
          </thead>
          <tbody>
            {rows.map((k) => (
              <tr key={k.id}>
                <td>{k.kind}</td>
                <td className="mono">{k.masked}</td>
                <td>{dt(k.created_at)}</td>
                <td>{k.revoked_at ? <span className="badge badge--bad">revoked</span> : <span className="badge badge--ok">active</span>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <p className="hint muted" style={{ marginTop: "1rem", fontSize: "0.78rem" }}>
        Secrets are never shown — only the non-secret prefix. Create or revoke keys via the API.
      </p>
    </div>
  );
}
