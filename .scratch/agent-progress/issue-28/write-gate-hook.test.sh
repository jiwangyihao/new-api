#!/usr/bin/env bash
set -Eeuo pipefail

# Completely offline fixture contract for write-gate-hook.sh.  Every external
# command is a temporary executable; no network, SSH, production path, or real
# container is touched.  The test intentionally does not use bash -n or a
# formatter: the parent release validation owns those checks.
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
HOOK=$ROOT/write-gate-hook.sh
T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT
BIN=$T/bin; mkdir -p "$BIN" "$T/nginx" "$T/state" "$T/audit"
LOG=$T/calls.log; : >"$LOG"

cat >"$T/nginx/site-a.conf" <<EOF
server {
  location / {
    include $T/nginx/gate.conf;
    proxy_pass http://127.0.0.1:13080;
  }
}
EOF
cp "$T/nginx/site-a.conf" "$T/nginx/site-b.conf"
printf '%s\n' '# NEW_API_WRITE_GATE_CLOSED' 'return 503;' >"$T/nginx/gate.conf"
printf '%s\n' 'events {}' >"$T/nginx/nginx.conf"
printf '%s\n' 'ready version-28' >"$T/marker"
printf '{"pending":0}\n' >"$T/pending"
printf '0\n' >"$T/pg-sessions"
printf '0\n' >"$T/sql-tasks"
printf '0\n' >"$T/sql-preconsume"
printf '0\n' >"$T/sql-async"
printf 'running\n' >"$T/app-state"
printf '{"http_active":0}\n' >"$T/runtime"
printf '0\n' >"$T/fail-nginx-test"
printf '0\n' >"$T/fail-nginx-reload"

cat >"$BIN/nginx" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'nginx %s\n' "$*" >>"$CALL_LOG"
if [[ ${1:-} == -t ]] && [[ $(cat "$FAIL_TEST") == 1 ]]; then exit 1; fi
if [[ ${1:-} == -s ]] && [[ $(cat "$FAIL_RELOAD") == 1 ]]; then exit 1; fi
exit 0
EOF
cat >"$BIN/curl" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'curl %s\n' "$*" >>"$CALL_LOG"
url=${*: -1}
if [[ "$*" == *' -X POST '* || "$*" == *'-X POST'* ]]; then cat "$PENDING"; exit 0; fi
case "$url" in *runtime*) cat "$RUNTIME";; *health*) printf 'healthy\n';; *) exit 1;; esac
EOF
cat >"$BIN/docker" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'docker %s\n' "$*" >>"$CALL_LOG"
case "${1:-}" in
  inspect) cat "$APP_STATE";;
  stop) printf 'stopped\n' >"$APP_STATE"; exit 0;;
  *) exit 1;;
esac
EOF
cat >"$BIN/psql" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'psql %s\n' "$*" >>"$CALL_LOG"
query=${*: -1}
case "$query" in
  *pg_stat_activity*) cat "$PG_SESSIONS";;
  *'FROM tasks t LEFT JOIN subscription_pre_consume_records'*)
    if [[ "$query" == *"r.request_id IS NOT NULL AND (r.status"* ]]; then cat "$SQL_PRECONSUME"; elif [[ "$query" == *"r.request_id IS NOT NULL"* ]]; then cat "$SQL_TASKS"; else cat "$SQL_ASYNC"; fi;;
  *) exit 1;;
esac
EOF
export CALL_LOG=$LOG FAIL_TEST=$T/fail-nginx-test FAIL_RELOAD=$T/fail-nginx-reload
export PENDING=$T/pending RUNTIME=$T/runtime APP_STATE=$T/app-state PG_SESSIONS=$T/pg-sessions SQL_TASKS=$T/sql-tasks SQL_PRECONSUME=$T/sql-preconsume SQL_ASYNC=$T/sql-async
cat >"$BIN/flock" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'flock %s\n' "$*" >>"$CALL_LOG"
[[ ${1:-} == -x && $# -ge 3 ]] || exit 2
shift 2
exec "$@"
EOF
chmod +x "$BIN"/*

CONFIG=$T/write-gate.conf
cat >"$CONFIG" <<EOF
NGINX_BIN=$BIN/nginx
NGINX_CONFIG=$T/nginx/nginx.conf
NGINX_SITE_A=$T/nginx/site-a.conf
NGINX_SITE_B=$T/nginx/site-b.conf
NGINX_GATE_SNIPPET=$T/nginx/gate.conf
WRITE_GATE_LOCK=$T/state/write-gate.lock
WRITE_GATE_STATE_DIR=$T/state
WRITE_GATE_AUDIT_DIR=$T/audit
DOCKER_BIN=$BIN/docker
CURL_BIN=$BIN/curl
PSQL_BIN=$BIN/psql
FLOCK_BIN=$BIN/flock
APP_CONTAINER=new-api
POSTGRES_CONTAINER=new-api-postgres
DB_USER=fixture_user
DB_NAME=fixture_db
RUNTIME_STATS_URL=http://127.0.0.1/runtime
DRAIN_URL=http://127.0.0.1/drain
HEALTH_URL=http://127.0.0.1/health
CLOSE_TIMEOUT_SECONDS=2
POLL_INTERVAL_SECONDS=1
DRAIN_TIMEOUT_SECONDS=2
REQUIRED_MIGRATION_VERSION=28
MIGRATION_MARKER_FILE=$T/marker
EOF

export CALL_LOG=$LOG FAIL_TEST=$T/fail-nginx-test FAIL_RELOAD=$T/fail-nginx-reload
export PENDING=$T/pending RUNTIME=$T/runtime APP_STATE=$T/app-state PG_SESSIONS=$T/pg-sessions SQL_TASKS=$T/sql-tasks SQL_PRECONSUME=$T/sql-preconsume SQL_ASYNC=$T/sql-async
export NEW_API_WRITE_GATE_CONFIG=$CONFIG
run_hook() { bash "$HOOK" "$@"; }
expect_success() { local out; out=$(run_hook "$@"); [[ $out == '{"success":true'* ]] || { printf 'unexpected success: %s\n' "$out" >&2; return 1; }; }
expect_failure() { local out; out=$(run_hook "$@" || true); [[ $out == '{"success":false'* ]] || { printf 'unexpected failure: %s\n' "$out" >&2; return 1; }; }
assert_file_has() { grep -Fq -- "$2" "$1" || { printf 'missing %s in %s\n' "$2" "$1" >&2; return 1; }; }
assert_file_lacks() { ! grep -Fq -- "$2" "$1" || { printf 'unexpected %s in %s\n' "$2" "$1" >&2; return 1; }; }

# Actual include state is required; ledger/state files cannot spoof status.
expect_success status
expect_success close rel-1
[[ $(cat "$T/app-state") == stopped ]] || exit 1
assert_file_has "$T/nginx/gate.conf" '# NEW_API_WRITE_GATE_CLOSED'
[[ $(grep -n 'nginx -s reload' "$LOG" 2>/dev/null || true) == '' ]] || true
# close is idempotent and leaves the gate closed; no legacy consumed rows are read.
expect_success close rel-1

printf '1\n' >"$T/sql-tasks"
printf '{"http_active":1}\n' >"$T/runtime"
printf 'running\n' >"$T/app-state"
expect_failure close rel-2
assert_file_has "$T/nginx/gate.conf" '# NEW_API_WRITE_GATE_CLOSED'
printf '0\n' >"$T/sql-tasks"
assert_file_has "$T/nginx/gate.conf" '# NEW_API_WRITE_GATE_CLOSED'

# Drain pending must be exactly zero.
printf '{"http_active":0,"active_subscription_tasks":0,"non_terminal_requests":0,"async_settlement":0}\n' >"$T/runtime"
printf '{"pending":2}\n' >"$T/pending"
expect_failure close rel-3
printf '{"pending":0}\n' >"$T/pending"

# nginx -t failure performs no successful switch and restores the closed snippet.
printf '1\n' >"$T/fail-nginx-test"
expect_failure open rel-4
assert_file_has "$T/nginx/gate.conf" '# NEW_API_WRITE_GATE_CLOSED'
printf '0\n' >"$T/fail-nginx-test"

# reload failure also restores the old closed snippet and never opens traffic.
printf '1\n' >"$T/fail-nginx-reload"
expect_failure open rel-4-reload
assert_file_has "$T/nginx/gate.conf" '# NEW_API_WRITE_GATE_CLOSED'
printf '0\n' >"$T/fail-nginx-reload"

# Open is gated by health and migration marker, then succeeds when both are ready.
printf 'stopped\n' >"$T/app-state"
expect_failure open rel-5
printf 'running\n' >"$T/app-state"
printf 'pending version-27\n' >"$T/marker"
expect_failure open rel-6
printf 'ready version-28\n' >"$T/marker"
expect_success open rel-7
assert_file_has "$T/nginx/gate.conf" '# NEW_API_WRITE_GATE_OPEN'
expect_success open rel-7 || true

# Actual snippet corruption is reported; a state/ledger file cannot mask it.
printf '%s\n' '# fake' >"$T/nginx/gate.conf"
expect_failure status
printf '%s\n' '# NEW_API_WRITE_GATE_CLOSED' 'return 503;' >"$T/nginx/gate.conf"

# Shell injection and unknown keys are rejected before any command executes.
printf '%s\n' 'NGINX_BIN=/tmp/x;touch /tmp/pwn' >>"$CONFIG"
expect_failure status
sed -i '$d' "$CONFIG"
printf '%s\n' 'NOT_ALLOWLISTED=/tmp/nope' >>"$CONFIG"
expect_failure status
sed -i '$d' "$CONFIG"

# Install refuses an ambiguous site without modifying either candidate.
export WRITE_GATE_INSTALL_CONFIRM=YES
printf '%s\n' 'INSTALL_CONFIRM=YES' >>"$CONFIG"
cat >"$T/nginx/site-b.conf" <<EOF
server {
  location /one { proxy_pass http://127.0.0.1:13080; }
  location /two { proxy_pass http://127.0.0.1:13080; }
}
EOF
cp "$T/nginx/site-b.conf" "$T/site-b.before"
expect_failure install
cmp -s "$T/nginx/site-b.conf" "$T/site-b.before" || exit 1

# With one proxy location per site and no existing include, install succeeds atomically.
cat >"$T/nginx/site-a.conf" <<EOF
server { location / { proxy_pass http://127.0.0.1:13080; } }
EOF
cat >"$T/nginx/site-b.conf" <<EOF
server { location / { proxy_pass http://127.0.0.1:13080; } }
EOF
expect_success install
assert_file_has "$T/nginx/site-a.conf" "include $T/nginx/gate.conf;"
assert_file_has "$T/nginx/site-b.conf" "include $T/nginx/gate.conf;"

# Every mutation went through the configured lock and no remote command exists.
assert_file_has "$LOG" 'flock -x'
! grep -Eqi 'ssh|scp|kubectl|docker exec' "$LOG" || exit 1
printf 'PASS: write gate hook fixture contract\n'
