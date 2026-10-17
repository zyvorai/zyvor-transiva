// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0

import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import ExportWorkflowRouter from "./ExportWorkflowRouter";

vi.mock("./NutanixExportWorkflow", () => ({
  default: () => <h2>Connect to Nutanix Prism</h2>,
}));
vi.mock("./VSphereExportWorkflow", () => ({
  default: () => <h2>Connect to vSphere</h2>,
}));

describe("New export provider tabs", () => {
  it("lists Nutanix first and vSphere second", () => {
    render(<ExportWorkflowRouter />);
    const tabs = screen.getAllByRole("button").map((b) => b.textContent);
    expect(tabs).toEqual(["Nutanix AHV", "vSphere"]);
  });

  it("opens on Nutanix", () => {
    render(<ExportWorkflowRouter />);
    expect(screen.getByRole("heading", { name: "Connect to Nutanix Prism" })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Connect to vSphere" })).toBeNull();
  });

  it("switches to vSphere and back", () => {
    render(<ExportWorkflowRouter />);
    fireEvent.click(screen.getByRole("button", { name: "vSphere" }));
    expect(screen.getByRole("heading", { name: "Connect to vSphere" })).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Connect to Nutanix Prism" })).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Nutanix AHV" }));
    expect(screen.getByRole("heading", { name: "Connect to Nutanix Prism" })).toBeTruthy();
  });
});
