#!/usr/bin/env bash
set -Eeuo pipefail

usage() {
  echo 'usage: server-release.sh --config FILE PHASE' >&2
  echo 'phases: preflight stage-runtime read-only-dry-run stop-writes frozen-dry-run backup stage-schema apply verify start-closed probe open-writes observe rollback-before-ready rollback-ready-before-open rollback-suspend' >&2
  exit 2
}

contract_error() {
  echo "contract error: $*" >&2
  exit 2
}

require_value() {
  local name="$1"
  [[ -n "${!name:-}" ]] || contract_error "$name is required"
}

require_absolute_file() {
  local name="$1" value="${!1:-}"
  [[ "$value" == /* ]] || contract_error "$name must be an absolute path"
  [[ -f "$value" ]] || contract_error "$name must name an existing file"
}

require_absolute_directory() {
  local name="$1" value="${!1:-}"
  [[ "$value" == /* ]] || contract_error "$name must be an absolute path"
  [[ -d "$value" ]] || contract_error "$name must name an existing directory"
}

require_absolute_executable() {
  local name="$1" value="${!1:-}"
  [[ "$value" == /* && -f "$value" && -x "$value" ]] || contract_error "$name must be an absolute executable path"
}

is_allowed_config_key() {
  case "$1" in
    RELEASE_ID|EXPECTED_REVISION|TARGET_IMAGE|CURRENT_IMAGE|MIGRATION_VERSION|MAINTENANCE_MODE|ROOT|COMPOSE_BASE|COMPOSE_NETWORK|COMPOSE_PRIMARY|COMPOSE_RELEASE|APP_ENV_FILE|DOCKER_NETWORK|WRITE_GATE_HOOK|WRITE_GATE_CONFIG|READ_ONLY_PROBE_HOOK|PRODUCTION_PROBE_CONFIG|CLONE_PROBE_HOOK|CLONE_PROBE_CONFIG|OBSERVE_HOOK|OBSERVE_CONFIG|BROWSER_EVIDENCE_FILE|AUDIT_DIR|BACKUP_DIR|STATE_DIR|LOCK_FILE|POSTGRES_CONTAINER|POSTGRES_USER|POSTGRES_DB|BATCH_SIZE|HEALTH_TIMEOUT_SECONDS|OBSERVE_SECONDS|MUTATION_APPROVAL_FILE|OPEN_WRITES_APPROVAL_FILE|ROLLBACK_APPROVAL_FILE|SUSPEND_REASON)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

parse_config() {
  local file="$1" line key value line_number=0
  declare -A seen=()
  while IFS= read -r line || [[ -n "$line" ]]; do
    line_number=$((line_number + 1))
    [[ -z "$line" || "$line" == \#* ]] && continue
    [[ "$line" == *=* ]] || contract_error "config line $line_number must be KEY=VALUE"
    key="${line%%=*}"
    value="${line#*=}"
    [[ "$key" =~ ^[A-Z][A-Z0-9_]*$ ]] || contract_error "config line $line_number has an invalid key"
    is_allowed_config_key "$key" || contract_error "config line $line_number uses unknown key $key"
    [[ -z "${seen[$key]:-}" ]] || contract_error "config key $key must not be repeated"
    [[ -n "$value" ]] || contract_error "config value for $key must not be empty"
    [[ "$value" =~ ^[A-Za-z0-9_./:@,+-]+$ ]] || contract_error "config value contains forbidden shell syntax at line $line_number"
    seen[$key]=1
    printf -v "$key" '%s' "$value"
  done < "$file"
}

validate_config() {
  local image_pattern='^ghcr\.io/jiwangyihao/new-api@sha256:[0-9a-f]{64}$'
  local revision_pattern='^[0-9a-f]{40}$'

  require_value RELEASE_ID
  require_value EXPECTED_REVISION
  require_value TARGET_IMAGE
  require_value CURRENT_IMAGE
  require_value MIGRATION_VERSION
  require_value MAINTENANCE_MODE
  require_value DOCKER_NETWORK
  [[ "$MAINTENANCE_MODE" == true ]] || contract_error 'MAINTENANCE_MODE must be true for the release state machine'
  require_absolute_directory ROOT
  require_absolute_file COMPOSE_BASE
  require_absolute_file COMPOSE_NETWORK
  require_absolute_file COMPOSE_PRIMARY
  require_absolute_file COMPOSE_RELEASE
  require_absolute_file APP_ENV_FILE
  require_absolute_file WRITE_GATE_CONFIG
  require_absolute_file PRODUCTION_PROBE_CONFIG
  require_absolute_file CLONE_PROBE_CONFIG
  require_absolute_file OBSERVE_CONFIG
  require_absolute_executable WRITE_GATE_HOOK
  require_value BROWSER_EVIDENCE_FILE
  [[ "$BROWSER_EVIDENCE_FILE" == /* ]] || contract_error 'BROWSER_EVIDENCE_FILE must be an absolute path'

  [[ "$RELEASE_ID" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,80}$ && "$RELEASE_ID" != *..* ]] || contract_error 'RELEASE_ID is invalid'
  [[ "$EXPECTED_REVISION" =~ $revision_pattern ]] || contract_error 'EXPECTED_REVISION must be a full lowercase commit SHA'
  [[ "$TARGET_IMAGE" =~ $image_pattern ]] || contract_error 'TARGET_IMAGE must be an immutable jiwangyihao/new-api digest'
  [[ "$CURRENT_IMAGE" =~ $image_pattern ]] || contract_error 'CURRENT_IMAGE must be an immutable jiwangyihao/new-api digest'
  [[ "$TARGET_IMAGE" != "$CURRENT_IMAGE" ]] || contract_error 'TARGET_IMAGE and CURRENT_IMAGE must differ'
  [[ "$MIGRATION_VERSION" =~ ^[1-9][0-9]*$ ]] || contract_error 'MIGRATION_VERSION must be a positive integer'
  [[ "$DOCKER_NETWORK" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || contract_error 'DOCKER_NETWORK is invalid'

  AUDIT_DIR="${AUDIT_DIR:-$ROOT/audits}"
  BACKUP_DIR="${BACKUP_DIR:-$ROOT/backups}"
  STATE_DIR="${STATE_DIR:-$ROOT/release-state}"
  LOCK_FILE="${LOCK_FILE:-/run/lock/new-api-credit-valuation-release.lock}"
  POSTGRES_CONTAINER="${POSTGRES_CONTAINER:-new-api-postgres}"
  POSTGRES_USER="${POSTGRES_USER:-new_api}"
  POSTGRES_DB="${POSTGRES_DB:-new_api}"
  BATCH_SIZE="${BATCH_SIZE:-100}"
  HEALTH_TIMEOUT_SECONDS="${HEALTH_TIMEOUT_SECONDS:-180}"
  OBSERVE_SECONDS="${OBSERVE_SECONDS:-900}"

  [[ "$AUDIT_DIR" == /* ]] || contract_error 'AUDIT_DIR must be an absolute path'
  [[ "$BACKUP_DIR" == /* ]] || contract_error 'BACKUP_DIR must be an absolute path'
  [[ "$STATE_DIR" == /* ]] || contract_error 'STATE_DIR must be an absolute path'
  [[ "$LOCK_FILE" == /* ]] || contract_error 'LOCK_FILE must be an absolute path'
  [[ "$POSTGRES_CONTAINER" =~ ^[A-Za-z0-9][A-Za-z0-9_.-]*$ ]] || contract_error 'POSTGRES_CONTAINER is invalid'
  [[ "$POSTGRES_USER" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || contract_error 'POSTGRES_USER is invalid'
  [[ "$POSTGRES_DB" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || contract_error 'POSTGRES_DB is invalid'
  [[ "$BATCH_SIZE" == 100 ]] || contract_error 'BATCH_SIZE must remain 100 to match dry-run and verify checksum batches'
  [[ "$HEALTH_TIMEOUT_SECONDS" =~ ^[1-9][0-9]*$ ]] || contract_error 'HEALTH_TIMEOUT_SECONDS must be a positive integer'
  [[ "$OBSERVE_SECONDS" =~ ^[1-9][0-9]*$ ]] || contract_error 'OBSERVE_SECONDS must be a positive integer'

  local optional_hook
  for config_file in WRITE_GATE_CONFIG PRODUCTION_PROBE_CONFIG CLONE_PROBE_CONFIG OBSERVE_CONFIG; do
    [[ "$(stat -c '%a' "${!config_file}")" == 600 ]] || contract_error "$config_file permissions must be 0600"
  done
  for optional_hook in READ_ONLY_PROBE_HOOK CLONE_PROBE_HOOK OBSERVE_HOOK; do
    if [[ -n "${!optional_hook:-}" ]]; then
      require_absolute_executable "$optional_hook"
    fi
  done
  # Browser evidence is produced after deployment and validated in probe.
}
write_gate_call() {
  NEW_API_WRITE_GATE_CONFIG="$WRITE_GATE_CONFIG" "$WRITE_GATE_HOOK" "$@"
}
production_probe_call() {
  PRODUCTION_PROBE_CONFIG="$PRODUCTION_PROBE_CONFIG" "$READ_ONLY_PROBE_HOOK" "$@"
}
clone_probe_call() {
  CLONE_PROBE_CONFIG="$CLONE_PROBE_CONFIG" "$CLONE_PROBE_HOOK" "$@"
}
observe_call() {
  OBSERVE_CONFIG="$OBSERVE_CONFIG" "$OBSERVE_HOOK" "$@"
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || contract_error "required command is unavailable: $1"
}
AUDIT_ORIGINAL_STDOUT_FD=''
AUDIT_PHASE_LOG=''

start_audit_logging() {
  exec {AUDIT_ORIGINAL_STDOUT_FD}>&1
  AUDIT_PHASE_LOG="$AUDIT_DIR/${RELEASE_ID}.${PHASE}.$$.phase.log"
  : > "$AUDIT_PHASE_LOG"
  chmod 0600 "$AUDIT_PHASE_LOG"
  exec 1>"$AUDIT_PHASE_LOG" 2>&1
}

finish_audit_logging() {
  local append_rc=0 replay_rc=0
  [[ -n "${AUDIT_ORIGINAL_STDOUT_FD:-}" ]] || return 0
  exec 1>&"$AUDIT_ORIGINAL_STDOUT_FD" 2>&1
  cat "$AUDIT_PHASE_LOG" >> "$AUDIT_LOG" || append_rc=$?
  chmod 0600 "$AUDIT_LOG" || append_rc=$?
  cat "$AUDIT_PHASE_LOG" >&"$AUDIT_ORIGINAL_STDOUT_FD" || replay_rc=$?
  rm -f "$AUDIT_PHASE_LOG"
  AUDIT_PHASE_LOG=''
  exec {AUDIT_ORIGINAL_STDOUT_FD}>&-
  AUDIT_ORIGINAL_STDOUT_FD=''
  (( append_rc == 0 )) || return "$append_rc"
  return "$replay_rc"
}


init_runtime() {
  require_command docker
  require_command flock
  require_command jq
  require_command sha256sum
  require_command stat
  require_command cmp
  require_command cat

  mkdir -p "$AUDIT_DIR" "$BACKUP_DIR" "$STATE_DIR"
  exec 9>"$LOCK_FILE"
  flock -n 9 || contract_error 'another Credit valuation release holds the lock'
  AUDIT_LOG="$AUDIT_DIR/${RELEASE_ID}.log"
  STATE_FILE="$STATE_DIR/${RELEASE_ID}.state"
  STATE_CHECKSUM_FILE="$STATE_FILE.sha256"
  COMPOSE=(
    docker compose --project-name new-api
    -f "$COMPOSE_BASE"
    -f "$COMPOSE_NETWORK"
    -f "$COMPOSE_PRIMARY"
    -f "$COMPOSE_RELEASE"
  )
  start_audit_logging
}

cleanup() {
  local rc=$? audit_rc=0 status current_phase=''
  trap - EXIT HUP INT TERM
  set +e
  [[ -n "${STATE_FILE:-}" ]] && rm -f "${STATE_FILE}.tmp.$$" "${STATE_CHECKSUM_FILE:-}.tmp.$$"
  [[ -n "${COMPOSE_RELEASE:-}" ]] && rm -f "${COMPOSE_RELEASE}.tmp.$$"
  if (( rc != 0 )); then
    if [[ -n "${STATE_FILE:-}" && -f "$STATE_FILE" ]]; then
      current_phase="$(state_value PHASE 2>/dev/null || true)"
    fi
    case "$PHASE:$current_phase" in
      stage-runtime:stage-runtime-starting|stage-runtime:stage-runtime-failed)
        status="$(write_gate_call close "$RELEASE_ID-failure" 2>/dev/null)"
        if jq -e '.success == true and .state == "closed"' <<<"$status" >/dev/null 2>&1; then
          STATE_WRITE_GATE_STATE=closed
          write_state stage-runtime-failed
          echo "failure safeguard closed writes; target runtime staging remains recoverable" >&2
        else
          echo "failure safeguard could not prove writes closed; manual intervention required" >&2
        fi
        ;;
      stage-schema:stage-schema-starting|open-writes:probe|observe:open-writes|observe:observe)
        status="$(write_gate_call close "$RELEASE_ID-failure" 2>/dev/null)"
        if jq -e '.success == true and .state == "closed"' <<<"$status" >/dev/null 2>&1; then
          STATE_WRITE_GATE_STATE=closed
          write_state "${PHASE}-failed"
          echo "failure safeguard closed writes" >&2
        else
          echo "failure safeguard could not prove writes closed; manual intervention required" >&2
        fi
        ;;
      rollback-suspend:open-writes|rollback-suspend:observe|rollback-suspend:rollback-suspend-suspended|rollback-suspend:rollback-suspend-backup)
        status="$(write_gate_call close "$RELEASE_ID-failure" 2>/dev/null)"
        if jq -e '.success == true and .state == "closed"' <<<"$status" >/dev/null 2>&1; then
          echo "failure safeguard closed writes; rollback intermediate state preserved" >&2
        else
          echo "failure safeguard could not prove writes closed; manual intervention required" >&2
        fi
        ;;
    esac
    echo "release=$RELEASE_ID phase=$PHASE failed_at=$(date -u +%Y-%m-%dT%H:%M:%SZ) writes_are_not_automatically_opened=true" >&2
  fi
  finish_audit_logging
  audit_rc=$?
  if (( audit_rc != 0 )); then
    echo "release=$RELEASE_ID phase=$PHASE audit_tee_failed=true audit_rc=$audit_rc" >&2
    (( rc != 0 )) || rc=$audit_rc
  fi
  exit "$rc"
}
release_override_image() {
  awk '
    function indent_of(value, spaces) {
      spaces = value
      sub(/[^[:space:]].*$/, "", spaces)
      return length(spaces)
    }
    BEGIN { in_services = 0; in_new_api = 0; image = ""; count = 0 }
    {
      line = $0
      indent = indent_of(line)
      if (line ~ /^[[:space:]]*services:[[:space:]]*$/) {
        in_services = 1
        services_indent = indent
        next
      }
      if (in_services && line !~ /^[[:space:]]*$/ && indent <= services_indent) {
        in_services = 0
        in_new_api = 0
      }
      if (in_services && line ~ /^[[:space:]]*new-api:[[:space:]]*$/ && indent > services_indent) {
        in_new_api = 1
        service_indent = indent
        next
      }
      if (in_new_api && line !~ /^[[:space:]]*$/ && indent <= service_indent) {
        in_new_api = 0
      }
      if (in_new_api && line ~ /^[[:space:]]*image:[[:space:]]*/ && indent > service_indent) {
        sub(/^[[:space:]]*image:[[:space:]]*/, "", line)
        image = line
        count++
      }
    }
    END { if (count != 1) exit 1; print image }
  ' "$COMPOSE_RELEASE"
}


write_release_override() {
  local image="$1" maintenance_mode="${2:-false}" temporary="${COMPOSE_RELEASE}.tmp.$$" mode
  [[ "$maintenance_mode" == true || "$maintenance_mode" == false ]] || contract_error 'write_release_override maintenance mode must be true or false'
  mode="$(stat -c '%a' "$COMPOSE_RELEASE")"
  awk -v image="$image" -v maintenance_mode="$maintenance_mode" '
    function indent_of(value, spaces) {
      spaces = value
      sub(/[^[:space:]].*$/, "", spaces)
      return length(spaces)
    }
    function padding(count) {
      return sprintf("%*s", count, "")
    }
    function append_environment_entry() {
      if (maintenance_mode != "true" || maintenance_present) {
        return
      }
      if (environment_style == "list") {
        print padding(environment_indent + 2) "- MAINTENANCE_MODE=true"
      } else {
        print padding(environment_indent + 2) "MAINTENANCE_MODE: \"true\""
      }
      maintenance_present = 1
    }
    function finish_environment() {
      if (!in_environment) {
        return
      }
      append_environment_entry()
      in_environment = 0
    }
    function finish_service() {
      if (!in_new_api) {
        return
      }
      if (maintenance_mode == "true" && !environment_seen) {
        print padding(service_indent + 2) "environment:"
        print padding(service_indent + 4) "MAINTENANCE_MODE: \"true\""
        environment_seen = 1
        maintenance_present = 1
      }
    }
    BEGIN {
      in_services = 0
      in_new_api = 0
      in_environment = 0
      services_indent = -1
      service_indent = -1
      environment_indent = -1
      environment_seen = 0
      environment_style = ""
      maintenance_present = 0
      image_count = 0
    }
    {
      line = $0
      indent = indent_of(line)

      if (in_environment && line !~ /^[[:space:]]*$/ && indent <= environment_indent) {
        finish_environment()
      }
      if (in_new_api && line !~ /^[[:space:]]*$/ && indent <= service_indent) {
        finish_service()
        in_new_api = 0
      }
      if (in_services && line !~ /^[[:space:]]*$/ && indent <= services_indent && line !~ /^[[:space:]]*services:[[:space:]]*$/) {
        in_services = 0
      }

      if (line ~ /^[[:space:]]*services:[[:space:]]*$/) {
        in_services = 1
        services_indent = indent
        print line
        next
      }
      if (in_services && line ~ /^[[:space:]]*new-api:[[:space:]]*$/ && indent > services_indent) {
        in_new_api = 1
        service_indent = indent
        environment_seen = 0
        environment_style = ""
        maintenance_present = 0
        print line
        next
      }
      if (in_new_api && line ~ /^[[:space:]]*image:[[:space:]]*.*$/ && indent > service_indent) {
        sub(/image:[[:space:]]*.*/, "image: " image, line)
        image_count++
        print line
        next
      }
      if (in_new_api && line ~ /^[[:space:]]*environment:[[:space:]]*$/ && indent > service_indent) {
        in_environment = 1
        environment_seen = 1
        environment_indent = indent
        environment_style = ""
        maintenance_present = 0
        print line
        next
      }
      if (in_environment && indent > environment_indent) {
        if (environment_style == "" && line ~ /^[[:space:]]*-[[:space:]]*/) {
          environment_style = "list"
        }
        if (line ~ /^[[:space:]]*MAINTENANCE_MODE[[:space:]]*:/ || line ~ /^[[:space:]]*-[[:space:]]*MAINTENANCE_MODE([=:]|$)/) {
          next
        }
        print line
        next
      }
      print line
    }
    END {
      finish_environment()
      finish_service()
      if (image_count != 1 || (maintenance_mode == "true" && !environment_seen)) {
        exit 2
      }
    }
  ' "$COMPOSE_RELEASE" > "$temporary" || {
    rm -f "$temporary"
    contract_error 'COMPOSE_RELEASE must contain exactly one new-api image and the required maintenance environment'
  }
  chmod "$mode" "$temporary"
  mv -f "$temporary" "$COMPOSE_RELEASE"
}


restore_release_override() {
  local temporary="${COMPOSE_RELEASE}.tmp.$$" mode
  if [[ -z "$STATE_RELEASE_OVERRIDE_BACKUP" ]]; then
    [[ "$(release_override_image)" == "$CURRENT_IMAGE" ]] || contract_error 'current compose overlay is not pinned to CURRENT_IMAGE'
    return 0
  fi
  [[ -f "$STATE_RELEASE_OVERRIDE_BACKUP" ]] || contract_error 'saved Compose release overlay is missing'
  mode="${STATE_RELEASE_OVERRIDE_MODE:-}"
  [[ "$mode" =~ ^[0-7]{3,4}$ ]] || contract_error 'saved Compose release overlay mode is invalid'
  cat "$STATE_RELEASE_OVERRIDE_BACKUP" > "$temporary"
  chmod "$mode" "$temporary"
  mv -f "$temporary" "$COMPOSE_RELEASE"
  cmp -s "$STATE_RELEASE_OVERRIDE_BACKUP" "$COMPOSE_RELEASE" || contract_error 'restored Compose release overlay content mismatch'
  [[ "$(release_override_image)" == "$CURRENT_IMAGE" ]] || contract_error 'restored Compose release overlay is not pinned to CURRENT_IMAGE'
  [[ "$(stat -c '%a' "$COMPOSE_RELEASE")" == "$mode" ]] || contract_error 'restored Compose release overlay mode mismatch'
}

STATE_TARGET_CONFIG_ID=''
STATE_CURRENT_CONFIG_ID=''
STATE_WRITE_GATE_STATE=''
STATE_BOOTSTRAP_REPORT=''
STATE_RUNTIME_STARTED_AT=''
STATE_ONLINE_DRY_RUN_REPORT_1=''
STATE_ONLINE_DRY_RUN_REPORT_2=''
STATE_DRY_RUN_CHECKSUM=''
STATE_DRY_RUN_REPORT_1=''
STATE_DRY_RUN_REPORT_2=''
STATE_BACKUP_PATH=''
STATE_BACKUP_CHECKSUM=''
STATE_BACKUP_SIZE=''
STATE_BACKUP_UTC=''
STATE_APPLY_REPORT=''
STATE_APPLY_CHECKSUM=''
STATE_VERIFY_REPORT=''
STATE_RELEASE_OVERRIDE_BACKUP=''
STATE_RELEASE_OVERRIDE_MODE=''
STATE_PRODUCTION_PROBE_REPORT=''
STATE_CLONE_PROBE_REPORT=''
STATE_DUAL_WRITE_OPENED_AT=''
STATE_OBSERVE_REPORT=''
STATE_ROLLBACK_REASON=''
STATE_SUSPEND_REPORT=''
declare -A APPROVAL_VALUES=()
state_value() {
  local key="$1" line found=''
  [[ -f "$STATE_FILE" ]] || return 1
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ "$line" == "$key="* ]]; then
      [[ -z "$found" ]] || contract_error "state contains duplicate key $key"
      found="${line#*=}"
    fi
  done < "$STATE_FILE"
  [[ -n "$found" ]] || return 1
  printf '%s\n' "$found"
}

verify_state_integrity() {
  local expected actual
  [[ -f "$STATE_FILE" && -f "$STATE_CHECKSUM_FILE" ]] || contract_error 'release state or checksum is missing'
  [[ "$(stat -c '%a' "$STATE_FILE")" == 600 ]] || contract_error 'release state permissions are unsafe'
  [[ "$(stat -c '%a' "$STATE_CHECKSUM_FILE")" == 600 ]] || contract_error 'release state checksum permissions are unsafe'
  read -r expected < "$STATE_CHECKSUM_FILE" || contract_error 'release state checksum is unreadable'
  [[ "$expected" =~ ^[0-9a-f]{64}$ ]] || contract_error 'release state checksum format is invalid'
  read -r actual _ < <(sha256sum "$STATE_FILE")
  [[ "$actual" == "$expected" ]] || contract_error 'release state checksum mismatch'
}

load_optional_state() {
  local key="$1" variable="$2" value=''
  if value="$(state_value "$key" 2>/dev/null)"; then
    printf -v "$variable" '%s' "$value"
  fi
}

load_state_context() {
  load_optional_state TARGET_CONFIG_ID STATE_TARGET_CONFIG_ID
  load_optional_state CURRENT_CONFIG_ID STATE_CURRENT_CONFIG_ID
  load_optional_state WRITE_GATE_STATE STATE_WRITE_GATE_STATE
  load_optional_state BOOTSTRAP_REPORT STATE_BOOTSTRAP_REPORT
  load_optional_state RUNTIME_STARTED_AT STATE_RUNTIME_STARTED_AT
  load_optional_state ONLINE_DRY_RUN_REPORT_1 STATE_ONLINE_DRY_RUN_REPORT_1
  load_optional_state ONLINE_DRY_RUN_REPORT_2 STATE_ONLINE_DRY_RUN_REPORT_2
  load_optional_state DRY_RUN_CHECKSUM STATE_DRY_RUN_CHECKSUM
  load_optional_state DRY_RUN_REPORT_1 STATE_DRY_RUN_REPORT_1
  load_optional_state DRY_RUN_REPORT_2 STATE_DRY_RUN_REPORT_2
  load_optional_state BACKUP_PATH STATE_BACKUP_PATH
  load_optional_state BACKUP_CHECKSUM STATE_BACKUP_CHECKSUM
  load_optional_state BACKUP_SIZE STATE_BACKUP_SIZE
  load_optional_state BACKUP_UTC STATE_BACKUP_UTC
  load_optional_state APPLY_REPORT STATE_APPLY_REPORT
  load_optional_state APPLY_CHECKSUM STATE_APPLY_CHECKSUM
  load_optional_state VERIFY_REPORT STATE_VERIFY_REPORT
  load_optional_state RELEASE_OVERRIDE_BACKUP STATE_RELEASE_OVERRIDE_BACKUP
  load_optional_state RELEASE_OVERRIDE_MODE STATE_RELEASE_OVERRIDE_MODE
  load_optional_state PRODUCTION_PROBE_REPORT STATE_PRODUCTION_PROBE_REPORT
  load_optional_state CLONE_PROBE_REPORT STATE_CLONE_PROBE_REPORT
  load_optional_state DUAL_WRITE_OPENED_AT STATE_DUAL_WRITE_OPENED_AT
  load_optional_state OBSERVE_REPORT STATE_OBSERVE_REPORT
  load_optional_state ROLLBACK_REASON STATE_ROLLBACK_REASON
  load_optional_state SUSPEND_REPORT STATE_SUSPEND_REPORT
}

require_state_field() {
  local key value
  for key in "$@"; do
    value="$(state_value "$key")" || contract_error "state field $key is required"
    [[ -n "$value" ]] || contract_error "state field $key is empty"
  done
}

require_state_evidence() {
  local phase="$1"
  case "$phase" in
    preflight)
      require_state_field TARGET_CONFIG_ID CURRENT_CONFIG_ID WRITE_GATE_STATE BOOTSTRAP_REPORT
      ;;
    stage-runtime-starting|stage-runtime|stage-runtime-failed)
      require_state_field TARGET_CONFIG_ID CURRENT_CONFIG_ID WRITE_GATE_STATE BOOTSTRAP_REPORT RELEASE_OVERRIDE_BACKUP RELEASE_OVERRIDE_MODE
      ;;
    read-only-dry-run)
      require_state_field TARGET_CONFIG_ID CURRENT_CONFIG_ID WRITE_GATE_STATE BOOTSTRAP_REPORT RUNTIME_STARTED_AT ONLINE_DRY_RUN_REPORT_1 ONLINE_DRY_RUN_REPORT_2 RELEASE_OVERRIDE_BACKUP RELEASE_OVERRIDE_MODE
      ;;
    stop-writes)
      require_state_field TARGET_CONFIG_ID CURRENT_CONFIG_ID WRITE_GATE_STATE BOOTSTRAP_REPORT RUNTIME_STARTED_AT ONLINE_DRY_RUN_REPORT_1 ONLINE_DRY_RUN_REPORT_2 RELEASE_OVERRIDE_BACKUP RELEASE_OVERRIDE_MODE
      ;;
    frozen-dry-run)
      require_state_field TARGET_CONFIG_ID CURRENT_CONFIG_ID WRITE_GATE_STATE BOOTSTRAP_REPORT RUNTIME_STARTED_AT ONLINE_DRY_RUN_REPORT_1 ONLINE_DRY_RUN_REPORT_2 DRY_RUN_CHECKSUM DRY_RUN_REPORT_1 DRY_RUN_REPORT_2 RELEASE_OVERRIDE_BACKUP RELEASE_OVERRIDE_MODE
      ;;
    backup)
      require_state_field TARGET_CONFIG_ID CURRENT_CONFIG_ID WRITE_GATE_STATE BOOTSTRAP_REPORT RUNTIME_STARTED_AT ONLINE_DRY_RUN_REPORT_1 ONLINE_DRY_RUN_REPORT_2 DRY_RUN_CHECKSUM DRY_RUN_REPORT_1 DRY_RUN_REPORT_2 BACKUP_PATH BACKUP_CHECKSUM BACKUP_SIZE RELEASE_OVERRIDE_BACKUP RELEASE_OVERRIDE_MODE
      ;;
    stage-schema-starting|stage-schema)
      require_state_field TARGET_CONFIG_ID CURRENT_CONFIG_ID WRITE_GATE_STATE BOOTSTRAP_REPORT RUNTIME_STARTED_AT ONLINE_DRY_RUN_REPORT_1 ONLINE_DRY_RUN_REPORT_2 DRY_RUN_CHECKSUM DRY_RUN_REPORT_1 DRY_RUN_REPORT_2 BACKUP_PATH BACKUP_CHECKSUM BACKUP_SIZE RELEASE_OVERRIDE_BACKUP RELEASE_OVERRIDE_MODE
      ;;
    apply)
      require_state_field TARGET_CONFIG_ID CURRENT_CONFIG_ID WRITE_GATE_STATE BOOTSTRAP_REPORT RUNTIME_STARTED_AT ONLINE_DRY_RUN_REPORT_1 ONLINE_DRY_RUN_REPORT_2 DRY_RUN_CHECKSUM DRY_RUN_REPORT_1 DRY_RUN_REPORT_2 BACKUP_PATH BACKUP_CHECKSUM BACKUP_SIZE APPLY_REPORT APPLY_CHECKSUM RELEASE_OVERRIDE_BACKUP RELEASE_OVERRIDE_MODE
      ;;
    verify|start-closed)
      require_state_field TARGET_CONFIG_ID CURRENT_CONFIG_ID WRITE_GATE_STATE BOOTSTRAP_REPORT RUNTIME_STARTED_AT ONLINE_DRY_RUN_REPORT_1 ONLINE_DRY_RUN_REPORT_2 BACKUP_PATH BACKUP_CHECKSUM BACKUP_SIZE DRY_RUN_CHECKSUM APPLY_REPORT APPLY_CHECKSUM VERIFY_REPORT RELEASE_OVERRIDE_BACKUP RELEASE_OVERRIDE_MODE
      ;;
    probe)
      require_state_field TARGET_CONFIG_ID CURRENT_CONFIG_ID WRITE_GATE_STATE BOOTSTRAP_REPORT RUNTIME_STARTED_AT ONLINE_DRY_RUN_REPORT_1 ONLINE_DRY_RUN_REPORT_2 BACKUP_PATH BACKUP_CHECKSUM BACKUP_SIZE DRY_RUN_CHECKSUM APPLY_CHECKSUM VERIFY_REPORT RELEASE_OVERRIDE_BACKUP RELEASE_OVERRIDE_MODE PRODUCTION_PROBE_REPORT CLONE_PROBE_REPORT
      ;;
    open-writes|observe)
      require_state_field TARGET_CONFIG_ID CURRENT_CONFIG_ID WRITE_GATE_STATE BOOTSTRAP_REPORT RUNTIME_STARTED_AT ONLINE_DRY_RUN_REPORT_1 ONLINE_DRY_RUN_REPORT_2 DRY_RUN_CHECKSUM APPLY_CHECKSUM PRODUCTION_PROBE_REPORT CLONE_PROBE_REPORT
      ;;
    rollback-suspend-suspended)
      require_state_field TARGET_CONFIG_ID CURRENT_CONFIG_ID DRY_RUN_CHECKSUM APPLY_CHECKSUM SUSPEND_REPORT RELEASE_OVERRIDE_BACKUP RELEASE_OVERRIDE_MODE ROLLBACK_REASON
      ;;
    rollback-suspend-backup|rollback-suspend)
      require_state_field TARGET_CONFIG_ID CURRENT_CONFIG_ID DRY_RUN_CHECKSUM APPLY_CHECKSUM SUSPEND_REPORT BACKUP_PATH BACKUP_CHECKSUM BACKUP_SIZE RELEASE_OVERRIDE_BACKUP RELEASE_OVERRIDE_MODE ROLLBACK_REASON
      ;;
  esac
}


write_optional_state() {
  local key="$1" value="$2"
  if [[ -n "$value" ]]; then
    printf '%s=%s\n' "$key" "$value"
  fi
  return 0
}

write_state() {
  local phase="$1" temporary="${STATE_FILE}.tmp.$$" checksum_temporary="${STATE_CHECKSUM_FILE}.tmp.$$" checksum
  {
    printf 'RELEASE_ID=%s\n' "$RELEASE_ID"
    printf 'PHASE=%s\n' "$phase"
    printf 'TARGET_IMAGE=%s\n' "$TARGET_IMAGE"
    printf 'CURRENT_IMAGE=%s\n' "$CURRENT_IMAGE"
    printf 'EXPECTED_REVISION=%s\n' "$EXPECTED_REVISION"
    printf 'MIGRATION_VERSION=%s\n' "$MIGRATION_VERSION"
    printf 'UPDATED_AT=%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
    write_optional_state TARGET_CONFIG_ID "$STATE_TARGET_CONFIG_ID"
    write_optional_state CURRENT_CONFIG_ID "$STATE_CURRENT_CONFIG_ID"
    write_optional_state WRITE_GATE_STATE "$STATE_WRITE_GATE_STATE"
    write_optional_state BOOTSTRAP_REPORT "$STATE_BOOTSTRAP_REPORT"
    write_optional_state RUNTIME_STARTED_AT "$STATE_RUNTIME_STARTED_AT"
    write_optional_state ONLINE_DRY_RUN_REPORT_1 "$STATE_ONLINE_DRY_RUN_REPORT_1"
    write_optional_state ONLINE_DRY_RUN_REPORT_2 "$STATE_ONLINE_DRY_RUN_REPORT_2"
    write_optional_state DRY_RUN_CHECKSUM "$STATE_DRY_RUN_CHECKSUM"
    write_optional_state DRY_RUN_REPORT_1 "$STATE_DRY_RUN_REPORT_1"
    write_optional_state DRY_RUN_REPORT_2 "$STATE_DRY_RUN_REPORT_2"
    write_optional_state BACKUP_PATH "$STATE_BACKUP_PATH"
    write_optional_state BACKUP_CHECKSUM "$STATE_BACKUP_CHECKSUM"
    write_optional_state BACKUP_SIZE "$STATE_BACKUP_SIZE"
    write_optional_state BACKUP_UTC "$STATE_BACKUP_UTC"
    write_optional_state APPLY_REPORT "$STATE_APPLY_REPORT"
    write_optional_state APPLY_CHECKSUM "$STATE_APPLY_CHECKSUM"
    write_optional_state VERIFY_REPORT "$STATE_VERIFY_REPORT"
    write_optional_state RELEASE_OVERRIDE_BACKUP "$STATE_RELEASE_OVERRIDE_BACKUP"
    write_optional_state RELEASE_OVERRIDE_MODE "$STATE_RELEASE_OVERRIDE_MODE"
    write_optional_state PRODUCTION_PROBE_REPORT "$STATE_PRODUCTION_PROBE_REPORT"
    write_optional_state CLONE_PROBE_REPORT "$STATE_CLONE_PROBE_REPORT"
    write_optional_state DUAL_WRITE_OPENED_AT "$STATE_DUAL_WRITE_OPENED_AT"
    write_optional_state OBSERVE_REPORT "$STATE_OBSERVE_REPORT"
    write_optional_state ROLLBACK_REASON "$STATE_ROLLBACK_REASON"
    write_optional_state SUSPEND_REPORT "$STATE_SUSPEND_REPORT"
  } > "$temporary"
  chmod 0600 "$temporary"
  read -r checksum _ < <(sha256sum "$temporary")
  [[ "$checksum" =~ ^[0-9a-f]{64}$ ]] || contract_error 'release state checksum could not be computed'
  printf '%s\n' "$checksum" > "$checksum_temporary"
  chmod 0600 "$checksum_temporary"
  mv -f "$temporary" "$STATE_FILE"
  mv -f "$checksum_temporary" "$STATE_CHECKSUM_FILE"
}

require_state_phase() {
  local actual expected matched=false
  verify_state_integrity
  actual="$(state_value PHASE)" || contract_error 'release state is missing; run preflight first'
  shift 0
  for expected in "$@"; do
    [[ "$actual" == "$expected" ]] && matched=true
  done
  [[ "$matched" == true ]] || contract_error "phase requires one of [$*], found $actual"
  [[ "$(state_value RELEASE_ID)" == "$RELEASE_ID" ]] || contract_error 'state RELEASE_ID does not match config'
  [[ "$(state_value TARGET_IMAGE)" == "$TARGET_IMAGE" ]] || contract_error 'state TARGET_IMAGE does not match config'
  [[ "$(state_value CURRENT_IMAGE)" == "$CURRENT_IMAGE" ]] || contract_error 'state CURRENT_IMAGE does not match config'
  [[ "$(state_value EXPECTED_REVISION)" == "$EXPECTED_REVISION" ]] || contract_error 'state EXPECTED_REVISION does not match config'
  [[ "$(state_value MIGRATION_VERSION)" == "$MIGRATION_VERSION" ]] || contract_error 'state MIGRATION_VERSION does not match config'
  load_state_context
  require_state_evidence "$actual"
}

read_approval_snapshot() {
  local file="$1" line key value line_number=0 approval_fd
  APPROVAL_VALUES=()
  exec {approval_fd}< "$file" || contract_error 'approval file cannot be opened'
  while IFS= read -r line <&"$approval_fd" || [[ -n "$line" ]]; do
    line_number=$((line_number + 1))
    [[ -z "$line" || "$line" == \#* ]] && continue
    [[ "$line" == *=* ]] || contract_error "approval line $line_number must be KEY=VALUE"
    key="${line%%=*}"
    value="${line#*=}"
    case "$key" in
      RELEASE_ID|EXPECTED_REVISION|TARGET_IMAGE|MIGRATION_VERSION|DRY_RUN_CHECKSUM|APPLY_CHECKSUM|APPROVED_ACTION) ;;
      *) contract_error "approval line $line_number uses unknown key $key" ;;
    esac
    [[ -z "${APPROVAL_VALUES[$key]:-}" ]] || contract_error "approval key $key must not be repeated"
    [[ "$value" =~ ^[A-Za-z0-9_./:@,+-]+$ ]] || contract_error "approval value contains forbidden shell syntax at line $line_number"
    APPROVAL_VALUES[$key]="$value"
  done
  exec {approval_fd}<&-
}

approval_snapshot_value() {
  local key="$1" value="${APPROVAL_VALUES[$1]:-}"
  [[ -n "$value" ]] || contract_error "approval key $key is required"
  printf '%s\n' "$value"
}

require_approval() {
  local file_name="$1" action="$2" expected_dry="$3" expected_apply="$4" file="${!1:-}"
  [[ "$file" == /* && -f "$file" ]] || contract_error "$file_name must be an absolute existing approval file"
  [[ "$(stat -c '%a' "$file")" == 600 ]] || contract_error "$file_name permissions must be 0600"
  read_approval_snapshot "$file"
  [[ "$(approval_snapshot_value RELEASE_ID)" == "$RELEASE_ID" ]] || contract_error 'approval RELEASE_ID mismatch'
  [[ "$(approval_snapshot_value EXPECTED_REVISION)" == "$EXPECTED_REVISION" ]] || contract_error 'approval EXPECTED_REVISION mismatch'
  [[ "$(approval_snapshot_value TARGET_IMAGE)" == "$TARGET_IMAGE" ]] || contract_error 'approval TARGET_IMAGE mismatch'
  [[ "$(approval_snapshot_value MIGRATION_VERSION)" == "$MIGRATION_VERSION" ]] || contract_error 'approval MIGRATION_VERSION mismatch'
  [[ "$(approval_snapshot_value APPROVED_ACTION)" == "$action" ]] || contract_error 'approval action mismatch'
  if [[ -n "$expected_dry" ]]; then
    [[ "$(approval_snapshot_value DRY_RUN_CHECKSUM)" == "$expected_dry" ]] || contract_error 'approval DRY_RUN_CHECKSUM mismatch'
  fi
  if [[ -n "$expected_apply" ]]; then
    [[ "$(approval_snapshot_value APPLY_CHECKSUM)" == "$expected_apply" ]] || contract_error 'approval APPLY_CHECKSUM mismatch'
  fi
}

write_gate_status() {
  local output
  output="$(write_gate_call status)"
  jq -e '
    .success == true and
    (.state == "open" or .state == "closed") and
    ([.external_writers, .background_writers, .non_terminal_preconsume, .async_settlement, .legacy_writer_sessions] |
      all(type == "number" and . >= 0))
  ' <<<"$output" >/dev/null || contract_error 'WRITE_GATE_HOOK status returned an invalid contract'
  printf '%s\n' "$output"
}

require_gate_closed() {
  local output="$1"
  jq -e '
    .success == true and .state == "closed" and
    .external_writers == 0 and .background_writers == 0 and
    .non_terminal_preconsume == 0 and .async_settlement == 0 and
    .legacy_writer_sessions == 0
  ' <<<"$output" >/dev/null || contract_error 'write gate is not closed and drained'
}
validate_browser_evidence() {
  local expected_snapshot="$1"
  [[ "$expected_snapshot" =~ ^[1-9][0-9]*$ ]] || contract_error 'production probe snapshot is invalid'
  [[ -f "$BROWSER_EVIDENCE_FILE" && "$(stat -c '%a' "$BROWSER_EVIDENCE_FILE")" == 600 ]] || contract_error 'browser evidence is missing or unsafe'
  jq -e --arg release "$RELEASE_ID" --arg digest "$TARGET_IMAGE" --arg revision "$EXPECTED_REVISION" --argjson snapshot "$expected_snapshot" '
    .success == true and .environment == "production_browser" and
    .release_id == $release and .digest == $digest and .revision == $revision and
    .snapshot_at == $snapshot and .authenticated == true and
    .api_origin == "http://127.0.0.1:13080" and
    .frontend_origin == "https://api.pqapi.shop" and
    .rendered_32_cny == true and .rendered_exact == true and
    .rendered_estimated == true and .rendered_unknown == true and
    .rendered_credit_time_not_applicable == true and
    .rendered_required_warning == true and
    .network_api_authenticated == true and
    .static_interception_used == false and
    (.captured_at | type == "string" and test("^[0-9]{4}-[0-9]{2}-[0-9]{2}T"))
  ' "$BROWSER_EVIDENCE_FILE" >/dev/null || contract_error 'production browser evidence failed the release contract'
}

require_container_maintenance_mode() {
  local environment expected="MAINTENANCE_MODE=$MAINTENANCE_MODE"
  environment="$(docker inspect --format '{{json .Config.Env}}' new-api)" || contract_error 'unable to inspect running maintenance environment'
  jq -e --arg expected "$expected" 'type == "array" and index($expected) != null' <<<"$environment" >/dev/null || contract_error 'running new-api container is not in explicit maintenance mode'
}

require_container_not_in_maintenance_mode() {
  local environment expected="MAINTENANCE_MODE=$MAINTENANCE_MODE"
  environment="$(docker inspect --format '{{json .Config.Env}}' new-api)" || contract_error 'unable to inspect running application environment'
  jq -e --arg expected "$expected" 'type == "array" and index($expected) == null' <<<"$environment" >/dev/null || contract_error 'running new-api container retained maintenance mode'
}

run_migration_command() {
  local mode="$1" output="$2" error_output
  error_output="${output}.stderr"
  local -a arguments=(credit-valuation-migrate "$mode" --version "$MIGRATION_VERSION")
  case "$mode" in
    --apply)
      arguments+=(--batch-size "$BATCH_SIZE")
      ;;
    --suspend)
      require_value SUSPEND_REASON
      arguments+=(--reason "$SUSPEND_REASON")
      ;;
  esac
  if ! docker run --rm --network "$DOCKER_NETWORK" --env "MAINTENANCE_MODE=$MAINTENANCE_MODE" --env-file "$APP_ENV_FILE" "$TARGET_IMAGE" "${arguments[@]}" > "$output" 2> "$error_output"; then
    cat "$error_output" >&2
    return 1
  fi
  chmod 0600 "$output" "$error_output"
}

validate_dry_run_report() {
  local path="$1"
  jq -e --argjson version "$MIGRATION_VERSION" '
    .success == true and .report.version == $version and
    .report.mode == "dry_run" and .report.status == "pending" and
    .report.read_only == true and .report.changed == false and .report.ready == false and
    (.report.blockers | type == "array" and length == 0) and
    .report.price.rows_invalid == 0 and
    (.report.price.rows_total | type == "number" and . > 0) and
    (.report.credit.rows_total | type == "number" and . > 0) and
    (.report.checksum | type == "string" and test("^[0-9a-f]{64}$"))
  ' "$path" >/dev/null || contract_error 'dry-run report failed the release contract'
}

validate_apply_report() {
  local path="$1"
  jq -e --argjson version "$MIGRATION_VERSION" '
    .success == true and .report.version == $version and
    .report.mode == "apply" and .report.status == "ready" and
    .report.read_only == false and (.report.changed | type == "boolean") and .report.ready == true and
    (.report.blockers | type == "array" and length == 0) and .report.price.rows_invalid == 0 and
    (.report.checksum | type == "string" and test("^[0-9a-f]{64}$"))
  ' "$path" >/dev/null || contract_error 'apply report failed the release contract'
}

validate_verify_report() {
  local path="$1" expected_checksum="$2"
  jq -e --argjson version "$MIGRATION_VERSION" --arg checksum "$expected_checksum" '
    .success == true and .report.version == $version and
    .report.mode == "verify" and .report.status == "ready" and
    .report.read_only == true and .report.changed == false and .report.ready == true and
    (.report.blockers | type == "array" and length == 0) and .report.price.rows_invalid == 0 and
    .report.checksum == $checksum
  ' "$path" >/dev/null || contract_error 'verify report failed the release contract'
}

run_preflight() {
  local configured_image target_revision target_repo_digests current_repo_digests
  local target_config_id current_config_id running_configured_image running_config_id bootstrap bootstrap_path
  if [[ -f "$STATE_FILE" ]]; then
    verify_state_integrity
    local existing_phase
    existing_phase="$(state_value PHASE)"
    [[ "$existing_phase" == preflight ]] || contract_error "preflight refuses existing phase $existing_phase"
  fi
  configured_image="$(release_override_image)" || contract_error 'COMPOSE_RELEASE must contain exactly one image entry'
  [[ "$configured_image" == "$CURRENT_IMAGE" ]] || contract_error 'COMPOSE_RELEASE is not pinned to CURRENT_IMAGE'

  target_revision="$(docker image inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' "$TARGET_IMAGE")"
  [[ "$target_revision" == "$EXPECTED_REVISION" ]] || contract_error 'target image revision does not match EXPECTED_REVISION'
  target_repo_digests="$(docker image inspect --format '{{json .RepoDigests}}' "$TARGET_IMAGE")"
  jq -e --arg reference "$TARGET_IMAGE" 'index($reference) != null' <<<"$target_repo_digests" >/dev/null || contract_error 'target image RepoDigests does not contain TARGET_IMAGE'
  current_repo_digests="$(docker image inspect --format '{{json .RepoDigests}}' "$CURRENT_IMAGE")"
  jq -e --arg reference "$CURRENT_IMAGE" 'index($reference) != null' <<<"$current_repo_digests" >/dev/null || contract_error 'current image RepoDigests does not contain CURRENT_IMAGE'
  target_config_id="$(docker image inspect --format '{{.Id}}' "$TARGET_IMAGE")"
  current_config_id="$(docker image inspect --format '{{.Id}}' "$CURRENT_IMAGE")"
  [[ "$target_config_id" =~ ^sha256:[0-9a-f]{64}$ ]] || contract_error 'target image config ID is invalid'
  [[ "$current_config_id" =~ ^sha256:[0-9a-f]{64}$ ]] || contract_error 'current image config ID is invalid'
  running_configured_image="$(docker inspect --format '{{.Config.Image}}' new-api)"
  [[ "$running_configured_image" == "$CURRENT_IMAGE" ]] || contract_error 'running container config is not CURRENT_IMAGE'
  running_config_id="$(docker inspect --format '{{.Image}}' new-api)"
  [[ "$running_config_id" == "$current_config_id" ]] || contract_error 'running container config ID is not the verified current image'

  bootstrap="$(write_gate_call bootstrap-status)"
  jq -e '
    .success == true and .bootstrap == true and .state == "open" and
    .runtime_stats == false and .full_writer_drain == false and
    .external_writers == null and
    ([.background_writers, .non_terminal_preconsume, .async_settlement, .legacy_writer_sessions] |
      all(type == "number" and . >= 0))
  ' <<<"$bootstrap" >/dev/null || contract_error 'legacy runtime bootstrap status failed; target runtime must be staged before drain capability is required'
  bootstrap_path="$AUDIT_DIR/${RELEASE_ID}-bootstrap.json"
  printf '%s\n' "$bootstrap" > "$bootstrap_path"
  chmod 0600 "$bootstrap_path"

  STATE_TARGET_CONFIG_ID="$target_config_id"
  STATE_CURRENT_CONFIG_ID="$current_config_id"
  STATE_BOOTSTRAP_REPORT="$bootstrap_path"
  STATE_WRITE_GATE_STATE=open
  write_state preflight
  echo "release=$RELEASE_ID phase=preflight result=pass target_revision=$target_revision bootstrap=legacy-safe"
}


run_stage_runtime() {
  local status override_backup capability
  require_state_phase preflight stage-runtime
  require_approval MUTATION_APPROVAL_FILE production-maintenance '' ''
  if [[ "$(state_value PHASE)" == stage-runtime ]]; then
    check_running_image "$TARGET_IMAGE" "$STATE_TARGET_CONFIG_ID" "$EXPECTED_REVISION"
    require_container_not_in_maintenance_mode
    status="$(write_gate_status)"
    jq -e '.success == true and .state == "open"' <<<"$status" >/dev/null || contract_error 'target runtime write gate is not open'
    capability="$(write_gate_call drain-capability)"
    jq -e '.success == true and .state == "open" and .runtime_stats == true and .full_writer_drain == true' <<<"$capability" >/dev/null || contract_error 'target runtime lacks an auditable full-writer drain capability'
    echo "release=$RELEASE_ID phase=stage-runtime result=no-op runtime_started_at=$STATE_RUNTIME_STARTED_AT"
    return
  fi
  status="$(write_gate_call bootstrap-status)"
  jq -e '.success == true and .bootstrap == true and .state == "open"' <<<"$status" >/dev/null || contract_error 'stage-runtime requires the legacy-safe write gate to remain open'
  override_backup="$AUDIT_DIR/${RELEASE_ID}-compose.release.before.yml"
  STATE_RELEASE_OVERRIDE_MODE="$(stat -c '%a' "$COMPOSE_RELEASE")"
  cp "$COMPOSE_RELEASE" "$override_backup"
  chmod 0600 "$override_backup"
  STATE_RELEASE_OVERRIDE_BACKUP="$override_backup"
  STATE_WRITE_GATE_STATE=open
  STATE_RUNTIME_STARTED_AT=''
  write_state stage-runtime-starting
  write_release_override "$TARGET_IMAGE" false
  "${COMPOSE[@]}" up -d --no-deps --force-recreate --pull never new-api
  wait_for_health
  check_running_image "$TARGET_IMAGE" "$STATE_TARGET_CONFIG_ID" "$EXPECTED_REVISION"
  require_container_not_in_maintenance_mode
  status="$(write_gate_status)"
  jq -e '.success == true and .state == "open"' <<<"$status" >/dev/null || contract_error 'target runtime did not expose a healthy open write-gate contract'
  capability="$(write_gate_call drain-capability)"
  jq -e '.success == true and .state == "open" and .runtime_stats == true and .full_writer_drain == true' <<<"$capability" >/dev/null || contract_error 'target runtime lacks an auditable full-writer drain capability'
  STATE_RUNTIME_STARTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  STATE_WRITE_GATE_STATE=open
  write_state stage-runtime
  echo "release=$RELEASE_ID phase=stage-runtime result=pass runtime_started_at=$STATE_RUNTIME_STARTED_AT drain_capability=verified"
}

run_stage_schema() {
  local status override_backup
  require_state_phase backup stage-schema
  require_approval MUTATION_APPROVAL_FILE production-maintenance '' ''
  status="$(write_gate_status)"
  require_gate_closed "$status"
  verify_recorded_backup
  if [[ "$(state_value PHASE)" == stage-schema ]]; then
    wait_for_maintenance_readiness
    check_running_image "$TARGET_IMAGE" "$STATE_TARGET_CONFIG_ID" "$EXPECTED_REVISION"
    require_container_maintenance_mode
    echo "release=$RELEASE_ID phase=stage-schema result=no-op"
    return
  fi
  if [[ -z "$STATE_RELEASE_OVERRIDE_BACKUP" ]]; then
    override_backup="$AUDIT_DIR/${RELEASE_ID}-compose.release.before.yml"
    STATE_RELEASE_OVERRIDE_MODE="$(stat -c '%a' "$COMPOSE_RELEASE")"
    cp "$COMPOSE_RELEASE" "$override_backup"
    chmod 0600 "$override_backup"
    STATE_RELEASE_OVERRIDE_BACKUP="$override_backup"
  else
    [[ -f "$STATE_RELEASE_OVERRIDE_BACKUP" ]] || contract_error 'saved Compose release overlay backup is missing'
  fi
  [[ "$(release_override_image)" == "$TARGET_IMAGE" ]] || contract_error 'target runtime is not pinned in COMPOSE_RELEASE before maintenance staging'
  STATE_WRITE_GATE_STATE=closed
  write_state stage-schema-starting
  write_release_override "$TARGET_IMAGE" true
  "${COMPOSE[@]}" up -d --no-deps --force-recreate --pull never new-api
  wait_for_maintenance_readiness
  check_running_image "$TARGET_IMAGE" "$STATE_TARGET_CONFIG_ID" "$EXPECTED_REVISION"
  require_container_maintenance_mode
  status="$(write_gate_status)"
  require_gate_closed "$status"
  STATE_WRITE_GATE_STATE=closed
  write_state stage-schema
  echo "release=$RELEASE_ID phase=stage-schema result=pass write_gate=closed maintenance_mode=true"
}

run_read_only_dry_run() {
  local first="$AUDIT_DIR/${RELEASE_ID}-online-dry-run-1.json"
  local second="$AUDIT_DIR/${RELEASE_ID}-online-dry-run-2.json"
  local gate
  require_state_phase stage-runtime read-only-dry-run
  gate="$(write_gate_status)"
  jq -e '.success == true and .state == "open"' <<<"$gate" >/dev/null || contract_error 'online dry-run requires the target runtime write gate to remain open'
  check_running_image "$TARGET_IMAGE" "$STATE_TARGET_CONFIG_ID" "$EXPECTED_REVISION"
  if [[ "$(state_value PHASE)" == read-only-dry-run ]]; then
    [[ -f "$STATE_ONLINE_DRY_RUN_REPORT_1" && -f "$STATE_ONLINE_DRY_RUN_REPORT_2" ]] || contract_error 'recorded online dry-run reports are missing'
    validate_dry_run_report "$STATE_ONLINE_DRY_RUN_REPORT_1"
    validate_dry_run_report "$STATE_ONLINE_DRY_RUN_REPORT_2"
    echo "release=$RELEASE_ID phase=read-only-dry-run result=no-op online_reports_recorded=true runtime_open=true"
    return
  fi
  run_migration_command --dry-run "$first"
  validate_dry_run_report "$first"
  run_migration_command --dry-run "$second"
  validate_dry_run_report "$second"
  STATE_ONLINE_DRY_RUN_REPORT_1="$first"
  STATE_ONLINE_DRY_RUN_REPORT_2="$second"
  STATE_WRITE_GATE_STATE=open
  write_state read-only-dry-run
  echo "release=$RELEASE_ID phase=read-only-dry-run result=pass online_reports_recorded=true runtime_open=true"
}



run_stop_writes() {
  local transition status
  require_state_phase read-only-dry-run stop-writes
  require_approval MUTATION_APPROVAL_FILE production-maintenance '' ''
  if [[ "$(state_value PHASE)" == stop-writes ]]; then
    status="$(write_gate_status)"
    require_gate_closed "$status"
    echo "release=$RELEASE_ID phase=stop-writes result=no-op"
    return
  fi
  transition="$(write_gate_call close "$RELEASE_ID")"
  require_gate_closed "$transition"
  status="$(write_gate_status)"
  require_gate_closed "$status"
  STATE_WRITE_GATE_STATE=closed
  write_state stop-writes
  echo "release=$RELEASE_ID phase=stop-writes result=pass"
}
run_frozen_dry_run() {
  local first="$AUDIT_DIR/${RELEASE_ID}-frozen-dry-run-1.json"
  local second="$AUDIT_DIR/${RELEASE_ID}-frozen-dry-run-2.json"
  local checksum gate
  require_state_phase stop-writes frozen-dry-run
  gate="$(write_gate_status)"
  require_gate_closed "$gate"
  if [[ "$(state_value PHASE)" == frozen-dry-run ]]; then
    [[ -f "$STATE_DRY_RUN_REPORT_1" && -f "$STATE_DRY_RUN_REPORT_2" ]] || contract_error 'recorded frozen dry-run reports are missing'
    validate_dry_run_report "$STATE_DRY_RUN_REPORT_1"
    validate_dry_run_report "$STATE_DRY_RUN_REPORT_2"
    cmp <(jq -S -c 'del(.report.fx.captured_at)' "$STATE_DRY_RUN_REPORT_1") <(jq -S -c 'del(.report.fx.captured_at)' "$STATE_DRY_RUN_REPORT_2") >/dev/null || contract_error 'recorded frozen dry-run business reports are not identical'
    [[ "$(jq -r '.report.checksum' "$STATE_DRY_RUN_REPORT_1")" == "$STATE_DRY_RUN_CHECKSUM" ]] || contract_error 'recorded frozen dry-run checksum mismatch'
    echo "release=$RELEASE_ID phase=frozen-dry-run result=no-op checksum=$STATE_DRY_RUN_CHECKSUM writes_closed=true"
    return
  fi
  run_migration_command --dry-run "$first"
  validate_dry_run_report "$first"
  run_migration_command --dry-run "$second"
  validate_dry_run_report "$second"
  cmp <(jq -S -c 'del(.report.fx.captured_at)' "$first") <(jq -S -c 'del(.report.fx.captured_at)' "$second") >/dev/null || contract_error 'two frozen dry-run business reports are not identical'
  checksum="$(jq -r '.report.checksum' "$first")"
  [[ "$checksum" =~ ^[0-9a-f]{64}$ ]] || contract_error 'frozen dry-run checksum is invalid'
  [[ "$checksum" == "$(jq -r '.report.checksum' "$second")" ]] || contract_error 'frozen dry-run checksums differ'
  STATE_DRY_RUN_CHECKSUM="$checksum"
  STATE_DRY_RUN_REPORT_1="$first"
  STATE_DRY_RUN_REPORT_2="$second"
  STATE_WRITE_GATE_STATE=closed
  write_state frozen-dry-run
  echo "release=$RELEASE_ID phase=frozen-dry-run result=pass checksum=$checksum writes_closed=true"
}


backup_checksum() {
  local path="$1" checksum remainder
  read -r checksum remainder < <(sha256sum "$path")
  [[ "$checksum" =~ ^[0-9a-f]{64}$ ]] || contract_error 'backup checksum is invalid'
  printf '%s\n' "$checksum"
}

create_consistent_backup() {
  local suffix="$1" timestamp path checksum size
  timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
  path="$BACKUP_DIR/new_api_${suffix}_${timestamp}.dump"
  docker exec "$POSTGRES_CONTAINER" pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc > "$path"
  [[ -s "$path" ]] || contract_error 'PostgreSQL backup is empty'
  docker exec -i "$POSTGRES_CONTAINER" pg_restore --list < "$path" >/dev/null
  chmod 0600 "$path"
  checksum="$(backup_checksum "$path")"
  size="$(stat -c %s "$path")"
  [[ "$size" =~ ^[1-9][0-9]*$ ]] || contract_error 'backup size is invalid'
  printf '%s  %s\n' "$checksum" "$path" > "$path.sha256"
  chmod 0600 "$path.sha256"
  STATE_BACKUP_PATH="$path"
  STATE_BACKUP_CHECKSUM="$checksum"
  STATE_BACKUP_SIZE="$size"
  STATE_BACKUP_UTC="$timestamp"
}

verify_recorded_backup() {
  [[ -f "$STATE_BACKUP_PATH" && -s "$STATE_BACKUP_PATH" ]] || contract_error 'recorded backup is missing'
  [[ "$(backup_checksum "$STATE_BACKUP_PATH")" == "$STATE_BACKUP_CHECKSUM" ]] || contract_error 'recorded backup checksum mismatch'
  [[ "$(stat -c %s "$STATE_BACKUP_PATH")" == "$STATE_BACKUP_SIZE" ]] || contract_error 'recorded backup size mismatch'
  docker exec -i "$POSTGRES_CONTAINER" pg_restore --list < "$STATE_BACKUP_PATH" >/dev/null
}

run_backup() {
  local status
  require_state_phase frozen-dry-run backup
  require_approval MUTATION_APPROVAL_FILE production-maintenance '' ''
  status="$(write_gate_status)"
  require_gate_closed "$status"
  if [[ "$(state_value PHASE)" == backup ]]; then
    verify_recorded_backup
    echo "release=$RELEASE_ID phase=backup result=no-op backup=$STATE_BACKUP_PATH"
    return
  fi
  create_consistent_backup "before_${RELEASE_ID}"
  STATE_WRITE_GATE_STATE=closed
  write_state backup
  echo "release=$RELEASE_ID phase=backup result=pass backup=$STATE_BACKUP_PATH sha256=$STATE_BACKUP_CHECKSUM size=$STATE_BACKUP_SIZE"
}


run_apply() {
  local report="$AUDIT_DIR/${RELEASE_ID}-apply.json" checksum status
  require_state_phase stage-schema apply
  require_approval MUTATION_APPROVAL_FILE apply-migration "$STATE_DRY_RUN_CHECKSUM" ''
  status="$(write_gate_status)"
  require_gate_closed "$status"
  verify_recorded_backup
  if [[ "$(state_value PHASE)" == apply ]]; then
    [[ -f "$STATE_APPLY_REPORT" ]] || contract_error 'recorded apply report is missing'
    validate_apply_report "$STATE_APPLY_REPORT"
    [[ "$STATE_APPLY_CHECKSUM" == "$STATE_DRY_RUN_CHECKSUM" ]] || contract_error 'recorded apply checksum does not match dry-run checksum'
    echo "release=$RELEASE_ID phase=apply result=no-op checksum=$STATE_APPLY_CHECKSUM"
    return
  fi
  run_migration_command --apply "$report"
  validate_apply_report "$report"
  checksum="$(jq -r '.report.checksum' "$report")"
  [[ "$checksum" =~ ^[0-9a-f]{64}$ ]] || contract_error 'apply checksum is invalid'
  [[ "$checksum" == "$STATE_DRY_RUN_CHECKSUM" ]] || contract_error 'apply checksum does not match dry-run checksum'
  STATE_APPLY_REPORT="$report"
  STATE_APPLY_CHECKSUM="$checksum"
  STATE_WRITE_GATE_STATE=closed
  write_state apply
  echo "release=$RELEASE_ID phase=apply result=pass checksum=$checksum"
}

run_verify() {
  local report="$AUDIT_DIR/${RELEASE_ID}-verify.json" status
  require_state_phase apply verify
  require_approval MUTATION_APPROVAL_FILE apply-migration "$STATE_DRY_RUN_CHECKSUM" ''
  status="$(write_gate_status)"
  require_gate_closed "$status"
  verify_recorded_backup
  run_migration_command --verify "$report"
  validate_verify_report "$report" "$STATE_APPLY_CHECKSUM"
  STATE_VERIFY_REPORT="$report"
  STATE_WRITE_GATE_STATE=closed
  write_state verify
  echo "release=$RELEASE_ID phase=verify result=pass checksum=$STATE_APPLY_CHECKSUM"
}

wait_for_maintenance_readiness() {
  local started now
  started="$(date +%s)"
  while true; do
    if docker exec new-api test -s /tmp/new-api-maintenance-ready >/dev/null 2>&1 && docker exec new-api test -s /tmp/new-api-credit-valuation-schema-ready >/dev/null 2>&1; then
      return 0
    fi
    now="$(date +%s)"
    (( now - started < HEALTH_TIMEOUT_SECONDS )) || contract_error 'new-api maintenance/schema readiness did not become available while writes remained closed'
    sleep 1
  done
}

wait_for_health() {
  local started now status
  started="$(date +%s)"
  while true; do
    status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' new-api 2>/dev/null || true)"
    [[ "$status" == healthy ]] && return 0
    now="$(date +%s)"
    (( now - started < HEALTH_TIMEOUT_SECONDS )) || contract_error 'new-api did not become healthy while writes remained closed'
    sleep 1
  done
}

check_running_image() {
  local configured config_id revision
  configured="$(docker inspect --format '{{.Config.Image}}' new-api)"
  [[ "$configured" == "$1" ]] || contract_error 'running container image reference mismatch'
  config_id="$(docker inspect --format '{{.Image}}' new-api)"
  [[ "$config_id" == "$2" ]] || contract_error 'running container config ID mismatch'
  revision="$(docker inspect --format '{{index .Config.Labels "org.opencontainers.image.revision"}}' new-api)"
  [[ "$revision" == "$3" ]] || contract_error 'running container revision mismatch'
}

run_start_closed() {
  local status
  require_state_phase verify start-closed
  require_approval MUTATION_APPROVAL_FILE apply-migration "$STATE_DRY_RUN_CHECKSUM" ''
  status="$(write_gate_status)"
  require_gate_closed "$status"
  if [[ "$(state_value PHASE)" == start-closed ]]; then
    wait_for_health
    check_running_image "$TARGET_IMAGE" "$STATE_TARGET_CONFIG_ID" "$EXPECTED_REVISION"
    require_container_not_in_maintenance_mode
    echo "release=$RELEASE_ID phase=start-closed result=no-op"
    return
  fi
  [[ -n "$STATE_RELEASE_OVERRIDE_BACKUP" && -f "$STATE_RELEASE_OVERRIDE_BACKUP" ]] || contract_error 'original Compose release overlay backup is missing'
  write_release_override "$TARGET_IMAGE" false
  "${COMPOSE[@]}" up -d --no-deps --force-recreate --pull never new-api
  wait_for_health
  check_running_image "$TARGET_IMAGE" "$STATE_TARGET_CONFIG_ID" "$EXPECTED_REVISION"
  require_container_not_in_maintenance_mode
  status="$(write_gate_status)"
  require_gate_closed "$status"
  STATE_WRITE_GATE_STATE=closed
  write_state start-closed
  echo "release=$RELEASE_ID phase=start-closed result=pass maintenance_mode=false"
}

run_probe() {
  local production_report="$AUDIT_DIR/${RELEASE_ID}-production-probe.json"
  local clone_report="$AUDIT_DIR/${RELEASE_ID}-clone-probe.json" status browser_snapshot
  require_state_phase start-closed probe
  require_absolute_executable READ_ONLY_PROBE_HOOK
  require_absolute_executable CLONE_PROBE_HOOK
  status="$(write_gate_status)"
  require_gate_closed "$status"
  browser_snapshot="$(jq -r '.snapshot_at // empty' "$BROWSER_EVIDENCE_FILE")"
  validate_browser_evidence "$browser_snapshot"
  PRODUCTION_SNAPSHOT_AT="$browser_snapshot" production_probe_call "$RELEASE_ID" "$TARGET_IMAGE" "$EXPECTED_REVISION" "$MIGRATION_VERSION" > "$production_report"
  jq -e --arg digest "$TARGET_IMAGE" --arg revision "$EXPECTED_REVISION" --argjson version "$MIGRATION_VERSION" --argjson snapshot "$browser_snapshot" '
    .success == true and .environment == "production" and .read_only == true and
    .digest == $digest and .revision == $revision and
    .marker_status == "ready" and .migration_version == $version and
    .invariants == true and .authenticated_api == true and
    .administrator_identity == true and
    .paid_value_endpoints_structurally_consistent == true and
    .snapshot_at == $snapshot and
    .api_origin == "http://127.0.0.1:13080" and
    .browser_evidence_required == true
  ' "$production_report" >/dev/null || contract_error 'production read-only probe failed'
  validate_browser_evidence "$(jq -r '.snapshot_at' "$production_report")"
  MIGRATION_VERSION="$MIGRATION_VERSION" clone_probe_call "$RELEASE_ID" "$STATE_BACKUP_PATH" "$STATE_BACKUP_CHECKSUM" "$TARGET_IMAGE" > "$clone_report"
  chmod 0600 "$clone_report"
  jq -e --arg backup_checksum "$STATE_BACKUP_CHECKSUM" --arg target_digest "$TARGET_IMAGE" '
    .success == true and .environment == "isolated_clone" and
    .clone_isolated == true and .production_identity_collision == false and
    .source_backup_sha256 == $backup_checksum and .target_digest == $target_digest and
    .cleanup_complete == true and
    .fixture.price_amount_micros == "40000000" and .fixture.plan_credit == 1000 and
    .fixture.consumed_credit == 200 and .fixture.available_credit == 800 and
    .fixture.end_time == 0 and .fixture.exact_cost_micros == "32000000" and
    .fixture.currency == "CNY" and .fixture.active_paid_subscription_count == 1 and
    .fixture.estimated_cost_micros == "0" and .fixture.unknown_credit == 0 and
    .fixture.five_analytics_endpoints_consistent == true and
    .fixture.disabled_plan_existing_consumable == true and
    .fixture.disabled_plan_new_allocations_rejected == true and
    .fixture.model_scope_ignored == true
  ' "$clone_report" >/dev/null || contract_error 'isolated-clone 32 CNY and disabled-plan probe failed'
  STATE_PRODUCTION_PROBE_REPORT="$production_report"
  STATE_CLONE_PROBE_REPORT="$clone_report"
  STATE_WRITE_GATE_STATE=closed
  write_state probe
  echo "release=$RELEASE_ID phase=probe result=pass"
}

run_open_writes() {
  local transition status
  require_state_phase probe open-writes
  require_approval OPEN_WRITES_APPROVAL_FILE open-writes "$STATE_DRY_RUN_CHECKSUM" "$STATE_APPLY_CHECKSUM"
  if [[ "$(state_value PHASE)" == open-writes ]]; then
    status="$(write_gate_status)"
    jq -e '.success == true and .state == "open"' <<<"$status" >/dev/null || contract_error 'write gate is not open'
    echo "release=$RELEASE_ID phase=open-writes result=no-op opened_at=$STATE_DUAL_WRITE_OPENED_AT"
    return
  fi
  transition="$(write_gate_call open "$RELEASE_ID")"
  jq -e '.success == true and .state == "open"' <<<"$transition" >/dev/null || contract_error 'write gate refused to open'
  status="$(write_gate_status)"
  jq -e '.success == true and .state == "open"' <<<"$status" >/dev/null || contract_error 'write gate did not remain open'
  STATE_DUAL_WRITE_OPENED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  STATE_WRITE_GATE_STATE=open
  write_state open-writes
  echo "release=$RELEASE_ID phase=open-writes result=pass opened_at=$STATE_DUAL_WRITE_OPENED_AT"
}

run_observe() {
  local report="$AUDIT_DIR/${RELEASE_ID}-observe.json" status
  require_state_phase open-writes observe
  require_absolute_executable OBSERVE_HOOK
  status="$(write_gate_status)"
  jq -e '.success == true and .state == "open"' <<<"$status" >/dev/null || contract_error 'observe requires open writes'
  observe_call "$RELEASE_ID" "$OBSERVE_SECONDS" "$TARGET_IMAGE" "$EXPECTED_REVISION" > "$report"
  chmod 0600 "$report"
  jq -e --argjson seconds "$OBSERVE_SECONDS" '
    .success == true and .aggregated == true and .window_seconds >= $seconds and
    .health_failures == 0 and .credit_valuation_state_missing == 0 and
    .credit_valuation_state_mismatch == 0 and .unsupported_fx_errors == 0 and
    .panic_count == 0 and .abnormal_restarts == 0 and
    .unknown_credit_regression == false and
    .postgres_lock_wait_regression == false and .write_load_regression == false and
    (.http_request_count | type == "number" and . >= 0) and
    (.http_error_count | type == "number" and . >= 0) and
    (.http_error_rate_ppm | type == "number" and . >= 0) and
    (.postgres_write_delta | type == "number" and . >= 0) and
    (.postgres_connection_delta | type == "number") and
    (.postgres_connections_after | type == "number" and . >= 0) and
    (.batch_update_pending_total | type == "number" and . == 0) and
    (.settlement_replay_count | type == "number" and . >= 0) and
    (.settlement_replay_growth | type == "number" and . >= 0) and
    (.settlement_max_latency_seconds | type == "number" and . >= 0) and
    (.resource_snapshot | type == "object")
  ' "$report" >/dev/null || contract_error 'observation window failed'
  STATE_OBSERVE_REPORT="$report"
  STATE_WRITE_GATE_STATE=open
  write_state observe
  echo "release=$RELEASE_ID phase=observe result=pass report=$report"
}

restart_current_image_closed() {
  local status
  restore_release_override
  "${COMPOSE[@]}" up -d --no-deps --force-recreate --pull never new-api
  wait_for_health
  check_running_image "$CURRENT_IMAGE" "$STATE_CURRENT_CONFIG_ID" "$(docker image inspect --format '{{index .Config.Labels \"org.opencontainers.image.revision\"}}' "$CURRENT_IMAGE")"
  status="$(write_gate_status)"
  require_gate_closed "$status"
}

run_rollback_before_ready() {
  local transition status phase
  require_state_phase frozen-dry-run backup stage-schema-starting stage-schema read-only-dry-run stop-writes rollback-before-ready stage-schema-failed
  if [[ -n "$STATE_DRY_RUN_CHECKSUM" ]]; then
    require_approval ROLLBACK_APPROVAL_FILE rollback-before-ready "$STATE_DRY_RUN_CHECKSUM" ''
  else
    require_approval ROLLBACK_APPROVAL_FILE rollback-before-ready '' ''
  fi
  phase="$(state_value PHASE)"
  if [[ "$phase" == rollback-before-ready ]]; then
    status="$(write_gate_status)"
    require_gate_closed "$status"
    echo "release=$RELEASE_ID phase=rollback-before-ready result=no-op"
    return
  fi
  status="$(write_gate_status)"
  if ! jq -e '.success == true and .state == "closed"' <<<"$status" >/dev/null; then
    transition="$(write_gate_call close "$RELEASE_ID-rollback-before-ready")"
    require_gate_closed "$transition"
  fi
  status="$(write_gate_status)"
  require_gate_closed "$status"
  restart_current_image_closed
  STATE_ROLLBACK_REASON=before-ready
  STATE_WRITE_GATE_STATE=closed
  write_state rollback-before-ready
  echo "release=$RELEASE_ID phase=rollback-before-ready result=pass marker_ready=false writes_closed=true"
}


run_rollback_ready_before_open() {
  local status
  require_state_phase start-closed probe rollback-ready-before-open
  require_approval ROLLBACK_APPROVAL_FILE rollback-ready-before-open "$STATE_DRY_RUN_CHECKSUM" "$STATE_APPLY_CHECKSUM"
  status="$(write_gate_status)"
  require_gate_closed "$status"
  if [[ "$(state_value PHASE)" != rollback-ready-before-open ]]; then
    restart_current_image_closed
    STATE_ROLLBACK_REASON=ready-before-open
    STATE_WRITE_GATE_STATE=closed
    write_state rollback-ready-before-open
  fi
  echo "release=$RELEASE_ID phase=rollback-ready-before-open result=pass marker_preserved=true writes_closed=true"
}

run_rollback_suspend() {
  local transition status phase suspend_report="$AUDIT_DIR/${RELEASE_ID}-suspend.json"
  require_state_phase open-writes observe rollback-suspend rollback-suspend-suspended rollback-suspend-backup
  require_value SUSPEND_REASON
  require_approval ROLLBACK_APPROVAL_FILE rollback-suspend "$STATE_DRY_RUN_CHECKSUM" "$STATE_APPLY_CHECKSUM"
  phase="$(state_value PHASE)"
  if [[ "$phase" == rollback-suspend ]]; then
    status="$(write_gate_status)"
    require_gate_closed "$status"
    echo "release=$RELEASE_ID phase=rollback-suspend result=no-op"
    return
  fi
  if [[ "$phase" == open-writes || "$phase" == observe ]]; then
    transition="$(write_gate_call close "$RELEASE_ID")"
    require_gate_closed "$transition"
    status="$(write_gate_status)"
    require_gate_closed "$status"
    run_migration_command --suspend "$suspend_report"
    jq -e --argjson version "$MIGRATION_VERSION" '
      .success == true and .report.version == $version and
      .report.mode == "suspend" and .report.status == "suspended" and .report.ready == false
    ' "$suspend_report" >/dev/null || contract_error 'suspend report failed the release contract'
    STATE_SUSPEND_REPORT="$suspend_report"
    STATE_ROLLBACK_REASON="$SUSPEND_REASON"
    STATE_WRITE_GATE_STATE=closed
    write_state rollback-suspend-suspended
    phase=rollback-suspend-suspended
  fi
  if [[ "$phase" == rollback-suspend-suspended ]]; then
    [[ -n "$STATE_SUSPEND_REPORT" && -f "$STATE_SUSPEND_REPORT" ]] || contract_error 'suspend report is missing'
    jq -e --argjson version "$MIGRATION_VERSION" '
      .success == true and .report.version == $version and
      .report.mode == "suspend" and .report.status == "suspended" and .report.ready == false
    ' "$STATE_SUSPEND_REPORT" >/dev/null || contract_error 'recorded suspend report failed the release contract'
    create_consistent_backup "after_suspend_${RELEASE_ID}"
    verify_recorded_backup
    STATE_WRITE_GATE_STATE=closed
    write_state rollback-suspend-backup
    phase=rollback-suspend-backup
  fi
  if [[ "$phase" == rollback-suspend-backup ]]; then
    verify_recorded_backup
    restart_current_image_closed
    STATE_ROLLBACK_REASON="$SUSPEND_REASON"
    STATE_WRITE_GATE_STATE=closed
    write_state rollback-suspend
    echo "release=$RELEASE_ID phase=rollback-suspend result=pass new_backup=$STATE_BACKUP_PATH marker=suspended writes_closed=true"
    return
  fi
  contract_error "rollback-suspend reached unexpected phase $phase"
}

[[ "$#" -eq 3 && "$1" == '--config' ]] || usage
CONFIG_FILE="$2"
PHASE="$3"
[[ "$CONFIG_FILE" == /* && -f "$CONFIG_FILE" ]] || contract_error 'config must be an absolute existing file'
# The config is a reviewed, declarative KEY=VALUE file. Values intentionally
# reject shell syntax; APP_ENV_FILE points at the protected application env.
parse_config "$CONFIG_FILE"
validate_config
case "$PHASE" in
  preflight|stage-runtime|read-only-dry-run|stop-writes|frozen-dry-run|backup|stage-schema|apply|verify|start-closed|probe|open-writes|observe|rollback-before-ready|rollback-ready-before-open|rollback-suspend)
    ;;
  *)
    usage
    ;;
esac
init_runtime
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

case "$PHASE" in
  preflight) run_preflight ;;
  stage-runtime) run_stage_runtime ;;
  read-only-dry-run) run_read_only_dry_run ;;
  stop-writes) run_stop_writes ;;
  frozen-dry-run) run_frozen_dry_run ;;
  backup) run_backup ;;
  stage-schema) run_stage_schema ;;
  apply) run_apply ;;
  verify) run_verify ;;
  start-closed) run_start_closed ;;
  probe) run_probe ;;
  open-writes) run_open_writes ;;
  observe) run_observe ;;
  rollback-ready-before-open) run_rollback_ready_before_open ;;
  rollback-before-ready) run_rollback_before_ready ;;
  rollback-suspend) run_rollback_suspend ;;
esac
