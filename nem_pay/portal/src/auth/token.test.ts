import { describe, it, expect, beforeEach } from "vitest";
import { setSession, setAccessToken, clearToken, getToken, getRefreshToken, getMerchant } from "./token";

describe("token store (memory-only)", () => {
  beforeEach(() => clearToken());

  it("holds access, refresh and merchant after setSession", () => {
    setSession("a1", "r1", { id: "m1", name: "Merchant One" });
    expect(getToken()).toBe("a1");
    expect(getRefreshToken()).toBe("r1");
    expect(getMerchant()).toEqual({ id: "m1", name: "Merchant One" });
  });

  it("setAccessToken updates only the access token (refresh unchanged)", () => {
    setSession("a1", "r1", { id: "m1", name: "M" });
    setAccessToken("a2");
    expect(getToken()).toBe("a2");
    expect(getRefreshToken()).toBe("r1");
  });

  it("clearToken drops everything", () => {
    setSession("a1", "r1", { id: "m1", name: "M" });
    clearToken();
    expect(getToken()).toBeNull();
    expect(getRefreshToken()).toBeNull();
    expect(getMerchant()).toBeNull();
  });

  it("never writes tokens to localStorage or sessionStorage (spec 007 AC7)", () => {
    setSession("secret-access", "secret-refresh", { id: "m1", name: "M" });
    expect(localStorage.length).toBe(0);
    expect(sessionStorage.length).toBe(0);
  });
});
