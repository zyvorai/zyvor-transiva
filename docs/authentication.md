# Dashboard and API sign-in

Transiva checks dashboard sign-ins against the host's accounts through **PAM**. A wrong password is rejected.

## How it is built and run

- Build the daemon with the `pam` tag on Linux with cgo and the PAM headers installed
  (`apt install libpam0g-dev` or `dnf install pam-devel`):
  `CGO_ENABLED=1 go build -tags pam ./cmd/hypervisord`, or `make build-daemon GO_TAGS=pam`.
  `scripts/deploy-remote.sh` does this for you.
- The daemon must run as **root** so `pam_unix` can read the shadow database. The deploy script installs
  a systemd drop-in (`transivad.service.d/10-pam-auth.conf`) that sets `User=root`.
- The PAM service is `$TRANSIVA_PAM_SERVICE`, else `/etc/pam.d/transiva` (installed by the deploy script,
  `pam_unix` only), else the system `login` stack. To add lockout policy, LDAP or SSSD, change that file.

## Builds without PAM fail closed

A binary built without the `pam` tag (the default `go build`, cross-compiled release binaries, the RPM and
container images today) cannot check passwords, so **every sign-in is refused** with a 503 and a message
saying so. It never falls back to accepting any password.

For local development only you can set `TRANSIVA_ALLOW_INSECURE_AUTH=1`: the daemon then accepts any
password for an account that exists on the machine and logs a warning. Never set it on a reachable host.

## The API is protected too

The dashboard sign-in is not just a screen: **every API route except `/health`, the dashboard files and `/api/login`
requires either a session token (`Authorization: Bearer <token>` from `/api/login`) or the API key**
(`X-API-Key: <key>`, set with `TRANSIVA_API_KEY`). Requests with neither get a 401.

`--no-auth` or `TRANSIVA_DISABLE_AUTH=1` turn this off for local development and print a warning at startup.
Scripts and CLIs that call the daemon (for example `scripts/test-api.sh`) need the API key, or must run against a
development daemon started with `--no-auth`.

## Sessions and lockout

- After sign-in the dashboard keeps a session token. Sessions survive a page refresh and a daemon restart
  (stored as a hash in `$TRANSIVA_SESSION_FILE`, default `/var/lib/transiva/sessions.json`, mode 0600), renew
  while in use (24 hour idle window) and end 7 days after sign-in at the latest.
- Five failed sign-ins for an account within five minutes lock that account for five minutes (HTTP 429).
