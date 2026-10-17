// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

import { useState } from "react";
import { Copy, Check, Terminal } from "lucide-react";
export interface TerminalLine {
  label: string;
  value: string;
  tone?: "good" | "warn" | "info";
}
export function TerminalPanel({
  title,
  lines,
}: {
  title: string;
  lines: TerminalLine[];
}) {
  const [copied, setCopied] = useState(false);
  const [error, setError] = useState("");
  async function copy() {
    try {
      await navigator.clipboard.writeText(
        lines.map((l) => `${l.label}  ${l.value}`).join("\n"),
      );
      setCopied(true);
      setError("");
    } catch {
      setError("Copy unavailable. Select the text to copy it.");
    }
  }
  return (
    <section className="terminal-window" aria-label={title}>
      <div className="terminal-title">
        <div className="traffic-lights" aria-hidden="true">
          <i />
          <i />
          <i />
        </div>
        <span>
          <Terminal size={14} /> {title}
        </span>
        <button onClick={copy} aria-label="Copy terminal output">
          {copied ? <Check size={15} /> : <Copy size={15} />}
        </button>
      </div>
      <div className="terminal-body">
        {lines.map((line, i) => (
          <div className="terminal-line" key={i}>
            <span className="terminal-label">{line.label}</span>
            <span className={line.tone || "info"}>{line.value}</span>
          </div>
        ))}
        {error && <p role="status">{error}</p>}
        <span className="terminal-cursor" aria-hidden="true">
          ▍
        </span>
      </div>
    </section>
  );
}
