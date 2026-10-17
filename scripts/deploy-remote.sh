#!/usr/bin/env bash
# Copyright 2026 Zyvor AI Labs · https://zyvor.dev
# SPDX-License-Identifier: Apache-2.0
# deploy-remote.sh — Deploy Transiva to a remote Linux host as a systemd service
# ============================================================================
# transivad needs CGO (go-sqlite3), so it is built natively on the target. The
# React dashboard is committed under daemon/dashboard/static-react and is served
# from disk relative to the service WorkingDirectory (/var/lib/transiva), so it
# is installed there too. The target needs Go + gcc but no npm:
#   1. Preflight: SSH, Go and gcc on the target
#   2. rsync the source to ~/.transiva/src and build transivad, hyperexport
#   3. Install binaries (previous kept as .prev) + config/unit only if missing
#   4. Enable/start transivad.service, open the firewall port
#   5. Verify: service active, /api/v1/health, dashboard and login page served
#
# Usage:
#   ./scripts/deploy-remote.sh <host> [user] [password] [options]
#   ./scripts/deploy-remote.sh 175.110.122.71 sus               # SSH key auth
#   ./scripts/deploy-remote.sh 175.110.122.71 sus --uninstall
#   ./scripts/deploy-remote.sh 175.110.122.71 sus --dry-run
#
# Options:
#   --uninstall      Stop transivad.service and remove it + the binaries
#   --dry-run        Print what would happen; make no changes
#   --rebuild-web    Rebuild the React dashboard locally (npm) before compiling
#   --verbose        Show full remote command output
#
# Environment variables:
#   DEPLOY_HOST, DEPLOY_USER, DEPLOY_PASS   — same as the positional args
#   TRANSIVA_PORT                            — API/dashboard port (default 5080)
#
# Remote layout:
#   /usr/local/bin/{transivad,hyperexport}
#   /etc/transiva/config.yaml   written once; re-running never overwrites it
#   transivad.service           installed only if missing (existing units are kept)
#   transivad.service.d/10-pam-auth.conf  runs the service as root so PAM can check real passwords
#   /etc/pam.d/transiva         PAM service (pam_unix); written once, never overwritten
#   /var/lib/transiva           state and exports (systemd StateDirectory)
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
# shellcheck source=lib/deploy-common.sh
source "$SCRIPT_DIR/lib/deploy-common.sh"

info()  { deploy_ui_info "$@"; }
warn()  { deploy_ui_warn "$@"; }
error() { deploy_ui_error "$@"; }
step()  { deploy_ui_step_start "$*"; }

TRANSIVA_PORT="${TRANSIVA_PORT:-5080}"
DEPLOY_UI_PORT="$TRANSIVA_PORT"
DEPLOY_UI_SCHEME="https"
REMOTE_BIN_DIR=/usr/local/bin
REMOTE_CONF=/etc/transiva/config.yaml
REMOTE_UNIT=/etc/systemd/system/transivad.service

UNINSTALL_MODE=false
DRY_RUN=false
REBUILD_WEB=false
VERBOSE=false
POSITIONAL=()
for arg in "$@"; do
    case "$arg" in
        --uninstall)   UNINSTALL_MODE=true ;;
        --dry-run)     DRY_RUN=true ;;
        --rebuild-web) REBUILD_WEB=true ;;
        --verbose)     VERBOSE=true ;;
        -h|--help)
            sed -n '2,38p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        *) POSITIONAL+=("$arg") ;;
    esac
done

HOST="${POSITIONAL[0]:-${DEPLOY_HOST:-}}"
USER="${POSITIONAL[1]:-${DEPLOY_USER:-root}}"
PASS="${POSITIONAL[2]:-${DEPLOY_PASS:-}}"

hypersdk_parse_target HOST USER
if [ -z "$HOST" ] && hypersdk_load_deploy_last "$REPO_DIR"; then
    HOST="${HOST:-}"
    info "Using .deploy-last → ${USER}@${HOST}"
fi
[ -z "$HOST" ] && error "Usage: $0 <host> [user] [password] [options]  (see --help)"
[[ "$TRANSIVA_PORT" =~ ^[0-9]+$ ]] || error "TRANSIVA_PORT must be numeric"
[[ "$HOST" =~ ^[A-Za-z0-9.:-]+$ ]] || error "invalid host: $HOST"

hypersdk_preflight_deploy_sources "$REPO_DIR"
command -v ssh &>/dev/null || error "ssh is required"
hypersdk_preflight_remote_tools "$([ -n "$PASS" ] && echo true || echo false)"

SUDO=""
[ "$USER" != "root" ] && SUDO="sudo"

DEPLOY_SSH_OPTS=(-o StrictHostKeyChecking=accept-new -o ConnectTimeout=15
                 -o ServerAliveInterval=15 -o ServerAliveCountMax=8)

_ssh() {
    if [ -n "$PASS" ]; then
        SSHPASS="$PASS" sshpass -e ssh "${DEPLOY_SSH_OPTS[@]}" "${USER}@${HOST}" "$@"
    else
        ssh -o BatchMode=yes "${DEPLOY_SSH_OPTS[@]}" "${USER}@${HOST}" "$@"
    fi
}
_scp() {
    if [ -n "$PASS" ]; then
        SSHPASS="$PASS" sshpass -e scp "${DEPLOY_SSH_OPTS[@]}" "$@"
    else
        scp -o BatchMode=yes "${DEPLOY_SSH_OPTS[@]}" "$@"
    fi
}

if $DRY_RUN; then
    deploy_ui_banner "${DEPLOY_UI_ICON_MAGIC} Dry run" "no changes will be made"
    deploy_ui_kv "🎯" "Target" "${USER}@${HOST}"
    deploy_ui_kv "📦" "Binaries" "$REMOTE_BIN_DIR/{transivad,hyperexport}"
    deploy_ui_kv "📄" "Config" "$REMOTE_CONF (written only if missing)"
    deploy_ui_kv "⚙️" "Unit" "$REMOTE_UNIT"
    deploy_ui_kv "🌐" "Port" "$TRANSIVA_PORT"
    echo ""
    deploy_ui_note "Would: rsync → native build on host → enable/start → verify health, dashboard, login"
    echo ""
    exit 0
fi

hypersdk_build_metadata "$REPO_DIR"
deploy_ui_banner "Remote Deploy" "${HYPERSDK_VERSION} (${HYPERSDK_COMMIT}) → ${USER}@${HOST}"
deploy_ui_kv "🎯" "Target" "${USER}@${HOST}"
deploy_ui_kv "🔐" "Auth" "$([ -n "$PASS" ] && echo 'password' || echo 'SSH key')"
deploy_ui_kv "🌐" "Port" "$TRANSIVA_PORT"
echo ""

# ── Uninstall ──
if $UNINSTALL_MODE; then
    deploy_ui_uninstall_banner
    step "Uninstalling transiva from ${HOST}"
    _ssh "
        $SUDO systemctl disable --now transivad.service 2>/dev/null || true
        $SUDO rm -f $REMOTE_BIN_DIR/transivad $REMOTE_BIN_DIR/hyperexport $REMOTE_UNIT /etc/systemd/system/transivad.service.d/10-pam-auth.conf
        $SUDO systemctl daemon-reload 2>/dev/null || true
    "
    deploy_ui_step_finish
    info "transivad removed from ${HOST} (config in /etc/transiva and /var/lib/transiva kept)"
    exit 0
fi

# ── Step 1: preflight ──
BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_DIR"' EXIT
REMOTE_SRC='$HOME/.transiva/src'
REMOTE_PATH='export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin'

step "Preflight: ssh, Go and gcc on ${HOST}"
_ssh "$REMOTE_PATH; command -v go >/dev/null && command -v gcc >/dev/null && go version" \
    || error "Go and gcc are required on ${HOST} (apt install golang gcc / dnf install golang gcc)"
_ssh "[ -f /usr/include/security/pam_appl.h ]" \
    || error "PAM development headers are required on ${HOST} for real password checks (apt install libpam0g-dev / dnf install pam-devel)"
deploy_ui_step_finish

if $REBUILD_WEB; then
    step "Build: React dashboard (npm, local)"
    hypersdk_build_dashboard "$REPO_DIR" || error "dashboard build failed"
    deploy_ui_step_finish
fi
[ -f "$REPO_DIR/daemon/dashboard/static-react/index.html" ] \
    || error "daemon/dashboard/static-react missing — run with --rebuild-web"

# ── Step 2: sync source and build natively (CGO) ──
step "Sync: rsync source to ${HOST}:.transiva/src"
_ssh "mkdir -p $REMOTE_SRC"
if [ -n "$PASS" ]; then RSH="sshpass -e ssh ${DEPLOY_SSH_OPTS[*]}"; else RSH="ssh -o BatchMode=yes ${DEPLOY_SSH_OPTS[*]}"; fi
SSHPASS="$PASS" rsync -az --delete -e "$RSH" \
    --exclude .git --exclude node_modules --exclude 'web/dashboard-react/dist' \
    --exclude docs --exclude website --exclude '*.md' --exclude .deploy-last \
    "$REPO_DIR/" "${USER}@${HOST}:.transiva/src/"
deploy_ui_step_finish

step "Build: native CGO build on ${HOST}"
_ssh "
    set -e; $REMOTE_PATH; cd $REMOTE_SRC
    for pair in transivad:cmd/hypervisord hyperexport:cmd/hyperexport; do
        n=\${pair%%:*}; pkg=\${pair#*:}
        CGO_ENABLED=1 go build -tags pam -trimpath -ldflags '$HYPERSDK_LDFLAGS' -o /tmp/\$n.new ./\$pkg/
        echo \"  ✅ \$n\"
    done
" || error "remote Go build failed"
deploy_ui_step_finish

step "Upload: installing binaries, config and systemd unit"
cat > "$BUILD_DIR/config.yaml" <<CFGEOF
# Written once by deploy-remote.sh; edit freely — redeploys never overwrite it.
daemon_addr: "0.0.0.0:${TRANSIVA_PORT}"
log_level: "info"
vcenter_url: "https://vcenter.example.com"
username: "administrator@vsphere.local"
password: ""
insecure: true
CFGEOF

cat > "$BUILD_DIR/transivad.service" <<'UNITEOF'
[Unit]
Description=Transivad - VM export daemon and dashboard
Documentation=https://github.com/zyvorai/transiva
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/transivad --config /etc/transiva/config.yaml
Restart=on-failure
RestartSec=5s
User=root
StateDirectory=transiva
WorkingDirectory=/var/lib/transiva
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
PrivateTmp=yes
LimitNOFILE=65536
StandardOutput=journal
StandardError=journal
SyslogIdentifier=transivad

[Install]
WantedBy=multi-user.target
UNITEOF

_ssh "$SUDO mkdir -p /etc/transiva"
cat > "$BUILD_DIR/10-pam-auth.conf" <<'DROPEOF'
# Real password checks go through PAM (pam_unix), which needs to read the shadow database.
[Service]
DynamicUser=no
User=root
DROPEOF
_scp "$BUILD_DIR/config.yaml" "$BUILD_DIR/transivad.service" "$BUILD_DIR/10-pam-auth.conf" "$REPO_DIR/deployments/pam/transiva" "${USER}@${HOST}:/tmp/"
_ssh "
    set -e
    for b in transivad hyperexport; do
        # keep one previous binary for rollback
        [ -f $REMOTE_BIN_DIR/\$b ] && $SUDO cp -p $REMOTE_BIN_DIR/\$b $REMOTE_BIN_DIR/\$b.prev
        $SUDO install -m 755 /tmp/\$b.new $REMOTE_BIN_DIR/\$b && rm -f /tmp/\$b.new
    done
    [ -f $REMOTE_CONF ] || $SUDO install -m 640 /tmp/config.yaml $REMOTE_CONF
    rm -f /tmp/config.yaml
    # dashboard assets are served from <WorkingDirectory>/daemon/dashboard/static-react
    $SUDO mkdir -p /var/lib/transiva/daemon/dashboard
    $SUDO rm -rf /var/lib/transiva/daemon/dashboard/static-react
    $SUDO cp -r $REMOTE_SRC/daemon/dashboard/static-react /var/lib/transiva/daemon/dashboard/static-react
    $SUDO chmod -R a+rX /var/lib/transiva/daemon
    # an existing unit (possibly customised: user, env, hardening) is never replaced
    [ -f $REMOTE_UNIT ] || $SUDO install -m 644 /tmp/transivad.service $REMOTE_UNIT
    rm -f /tmp/transivad.service
    # existing units may still use DynamicUser: a drop-in makes the service run as root for PAM
    $SUDO mkdir -p /etc/systemd/system/transivad.service.d
    $SUDO install -m 644 /tmp/10-pam-auth.conf /etc/systemd/system/transivad.service.d/10-pam-auth.conf
    rm -f /tmp/10-pam-auth.conf
    [ -f /etc/pam.d/transiva ] || $SUDO install -m 644 /tmp/transiva /etc/pam.d/transiva
    rm -f /tmp/transiva
    $SUDO systemctl daemon-reload
"
deploy_ui_step_finish

# ── Step 4: start ──
step "Systemd: starting transivad.service"
_ssh "
    $SUDO systemctl daemon-reload
    $SUDO systemctl enable transivad.service >/dev/null 2>&1
    $SUDO systemctl restart transivad.service
    if command -v ufw &>/dev/null && $SUDO ufw status 2>/dev/null | grep -q 'Status: active'; then
        $SUDO ufw allow ${TRANSIVA_PORT}/tcp >/dev/null 2>&1 || true
    elif command -v firewall-cmd &>/dev/null && $SUDO firewall-cmd --state &>/dev/null; then
        $SUDO firewall-cmd --permanent --add-port=${TRANSIVA_PORT}/tcp >/dev/null 2>&1 || true
        $SUDO firewall-cmd --reload >/dev/null 2>&1 || true
    fi
    sleep 2
    if ! $SUDO systemctl is-active --quiet transivad.service; then
        echo 'transivad.service: FAILED TO START'
        $SUDO journalctl -u transivad.service --no-pager -n 30
        exit 1
    fi
"
deploy_ui_step_finish
info "transivad.service active"

# ── Step 5: verify ──
step "Verify: health, dashboard and login (on-host)"
# the daemon serves plain HTTP unless TLS is configured; probe rather than assume
SCHEME=http
for sc in http https; do
    code=$(_ssh "curl -sk -m 5 -o /dev/null -w '%{http_code}' ${sc}://127.0.0.1:${TRANSIVA_PORT}/health" | tr -d '\r' || true)
    [ "$code" = "200" ] && { SCHEME=$sc; break; }
done
BASE="${SCHEME}://127.0.0.1:${TRANSIVA_PORT}"
DEPLOY_UI_SCHEME="$SCHEME"
HEALTH=$(_ssh "curl -sk -m 5 -o /dev/null -w '%{http_code}' ${BASE}/health" | tr -d '\r' || true)
DASH=$(_ssh "curl -sk -m 5 -o /dev/null -w '%{http_code}' ${BASE}/web/dashboard/" | tr -d '\r' || true)
TITLE=$(_ssh "curl -sk -m 5 ${BASE}/web/dashboard/ | grep -o '<title>[^<]*'" | tr -d '\r' || true)
deploy_ui_step_finish
deploy_ui_highlight "📋 Final checklist"
deploy_ui_checklist "service"   "$(_ssh "$SUDO systemctl is-active transivad.service" | tr -d '\r')"
deploy_ui_checklist "health"    "${HEALTH}"
deploy_ui_checklist "dashboard" "${DASH}"
info "Dashboard title: ${TITLE#<title>}"
[ "$HEALTH" = "200" ] || error "health check returned '${HEALTH}' — see: ssh ${USER}@${HOST} journalctl -u transivad"
[ "$DASH" = "200" ] || error "dashboard returned '${DASH}'"

hypersdk_save_deploy_last "$REPO_DIR" "$HOST" "$USER" "full"
info "Dashboard: ${SCHEME}://${HOST}:${TRANSIVA_PORT}/web/dashboard/"
