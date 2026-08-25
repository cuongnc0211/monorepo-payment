// Presentation-only formatting. The portal never does money math — it renders the integer minor
// units the API returns. This just groups digits and places the decimal point for display.
export function money(minor: number, currency: string): string {
  const neg = minor < 0;
  const abs = Math.abs(minor);
  const major = Math.trunc(abs / 100).toLocaleString();
  const cents = String(abs % 100).padStart(2, "0");
  return `${neg ? "-" : ""}${major}.${cents} ${currency}`;
}

export function shortId(id: string): string {
  return id.slice(0, 8);
}

export function dt(s?: string | null): string {
  return s ? new Date(s).toLocaleString() : "—";
}

// Maps a payment/webhook status to a badge class.
export function badgeClass(status: string): string {
  if (["captured", "settled", "paid", "delivered", "released", "held_in_escrow"].includes(status)) return "badge badge--ok";
  if (["failed", "dead", "refunded"].includes(status)) return "badge badge--bad";
  return "badge badge--warn";
}

// Derives a human delivery status for a webhook event from its outbox status + attempts.
export function webhookStatus(status: string, attempts: number): string {
  if (status === "delivered") return "delivered";
  if (status === "dead") return "failed";
  return attempts > 0 ? "retrying" : "pending";
}
