// Session token store — IN MEMORY ONLY, never localStorage (constitution: portal auth is held in
// memory). A full-page reload drops the session and the user logs in again (no refresh in the first
// cut). Kept outside React so the API client middleware can read it without a hook.

type Merchant = { id: string; name: string };

let token: string | null = null;
let merchant: Merchant | null = null;

export const getToken = (): string | null => token;
export const getMerchant = (): Merchant | null => merchant;

export function setSession(t: string, m: Merchant): void {
  token = t;
  merchant = m;
}

export function clearToken(): void {
  token = null;
  merchant = null;
}
