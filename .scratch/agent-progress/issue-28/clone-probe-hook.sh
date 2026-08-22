#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

out_done=0
fail() {
  local message=${1:-probe failed}
  if (( out_done == 0 )); then
    printf '{"success":false,"environment":"isolated_clone","error":"probe_failed"}\n'
    out_done=1
  fi
  printf 'clone probe: %s\n' "$message" >&2
  exit 1
}
on_exit() {
  local rc=$?
  if (( rc != 0 && out_done == 0 )); then
    printf '{"success":false,"environment":"isolated_clone","error":"probe_failed"}\n'
  fi
}
trap on_exit EXIT
[[ $# -eq 4 ]] || fail 'exactly RELEASE_ID BACKUP_PATH BACKUP_SHA256 TARGET_IMAGE is required'
release_id=$1
backup=$2
expected_sha=$3
target_image=$4
[[ $release_id =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,80}$ && $release_id != *..* ]] || fail 'invalid release id'
[[ $backup == /* && -f $backup && $expected_sha =~ ^[0-9a-f]{64}$ ]] || fail 'invalid backup identity'
[[ $target_image =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ ]] || fail 'target image must be immutable digest'
[[ $(stat -c '%a' "$backup" 2>/dev/null || printf 000) == 600 ]] || fail 'backup must have mode 0600'

cfg=${CLONE_PROBE_CONFIG:-${RELEASE_PROBE_CONFIG:-}}
[[ $cfg == /* && -f $cfg ]] || fail 'absolute probe config is required'
declare -A C=()
while IFS= read -r line || [[ -n $line ]]; do
  [[ $line != *$'\r'* ]] || fail 'invalid config line'
  [[ -z $line || $line == \#* ]] && continue
  [[ $line =~ ^([A-Z][A-Z0-9_]*)=(.*)$ ]] || fail 'malformed config'
  key=${BASH_REMATCH[1]}; value=${BASH_REMATCH[2]}
  [[ -z ${C[$key]+x} ]] || fail 'duplicate config key'
  case $key in
    DOCKER_BIN|JQ_BIN|SHA256_BIN|PG_RESTORE_BIN|CLONE_FIXTURE_RUNNER|CLONE_POSTGRES_IMAGE|CLONE_POSTGRES_USER|CLONE_POSTGRES_DB|CLONE_TIMEOUT_SECONDS|CLONE_CONTAINER_PREFIX|TMP_ROOT|PRODUCTION_CONTAINER_NAME|PRODUCTION_NETWORK) ;;
    *) fail 'unknown config key' ;;
  esac
  [[ "$value" != *'$('* && "$value" != *'`'* && "$value" != *';'* && "$value" != *'|'* && "$value" != *'&'* ]] || fail 'unsafe config value'
  C[$key]=$value
done < "$cfg"
get() { printf '%s' "${C[$1]-}"; }
for key in DOCKER_BIN JQ_BIN SHA256_BIN PG_RESTORE_BIN CLONE_FIXTURE_RUNNER; do
  value=$(get "$key")
  [[ $value == /* && -x $value ]] || fail "$key must be an absolute executable"
done
root=$(get TMP_ROOT)
[[ $root == /* && -d $root && -w $root ]] || fail 'temporary root must be absolute and writable'
checksum=$($(get SHA256_BIN) "$backup" 2>/dev/null | { read -r sum _; printf '%s' "$sum"; }) || fail 'backup checksum failed'
[[ $checksum == "$expected_sha" ]] || fail 'backup checksum mismatch'
$(get PG_RESTORE_BIN) --list "$backup" >/dev/null 2>&1 || fail 'pg_restore validation failed'
workdir=$(mktemp -d "$root/issue28-clone.XXXXXX") || fail 'cannot create clone workdir'
chmod 700 "$workdir"

runner=$(get CLONE_FIXTURE_RUNNER)
set +e
report=$(CLONE_WORKDIR="$workdir" CLONE_READ_ONLY=1 PROBE_CONFIG="$cfg" MIGRATION_VERSION="${MIGRATION_VERSION:-1}" "$runner" "$release_id" "$backup" "$expected_sha" "$target_image" 2>/dev/null)
rc=$?
set -e
rm -rf -- "$workdir"
(( rc == 0 )) || fail 'clone evidence runner failed'
[[ $report != *$'\n'* ]] || fail 'clone evidence runner returned multiple lines'
jq_bin=$(get JQ_BIN)
"$jq_bin" -e --arg sha "$expected_sha" --arg image "$target_image" 'type == "object" and .success == true and .environment == "isolated_clone" and .clone_isolated == true and .production_identity_collision == false and .source_backup_sha256 == $sha and .target_digest == $image and .cleanup_complete == true and .fixture.price_amount_micros == "40000000" and .fixture.plan_credit == 1000 and .fixture.consumed_credit == 200 and .fixture.available_credit == 800 and .fixture.end_time == 0 and .fixture.exact_cost_micros == "32000000" and .fixture.currency == "CNY" and .fixture.active_paid_subscription_count == 1 and .fixture.estimated_cost_micros == "0" and .fixture.unknown_credit == 0 and .fixture.five_analytics_endpoints_consistent == true and .fixture.disabled_plan_existing_consumable == true and .fixture.disabled_plan_new_allocations_rejected == true and .fixture.model_scope_ignored == true' <<<"$report" >/dev/null 2>&1 || fail 'clone evidence contract failed'
printf '%s\n' "$report" | "$jq_bin" -c '{success:true,environment:"isolated_clone",clone_isolated:.clone_isolated,production_identity_collision:.production_identity_collision,source_backup_sha256:.source_backup_sha256,target_digest:.target_digest,cleanup_complete:.cleanup_complete,fixture:.fixture}'
out_done=1
