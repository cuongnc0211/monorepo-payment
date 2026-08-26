import createClient, { type Middleware } from "openapi-fetch";
import type { paths } from "./schema";
import { getToken, clearToken } from "../auth/token";

// The gateway base URL. Same-origin dev default is the local gateway; override via env for other
// setups. All portal data flows through this typed /v1 client — no hand-written fetch, no backdoor.
// Exported so the UI can link to gateway-served pages (e.g. the API docs at /docs).
export const apiBaseUrl = (import.meta.env.VITE_NEMPAY_API_URL as string | undefined) ?? "http://localhost:8080";
const baseUrl = apiBaseUrl;

// A 401 (expired/invalid session) clears the token and notifies the app to bounce to login.
let onUnauthorized: (() => void) | null = null;
export function setUnauthorizedHandler(fn: () => void): void {
  onUnauthorized = fn;
}

const authMiddleware: Middleware = {
  onRequest({ request }) {
    const token = getToken();
    if (token) request.headers.set("Authorization", `Bearer ${token}`);
    return request;
  },
  onResponse({ response }) {
    if (response.status === 401) {
      clearToken();
      onUnauthorized?.();
    }
    return response;
  },
};

export const api = createClient<paths>({ baseUrl });
api.use(authMiddleware);
