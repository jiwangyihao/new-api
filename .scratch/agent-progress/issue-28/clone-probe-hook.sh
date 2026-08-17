#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
out_done=0
fail() {
  if (( out_done == 0 )); then
    printf '{"success":false,"environment":"isolated_clone","error":"probe_failed"}\n'
    out_done=1
  fi
  printf 'clone probe: %s\n' "${1:-probe failed}" >&2
  exit 1
}
on_exit() {
  local rc=$?
  if (( rc != 0 && out_done == 0 )); then
    printf '{"success":false,"environment":"isolated_clone","error":"probe_failed"}\n'
  fi
  if [[ -n ${workdir-} && -d ${workdir-} ]]; then
    rm -rf -- "$workdir"
  fi
}
trap on_exit EXIT
[[ $# -eq 4 ]] || fail 'exactly RELEASE_ID BACKUP_PATH BACKUP_SHA256 TARGET_IMAGE is required'
release_id=$1; backup=$2; expected_sha=$3; target_image=$4
[[ $release_id =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,80}$ && $release_id != *..* ]] || fail 'invalid release id'
[[ $backup == /* && -f $backup ]] || fail 'backup must be an absolute regular file'
[[ $expected_sha =~ ^[0-9a-f]{64}$ ]] || fail 'invalid backup checksum'
[[ $target_image =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ ]] || fail 'target image must be immutable digest'
mode=$(stat -c '%a' "$backup" 2>/dev/null || printf 000); [[ $mode == 600 ]] || fail 'backup must have mode 0600'
cfg=${CLONE_PROBE_CONFIG:-${RELEASE_PROBE_CONFIG:-}}; [[ -n $cfg && $cfg == /* && -f $cfg ]] || fail 'absolute probe config is required'
declare -A C=()
while IFS= read -r line || [[ -n $line ]]; do
 [[ $line != *$'\r'* ]] || fail 'invalid config line'; [[ -z $line || $line == \#* ]] && continue
 [[ $line =~ ^([A-Z][A-Z0-9_]*)=(.*)$ ]] || fail 'malformed config'; key=${BASH_REMATCH[1]}; val=${BASH_REMATCH[2]}
 [[ "$val" != *'$('* && "$val" != *'`'* && "$val" != *';'* && "$val" != *'|'* && "$val" != *'&'* ]] || fail 'unsafe config value'
 case $key in SHA256_BIN|PG_RESTORE_BIN|CLONE_FIXTURE_RUNNER|TMP_ROOT|PRODUCTION_CONTAINER_NAME|PRODUCTION_NETWORK|CLONE_TIMEOUT_SECONDS|JQ_BIN) ;; *) fail 'unknown config key' ;; esac
 [[ -z ${C[$key]+x} ]] || fail 'duplicate config key'; C[$key]=$val
done < "$cfg"
get(){ printf '%s' "${C[$1]-}"; }
for key in SHA256_BIN PG_RESTORE_BIN CLONE_FIXTURE_RUNNER JQ_BIN; do v=$(get "$key"); [[ -n $v && $v == /* && -x $v ]] || fail "$key must be an absolute executable"; done
root=$(get TMP_ROOT); [[ $root == /* && -d $root && -w $root ]] || fail 'temporary root must be absolute and writable'
prod_container=$(get PRODUCTION_CONTAINER_NAME); prod_network=$(get PRODUCTION_NETWORK)
[[ -n $prod_container && -n $prod_network ]] || fail 'production identity is required'
sha256_bin=$(get SHA256_BIN)
checksum=$("$sha256_bin" "$backup" 2>/dev/null | { IFS=' ' read -r sum _; printf '%s' "$sum"; }) || fail 'backup checksum failed'
[[ $checksum == "$expected_sha" ]] || fail 'backup checksum mismatch'
restore=$(get PG_RESTORE_BIN)
$restore --list "$backup" >/dev/null 2>&1 || fail 'pg_restore validation failed'
workdir=$(mktemp -d "$root/issue28-clone.XXXXXX") || fail 'cannot create isolated workspace'
chmod 700 "$workdir"
runner=$(get CLONE_FIXTURE_RUNNER)
set +e
report=$(CLONE_WORKDIR="$workdir" CLONE_READ_ONLY=1 PROBE_CONFIG="$cfg" PRODUCTION_CONTAINER_NAME="$prod_container" PRODUCTION_NETWORK="$prod_network" "$runner" "$release_id" "$backup" "$expected_sha" "$target_image" 2>/dev/null)
rc=$?
set -e
(( rc == 0 )) || fail 'fixture runner failed'
[[ $report != *$'\n'* ]] || fail 'runner returned multiple lines'
jq_bin=$(get JQ_BIN)
"$jq_bin" -e '((type == "object" and .success == true and .environment == "isolated_clone" and .clone_isolated == true and .production_identity_collision == false) and (.fixture | type == "object"))' <<<"$report" >/dev/null 2>&1 || fail 'runner did not prove isolated clone'
"$jq_bin" -e --arg sha "$expected_sha" --arg img "$target_image" '.source_backup_sha256 == $sha and .target_digest == $img and .cleanup_complete == true and .fixture.price_amount_micros == "40000000" and .fixture.plan_credit == 1000 and .fixture.consumed_credit == 200 and .fixture.available_credit == 800 and .fixture.end_time == 0 and .fixture.exact_cost_micros == "32000000" and .fixture.currency == "CNY" and .fixture.active_paid_subscription_count == 1 and .fixture.estimated_cost_micros == "0" and .fixture.unknown_credit == 0 and .fixture.five_analytics_endpoints_consistent == true' <<<"$report" >/dev/null 2>&1 || fail 'fixture evidence does not satisfy frozen contract'
printf '%s\n' "$report" | "$jq_bin" -c '{success:true,environment:"isolated_clone",source_backup_sha256:.source_backup_sha256,fixture:{price_amount_micros:.fixture.price_amount_micros,plan_credit:.fixture.plan_credit,consumed_credit:.fixture.consumed_credit,available_credit:.fixture.available_credit,end_time:.fixture.end_time,exact_cost_micros:.fixture.exact_cost_micros,currency:.fixture.currency,active_paid_subscription_count:.fixture.active_paid_subscription_count,estimated_cost_micros:.fixture.estimated_cost_micros,unknown_credit:.fixture.unknown_credit,five_analytics_endpoints_consistent:.fixture.five_analytics_endpoints_consistent}}'
out_done=1
