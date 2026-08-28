// Session tokens — IN MEMORY ONLY, never localStorage/cookie (constitution). Both the short-lived
// access token and the longer-lived refresh token live only in the running page; a full reload
// drops them and the user logs in again (reload-persistence is out of scope, spec 007).

type Merchant = { id: string; name: string };

let accessToken: string | null = null;
let refreshToken: string | null = null;
let merchant: Merchant | null = null;

export const getToken = (): string | null => accessToken;
export const getRefreshToken = (): string | null => refreshToken;
export const getMerchant = (): Merchant | null => merchant;

export function setSession(access: string, refresh: string, m: Merchant): void {
  accessToken = access;
  refreshToken = refresh;
  merchant = m;
}

// setAccessToken updates just the access token after a refresh (the refresh token is unchanged).
export function setAccessToken(access: string): void {
  accessToken = access;
}

export function clearToken(): void {
  accessToken = null;
  refreshToken = null;
  merchant = null;
}
