# Transiva workspace redesign

The React dashboard now uses a single navigation shell, system typography, generous spacing, blue actions, pastel migration artwork, and six distinct workspaces. The visual direction follows the story/browse separation in Netra without copying its product or backend.

## Surfaces

- Sign in: responsive product story, original SVG/CSS migration illustration, labeled inputs, password visibility, disabled/pending/error states, username-only remembering.
- Overview: live metrics, activity chart, resource summary, actionable navigation, recent jobs, and a macOS-style status window with colored output and copy feedback.
- Jobs: searchable recent jobs, status filter, sortable table, and existing cancellation API.
- Exports: preserved vSphere and Nutanix discovery/export forms and pipeline options, with theme tokens, accessible names, and responsive layout.
- Workflows, providers, and manifests: separate routes around the existing daemon-backed components. Manifest form/JSON modes remain available.
- Legacy daemon dashboard, Kubernetes/chart/VM pages, serial/VNC console, and standalone HTML dashboard: matching shared stylesheet. Console traffic lights are decorative, not fake window controls.

The appearance switch is persisted; Cmd/Ctrl+K opens page search. Hash links and browser history select workspaces. Reduced-motion preferences disable CSS transitions; live charts do not animate between samples. Missing metrics show dashes and waiting states rather than generated numbers.

## Integration

Run the enhanced API server with its existing authenticated `/api/login` endpoint. The React app forwards the returned in-memory token as a Bearer header for API calls. Reloading signs the user out. Legacy stored dashboard passwords are removed on mount.

Browsers cannot add custom headers to a WebSocket handshake. The React app therefore offers the existing session as a base64url-encoded `transiva-session.*` protocol header. The server decodes it and calls the existing `ValidateSession` before upgrade; API-key clients remain supported. The credential is never added to the URL. Same-origin/origin checks remain in the existing upgrader. Rebuild the daemon as well as the frontend to use authenticated live metrics.

This change connects the UI to the **existing** authentication service; it does not replace it. `daemon/auth/pam_auth.go` currently checks that a system user exists and explicitly leaves actual PAM password validation to a production implementation. Production authentication hardening remains necessary. Existing optional provider-credential persistence and legacy standalone demo-login behavior are not redesigned authentication systems.

## Build and test

```bash
cd web/dashboard-react
npm ci
npm run build
npm test -- --run
```

`build` regenerates tracked assets under `daemon/dashboard/static-react`. The base path remains `/web/dashboard/`. System fonts and locally rendered graphics require no font/image CDN. Vite reports a large-chunk warning for the existing Recharts bundle.

Browser validation in another terminal:

```bash
npm run dev -- --host 127.0.0.1
# In a second terminal, from web/dashboard-react:
npx playwright install chromium
npm run test:e2e
```

The browser harness intercepts API and WebSocket requests. It checks rejected/successful sign-in, a real rendered chart, all six desktop/mobile routes, Nutanix tab switching, page/job search, appearance switching, logout, horizontal overflow, and runtime errors. It performs **no real VM migration**. Optional `TRANSIVA_CHROMIUM` selects an installed Chromium executable; `TRANSIVA_URL` changes the local test URL.

Backend regression checks to run with the repository's required Go toolchain:

```bash
go test ./daemon/api -run 'TestWebSocket(SessionToken|RejectsUnvalidatedSession)$'
go test ./daemon/api ./daemon/dashboard
```

Go tests were not run in the delivery environment: no Go executable is installed. New protocol decoding/rejection regression tests are included. Successful real session handshakes, PAM integration, and end-to-end infrastructure exports need deployment validation.

## Maintaining the legacy theme

Edit `daemon/dashboard/static/css/transiva-workspace.css`, then run:

```bash
python3 scripts/sync-dashboard-theme.py
```

This updates the marked inline themes in both standalone HTML files. Daemon templates load the same stylesheet from `/static/css/transiva-workspace.css`. Existing DOM IDs and event handlers are retained. The public documentation website and backend migration engine are outside this workspace redesign.
