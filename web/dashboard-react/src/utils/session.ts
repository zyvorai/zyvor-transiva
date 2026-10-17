// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

// The session token lives in memory and in sessionStorage, so a page refresh keeps you signed in
// but closing the tab does not (sessionStorage is per tab and never shared or persisted to disk).
const STORAGE_KEY = "transiva_session";
export const SESSION_ENDED_EVENT = "transiva:session-ended";

let token = "";

function store(value: { token: string; expiresAt?: string } | null) {
  try {
    if (value) sessionStorage.setItem(STORAGE_KEY, JSON.stringify(value));
    else sessionStorage.removeItem(STORAGE_KEY);
  } catch {
    // Storage can be unavailable (private mode, blocked site data): the session is then memory-only.
  }
}

function stored(): { token: string; expiresAt?: string } | null {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const value = JSON.parse(raw);
    return typeof value?.token === "string" && value.token ? value : null;
  } catch {
    return null;
  }
}

export function clearSession() {
  token = "";
  store(null);
}

export async function signIn(username: string, password: string) {
  const response = await fetch("/api/login", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!response.ok)
    throw new Error(
      response.status === 401
        ? "Check your username and password."
        : "Sign in is unavailable. Check that the authenticated daemon is running.",
    );
  const session = await response.json();
  if (!session.token) throw new Error("The server did not return a session.");
  token = session.token;
  store({ token, expiresAt: session.expires_at });
}

/** Ask the daemon whether the current token is still valid. */
async function sessionIsValid(): Promise<boolean | undefined> {
  try {
    const response = await fetch("/api/session", {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (response.ok) return true;
    if (response.status === 401) return false;
  } catch {
    // Network error: unknown, not proof the session ended.
  }
  return undefined;
}

/**
 * Restore a session saved by a previous page load. Resolves true when the daemon still accepts it.
 * A rejected token is cleared; an unreachable daemon leaves it in place for the next attempt.
 */
export async function restoreSession(): Promise<boolean> {
  const saved = stored();
  if (!saved) return false;
  if (saved.expiresAt && Date.parse(saved.expiresAt) < Date.now()) {
    clearSession();
    return false;
  }
  token = saved.token;
  const valid = await sessionIsValid();
  if (valid === false) clearSession();
  return valid === true;
}

let checking: Promise<void> | null = null;

// A 401 can also come from a provider (wrong Prism or vCenter password), so it only ends the
// dashboard session when the daemon itself says the session is gone.
function checkSessionAfter401() {
  if (!token || checking) return;
  checking = sessionIsValid()
    .then((valid) => {
      if (valid === false) {
        clearSession();
        window.dispatchEvent(new Event(SESSION_ENDED_EVENT));
      }
    })
    .finally(() => {
      checking = null;
    });
}

export async function authenticatedFetch(
  input: RequestInfo | URL,
  init?: RequestInit,
) {
  const headers = new Headers(init?.headers);
  if (token) headers.set("Authorization", `Bearer ${token}`);
  const response = await fetch(input, { ...init, headers });
  if (response.status === 401) checkSessionAfter401();
  return response;
}

export async function signOut() {
  try {
    await authenticatedFetch("/api/logout", { method: "POST" });
  } finally {
    clearSession();
  }
}

// Browsers cannot add Authorization headers to a WebSocket handshake.
// Carry the existing session in a protocol token, never in the URL.
export function webSocketProtocols(): string[] | undefined {
  if (!token) return undefined;
  const encoded = btoa(token)
    .replace(/\+/g, "-")
    .replace(/\//g, "_")
    .replace(/=+$/, "");
  return ["transiva-session." + encoded];
}
