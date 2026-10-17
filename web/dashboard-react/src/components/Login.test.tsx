// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { Login } from "./Login";
describe("Transiva sign in", () => {
  beforeEach(() => localStorage.clear());
  it("keeps password visibility keyboard accessible", () => {
    render(<Login onLogin={vi.fn()} />);
    expect(screen.getByLabelText("Password").getAttribute("type")).toBe(
      "password",
    );
    fireEvent.click(screen.getByRole("button", { name: "Show password" }));
    expect(screen.getByLabelText("Password").getAttribute("type")).toBe("text");
    expect(screen.getByRole("button", { name: "Hide password" }).tabIndex).toBe(
      0,
    );
  });
  it("reports authentication errors and allows retry", async () => {
    const login = vi.fn().mockRejectedValue(new Error("Invalid credentials"));
    render(<Login onLogin={login} />);
    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: "operator" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "secret" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    expect(await screen.findByRole("alert")).toHaveProperty(
      "textContent",
      "Invalid credentials",
    );
    expect(screen.getByRole("button", { name: "Sign in" })).toHaveProperty(
      "disabled",
      false,
    );
  });
  it("remembers only the username after successful login", async () => {
    const login = vi.fn().mockResolvedValue(undefined);
    render(<Login onLogin={login} />);
    fireEvent.change(screen.getByLabelText("Username"), {
      target: { value: " operator " },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "not-for-storage" },
    });
    fireEvent.click(screen.getByLabelText("Remember my username"));
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));
    await waitFor(() =>
      expect(localStorage.getItem("transiva_username")).toBe("operator"),
    );
    expect(login).toHaveBeenCalledWith("operator", "not-for-storage");
    expect(JSON.stringify(localStorage)).not.toContain("not-for-storage");
  });
});
