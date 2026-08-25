import { useBalances } from "../api/queries";
import { money } from "../lib/format";

export default function Balances() {
  const { data, isLoading, isError } = useBalances();
  const rows = data?.balances ?? [];

  return (
    <div>
      <div className="page-head"><h1>Balances</h1></div>
      {isLoading && <p className="loading">Loading…</p>}
      {isError && <p className="error">Could not load balances.</p>}
      {!isLoading && !isError && rows.length === 0 && <p className="empty">No balances yet — balances appear once money moves through the ledger.</p>}
      <div className="cards">
        {rows.map((b, i) => (
          <div className="card" key={i}>
            <div className="card__label">{b.kind} · {b.currency}</div>
            <div className="card__value">{money(b.balance ?? 0, b.currency ?? "")}</div>
            <div className="muted" style={{ fontSize: "0.72rem", marginTop: "0.4rem" }}>{b.type}</div>
          </div>
        ))}
      </div>
    </div>
  );
}
