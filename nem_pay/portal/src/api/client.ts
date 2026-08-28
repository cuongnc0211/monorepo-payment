import createClient from "openapi-fetch";
import type { paths } from "./schema";
import { clearToken, getRefreshToken, getToken, setAccessToken } from "../auth/token";

// The gateway base URL. Same-origin dev default is the local gateway; override via env for other
// setups. All portal data flows through this typed /v1 client — no hand-written fetch, no backdoor.
// Exported so the UI can link to gateway-served pages (e.g. the API docs at /docs).
export const apiBaseUrl = (import.meta.env.VITE_NEMPAY_API_URL as string | undefined) ?? "http://localhost:8080";

const REFRESH_PATH = "/v1/portal/refresh";

// A 401 that even a refresh can't fix clears the session and notifies the app to bounce to login.
let onUnauthorized: (() => void) | null = null;
export function setUnauthorizedHandler(fn: () => void): void {
  onUnauthorized = fn;
}

// tryRefresh exchanges the in-memory refresh token for a fresh access token. Uses the raw fetch so
// it never recurses through the authed client.
async function tryRefresh(): Promise<boolean> {
  const refresh = getRefreshToken();
  if (!refresh) return false;
  const res = await fetch(`${apiBaseUrl}${REFRESH_PATH}`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refresh_token: refresh }),
  });
  if (!res.ok) return false;
  const data = (await res.json().catch(() => null)) as { token?: string } | null;
  if (!data?.token) return false;
  setAccessToken(data.token);
  return true;
}

// authedFetch injects the access bearer and, on a 401 caused by an expired access token,
// transparently refreshes once and retries the original request. It never refreshes the refresh
// call itself, and retries at most once — so there is no loop.
const authedFetch: typeof fetch = async (input, init) => {
  const base = input instanceof Request ? input : new Request(input, init);
  const attempt = () => {
    const req = base.clone();
    const token = getToken();
    if (token) req.headers.set("Authorization", `Bearer ${token}`);
    return fetch(req);
  };

  let res = await attempt();
  if (res.status === 401 && !base.url.endsWith(REFRESH_PATH) && getRefreshToken()) {
    if (await tryRefresh()) {
      res = await attempt();
    } else {
      clearToken();
      onUnauthorized?.();
    }
  }
  return res;
};

export const api = createClient<paths>({ baseUrl: apiBaseUrl, fetch: authedFetch });
