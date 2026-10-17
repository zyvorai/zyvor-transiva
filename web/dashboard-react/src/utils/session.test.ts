// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  authenticatedFetch,
  clearSession,
  webSocketProtocols,
  signIn,
  signOut,
} from "./session";
afterEach(() => {
  clearSession();
  vi.unstubAllGlobals();
});
describe("authenticated requests", () => {
  it("rejects invalid credentials without unlocking a session", async () => {
    const fetch = vi.fn().mockResolvedValue({ ok: false, status: 401 });
    vi.stubGlobal("fetch", fetch);
    await expect(signIn("operator", "wrong")).rejects.toThrow(
      "Check your username and password.",
    );
    await authenticatedFetch("/status");
    expect(fetch.mock.calls[1][1].headers.has("Authorization")).toBe(false);
  });
  it("authenticates API requests and clears the token on logout", async () => {
    const fetch = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({ token: "session-token" }),
    });
    vi.stubGlobal("fetch", fetch);
    await signIn("operator", "password");
    expect(webSocketProtocols()).toEqual([
      "transiva-session.c2Vzc2lvbi10b2tlbg",
    ]);
    await authenticatedFetch("/jobs/query", { headers: { "X-Test": "kept" } });
    expect(fetch.mock.calls[1][1].headers.get("Authorization")).toBe(
      "Bearer session-token",
    );
    expect(fetch.mock.calls[1][1].headers.get("X-Test")).toBe("kept");
    await signOut();
    await authenticatedFetch("/status");
    expect(fetch.mock.calls[3][1].headers.has("Authorization")).toBe(false);
    expect(webSocketProtocols()).toBeUndefined();
  });
  it("rejects malformed success responses", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({ ok: true, json: async () => ({}) }),
    );
    await expect(signIn("operator", "password")).rejects.toThrow(
      "did not return a session",
    );
  });
});
