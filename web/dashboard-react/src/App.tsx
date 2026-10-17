// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from "react";
import {
  Box,
  LayoutDashboard,
  ArrowUpRight,
  ListChecks,
  Workflow,
  Server,
  FileJson,
  Sun,
  Moon,
  LogOut,
  Search,
  X,
} from "lucide-react";
import { Login } from "./components/Login";
import { Dashboard } from "./components/Dashboard";
import { WorkflowDashboard } from "./components/WorkflowDashboard";
import { ManifestBuilder } from "./components/ManifestBuilder";
import ProviderManager from "./components/ProviderManager";
import ExportWorkflowRouter from "./components/ExportWorkflowRouter";
import {
  SESSION_ENDED_EVENT,
  restoreSession,
  signIn,
  signOut,
} from "./utils/session";
const views = [
  {
    id: "overview",
    label: "Overview",
    icon: LayoutDashboard,
    title: "Every move. In view.",
    description: "A clear picture of your migration operations.",
  },
  {
    id: "export",
    label: "New export",
    icon: ArrowUpRight,
    title: "A fresh start.",
    description: "Connect your source. Choose your machines. Make your move.",
  },
  {
    id: "jobs",
    label: "Jobs",
    icon: ListChecks,
    title: "Progress, made clear.",
    description: "Follow your exports from the queue to completion.",
  },
  {
    id: "workflows",
    label: "Workflows",
    icon: Workflow,
    title: "From source to ready.",
    description: "Keep your conversion pipeline moving with confidence.",
  },
  {
    id: "providers",
    label: "Providers",
    icon: Server,
    title: "All your connections.",
    description: "Bring your infrastructure together in one workspace.",
  },
  {
    id: "manifest",
    label: "Manifests",
    icon: FileJson,
    title: "The details that travel.",
    description: "Build a portable description of your VM artifacts.",
  },
] as const;
type View = (typeof views)[number]["id"];
function initialView(): View {
  const id = location.hash.slice(1);
  return views.find((v) => v.id === id)?.id || "overview";
}
export default function App() {
  const [authenticated, setAuthenticated] = useState(false);
  // True until we know whether a saved session can be restored, so the login form does not flash.
  const [restoring, setRestoring] = useState(true);
  const [view, setView] = useState<View>(initialView);
  const [dark, setDark] = useState(
    () => localStorage.getItem("transiva_theme") === "dark",
  );
  const [search, setSearch] = useState<string | null>(null);
  useEffect(() => {
    let active = true;
    restoreSession().then((ok) => {
      if (!active) return;
      setAuthenticated(ok);
      setRestoring(false);
    });
    const ended = () => setAuthenticated(false);
    window.addEventListener(SESSION_ENDED_EVENT, ended);
    return () => {
      active = false;
      window.removeEventListener(SESSION_ENDED_EVENT, ended);
    };
  }, []);
  useEffect(() => {
    ["hypersdk_username", "hypersdk_password", "hypersdk_remember"].forEach(
      (k) => localStorage.removeItem(k),
    );
    const sync = () => setView(initialView());
    window.addEventListener("hashchange", sync);
    return () => window.removeEventListener("hashchange", sync);
  }, []);
  useEffect(() => {
    document.documentElement.dataset.theme = dark ? "dark" : "light";
    localStorage.setItem("transiva_theme", dark ? "dark" : "light");
  }, [dark]);
  useEffect(() => {
    const handle = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === "k") {
        e.preventDefault();
        setSearch((s) => (s === null ? "" : null));
      }
      if (e.key === "Escape") setSearch(null);
    };
    window.addEventListener("keydown", handle);
    return () => window.removeEventListener("keydown", handle);
  }, []);
  function navigate(id: View) {
    location.hash = id;
    setView(id);
    setSearch(null);
    window.scrollTo(0, 0);
  }
  if (restoring) return null;
  if (!authenticated)
    return (
      <Login
        onLogin={async (u, p) => {
          await signIn(u, p);
          setAuthenticated(true);
        }}
      />
    );
  const page = views.find((v) => v.id === view)!;
  return (
    <div className="workspace">
      <a className="skip-link" href="#workspace-main">
        Skip to content
      </a>
      <aside className="sidebar">
        <a className="wordmark" href="#overview">
          <Box size={25} /> Transiva
        </a>
        <p className="sidebar-caption">MIGRATION WORKSPACE</p>
        <nav aria-label="Workspace">
          {views.map((v) => (
            <button
              key={v.id}
              aria-current={view === v.id ? "page" : undefined}
              onClick={() => navigate(v.id)}
            >
              <v.icon size={18} />
              {v.label}
            </button>
          ))}
        </nav>
        <div className="sidebar-bottom">
          <div className="suite-mark">
            <Box size={18} />
            <span>
              ZYVOR <small>Built for your next chapter.</small>
            </span>
          </div>
          <a href="https://github.com/zyvorai/zyvor-transiva/tree/main/docs">
            Guides & documentation ↗
          </a>
        </div>
      </aside>
      <div className="workspace-body">
        <header className="workspace-bar">
          <span>
            Workspace <span className="slash">/</span> <b>{page.label}</b>
          </span>
          <div>
            <button onClick={() => setSearch("")} aria-label="Search pages">
              <Search size={17} />
              <span>Search</span>
              <kbd>⌘ K</kbd>
            </button>
            <button
              onClick={() => setDark(!dark)}
              aria-label={dark ? "Use light appearance" : "Use dark appearance"}
            >
              {dark ? <Sun size={18} /> : <Moon size={18} />}
            </button>
            <button
              onClick={async () => {
                try {
                  await signOut();
                } catch {
                  // The local session is already cleared even if the server is unavailable.
                } finally {
                  setAuthenticated(false);
                }
              }}
              aria-label="Sign out"
            >
              <LogOut size={18} />
            </button>
          </div>
        </header>
        <main id="workspace-main" tabIndex={-1}>
          <div className="page-heading">
            <div>
              <p className="eyebrow">TRANSIVA / {page.label.toUpperCase()}</p>
              <h1>{page.title}</h1>
              <p className="lede">{page.description}</p>
            </div>
            {view !== "export" && (
              <button className="primary" onClick={() => navigate("export")}>
                New export <ArrowUpRight size={17} />
              </button>
            )}
          </div>
          {view === "overview" && <Dashboard onNavigate={navigate} />}
          {view === "jobs" && <Dashboard jobsOnly onNavigate={navigate} />}
          <div className="legacy-content">
            {view === "export" && <ExportWorkflowRouter />}
            {view === "workflows" && <WorkflowDashboard />}
            {view === "providers" && (
              <ProviderManager onProviderSelect={() => navigate("export")} />
            )}
            {view === "manifest" && <ManifestBuilder />}
          </div>
        </main>
        <footer className="workspace-footer">
          Transiva{" "}
          <span>Made by Zyvor AI Labs · Built to move you forward.</span>
        </footer>
      </div>
      {search !== null && (
        <div className="search-backdrop" onClick={() => setSearch(null)}>
          <section
            className="command-palette"
            role="dialog"
            aria-modal="true"
            aria-label="Go to page"
            onClick={(e) => e.stopPropagation()}
            onKeyDown={(e) => {
              if (e.key === "Tab") {
                const els =
                  e.currentTarget.querySelectorAll<HTMLElement>("input,button");
                const first = els[0],
                  last = els[els.length - 1];
                if (e.shiftKey && document.activeElement === first) {
                  e.preventDefault();
                  last.focus();
                } else if (!e.shiftKey && document.activeElement === last) {
                  e.preventDefault();
                  first.focus();
                }
              }
            }}
          >
            <div>
              <input
                autoFocus
                aria-label="Search workspace pages"
                placeholder="Where would you like to go?"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
              <button aria-label="Close search" onClick={() => setSearch(null)}>
                <X size={20} />
              </button>
            </div>
            {views
              .filter((v) =>
                v.label.toLowerCase().includes(search.toLowerCase()),
              )
              .map((v) => (
                <button key={v.id} onClick={() => navigate(v.id)}>
                  <v.icon size={18} />
                  {v.label}
                  <ArrowUpRight size={15} />
                </button>
              ))}
            {!views.some((v) =>
              v.label.toLowerCase().includes(search.toLowerCase()),
            ) && <p>No matching pages.</p>}
          </section>
        </div>
      )}
    </div>
  );
}
