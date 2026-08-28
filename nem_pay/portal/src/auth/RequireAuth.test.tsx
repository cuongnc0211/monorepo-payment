import { describe, it, expect, afterEach } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { AuthProvider, RequireAuth } from "./auth";
import { setSession, clearToken } from "./token";

function renderAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<div>LOGIN PAGE</div>} />
          <Route path="/" element={<RequireAuth><div>DASHBOARD</div></RequireAuth>} />
        </Routes>
      </AuthProvider>
    </MemoryRouter>,
  );
}

describe("RequireAuth guard (spec 005 AC1)", () => {
  afterEach(() => clearToken());

  it("redirects to login when there is no session", () => {
    clearToken();
    renderAt("/");
    expect(screen.getByText("LOGIN PAGE")).toBeInTheDocument();
    expect(screen.queryByText("DASHBOARD")).not.toBeInTheDocument();
  });

  it("renders the protected content when signed in", () => {
    setSession("a", "r", { id: "m1", name: "M" });
    renderAt("/");
    expect(screen.getByText("DASHBOARD")).toBeInTheDocument();
  });
});
