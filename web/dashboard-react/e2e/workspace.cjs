// Copyright 2026 Zyvor AI Labs · https://zyvor.dev
// SPDX-License-Identifier: Apache-2.0
// Run with NODE_PATH=<directory containing playwright> node e2e/workspace.cjs.
// Start Vite on 127.0.0.1:5173 first. API and WebSocket data are fixture-only.
const { chromium } = require("playwright");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
(async () => {
  const browser = await chromium.launch({
    headless: true,
    executablePath: process.env.TRANSIVA_CHROMIUM,
    args: ["--no-sandbox", "--disable-dev-shm-usage"],
  });
  const page = await browser.newPage({
    viewport: { width: 1440, height: 1000 },
    reducedMotion: "reduce",
  });
  page.setDefaultTimeout(10000);
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  const shots = process.env.TRANSIVA_SCREENSHOTS;
  if (shots) fs.mkdirSync(shots, { recursive: true });
  let rejectLogin = true;
  await page.route("**/api/**", async (route) => {
    const url = new URL(route.request().url());
    let data = {};
    let status = 200;
    if (url.pathname === "/api/login") {
      if (rejectLogin) {
        status = 401;
        data = { error: "unauthorized" };
      } else {
        data = { token: "test-only-token" };
      }
    }
    if (url.pathname === "/api/providers/list") data = [];
    if (url.pathname === "/api/workflow/jobs/active") data = { jobs: [] };
    if (url.pathname === "/api/workflow/status")
      data = {
        mode: "manifest",
        running: true,
        active_jobs: 0,
        queue_length: 0,
        total_processed: 0,
      };
    await route.fulfill({
      status,
      contentType: "application/json",
      body: JSON.stringify(data),
    });
  });
  await page.routeWebSocket("**/ws", (ws) => {
    let n = 0;
    const send = () =>
      ws.send(
        JSON.stringify({
          timestamp: new Date(Date.now() + n++ * 1000).toISOString(),
          jobs_active: 2,
          jobs_completed: 14,
          jobs_pending: 1,
          jobs_failed: 0,
          system_health: "healthy",
          recent_jobs: [
            {
              id: "fixture-job-01",
              name: "Finance workload",
              vm_name: "finance-vm",
              status: "running",
              progress: 48,
              start_time: new Date().toISOString(),
              duration_seconds: 90,
              provider: "vsphere",
              format: "qcow2",
              created_at: new Date().toISOString(),
              updated_at: new Date().toISOString(),
            },
          ],
          alerts: [],
        }),
      );
    send();
    const interval = setInterval(send, 800);
    ws.onClose(() => clearInterval(interval));
  });
  async function shot(name) {
    if (shots)
      await page.screenshot({
        path: path.join(shots, name + ".png"),
        fullPage: true,
      });
  }
  async function noOverflow(label) {
    assert(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth + 1,
      ),
      `Horizontal overflow: ${label}`,
    );
  }
  await page.goto(
    process.env.TRANSIVA_URL || "http://127.0.0.1:5173/web/dashboard/",
  );
  await page.getByRole("heading", { name: "Welcome to Transiva." }).waitFor();
  await noOverflow("desktop sign in");
  await shot("login-desktop");
  await page.getByLabel("Username", { exact: true }).fill("operator");
  await page.getByLabel("Password", { exact: true }).fill("fixture-password");
  console.log("Checking rejected sign in");
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await page.getByRole("alert").waitFor();
  assert(
    (await page.getByRole("alert").textContent()).includes(
      "Check your username",
    ),
  );
  console.log("Checking successful sign in");
  rejectLogin = false;
  await page.getByRole("button", { name: "Sign in", exact: true }).click();
  await page.getByRole("heading", { name: "Every move. In view." }).waitFor();
  console.log("Workspace opened");
  await page.getByText("Finance workload", { exact: true }).waitFor();
  // Wait for a second fixture sample and assert that chart series really render.
  await page
    .locator(".recharts-line-curve")
    .first()
    .waitFor({ state: "attached" });
  console.log("Live chart rendered");
  assert((await page.locator(".recharts-line-curve").first().getAttribute("d"))?.startsWith("M"));
  await noOverflow("desktop overview");
  await shot("overview-desktop");
  await page.getByRole("button", { name: "Use dark appearance" }).click();
  assert.equal(await page.locator("html").getAttribute("data-theme"), "dark");
  await shot("overview-dark");
  await page.getByRole("button", { name: "Search pages", exact: true }).click();
  await page.getByLabel("Search workspace pages").fill("jobs");
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Jobs", exact: true })
    .click();
  await page.getByRole("heading", { name: "Progress, made clear." }).waitFor();
  await page.getByLabel("Search jobs").fill("does-not-exist");
  await page.getByRole("heading", { name: "No matching jobs." }).waitFor();
  await page.getByLabel("Search jobs").fill("Finance");
  await page.getByText("Finance workload", { exact: true }).waitFor();
  for (const name of ["New export", "Workflows", "Providers", "Manifests"]) {
    await page
      .getByRole("navigation", { name: "Workspace" })
      .getByRole("button", { name, exact: true })
      .click();
    await noOverflow("desktop " + name);
    await shot("desktop-" + name.toLowerCase().replaceAll(" ", "-"));
  }
  await page
    .getByRole("navigation", { name: "Workspace" })
    .getByRole("button", { name: "New export", exact: true })
    .click();
  // Nutanix is the first tab and opens by default; vSphere is second.
  const providerTabs = await page
    .locator("main button")
    .filter({ hasText: /^(Nutanix AHV|vSphere)$/ })
    .allTextContents();
  if (providerTabs.join("|") !== "Nutanix AHV|vSphere")
    throw new Error("provider tabs out of order: " + providerTabs.join("|"));
  await page
    .getByRole("heading", { name: "Connect to Nutanix Prism" })
    .waitFor();
  await shot("export-nutanix");
  await page.getByRole("button", { name: "vSphere", exact: true }).click();
  await page.getByRole("heading", { name: "Connect to vSphere" }).waitFor();
  await shot("export-vsphere");
  await page.getByRole("button", { name: "Nutanix AHV", exact: true }).click();
  await page
    .getByRole("heading", { name: "Connect to Nutanix Prism" })
    .waitFor();
  await page.setViewportSize({ width: 390, height: 844 });
  for (const name of [
    "Overview",
    "New export",
    "Jobs",
    "Workflows",
    "Providers",
    "Manifests",
  ]) {
    await page
      .getByRole("navigation", { name: "Workspace" })
      .getByRole("button", { name, exact: true })
      .click();
    await noOverflow("mobile " + name);
  }
  await page
    .getByRole("navigation", { name: "Workspace" })
    .getByRole("button", { name: "Overview", exact: true })
    .click();
  await shot("overview-mobile");
  await page.getByRole("button", { name: "Sign out", exact: true }).click();
  await page.getByRole("heading", { name: "Welcome to Transiva." }).waitFor();
  await noOverflow("mobile sign in");
  await shot("login-mobile");
  assert.equal(
    await page.evaluate(() =>
      Object.values(localStorage).some((v) => v.includes("fixture-password")),
    ),
    false,
  );
  assert.deepEqual(errors, []);
  await browser.close();
  console.log(
    "PASS: login failure/success, live chart, 6 routes, Nutanix tab, page search, job search, theme, logout, mobile overflow, no runtime errors. Fixtures only; no live migration performed.",
  );
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
