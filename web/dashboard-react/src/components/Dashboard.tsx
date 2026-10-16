// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

import { useEffect, useState } from "react";
import {
  ArrowUpRight,
  Activity,
  CheckCircle2,
  Clock3,
  CircleAlert,
} from "lucide-react";
import { JobsTable } from "./JobsTable";
import { AlertsList } from "./AlertsList";
import { ChartContainer } from "./ChartContainer";
import { MigrationGraphic } from "./MigrationGraphic";
import { TerminalPanel } from "./TerminalPanel";
import { useWebSocket } from "../hooks/useWebSocket";
import { useMetricsHistory } from "../hooks/useMetricsHistory";
import { cancelJob } from "../utils/api";
import { formatBytes } from "../utils/formatters";
export function Dashboard({
  jobsOnly = false,
  onNavigate,
}: {
  jobsOnly?: boolean;
  onNavigate: (view: "export" | "jobs" | "providers" | "manifest") => void;
}) {
  const { data: metrics, connected } = useWebSocket({
    url: `${location.protocol === "https:" ? "wss" : "ws"}://${location.host}/ws`,
  });
  const { history, addMetrics } = useMetricsHistory(60);
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("all");
  const [error, setError] = useState("");
  useEffect(() => {
    if (metrics) addMetrics(metrics);
  }, [metrics, addMetrics]);
  const jobs = (metrics?.recent_jobs || []).filter(
    (j) =>
      (status === "all" || j.status === status) &&
      `${j.name} ${j.vm_name} ${j.id}`
        .toLowerCase()
        .includes(query.toLowerCase()),
  );
  const stats = [
    {
      name: "Active exports",
      value: metrics?.jobs_active,
      icon: Activity,
      tone: "blue",
    },
    {
      name: "Completed",
      value: metrics?.jobs_completed,
      icon: CheckCircle2,
      tone: "green",
    },
    {
      name: "In the queue",
      value: metrics?.jobs_pending,
      icon: Clock3,
      tone: "purple",
    },
    {
      name: "Needs attention",
      value: metrics?.jobs_failed,
      icon: CircleAlert,
      tone: "orange",
    },
  ];
  return (
    <div className="overview">
      <div
        className={`connection-pill ${connected ? "online" : ""}`}
        role="status"
      >
        <i />
        {connected ? "Live connection" : "Waiting for the metrics connection"}
        {metrics && (
          <span>
            Last update: {new Date(metrics.timestamp).toLocaleTimeString()}
          </span>
        )}
      </div>
      {!jobsOnly && (
        <>
          <section className="migration-banner">
            <div>
              <p className="eyebrow">INFRASTRUCTURE, IN MOTION</p>
              <h2>
                New possibilities.
                <br />
                <span>Same virtual machines.</span>
              </h2>
              <p>
                Discover, export, and prepare your workloads
                <br />
                for the infrastructure you choose.
              </p>
              <button
                className="text-link"
                onClick={() => onNavigate("export")}
              >
                Start your next move <ArrowUpRight size={16} />
              </button>
            </div>
            <MigrationGraphic />
          </section>
          <div className="metric-grid">
            {stats.map((s) => (
              <section className={`metric-card ${s.tone}`} key={s.name}>
                <s.icon size={21} />
                <span>{s.name}</span>
                <b>{s.value ?? "—"}</b>
                <small>
                  {metrics ? "Reported by daemon" : "Awaiting live data"}
                </small>
              </section>
            ))}
          </div>
          <div className="insight-grid">
            <section className="panel chart-panel">
              <div className="panel-heading">
                <div>
                  <p className="eyebrow">MIGRATION ACTIVITY</p>
                  <h2>A little perspective.</h2>
                </div>
                <span>Last 60 samples</span>
              </div>
              {history.length > 1 ? (
                <ChartContainer
                  title="Jobs over time"
                  type="line"
                  dataKeys={["Active", "Completed", "Failed"]}
                  colors={["#3887ed", "#32a87b", "#d96774"]}
                  data={history.map((m) => ({
                    timestamp: m.timestamp,
                    Active: m.jobs_active,
                    Completed: m.jobs_completed,
                    Failed: m.jobs_failed,
                  }))}
                />
              ) : (
                <div className="chart-empty">
                  <Activity size={34} />
                  <b>Your activity will take shape here.</b>
                  <p>Live charts begin after two metrics samples arrive.</p>
                </div>
              )}
            </section>
            <TerminalPanel
              title="transiva — connection status"
              lines={[
                {
                  label: "stream",
                  value: connected ? "Connected to /ws" : "Waiting for /ws",
                  tone: connected ? "good" : "warn",
                },
                {
                  label: "health",
                  value: metrics?.system_health || "Not reported",
                  tone: metrics?.system_health === "healthy" ? "good" : "info",
                },
                {
                  label: "active",
                  value: metrics ? String(metrics.jobs_active) : "Not reported",
                },
                {
                  label: "updated",
                  value: metrics
                    ? new Date(metrics.timestamp).toLocaleTimeString()
                    : "No metrics received",
                },
              ]}
            />
          </div>
          <section className="system-summary" aria-label="Daemon resources">
            {[
              {
                label: "CPU usage",
                value:
                  metrics?.cpu_usage == null
                    ? "—"
                    : `${metrics.cpu_usage.toFixed(1)}%`,
              },
              {
                label: "Memory",
                value:
                  metrics?.memory_usage == null
                    ? "—"
                    : formatBytes(metrics.memory_usage),
              },
              {
                label: "HTTP requests",
                value:
                  metrics?.http_requests == null
                    ? "—"
                    : metrics.http_requests.toLocaleString(),
              },
              {
                label: "Average response",
                value:
                  metrics?.avg_response_time == null
                    ? "—"
                    : `${metrics.avg_response_time.toFixed(1)} ms`,
              },
            ].map((item) => (
              <div key={item.label}>
                <span>{item.label}</span>
                <b>{item.value}</b>
              </div>
            ))}
          </section>
          <div className="action-grid">
            {[
              {
                title: "Connect a provider",
                text: "Bring your source infrastructure into view.",
                view: "providers" as const,
              },
              {
                title: "Create a manifest",
                text: "Give your artifacts a portable identity.",
                view: "manifest" as const,
              },
            ].map((a) => (
              <button
                className="action-card"
                key={a.title}
                onClick={() => onNavigate(a.view)}
              >
                <span>
                  <b>{a.title}</b>
                  <small>{a.text}</small>
                </span>
                <ArrowUpRight size={22} />
              </button>
            ))}
          </div>
          {metrics?.alerts?.length ? (
            <AlertsList alerts={metrics.alerts} />
          ) : null}
        </>
      )}
      <section className="panel jobs-panel">
        <div className="panel-heading">
          <div>
            <p className="eyebrow">YOUR OPERATIONS</p>
            <h2>{jobsOnly ? "Export jobs" : "Recent moves"}</h2>
          </div>
          {!jobsOnly && (
            <button className="text-link" onClick={() => onNavigate("jobs")}>
              All jobs <ArrowUpRight size={15} />
            </button>
          )}
        </div>
        <div className="jobs-toolbar">
          <input
            aria-label="Search jobs"
            placeholder="Search by name or job ID"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <select
            aria-label="Filter jobs by status"
            value={status}
            onChange={(e) => setStatus(e.target.value)}
          >
            {[
              "all",
              "pending",
              "running",
              "completed",
              "failed",
              "cancelled",
            ].map((s) => (
              <option key={s} value={s}>
                {s === "all" ? "All statuses" : s}
              </option>
            ))}
          </select>
        </div>
        {error && (
          <p role="alert" className="notice error">
            {error}
          </p>
        )}
        <div className="table-scroll">
          {jobs.length ? (
            <JobsTable
              showFilter={false}
              jobs={jobs}
              onCancelJob={async (id) => {
                setError("");
                try {
                  await cancelJob(id);
                } catch {
                  setError("Could not cancel this job. Please try again.");
                }
              }}
            />
          ) : (
            <div className="empty-state">
              <BoxIcon />
              <h3>
                {query || status !== "all"
                  ? "No matching jobs."
                  : metrics
                    ? "Your next move starts here."
                    : "Waiting for your operations."}
              </h3>
              <p>
                {query || status !== "all"
                  ? "Try another name or status."
                  : metrics
                    ? "Start an export to see its progress here."
                    : "Jobs appear when the daemon sends metrics."}
              </p>
              <button
                className="text-link"
                onClick={() => onNavigate("export")}
              >
                Create an export <ArrowUpRight size={16} />
              </button>
            </div>
          )}
        </div>
      </section>
    </div>
  );
}
function BoxIcon() {
  return <ArrowUpRight size={30} className="empty-icon" />;
}
