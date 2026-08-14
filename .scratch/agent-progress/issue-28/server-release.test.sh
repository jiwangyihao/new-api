#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT="$SCRIPT_DIR/server-release.sh"

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

posix_mode_semantics_available() {
  local probe_root probe_file mode
  probe_root="$(mktemp -d)"
  probe_file="$probe_root/mode-probe"
  : >"$probe_file"
  chmod 0600 "$probe_file" || { rm -rf "$probe_root"; return 1; }
  mode="$(stat -c '%a' "$probe_file" 2>/dev/null || true)"
  rm -rf "$probe_root"
  [[ "$mode" == 600 ]]
}

POSIX_MODE_SEMANTICS_AVAILABLE=true
if ! posix_mode_semantics_available; then
  POSIX_MODE_SEMANTICS_AVAILABLE=false
fi

write_permission_shims() {
  cat >"$TEST_MODE_LEDGER_HELPER" <<'LEDGER'
#!/usr/bin/env bash
set -Eeuo pipefail

ledger_rewrite_without() {
  local omitted="$1" temporary="${MODE_LEDGER_STUB}.tmp.$$" candidate mode
  : >"$temporary"
  if [[ -f "$MODE_LEDGER_STUB" ]]; then
    while IFS=$'\t' read -r candidate mode; do
      [[ "$candidate" == "$omitted" ]] || printf '%s\t%s\n' "$candidate" "$mode" >>"$temporary"
    done <"$MODE_LEDGER_STUB"
  fi
  "$REAL_MV_SHIM" -f "$temporary" "$MODE_LEDGER_STUB"
}

ledger_lookup() {
  local wanted="$1" candidate mode
  [[ -f "$MODE_LEDGER_STUB" ]] || return 1
  while IFS=$'\t' read -r candidate mode; do
    if [[ "$candidate" == "$wanted" ]]; then
      printf '%s\n' "$mode"
      return 0
    fi
  done <"$MODE_LEDGER_STUB"
  return 1
}

case "${1:-}" in
  lookup)
    [[ "$#" -eq 2 ]] || exit 2
    ledger_lookup "$2"
    ;;
  set)
    [[ "$#" -eq 3 ]] || exit 2
    ledger_rewrite_without "$2"
    printf '%s\t%s\n' "$2" "$3" >>"$MODE_LEDGER_STUB"
    ;;
  move)
    [[ "$#" -eq 3 ]] || exit 2
    source="$2"
    destination="$3"
    source_mode="$(ledger_lookup "$source" 2>/dev/null || true)"
    ledger_rewrite_without "$source"
    ledger_rewrite_without "$destination"
    if [[ -n "$source_mode" ]]; then
      printf '%s\t%s\n' "$destination" "$source_mode" >>"$MODE_LEDGER_STUB"
    fi
    ;;
  *)
    exit 2
    ;;
esac
LEDGER
  "$REAL_CHMOD_SHIM" +x "$TEST_MODE_LEDGER_HELPER"

  cat >"$TEST_BIN/stat" <<'STUB'
#!/usr/bin/env bash
set -Eeuo pipefail

if [[ "${1:-}" == '-c' && "${2:-}" == '%a' && "$#" -eq 3 ]]; then
  path="$3"
  case "${PERMISSION_OVERRIDE_STUB:-none}" in
    state)
      [[ "$path" == "${MODE_STATE_PATH_STUB:-}" ]] && { printf '644\n'; exit 0; }
      ;;
    checksum)
      [[ "$path" == "${MODE_CHECKSUM_PATH_STUB:-}" ]] && { printf '644\n'; exit 0; }
      ;;
    approval)
      [[ "$path" == "${MODE_APPROVAL_PATH_STUB:-}" ]] && { printf '644\n'; exit 0; }
      ;;
  esac
  mode="$("$MODE_LEDGER_HELPER" lookup "$path" 2>/dev/null || true)"
  if [[ -n "$mode" ]]; then
    printf '%s\n' "$mode"
    exit 0
  fi
fi
exec "$REAL_STAT_SHIM" "$@"
STUB
  "$REAL_CHMOD_SHIM" +x "$TEST_BIN/stat"

  cat >"$TEST_BIN/chmod" <<'STUB'
#!/usr/bin/env bash
set -Eeuo pipefail
if [[ "$#" -eq 2 && "${1:-}" =~ ^[0-7]{3,4}$ ]]; then
  "$REAL_CHMOD_SHIM" "$@"
  mode="$1"
  [[ "${#mode}" -eq 4 && "$mode" == 0* ]] && mode="${mode#0}"
  "$MODE_LEDGER_HELPER" set "$2" "$mode"
  exit 0
fi
exec "$REAL_CHMOD_SHIM" "$@"
STUB
  "$REAL_CHMOD_SHIM" +x "$TEST_BIN/chmod"

  cat >"$TEST_BIN/mv" <<'STUB'
#!/usr/bin/env bash
set -Eeuo pipefail
if [[ "${1:-}" == '-f' && "$#" -eq 3 ]]; then
  "$REAL_MV_SHIM" "$@" || { rc=$?; exit "$rc"; }
  "$MODE_LEDGER_HELPER" move "$2" "$3"
  exit 0
fi
exec "$REAL_MV_SHIM" "$@"
STUB
  "$REAL_CHMOD_SHIM" +x "$TEST_BIN/mv"
}
make_jq_stub() {
  cat >"$TEST_BIN/jq" <<'STUB'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'jq %s\n' "$*" >>"$CALL_LOG"
exec python "$JQ_IMPLEMENTATION" "$@"
STUB
  chmod +x "$TEST_BIN/jq"
}

write_jq_implementation() {
  cat >"$TEST_JQ_IMPLEMENTATION" <<'PY'
import json
import os
import sys

args = sys.argv[1:]
raw = "-r" in args
expr_parts = []
values = {}
i = 0
while i < len(args):
    arg = args[i]
    if arg in ("-e", "-r", "-c", "-M", "-S"):
        i += 1
        continue
    if arg in ("--arg", "--argjson"):
        key, value = args[i + 1], args[i + 2]
        values[key] = json.loads(value) if arg == "--argjson" else value
        i += 3
        continue
    if arg.startswith("-"):
        i += 1
        continue
    expr_parts.append(arg)
    i += 1
expr = " ".join(expr_parts)
if expr_parts and os.path.isfile(expr_parts[-1]):
    source = open(expr_parts[-1], encoding="utf-8").read()
else:
    source = sys.stdin.read()
try:
    data = json.loads(source)
except Exception as exc:
    print(f"invalid JSON input: {exc}", file=sys.stderr)
    sys.exit(2)

def report():
    return data.get("report", {}) if isinstance(data, dict) else {}

def gate(closed=False, opened=False):
    if not isinstance(data, dict) or data.get("success") is not True:
        return False
    state = data.get("state")
    if closed:
        return state == "closed" and all(data.get(k) == 0 for k in (
            "external_writers", "background_writers", "non_terminal_preconsume",
            "async_settlement", "legacy_writer_sessions"))
    if opened:
        return state == "open"
    return state in ("open", "closed") and all(
        isinstance(data.get(k), (int, float)) and data[k] >= 0 for k in (
            "external_writers", "background_writers", "non_terminal_preconsume",
            "async_settlement", "legacy_writer_sessions"))

def migration(mode):
    r = report()
    if not isinstance(data, dict) or data.get("success") is not True:
        return False
    if r.get("version") != int(values.get("version", os.environ.get("MIGRATION_VERSION", "1"))):
        return False
    if r.get("mode") != mode:
        return False
    if not isinstance(r.get("checksum"), str) or len(r["checksum"]) != 64:
        return False
    if mode == "dry_run":
        return (r.get("status") == "pending" and r.get("read_only") is True and
                r.get("changed") is False and r.get("ready") is False and
                r.get("blockers") == [] and r.get("price", {}).get("rows_invalid") == 0 and
                r.get("price", {}).get("rows_total", 0) > 0 and
                r.get("credit", {}).get("rows_total", 0) > 0)
    if mode == "apply":
        return (r.get("status") == "ready" and r.get("read_only") is False and
                isinstance(r.get("changed"), bool) and r.get("ready") is True and
                r.get("blockers") == [] and r.get("price", {}).get("rows_invalid") == 0)
    if mode == "verify":
        return (r.get("status") == "ready" and r.get("read_only") is True and
                r.get("changed") is False and r.get("ready") is True and
                r.get("blockers") == [] and r.get("checksum") == values.get("checksum"))
    if mode == "suspend":
        return r.get("status") == "suspended" and r.get("ready") is False
    return False

def probe(kind):
    if not isinstance(data, dict) or data.get("success") is not True:
        return False
    if kind == "production":
        return (data.get("environment") == "production" and data.get("read_only") is True and
                data.get("digest") == values.get("digest") and data.get("revision") == values.get("revision") and
                data.get("marker_status") == "ready" and data.get("migration_version") == int(values.get("version", 1)) and
                all(data.get(k) is True for k in ("invariants", "authenticated_frontend",
                    "disabled_plan_existing_consumable", "disabled_plan_new_allocations_rejected", "model_scope_ignored")))
    if kind == "clone":
        f = data.get("fixture", {})
        return (data.get("environment") == "isolated_clone" and
                data.get("source_backup_sha256") == values.get("backup_checksum") and
                f == {"price_amount_micros": "40000000", "plan_credit": 1000,
                     "consumed_credit": 200, "available_credit": 800, "end_time": 0,
                     "exact_cost_micros": "32000000", "currency": "CNY",
                     "active_paid_subscription_count": 1, "estimated_cost_micros": "0",
                     "unknown_credit": 0, "five_analytics_endpoints_consistent": True})
    if kind == "observe":
        return (data.get("aggregated") is True and data.get("window_seconds", 0) >= int(values.get("seconds", 1)) and
                all(data.get(k) == 0 for k in ("health_failures", "credit_valuation_state_missing",
                    "credit_valuation_state_mismatch", "unsupported_fx_errors", "panic_count", "abnormal_restarts")) and
                data.get("postgres_lock_wait_regression") is False and data.get("write_load_regression") is False)
    return False

if raw and ".state" in expr and "report" not in expr:
    print(data.get("state", "")); sys.exit(0)
if raw and ".report.checksum" in expr:
    print(report().get("checksum", "")); sys.exit(0)
if "index($reference)" in expr:
    result = values.get("reference") in data
elif '.report.mode == "dry_run"' in expr:
    result = migration("dry_run")
elif '.report.mode == "apply"' in expr:
    result = migration("apply")
elif '.report.mode == "verify"' in expr:
    result = migration("verify")
elif '.report.mode == "suspend"' in expr:
    result = migration("suspend")
elif '.environment == "production"' in expr:
    result = probe("production")
elif '.environment == "isolated_clone"' in expr:
    result = probe("clone")
elif ".aggregated == true" in expr:
    result = probe("observe")
elif '.success == true and .state == "closed"' in expr and ".external_writers" in expr:
    result = gate(closed=True)
elif '.success == true and .state == "open"' in expr:
    result = gate(opened=True)
elif ".external_writers" in expr:
    result = gate()
elif ".success == true" in expr:
    result = isinstance(data, dict) and data.get("success") is True
else:
    result = False
sys.exit(0 if result else 1)
PY
}

setup_fixture() {
  TEST_TMP="$(mktemp -d)"
  TEST_ROOT="$TEST_TMP/root"
  TEST_BIN="$TEST_TMP/bin"
  mkdir -p "$TEST_ROOT" "$TEST_BIN"
  : >"$TEST_ROOT/compose.yml"
  : >"$TEST_ROOT/compose.network.yml"
  : >"$TEST_ROOT/compose.primary.yml"
  : >"$TEST_ROOT/app.env"
  TEST_TARGET="ghcr.io/jiwangyihao/new-api@sha256:$(printf '1%.0s' {1..64})"
  TEST_CURRENT="ghcr.io/jiwangyihao/new-api@sha256:$(printf '2%.0s' {1..64})"
  TEST_REVISION=1111111111111111111111111111111111111111
  TEST_CURRENT_REVISION=2222222222222222222222222222222222222222
  TEST_TARGET_CONFIG="sha256:$(printf 'a%.0s' {1..64})"
  TEST_CURRENT_CONFIG="sha256:$(printf 'b%.0s' {1..64})"
  TEST_CHECKSUM="$(printf 'c%.0s' {1..64})"
  TEST_APPLY_CHECKSUM="$TEST_CHECKSUM"
  TEST_CALLS="$TEST_TMP/calls.log"
  TEST_STUB_STATE="$TEST_TMP/stub-state"
  TEST_JQ_IMPLEMENTATION="$TEST_TMP/jq.py"
  REAL_STAT_SHIM="$(command -v stat || true)"
  REAL_CHMOD_SHIM="$(command -v chmod || true)"
  REAL_MV_SHIM="$(command -v mv || true)"
  [[ "$REAL_STAT_SHIM" == /* && -x "$REAL_STAT_SHIM" ]] || fail 'real stat command is unavailable'
  [[ "$REAL_CHMOD_SHIM" == /* && -x "$REAL_CHMOD_SHIM" ]] || fail 'real chmod command is unavailable'
  [[ "$REAL_MV_SHIM" == /* && -x "$REAL_MV_SHIM" ]] || fail 'real mv command is unavailable'
  TEST_MODE_LEDGER_HELPER="$TEST_TMP/mode-ledger-helper"
  MODE_LEDGER_STUB="$TEST_TMP/mode-ledger.tsv"
  MODE_STATE_PATH_STUB="$TEST_ROOT/state/issue28-test.state"
  MODE_CHECKSUM_PATH_STUB="$MODE_STATE_PATH_STUB.sha256"
  MODE_APPROVAL_PATH_STUB="$TEST_TMP/mutation.approval"
  PERMISSION_OVERRIDE_STUB=none
  : >"$MODE_LEDGER_STUB"
  TEST_RUN_INDEX=0
  write_permission_shims
  : >"$TEST_CALLS"
  printf 'services:\n  new-api:\n    image: %s\n' "$TEST_CURRENT" >"$TEST_ROOT/compose.release.yml"
  write_jq_implementation
  make_jq_stub

  cat >"$TEST_BIN/flock" <<'STUB'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'flock %s\n' "$*" >>"$CALL_LOG"
exit 0
STUB
  chmod +x "$TEST_BIN/flock"

  cat >"$TEST_BIN/docker" <<'STUB'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'docker %s\n' "$*" >>"$CALL_LOG"
args="$*"
if [[ "$args" == *"image inspect --format"*"org.opencontainers.image.revision"* ]]; then
  if [[ "$args" == *"$TARGET_IMAGE_STUB" ]]; then
    printf '%s\n' "$EXPECTED_REVISION_STUB"
  elif [[ "$args" == *"$CURRENT_IMAGE_STUB" ]]; then
    printf '%s\n' "$CURRENT_REVISION_STUB"
  else
    running="$(cat "$STUB_STATE.running_image" 2>/dev/null || printf '%s' "$CURRENT_IMAGE_STUB")"
    [[ "$running" == "$TARGET_IMAGE_STUB" ]] && printf '%s\n' "$EXPECTED_REVISION_STUB" || printf '%s\n' "$CURRENT_REVISION_STUB"
  fi
elif [[ "$args" == "image inspect --format {{json .RepoDigests}} $TARGET_IMAGE_STUB" ]]; then
  printf '["%s"]\n' "$TARGET_IMAGE_STUB"
elif [[ "$args" == "image inspect --format {{json .RepoDigests}} $CURRENT_IMAGE_STUB" ]]; then
  printf '["%s"]\n' "$CURRENT_IMAGE_STUB"
elif [[ "$args" == "image inspect --format {{.Id}} $TARGET_IMAGE_STUB" ]]; then
  printf '%s\n' "$TARGET_CONFIG_STUB"
elif [[ "$args" == "image inspect --format {{.Id}} $CURRENT_IMAGE_STUB" ]]; then
  printf '%s\n' "$CURRENT_CONFIG_STUB"
elif [[ "$args" == "inspect --format {{.Config.Image}} new-api" ]]; then
  cat "$STUB_STATE.running_image" 2>/dev/null || printf '%s\n' "$CURRENT_IMAGE_STUB"
elif [[ "$args" == "inspect --format {{.Image}} new-api" ]]; then
  running="$(cat "$STUB_STATE.running_image" 2>/dev/null || printf '%s' "$CURRENT_IMAGE_STUB")"
  [[ "$running" == "$TARGET_IMAGE_STUB" ]] && printf '%s\n' "$TARGET_CONFIG_STUB" || printf '%s\n' "$CURRENT_CONFIG_STUB"
elif [[ "$args" == "inspect --format {{index .Config.Labels \"org.opencontainers.image.revision\"}} new-api" ]]; then
  running="$(cat "$STUB_STATE.running_image" 2>/dev/null || printf '%s' "$CURRENT_IMAGE_STUB")"
  [[ "$running" == "$TARGET_IMAGE_STUB" ]] && printf '%s\n' "$EXPECTED_REVISION_STUB" || printf '%s\n' "$CURRENT_REVISION_STUB"
elif [[ "$args" == "inspect --format {{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}} new-api" ]]; then
  printf 'healthy\n'
elif [[ "$args" == *"compose"*" up -d"* ]]; then
  if [[ "${FAIL_MODE:-}" == stage ]]; then
    printf 'simulated startup failure\n' >&2
    exit 54
  fi
  awk '/image:/{print $2}' "$COMPOSE_RELEASE_STUB" >"$STUB_STATE.running_image"
elif [[ "$args" == *"run --rm"*"credit-valuation-migrate --dry-run"* ]]; then
  printf '%s\n' "$DRY_REPORT_STUB"
elif [[ "$args" == *"run --rm"*"credit-valuation-migrate --apply"* ]]; then
  [[ "${FAIL_MODE:-}" != apply ]] || { printf 'simulated apply failure\n' >&2; exit 55; }
  printf '%s\n' "$APPLY_REPORT_STUB"
elif [[ "$args" == *"run --rm"*"credit-valuation-migrate --verify"* ]]; then
  printf '%s\n' "$VERIFY_REPORT_STUB"
elif [[ "$args" == *"run --rm"*"credit-valuation-migrate --suspend"* ]]; then
  printf suspended >"$STUB_STATE.marker"
  printf '%s\n' "$SUSPEND_REPORT_STUB"
  [[ "${FAIL_MODE:-}" != suspend_after_marker ]] || exit 56
elif [[ "$args" == *"exec"*"pg_dump"* ]]; then
  marker="$(cat "$STUB_STATE.marker" 2>/dev/null || printf ready)"
  [[ "${FAIL_MODE:-}" != backup_after_suspend || "$marker" != suspended ]] || { printf 'simulated post-suspend backup failure\n' >&2; exit 57; }
  printf 'fake-postgres-custom-format-backup marker=%s\n' "$marker"
elif [[ "$args" == *"exec"*"pg_restore --list"* ]]; then
  cat >/dev/null
else
  printf 'unexpected docker call: %s\n' "$args" >&2
  exit 96
fi
STUB
  chmod +x "$TEST_BIN/docker"

  cat >"$TEST_TMP/write-gate" <<'HOOK'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'write-gate %s\n' "$*" >>"$CALL_LOG"
state="$(cat "$STUB_STATE.gate" 2>/dev/null || printf open)"
case "${1:-}" in
  status)
    if [[ "$state" == closed ]]; then
      printf '%s\n' '{"success":true,"state":"closed","external_writers":0,"background_writers":0,"non_terminal_preconsume":0,"async_settlement":0,"legacy_writer_sessions":0}'
    else
      printf '%s\n' '{"success":true,"state":"open","external_writers":0,"background_writers":0,"non_terminal_preconsume":0,"async_settlement":0,"legacy_writer_sessions":0}'
    fi ;;
  close)
    printf closed >"$STUB_STATE.gate"
    printf '%s\n' '{"success":true,"state":"closed","external_writers":0,"background_writers":0,"non_terminal_preconsume":0,"async_settlement":0,"legacy_writer_sessions":0}' ;;
  open)
    printf open >"$STUB_STATE.gate"
    printf '%s\n' '{"success":true,"state":"open","external_writers":0,"background_writers":0,"non_terminal_preconsume":0,"async_settlement":0,"legacy_writer_sessions":0}' ;;
  *) exit 2 ;;
esac
HOOK
  chmod +x "$TEST_TMP/write-gate"

  cat >"$TEST_TMP/production-probe" <<'HOOK'
#!/usr/bin/env bash
set -Eeuo pipefail
printf '%s\n' '{"success":true,"environment":"production","read_only":true,"digest":"'"$2"'","revision":"'"$3"'","marker_status":"ready","migration_version":1,"invariants":true,"authenticated_frontend":true,"disabled_plan_existing_consumable":true,"disabled_plan_new_allocations_rejected":true,"model_scope_ignored":true}'
HOOK
  chmod +x "$TEST_TMP/production-probe"
  cat >"$TEST_TMP/clone-probe" <<'HOOK'
#!/usr/bin/env bash
set -Eeuo pipefail
printf '%s\n' '{"success":true,"environment":"isolated_clone","source_backup_sha256":"'"$3"'","fixture":{"price_amount_micros":"40000000","plan_credit":1000,"consumed_credit":200,"available_credit":800,"end_time":0,"exact_cost_micros":"32000000","currency":"CNY","active_paid_subscription_count":1,"estimated_cost_micros":"0","unknown_credit":0,"five_analytics_endpoints_consistent":true}}'
HOOK
  chmod +x "$TEST_TMP/clone-probe"
  cat >"$TEST_TMP/observe" <<'HOOK'
#!/usr/bin/env bash
set -Eeuo pipefail
printf '%s\n' '{"success":true,"aggregated":true,"window_seconds":1,"health_failures":0,"credit_valuation_state_missing":0,"credit_valuation_state_mismatch":0,"unsupported_fx_errors":0,"panic_count":0,"abnormal_restarts":0,"postgres_lock_wait_regression":false,"write_load_regression":false}'
HOOK
  chmod +x "$TEST_TMP/observe"

  TEST_CONFIG="$TEST_TMP/release.env"
  cat >"$TEST_CONFIG" <<EOF
RELEASE_ID=issue28-test
EXPECTED_REVISION=$TEST_REVISION
TARGET_IMAGE=$TEST_TARGET
CURRENT_IMAGE=$TEST_CURRENT
MIGRATION_VERSION=1
ROOT=$TEST_ROOT
COMPOSE_BASE=$TEST_ROOT/compose.yml
COMPOSE_NETWORK=$TEST_ROOT/compose.network.yml
COMPOSE_PRIMARY=$TEST_ROOT/compose.primary.yml
COMPOSE_RELEASE=$TEST_ROOT/compose.release.yml
APP_ENV_FILE=$TEST_ROOT/app.env
DOCKER_NETWORK=issue28-test-network
WRITE_GATE_HOOK=$TEST_TMP/write-gate
READ_ONLY_PROBE_HOOK=$TEST_TMP/production-probe
CLONE_PROBE_HOOK=$TEST_TMP/clone-probe
OBSERVE_HOOK=$TEST_TMP/observe
MUTATION_APPROVAL_FILE=$TEST_TMP/mutation.approval
OPEN_WRITES_APPROVAL_FILE=$TEST_TMP/open.approval
ROLLBACK_APPROVAL_FILE=$TEST_TMP/rollback.approval
AUDIT_DIR=$TEST_ROOT/audits
BACKUP_DIR=$TEST_ROOT/backups
STATE_DIR=$TEST_ROOT/state
LOCK_FILE=$TEST_ROOT/release.lock
OBSERVE_SECONDS=1
SUSPEND_REASON=incident-test
EOF
}

cleanup_fixture() { rm -rf "$TEST_TMP"; }

run_release() {
  local phase="$1" invocation_log rc
  TEST_RUN_INDEX=$((TEST_RUN_INDEX + 1))
  invocation_log="$TEST_TMP/run-${TEST_RUN_INDEX}-${phase}.log"
  if CALL_LOG="$TEST_CALLS" STUB_STATE="$TEST_STUB_STATE" JQ_IMPLEMENTATION="$TEST_JQ_IMPLEMENTATION" \
    REAL_STAT_SHIM="$REAL_STAT_SHIM" REAL_CHMOD_SHIM="$REAL_CHMOD_SHIM" REAL_MV_SHIM="$REAL_MV_SHIM" \
    MODE_LEDGER_STUB="$MODE_LEDGER_STUB" MODE_LEDGER_HELPER="$TEST_MODE_LEDGER_HELPER" \
    MODE_STATE_PATH_STUB="$MODE_STATE_PATH_STUB" MODE_CHECKSUM_PATH_STUB="$MODE_CHECKSUM_PATH_STUB" \
    MODE_APPROVAL_PATH_STUB="${MODE_APPROVAL_PATH_STUB:-$TEST_TMP/mutation.approval}" \
    PERMISSION_OVERRIDE_STUB="${PERMISSION_OVERRIDE_STUB:-none}" \
    TARGET_IMAGE_STUB="$TEST_TARGET" CURRENT_IMAGE_STUB="$TEST_CURRENT" EXPECTED_REVISION_STUB="$TEST_REVISION" \
    CURRENT_REVISION_STUB="$TEST_CURRENT_REVISION" TARGET_CONFIG_STUB="$TEST_TARGET_CONFIG" CURRENT_CONFIG_STUB="$TEST_CURRENT_CONFIG" \
    COMPOSE_RELEASE_STUB="$TEST_ROOT/compose.release.yml" MIGRATION_VERSION=1 \
    DRY_REPORT_STUB="{\"success\":true,\"report\":{\"version\":1,\"mode\":\"dry_run\",\"status\":\"pending\",\"price\":{\"rows_total\":10,\"rows_invalid\":0},\"credit\":{\"rows_total\":2},\"blockers\":[],\"checksum\":\"$TEST_CHECKSUM\",\"read_only\":true,\"changed\":false,\"ready\":false}}" \
    APPLY_REPORT_STUB="{\"success\":true,\"report\":{\"version\":1,\"mode\":\"apply\",\"status\":\"ready\",\"price\":{\"rows_invalid\":0},\"blockers\":[],\"checksum\":\"$TEST_APPLY_CHECKSUM\",\"read_only\":false,\"changed\":true,\"ready\":true}}" \
    VERIFY_REPORT_STUB="{\"success\":true,\"report\":{\"version\":1,\"mode\":\"verify\",\"status\":\"ready\",\"price\":{\"rows_invalid\":0},\"blockers\":[],\"checksum\":\"$TEST_APPLY_CHECKSUM\",\"read_only\":true,\"changed\":false,\"ready\":true}}" \
    SUSPEND_REPORT_STUB="{\"success\":true,\"report\":{\"version\":1,\"mode\":\"suspend\",\"status\":\"suspended\",\"checksum\":\"$TEST_APPLY_CHECKSUM\",\"ready\":false}}" \
    FAIL_MODE="${FAIL_MODE:-}" PATH="$TEST_BIN:$PATH" bash "$SCRIPT" --config "$TEST_CONFIG" "$phase" >"$invocation_log" 2>&1; then
    rc=0
  else
    rc=$?
  fi
  cat "$invocation_log"
  return "$rc"
}

write_approval() {
  local path="$1" action="$2"
  cat >"$path" <<EOF
RELEASE_ID=issue28-test
EXPECTED_REVISION=$TEST_REVISION
TARGET_IMAGE=$TEST_TARGET
MIGRATION_VERSION=1
DRY_RUN_CHECKSUM=$TEST_CHECKSUM
APPLY_CHECKSUM=$TEST_APPLY_CHECKSUM
APPROVED_ACTION=$action
EOF
  "$REAL_CHMOD_SHIM" 0600 "$path"
  MODE_APPROVAL_PATH_STUB="$path"
  MODE_LEDGER_STUB="$MODE_LEDGER_STUB" REAL_MV_SHIM="$REAL_MV_SHIM" "$TEST_MODE_LEDGER_HELPER" set "$path" 600
}


test_config_rejects_missing_write_gate() {
  setup_fixture
  sed -i '/WRITE_GATE_HOOK=/d' "$TEST_CONFIG"
  set +e
  output="$(CALL_LOG="$TEST_CALLS" PATH="$TEST_BIN:$PATH" bash "$SCRIPT" --config "$TEST_CONFIG" preflight 2>&1)"; rc=$?
  set -e
  [[ "$rc" -eq 2 && "$output" == *'WRITE_GATE_HOOK must be an absolute executable path'* ]] || fail 'missing write gate was not rejected'
  [[ ! -s "$TEST_CALLS" ]] || fail 'external command ran before config validation'
  cleanup_fixture
}

test_config_rejects_shell_substitution() {
  setup_fixture
  printf '\nWRITE_GATE_HOOK=$(docker config-injection)\n' >>"$TEST_CONFIG"
  set +e
  output="$(CALL_LOG="$TEST_CALLS" PATH="$TEST_BIN:$PATH" bash "$SCRIPT" --config "$TEST_CONFIG" preflight 2>&1)"; rc=$?
  set -e
  [[ "$rc" -eq 2 && "$output" == *'config key WRITE_GATE_HOOK must not be repeated'* ]] || fail 'unsafe config was not rejected'
  [[ ! -s "$TEST_CALLS" ]] || fail 'shell syntax reached external command'
  cleanup_fixture
}

read_state_field() {
  local state_path="$1" wanted="$2" key value
  while IFS='=' read -r key value; do
    if [[ "$key" == "$wanted" ]]; then
      printf '%s' "$value"
      return 0
    fi
  done <"$state_path"
  return 1
}

prepare_maintenance() {
  run_release preflight
  write_approval "$TEST_TMP/mutation.approval" production-maintenance
  run_release stop-writes
  run_release stop-writes
  run_release backup
  run_release backup
  write_approval "$TEST_TMP/mutation.approval" production-maintenance
  run_release stage-schema
  run_release stage-schema
  run_release read-only-dry-run
  write_approval "$TEST_TMP/mutation.approval" apply-migration
}

test_full_pipeline_and_idempotence() {
  setup_fixture
  prepare_maintenance
  run_release apply
  run_release apply
  run_release verify
  run_release start-closed
  run_release start-closed
  run_release probe
  write_approval "$TEST_TMP/open.approval" open-writes
  run_release open-writes
  run_release open-writes
  run_release observe
  run_release observe
  grep -Fq 'phase=observe result=pass' "$TEST_ROOT/audits/issue28-test.log" || fail 'observe result was not persisted in the audit log'
  [[ "$(cat "$TEST_STUB_STATE.gate")" == open ]] || fail 'writes did not remain open after observe'
  [[ "$(grep -Fc 'credit-valuation-migrate --dry-run --version 1' "$TEST_CALLS")" -eq 2 ]] || fail 'dry-run was not exactly twice'
  [[ "$(grep -Fc 'pg_dump -U new_api -d new_api -Fc' "$TEST_CALLS")" -eq 1 ]] || fail 'backup was not idempotent'
  write_approval "$TEST_TMP/rollback.approval" rollback-suspend
  run_release rollback-suspend
  run_release rollback-suspend
  [[ "$(cat "$TEST_STUB_STATE.gate")" == closed ]] || fail 'suspend rollback did not keep writes closed'
  [[ "$(grep -Fc 'pg_dump -U new_api -d new_api -Fc' "$TEST_CALLS")" -eq 2 ]] || fail 'suspend rollback did not create exactly one new backup'
  grep -Fq 'credit-valuation-migrate --suspend --version 1 --reason incident-test' "$TEST_CALLS" || fail 'suspend command missing'
  backup_path="$(read_state_field "$TEST_ROOT/state/issue28-test.state" BACKUP_PATH)" || fail 'BACKUP_PATH was not persisted in state'
  grep -Fq 'marker=suspended' "$backup_path" || fail 'suspend backup was captured before suspended marker'
  suspend_report="$(read_state_field "$TEST_ROOT/state/issue28-test.state" SUSPEND_REPORT)" || fail 'SUSPEND_REPORT was not persisted in state'
  [[ -s "$suspend_report" ]] || fail 'suspend report was not persisted in state'
  grep -Fq "$TEST_CURRENT" "$TEST_ROOT/compose.release.yml" || fail 'suspend rollback did not restore current image'
  cleanup_fixture
}

test_suspend_intermediate_state_resumes_before_backup() {
  setup_fixture
  prepare_maintenance
  run_release apply
  run_release verify
  run_release start-closed
  run_release probe
  write_approval "$TEST_TMP/open.approval" open-writes
  run_release open-writes
  write_approval "$TEST_TMP/rollback.approval" rollback-suspend
  set +e
  before_dump_count="$(grep -Fc 'pg_dump -U new_api -d new_api -Fc' "$TEST_CALLS")"
  FAIL_MODE=backup_after_suspend run_release rollback-suspend >"$TEST_TMP/suspend-interrupt.log" 2>&1
  rc=$?
  set -e
  [[ "$rc" -ne 0 ]] || fail 'simulated post-suspend interruption unexpectedly succeeded'
  [[ "$(grep '^PHASE=' "$TEST_ROOT/state/issue28-test.state")" == 'PHASE=rollback-suspend-suspended' ]] || fail 'suspended intermediate phase was not persisted'
  grep -q '^SUSPEND_REPORT=' "$TEST_ROOT/state/issue28-test.state" || fail 'suspend report path was not persisted'
  after_dump_count="$(grep -Fc 'pg_dump -U new_api -d new_api -Fc' "$TEST_CALLS")"
  [[ "$after_dump_count" -eq $((before_dump_count + 1)) ]] || fail "suspend backup attempt count changed unexpectedly: before=$before_dump_count after=$after_dump_count"
  run_release rollback-suspend
  backup_path="$(read_state_field "$TEST_ROOT/state/issue28-test.state" BACKUP_PATH)" || fail 'resumed BACKUP_PATH was not persisted in state'
  grep -Fq 'marker=suspended' "$backup_path" || fail 'resumed backup did not contain suspended marker'
  cleanup_fixture
}

test_apply_failure_keeps_writes_closed() {
  setup_fixture
  prepare_maintenance
  set +e
  FAIL_MODE=apply run_release apply >"$TEST_TMP/failure.log" 2>&1; rc=$?
  set -e
  [[ "$rc" -ne 0 ]] || fail 'failed apply unexpectedly succeeded'
  [[ "$(cat "$TEST_STUB_STATE.gate")" == closed ]] || fail 'failed apply reopened writes'
  [[ "$(grep '^PHASE=' "$TEST_ROOT/state/issue28-test.state")" == 'PHASE=read-only-dry-run' ]] || fail 'failed apply advanced state'
  ! grep -q 'write-gate open' "$TEST_CALLS" || fail 'failure path opened writes'
  cleanup_fixture
}

test_stage_schema_failure_stays_closed() {
  setup_fixture
  run_release preflight
  write_approval "$TEST_TMP/mutation.approval" production-maintenance
  run_release stop-writes
  run_release backup
  set +e
  FAIL_MODE=stage run_release stage-schema >"$TEST_TMP/stage-failure.log" 2>&1; rc=$?
  set -e
  [[ "$rc" -ne 0 ]] || fail 'failed stage-schema unexpectedly succeeded'
  [[ "$(cat "$TEST_STUB_STATE.gate")" == closed ]] || fail 'failed stage-schema did not stay closed'
  [[ "$(grep '^PHASE=' "$TEST_ROOT/state/issue28-test.state")" == 'PHASE=stage-schema-failed' ]] || fail 'failed stage-schema did not persist recovery state'
  cleanup_fixture
}

test_before_ready_rollback() {
  setup_fixture
  run_release preflight
  write_approval "$TEST_TMP/mutation.approval" production-maintenance
  run_release stop-writes
  run_release backup
  write_approval "$TEST_TMP/mutation.approval" production-maintenance
  run_release stage-schema
  run_release read-only-dry-run
  write_approval "$TEST_TMP/rollback.approval" rollback-before-ready
  run_release rollback-before-ready
  run_release rollback-before-ready
  [[ "$(cat "$TEST_STUB_STATE.gate")" == closed ]] || fail 'before-ready rollback reopened writes'
  grep -Fq "$TEST_CURRENT" "$TEST_ROOT/compose.release.yml" || fail 'before-ready rollback did not restore current image'
  cleanup_fixture
}

test_backup_can_rollback_before_target_start() {
  setup_fixture
  run_release preflight
  write_approval "$TEST_TMP/mutation.approval" production-maintenance
  run_release stop-writes
  run_release backup
  write_approval "$TEST_TMP/rollback.approval" rollback-before-ready
  run_release rollback-before-ready
  [[ "$(cat "$TEST_STUB_STATE.gate")" == closed ]] || fail 'backup rollback reopened writes'
  grep -Fq "$TEST_CURRENT" "$TEST_ROOT/compose.release.yml" || fail 'backup rollback did not retain current image'
  cleanup_fixture
}

test_ready_before_open_rollback() {
  setup_fixture
  prepare_maintenance
  run_release apply
  run_release verify
  run_release start-closed
  write_approval "$TEST_TMP/rollback.approval" rollback-ready-before-open
  run_release rollback-ready-before-open
  [[ "$(cat "$TEST_STUB_STATE.gate")" == closed ]] || fail 'ready-before-open rollback reopened writes'
  grep -Fq "$TEST_CURRENT" "$TEST_ROOT/compose.release.yml" || fail 'ready-before-open rollback did not restore current image'
  cleanup_fixture
}


run_permission_override_case() {
  local override="$1" expected="$2" output rc failure_log
  setup_fixture
  PERMISSION_OVERRIDE_STUB="$override"
  run_release preflight
  write_approval "$TEST_TMP/mutation.approval" production-maintenance
  failure_log="$TEST_TMP/permission-failure.log"
  set +e
  run_release stop-writes >"$failure_log" 2>&1
  rc=$?
  set -e
  output="$(<"$failure_log")"
  [[ "$rc" -eq 2 ]] || fail "permission override $override returned rc=$rc output=$output"
  [[ "$output" == *"$expected"* ]] || fail "permission override $override output mismatch: $output"
  [[ "$(cat "$TEST_STUB_STATE.gate" 2>/dev/null || printf open)" == open ]] || fail "permission override $override changed the write gate"
  printf 'PASS: permission override %s rejected\n' "$override"
  cleanup_fixture
}

test_permission_contract_with_mock_stat() {
  setup_fixture
  run_release preflight
  write_approval "$TEST_TMP/mutation.approval" production-maintenance
  run_release stop-writes
  [[ "$(grep '^PHASE=' "$TEST_ROOT/state/issue28-test.state")" == 'PHASE=stop-writes' ]] || fail 'simulated mode 600 did not allow guarded transition'
  printf 'PASS: simulated mode 600 accepted\n'
  cleanup_fixture

  run_permission_override_case state 'release state permissions are unsafe'
  run_permission_override_case checksum 'release state checksum permissions are unsafe'
  run_permission_override_case approval 'MUTATION_APPROVAL_FILE permissions must be 0600'
}

test_real_posix_permission_contract() {
  local root name path mode filesystem
  if [[ "$POSIX_MODE_SEMANTICS_AVAILABLE" != true ]]; then
    printf 'SKIP: POSIX mode semantics unavailable\n'
    return 0
  fi

  root="$(mktemp -d)"
  for name in state state.sha256 approval; do
    path="$root/$name"
    : >"$path"
    chmod 0600 "$path"
    mode="$(stat -c '%a' "$path")"
    [[ "$mode" == 600 ]] || fail "real POSIX mode check failed for $name: $mode"
  done
  mv -f "$root/state" "$root/state.final"
  mode="$(stat -c '%a' "$root/state.final")"
  [[ "$mode" == 600 ]] || fail "real POSIX mode was not preserved across atomic move: $mode"
  filesystem="$(stat -f -c '%T' "$root")"
  rm -rf "$root"
  printf 'PASS: real POSIX permission contract filesystem=%s\n' "$filesystem"
}

if [[ "${TEST_FILTER:-all}" == permission ]]; then
  test_permission_contract_with_mock_stat
  test_real_posix_permission_contract
  printf 'PASS: permission contract only\n'
  exit 0
fi

if [[ "${TEST_FILTER:-all}" == full ]]; then
  test_full_pipeline_and_idempotence
  printf 'PASS: full pipeline only\n'
  exit 0
fi

test_config_rejects_missing_write_gate
test_config_rejects_shell_substitution
test_full_pipeline_and_idempotence
test_suspend_intermediate_state_resumes_before_backup
test_apply_failure_keeps_writes_closed
test_stage_schema_failure_stays_closed
test_backup_can_rollback_before_target_start
test_before_ready_rollback
test_ready_before_open_rollback
test_permission_contract_with_mock_stat
test_real_posix_permission_contract
printf 'PASS: server release shell contract\n'
