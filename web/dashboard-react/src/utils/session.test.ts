// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  SESSION_ENDED_EVENT,
  authenticatedFetch,
  clearSession,
  restoreSession,
  signIn,
  webSocketProtocols,
} from "./session";

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status });

describe("dashboard session", () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    sessionStorage.clear();
    clearSession();
    fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
  });
  afterEach(() => vi.unstubAllGlobals());

  it("keeps the session in sessionStorage after sign-in", async () => {
    fetchMock.mockResolvedValueOnce(
      json({ token: "tok-1", expires_at: "2999-01-01T00:00:00Z" }),
    );
    await signIn("alice", "pw");
    expect(JSON.parse(sessionStorage.getItem("transiva_session")!).token).toBe(
      "tok-1",
    );
  });

  it("explains a lockout and an unconfigured server instead of a generic error", async () => {
    fetchMock.mockResolvedValueOnce(new Response("locked", { status: 429 }));
    await expect(signIn("alice", "pw")).rejects.toThrow(/Too many failed attempts/);
    fetchMock.mockResolvedValueOnce(new Response("no pam", { status: 503 }));
    await expect(signIn("alice", "pw")).rejects.toThrow(/PAM/);
  });

  it("restores a saved session after a page refresh when the daemon accepts it", async () => {
    sessionStorage.setItem(
      "transiva_session",
      JSON.stringify({ token: "tok-2", expiresAt: "2999-01-01T00:00:00Z" }),
    );
    fetchMock.mockResolvedValueOnce(json({ username: "alice" }));
    expect(await restoreSession()).toBe(true);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/session",
      expect.objectContaining({
        headers: { Authorization: "Bearer tok-2" },
      }),
    );
    expect(webSocketProtocols()).toBeDefined();
  });

  it("drops a saved session the daemon rejects", async () => {
    sessionStorage.setItem("transiva_session", JSON.stringify({ token: "old" }));
    fetchMock.mockResolvedValueOnce(new Response("no", { status: 401 }));
    expect(await restoreSession()).toBe(false);
    expect(sessionStorage.getItem("transiva_session")).toBeNull();
  });

  it("drops a saved session whose expiry has passed without calling the daemon", async () => {
    sessionStorage.setItem(
      "transiva_session",
      JSON.stringify({ token: "old", expiresAt: "2000-01-01T00:00:00Z" }),
    );
    expect(await restoreSession()).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(sessionStorage.getItem("transiva_session")).toBeNull();
  });

  it("keeps the saved session when the daemon is unreachable", async () => {
    sessionStorage.setItem("transiva_session", JSON.stringify({ token: "keep" }));
    fetchMock.mockRejectedValueOnce(new TypeError("network down"));
    expect(await restoreSession()).toBe(false);
    expect(sessionStorage.getItem("transiva_session")).not.toBeNull();
  });

  it("ends the session when a 401 is confirmed by /api/session", async () => {
    fetchMock.mockResolvedValueOnce(json({ token: "tok-3" }));
    await signIn("alice", "pw");
    const ended = vi.fn();
    window.addEventListener(SESSION_ENDED_EVENT, ended);
    fetchMock
      .mockResolvedValueOnce(new Response("no", { status: 401 })) // the API call
      .mockResolvedValueOnce(new Response("no", { status: 401 })); // /api/session
    await authenticatedFetch("/api/jobs");
    await vi.waitFor(() => expect(ended).toHaveBeenCalledTimes(1));
    expect(sessionStorage.getItem("transiva_session")).toBeNull();
    window.removeEventListener(SESSION_ENDED_EVENT, ended);
  });

  it("does not end the session for a provider 401 while the session is still valid", async () => {
    fetchMock.mockResolvedValueOnce(json({ token: "tok-4" }));
    await signIn("alice", "pw");
    const ended = vi.fn();
    window.addEventListener(SESSION_ENDED_EVENT, ended);
    fetchMock
      .mockResolvedValueOnce(new Response("bad prism password", { status: 401 }))
      .mockResolvedValueOnce(json({ username: "alice" })); // /api/session still fine
    await authenticatedFetch("/api/nutanix/vms");
    await new Promise((r) => setTimeout(r, 20));
    expect(ended).not.toHaveBeenCalled();
    expect(sessionStorage.getItem("transiva_session")).not.toBeNull();
    window.removeEventListener(SESSION_ENDED_EVENT, ended);
  });
});
