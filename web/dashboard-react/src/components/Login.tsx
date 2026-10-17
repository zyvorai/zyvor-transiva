// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

import { useState } from "react";
import { ArrowRight, Box, Eye, EyeOff, LockKeyhole } from "lucide-react";
import { MigrationGraphic } from "./MigrationGraphic";
interface LoginProps {
  onLogin: (username: string, password: string) => Promise<void>;
}
export function Login({ onLogin }: LoginProps) {
  const [username, setUsername] = useState(
    () => localStorage.getItem("transiva_username") || "",
  );
  const [password, setPassword] = useState("");
  const [remember, setRemember] = useState(false);
  const [visible, setVisible] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setLoading(true);
    setError("");
    try {
      await onLogin(username.trim(), password);
      if (remember) localStorage.setItem("transiva_username", username.trim());
      else localStorage.removeItem("transiva_username");
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Unable to sign in. Try again.",
      );
    } finally {
      setLoading(false);
    }
  }
  return (
    <div className="login-page">
      <header className="login-nav">
        <a className="wordmark" href="#">
          <Box size={23} /> Transiva <span>by Zyvor</span>
        </a>
        <a href="https://github.com/zyvorai/zyvor-transiva/tree/main/docs">
          Documentation <ArrowRight size={14} />
        </a>
      </header>
      <main className="login-stage">
        <section className="login-story">
          <p className="eyebrow">A NEW HOME FOR YOUR VIRTUAL MACHINES</p>
          <h1>
            Big moves.
            <br />
            <span>Beautifully simple.</span>
          </h1>
          <p className="lede">
            From your first export to your next infrastructure.
            <br />
            One calm place to make it happen.
          </p>
          <MigrationGraphic />
          <div className="story-notes">
            <span>
              01 <b>Discover</b>
            </span>
            <span>
              02 <b>Export</b>
            </span>
            <span>
              03 <b>Validate</b>
            </span>
          </div>
        </section>
        <section className="signin-card" aria-labelledby="signin-title">
          <div className="signin-icon">
            <LockKeyhole size={28} />
          </div>
          <p className="eyebrow">YOUR MIGRATION WORKSPACE</p>
          <h2 id="signin-title">Welcome to Transiva.</h2>
          <p>Sign in. Your next chapter starts here.</p>
          <form onSubmit={submit} aria-busy={loading}>
            {error && (
              <div className="notice error" role="alert">
                {error}
              </div>
            )}
            <label htmlFor="username">Username</label>
            <input
              id="username"
              autoComplete="username"
              required
              value={username}
              disabled={loading}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="System username"
            />
            <label htmlFor="password">Password</label>
            <div className="password-field">
              <input
                id="password"
                type={visible ? "text" : "password"}
                autoComplete="current-password"
                required
                value={password}
                disabled={loading}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="Enter your password"
              />
              <button
                type="button"
                aria-label={visible ? "Hide password" : "Show password"}
                onClick={() => setVisible(!visible)}
              >
                {visible ? <EyeOff size={18} /> : <Eye size={18} />}
              </button>
            </div>
            <label className="check-label">
              <input
                type="checkbox"
                checked={remember}
                onChange={(e) => setRemember(e.target.checked)}
              />
              Remember my username
            </label>
            <button
              className="primary signin-submit"
              disabled={loading || !username.trim() || !password}
            >
              {loading ? "Signing in…" : "Sign in"}
              <ArrowRight size={18} />
            </button>
          </form>
          <p className="signin-foot">
            <LockKeyhole size={13} /> Your sign-in password is never saved here
          </p>
        </section>
      </main>
      <footer className="login-footer">
        <span>© {new Date().getFullYear()} Zyvor AI Labs</span>
        <span>Thoughtfully built. Ready for what’s next.</span>
      </footer>
    </div>
  );
}
export function LoginLoading() {
  return (
    <div className="loading-state" role="status">
      Opening Transiva…
    </div>
  );
}
