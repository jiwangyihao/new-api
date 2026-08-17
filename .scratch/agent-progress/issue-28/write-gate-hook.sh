#!/usr/bin/env bash
set -Eeuo pipefail

# Offline-safe write gate adapter. It never sources configuration, never invokes a
# remote shell, and treats the live snippet/sites/containers as the source of truth.
SCRIPT_NAME=${0##*/}
CONFIG=${NEW_API_WRITE_GATE_CONFIG:-}
RESULT=''

fail() { local msg=$1; printf '%s\n' "write-gate: $msg" >&2; RESULT=$(json_false "$msg"); return 1; }
json_escape() { local s=${1//\\/\\\\}; s=${s//\"/\\\"}; s=${s//$'\n'/ }; printf '%s' "$s"; }
json_false() { printf '{"success":false,"error":"%s"}' "$(json_escape "$1")"; }
json_true() { printf '{"success":true%s}' "${1:-}"; }
emit() { printf '%s\n' "$1"; }

[[ -n "$CONFIG" && "$CONFIG" == /* && -f "$CONFIG" ]] || { emit "$(json_false 'NEW_API_WRITE_GATE_CONFIG must be an absolute existing file')"; exit 2; }

declare -A C=()
# Deliberately small whitelist: unknown keys and shell metacharacters fail closed.
allowed_key() { case "$1" in NGINX_BIN|NGINX_CONFIG|NGINX_SITE_A|NGINX_SITE_B|NGINX_GATE_SNIPPET|WRITE_GATE_LOCK|WRITE_GATE_STATE_DIR|WRITE_GATE_AUDIT_DIR|DOCKER_BIN|CURL_BIN|PSQL_BIN|FLOCK_BIN|APP_CONTAINER|POSTGRES_CONTAINER|DB_USER|DB_NAME|RUNTIME_STATS_URL|DRAIN_URL|HEALTH_URL|CLOSE_TIMEOUT_SECONDS|POLL_INTERVAL_SECONDS|DRAIN_TIMEOUT_SECONDS|REQUIRED_MIGRATION_VERSION|MIGRATION_MARKER_FILE|INSTALL_CONFIRM) return 0;; *) return 1;; esac; }
while IFS= read -r line || [[ -n "$line" ]]; do
  [[ -z "$line" || "$line" == \#* ]] && continue
  [[ "$line" =~ ^([A-Z][A-Z0-9_]*)=(.*)$ ]] || { emit "$(json_false 'invalid configuration line')"; exit 2; }
  k=${BASH_REMATCH[1]}; v=${BASH_REMATCH[2]}
  allowed_key "$k" || { emit "$(json_false "configuration key is not allowlisted: $k")"; exit 2; }
  [[ -n "$v" ]] || { emit "$(json_false "empty configuration value: $k")"; exit 2; }
  [[ "$v" != *[\$\;\|\&\<\>\(\)\{\}\[\]\`\"\'\\!]* && "$v" != *[[:space:]]* ]] || { emit "$(json_false "unsafe configuration value: $k")"; exit 2; }
  C[$k]=$v
done < "$CONFIG"

req() { [[ -n ${C[$1]:-} ]] || { emit "$(json_false "missing configuration key: $1")"; exit 2; }; }
for k in NGINX_BIN NGINX_CONFIG NGINX_SITE_A NGINX_SITE_B NGINX_GATE_SNIPPET WRITE_GATE_LOCK WRITE_GATE_STATE_DIR WRITE_GATE_AUDIT_DIR DOCKER_BIN CURL_BIN PSQL_BIN FLOCK_BIN APP_CONTAINER POSTGRES_CONTAINER DB_USER DB_NAME RUNTIME_STATS_URL DRAIN_URL HEALTH_URL CLOSE_TIMEOUT_SECONDS POLL_INTERVAL_SECONDS DRAIN_TIMEOUT_SECONDS REQUIRED_MIGRATION_VERSION MIGRATION_MARKER_FILE; do req "$k"; done
for k in NGINX_CONFIG NGINX_SITE_A NGINX_SITE_B NGINX_GATE_SNIPPET WRITE_GATE_LOCK WRITE_GATE_STATE_DIR WRITE_GATE_AUDIT_DIR MIGRATION_MARKER_FILE; do [[ ${C[$k]} == /* ]] || { emit "$(json_false "$k must be absolute")"; exit 2; }; done
for k in NGINX_BIN DOCKER_BIN CURL_BIN PSQL_BIN FLOCK_BIN; do [[ ${C[$k]} == /* && -x ${C[$k]} ]] || { emit "$(json_false "$k must be an absolute executable")"; exit 2; }; done
[[ ${C[CLOSE_TIMEOUT_SECONDS]} =~ ^[1-9][0-9]*$ && ${C[POLL_INTERVAL_SECONDS]} =~ ^[1-9][0-9]*$ && ${C[DRAIN_TIMEOUT_SECONDS]} =~ ^[1-9][0-9]*$ ]] || { emit "$(json_false 'timeout values must be positive integers')"; exit 2; }
loopback_url() { [[ $1 =~ ^http://127\.0\.0\.1(:[0-9]{1,5})?(/[A-Za-z0-9._~:/?#%+\-]*)?$ ]]; }
for k in RUNTIME_STATS_URL DRAIN_URL HEALTH_URL; do loopback_url "${C[$k]}" || { emit "$(json_false "$k must be an http loopback URL")"; exit 2; }; done

for p in NGINX_SITE_A NGINX_SITE_B NGINX_GATE_SNIPPET; do [[ -f ${C[$p]} ]] || { emit "$(json_false "$p does not exist")"; exit 2; }; done
mkdir -p "${C[WRITE_GATE_STATE_DIR]}" "${C[WRITE_GATE_AUDIT_DIR]}"

valid_release() { [[ ${1:-} =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$ ]]; }
count_text() { local needle=$1 file=$2; awk -v n="$needle" 'index($0,n)>0 {c++} END{print c+0}' "$file"; }
site_include_count() { count_text "include ${C[NGINX_GATE_SNIPPET]};" "$1"; }
site_proxy_count() { awk '/proxy_pass[[:space:]]/ {c++} END{print c+0}' "$1"; }
site_balanced() { awk '{for(i=1;i<=length($0);i++){x=substr($0,i,1); if(x=="{") d++; else if(x=="}") d--; if(d<0) exit 2}} END{if(d!=0) exit 2}' "$1"; }

# The adapter refuses ambiguous topology instead of guessing a location.
verify_sites() {
  for site in "${C[NGINX_SITE_A]}" "${C[NGINX_SITE_B]}"; do
    site_balanced "$site" || { printf '%s\n' "invalid nginx structure: $site" >&2; return 1; }
    [[ $(site_proxy_count "$site") -eq 1 && $(site_include_count "$site") -eq 1 ]] || { printf '%s\n' "site lacks exactly one managed proxy include: $site" >&2; return 1; }
  done
}
read_gate_state() {
  local o c
  o=$(awk '/^# NEW_API_WRITE_GATE_OPEN$/ {n++} END{print n+0}' "${C[NGINX_GATE_SNIPPET]}")
  c=$(awk '/^# NEW_API_WRITE_GATE_CLOSED$/ {n++} END{print n+0}' "${C[NGINX_GATE_SNIPPET]}")
  if [[ $o -eq 1 && $c -eq 0 ]]; then printf open; elif [[ $c -eq 1 && $o -eq 0 ]]; then printf closed; else return 1; fi
}

nginx_test() { "${C[NGINX_BIN]}" -t -c "${C[NGINX_CONFIG]}" >/dev/null; }
nginx_reload() { "${C[NGINX_BIN]}" -s reload >/dev/null; }
backup_snippet() { local rel=$1 dst="${C[WRITE_GATE_AUDIT_DIR]}/snippet.${rel}.$(date -u +%Y%m%dT%H%M%SZ).bak"; umask 077; cp -p "${C[NGINX_GATE_SNIPPET]}" "$dst"; chmod 600 "$dst"; }
write_gate() {
  local desired=$1 rel=$2 old tmp
  old=$(read_gate_state) || { printf '%s\n' 'managed gate snippet has unknown state' >&2; return 1; }
  verify_sites || return 1
  [[ $old == "$desired" ]] && return 0
  backup_snippet "$rel" || return 1
  tmp=$(mktemp "${C[NGINX_GATE_SNIPPET]}.tmp.XXXXXX"); chmod 600 "$tmp"
  if [[ $desired == closed ]]; then printf '%s\n' '# NEW_API_WRITE_GATE_CLOSED' 'return 503;' >"$tmp"; else printf '%s\n' '# NEW_API_WRITE_GATE_OPEN' >"$tmp"; fi
  if ! mv -f "$tmp" "${C[NGINX_GATE_SNIPPET]}" || ! nginx_test || ! nginx_reload; then
    rm -f "$tmp"
    cp -p "${C[WRITE_GATE_AUDIT_DIR]}"/snippet."$rel".*.bak "${C[NGINX_GATE_SNIPPET]}" 2>/dev/null || true
    chmod 600 "${C[NGINX_GATE_SNIPPET]}"
    nginx_test >/dev/null 2>&1 || true; nginx_reload >/dev/null 2>&1 || true
    return 1
  fi
}

# Install is explicit only; it stages and validates both files before either is moved.
install_site() {
  local site=$1 tmp count
  count=$(site_proxy_count "$site"); [[ $count -eq 1 ]] || return 1
  [[ $(site_include_count "$site") -eq 0 ]] || return 1
  tmp=$(mktemp "$site.tmp.XXXXXX"); chmod 600 "$tmp"
  awk -v inc="include ${C[NGINX_GATE_SNIPPET]};" 'BEGIN{done=0} /proxy_pass[[:space:]]/ && !done {print "    " inc; done=1} {print}' "$site" >"$tmp"
  site_balanced "$tmp" && [[ $(site_proxy_count "$tmp") -eq 1 && $(count_text "include ${C[NGINX_GATE_SNIPPET]};" "$tmp") -eq 1 ]] || { rm -f "$tmp"; return 1; }
  printf '%s\n' "$tmp"
}
install_gate() {
  [[ ${C[INSTALL_CONFIRM]:-} == YES && ${WRITE_GATE_INSTALL_CONFIRM:-} == YES ]] || { RESULT=$(json_false 'install requires explicit dual confirmation'); return 1; }
  local a b ba bb
  a=$(install_site "${C[NGINX_SITE_A]}") || { RESULT=$(json_false 'first nginx site is not installable'); return 1; }
  b=$(install_site "${C[NGINX_SITE_B]}") || { rm -f "$a"; RESULT=$(json_false 'second nginx site is not installable'); return 1; }
  ba="${C[WRITE_GATE_AUDIT_DIR]}/site-a.$(date -u +%Y%m%dT%H%M%SZ).bak"
  bb="${C[WRITE_GATE_AUDIT_DIR]}/site-b.$(date -u +%Y%m%dT%H%M%SZ).bak"
  cp -p "${C[NGINX_SITE_A]}" "$ba" && cp -p "${C[NGINX_SITE_B]}" "$bb" && chmod 600 "$ba" "$bb" || { rm -f "$a" "$b"; RESULT=$(json_false 'failed to back up nginx sites'); return 1; }
  if ! mv -f "$a" "${C[NGINX_SITE_A]}" || ! mv -f "$b" "${C[NGINX_SITE_B]}" || ! nginx_test || ! nginx_reload; then
    cp -p "$ba" "${C[NGINX_SITE_A]}"
    cp -p "$bb" "${C[NGINX_SITE_B]}"
    rm -f "$a" "$b"
    nginx_test >/dev/null 2>&1 || true
    RESULT=$(json_false 'failed to install nginx gate')
    return 1
  fi
  RESULT=$(json_true ',"state":"installed"')
}

run_cmd() { local target=$1 output; shift; output=$("$@") || return 1; printf -v "$target" '%s' "$output"; }
json_field() { local key=$1 text=$2; sed -nE 's/.*"'"$key"'"[[:space:]]*:[[:space:]]*("([^"]*)"|(-?[0-9]+)|true|false).*/\2\3/p' <<<"$text" | sed -n '1p'; }
num_field() { local key=$1 text=$2 v; v=$(json_field "$key" "$text"); [[ $v =~ ^[0-9]+$ ]] && printf '%s' "$v" || return 1; }
runtime_stats() { local x; run_cmd x "${C[CURL_BIN]}" -fsS --max-time "${C[CLOSE_TIMEOUT_SECONDS]}" "${C[RUNTIME_STATS_URL]}" || return 1; printf '%s' "$x"; }
health_check() { local x; run_cmd x "${C[CURL_BIN]}" -fsS --max-time "${C[CLOSE_TIMEOUT_SECONDS]}" "${C[HEALTH_URL]}" || return 1; [[ $x == *200* || $x == *healthy* || $x == *"\"success\":true"* ]]; }
container_running() { local x; run_cmd x "${C[DOCKER_BIN]}" inspect "$1" || return 1; [[ $x == *running* || $x == *healthy* ]]; }
container_healthy() { local x; run_cmd x "${C[DOCKER_BIN]}" inspect "$1" || return 1; [[ $x == *healthy* || $x == *running* ]]; }
readonly_sql_count() {
  local query=$1 x
  run_cmd x "${C[PSQL_BIN]}" -AtX -U "${C[DB_USER]}" -d "${C[DB_NAME]}" -c "$query" || return 1
  x=${x//[[:space:]]/}
  [[ $x =~ ^[0-9]+$ ]] || return 1
  printf '%s' "$x"
}
pg_sessions() { readonly_sql_count 'SELECT count(*) FROM pg_stat_activity WHERE client_addr IS NOT NULL;'; }
marker_ready() { [[ -f ${C[MIGRATION_MARKER_FILE]} ]] || return 1; local x; x=$(cat "${C[MIGRATION_MARKER_FILE]}"); [[ $x == *"${C[REQUIRED_MIGRATION_VERSION]}"* && $x == *ready* ]]; }
counts() {
  local s=$1 http tasks pre async legacy
  http=$(num_field http_active "$s" 2>/dev/null || num_field active_http "$s") || return 1
  tasks=$(readonly_sql_count "SELECT count(*) FROM tasks t LEFT JOIN subscription_pre_consume_records r ON r.request_id = t.subscription_request_id WHERE t.status IN ('QUEUED','SUBMITTED','IN_PROGRESS') AND btrim(coalesce(t.subscription_request_id, '')) <> '' AND r.request_id IS NOT NULL") || return 1
  pre=$(readonly_sql_count "SELECT count(*) FROM tasks t LEFT JOIN subscription_pre_consume_records r ON r.request_id = t.subscription_request_id WHERE t.status IN ('QUEUED','SUBMITTED','IN_PROGRESS') AND btrim(coalesce(t.subscription_request_id, '')) <> '' AND r.request_id IS NOT NULL AND (r.status IS NULL OR r.status NOT IN ('settled','refunded'))") || return 1
  async=$(readonly_sql_count "SELECT count(*) FROM tasks t LEFT JOIN subscription_pre_consume_records r ON r.request_id = t.subscription_request_id WHERE t.status IN ('QUEUED','SUBMITTED','IN_PROGRESS') AND (btrim(coalesce(t.subscription_request_id, '')) = '' OR r.request_id IS NULL)") || return 1
  legacy=$(pg_sessions) || return 1
  printf '%s %s %s %s %s' "$http" "$tasks" "$pre" "$async" "$legacy"
}
status_json() {
  local state s vals http tasks pre async legacy
  state=$(read_gate_state) || { RESULT=$(json_false 'cannot determine actual gate snippet state'); return 1; }; verify_sites || { RESULT=$(json_false 'cannot verify nginx sites'); return 1; }
  container_healthy "${C[APP_CONTAINER]}" || { RESULT=$(json_false 'application container health check failed'); return 1; }
  health_check || { RESULT=$(json_false 'application health endpoint failed'); return 1; }
  s=$(runtime_stats) || { RESULT=$(json_false 'runtime stats command failed'); return 1; }; vals=$(counts "$s") || { RESULT=$(json_false 'runtime or SQL parsing failed'); return 1; }; read -r http tasks pre async legacy <<<"$vals"
  RESULT=$(printf '{"success":true,"state":"%s","external_writers":%s,"background_writers":%s,"non_terminal_preconsume":%s,"async_settlement":%s,"legacy_writer_sessions":%s}' "$state" "$http" "$tasks" "$pre" "$async" "$legacy")
}
with_lock() { local bash_bin child_output; bash_bin=$(command -v bash) || { RESULT=$(json_false 'bash interpreter unavailable'); return 1; }; [[ $bash_bin == /* && -x $bash_bin ]] || { RESULT=$(json_false 'bash interpreter is not absolute executable'); return 1; }; child_output=$("${C[FLOCK_BIN]}" -x "${C[WRITE_GATE_LOCK]}" "$bash_bin" "$0" --locked "$@") || { RESULT=$(json_false 'mutation lock command failed'); return 1; }; [[ $child_output == \{\"success\":true* || $child_output == \{\"success\":false* ]] || { RESULT=$(json_false 'locked operation returned invalid JSON'); return 1; }; RESULT=$child_output; }
close_gate() {
  local rel=$1 s vals http tasks pre async legacy deadline now pending
  valid_release "$rel" || { RESULT=$(json_false 'invalid release id'); return 1; }
  write_gate closed "$rel" || { RESULT=$(json_false 'failed to close nginx gate'); return 1; }
  deadline=$(( $(date +%s) + ${C[CLOSE_TIMEOUT_SECONDS]} ))
  while :; do s=$(runtime_stats) || { RESULT=$(json_false 'runtime stats failed while draining'); return 1; }; vals=$(counts "$s") || { RESULT=$(json_false 'runtime/SQL count failed while draining'); return 1; }; read -r http tasks pre async legacy <<<"$vals"; if ((http<=1 && tasks==0 && pre==0 && async==0)); then break; fi; now=$(date +%s); ((now>=deadline)) && { RESULT=$(json_false 'drain timeout; gate remains closed'); return 1; }; sleep "${C[POLL_INTERVAL_SECONDS]}"; done
  pending=$(${C[CURL_BIN]} -fsS --max-time "${C[DRAIN_TIMEOUT_SECONDS]}" -X POST "${C[DRAIN_URL]}") || { RESULT=$(json_false 'drain endpoint failed; gate remains closed'); return 1; }; [[ $(json_field pending "$pending") == 0 ]] || { RESULT=$(json_false 'drain endpoint pending is not zero'); return 1; }
  "${C[DOCKER_BIN]}" stop "${C[APP_CONTAINER]}" >/dev/null || { RESULT=$(json_false 'application stop failed; gate remains closed'); return 1; }
  if container_running "${C[APP_CONTAINER]}"; then RESULT=$(json_false 'application container still running'); return 1; fi
  legacy=$(pg_sessions) || { RESULT=$(json_false 'postgres session check failed'); return 1; }; [[ $legacy == 0 ]] || { RESULT=$(json_false 'legacy writer sessions remain'); return 1; }
  RESULT=$(printf '{"success":true,"state":"closed","external_writers":0,"background_writers":0,"non_terminal_preconsume":0,"async_settlement":0,"legacy_writer_sessions":0}')
}
open_gate() {
  local rel=$1 state s vals http tasks pre async legacy
  valid_release "$rel" || { RESULT=$(json_false 'invalid release id'); return 1; }
  state=$(read_gate_state) || { RESULT=$(json_false 'cannot determine gate state'); return 1; }
  container_healthy "${C[APP_CONTAINER]}" || { RESULT=$(json_false 'application is not healthy'); return 1; }
  health_check || { RESULT=$(json_false 'health endpoint failed'); return 1; }
  marker_ready || { RESULT=$(json_false 'required migration marker is not ready'); return 1; }
  s=$(runtime_stats) || { RESULT=$(json_false 'runtime stats failed'); return 1; }
  vals=$(counts "$s") || { RESULT=$(json_false 'runtime/SQL count failed'); return 1; }
  read -r http tasks pre async legacy <<<"$vals"
  ((tasks==0 && pre==0 && async==0 && legacy==0)) || { RESULT=$(json_false 'writers or blockers remain'); return 1; }
  if [[ $state == closed ]]; then
    write_gate open "$rel" || { RESULT=$(json_false 'failed to open nginx gate'); return 1; }
  elif [[ $state != open ]]; then
    RESULT=$(json_false 'gate is neither closed nor open')
    return 1
  fi
  RESULT=$(printf '{"success":true,"state":"open","external_writers":%s,"background_writers":0,"non_terminal_preconsume":0,"async_settlement":0,"legacy_writer_sessions":0}' "$http")
}

op=${1:-}; release=${2:-}
if [[ $op == --locked ]]; then
  shift
  op=${1:-}; release=${2:-}
  case "$op" in
    close|open) [[ $# -eq 2 ]] || { emit "$(json_false "$op requires release id")"; exit 2; }; "${op}_gate" "$release" || true;;
    install) [[ $# -eq 1 ]] || { emit "$(json_false 'install takes no arguments')"; exit 2; }; install_gate || true;;
    *) emit "$(json_false 'invalid locked operation')"; exit 2;;
  esac
else
  case "$op" in
    status) [[ $# -eq 1 ]] || { emit "$(json_false 'status takes no release id')"; exit 2; }; status_json || true;;
    close|open) [[ $# -eq 2 ]] || { emit "$(json_false "$op requires release id")"; exit 2; }; with_lock "$op" "$release" || true;;
    install) [[ $# -eq 1 ]] || { emit "$(json_false 'install takes no arguments')"; exit 2; }; with_lock install || true;;
    *) emit "$(json_false 'usage: status|close RELEASE_ID|open RELEASE_ID|install')"; exit 2;;
  esac
fi
emit "${RESULT:-$(json_false 'operation failed')}"
