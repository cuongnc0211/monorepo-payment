import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";
import { api, setUnauthorizedHandler } from "./client";
import { setSession, clearToken, getToken } from "../auth/token";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
}

// authedFetch calls fetch(Request) for normal calls and fetch(urlString, init) for the refresh —
// normalize both shapes.
function urlOf(input: RequestInfo | URL): string {
  if (typeof input === "string") return input;
  if (input instanceof URL) return input.href;
  return input.url;
}

describe("client transparent refresh (spec 007 AC5/AC6)", () => {
  beforeEach(() => setSession("access-old", "refresh-1", { id: "m1", name: "M" }));
  afterEach(() => {
    vi.unstubAllGlobals();
    clearToken();
  });

  it("refreshes once on a 401 and retries the original request with the new token", async () => {
    let reads = 0;
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (urlOf(input).endsWith("/v1/portal/refresh")) return jsonResponse({ token: "access-new" });
      reads++;
      if (reads === 1) return jsonResponse({ error: { message: "expired" } }, 401);
      expect((input as Request).headers.get("Authorization")).toBe("Bearer access-new"); // retry carries the fresh token
      return jsonResponse({ balances: [] });
    });
    vi.stubGlobal("fetch", fetchMock);

    const { data, error } = await api.GET("/v1/balances", {});
    expect(error).toBeUndefined();
    expect(data).toEqual({ balances: [] });
    expect(getToken()).toBe("access-new");
    expect(reads).toBe(2); // original + one retry
  });

  it("clears the session and notifies on a failed refresh", async () => {
    const onUnauthorized = vi.fn();
    setUnauthorizedHandler(onUnauthorized);
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      // Both the read and the refresh return 401 → refresh fails.
      void urlOf(input);
      return jsonResponse({ error: { message: "expired" } }, 401);
    });
    vi.stubGlobal("fetch", fetchMock);

    await api.GET("/v1/balances", {});
    expect(onUnauthorized).toHaveBeenCalledOnce();
    expect(getToken()).toBeNull();
  });
});
