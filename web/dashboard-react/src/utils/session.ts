// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

let token = "";
export function clearSession() {
  token = "";
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
}
export function authenticatedFetch(
  input: RequestInfo | URL,
  init?: RequestInit,
) {
  const headers = new Headers(init?.headers);
  if (token) headers.set("Authorization", `Bearer ${token}`);
  return fetch(input, { ...init, headers });
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
