#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

out_done=0
fail() {
  local message=${1:-probe failed}
  if (( out_done == 0 )); then
    printf '{"success":false,"aggregated":false,"error":"probe_failed"}\n'
    out_done=1
  fi
  printf 'observe probe: %s\n' "$message" >&2
  exit 1
}
on_exit() {
  local rc=$?
  if (( rc != 0 && out_done == 0 )); then
    printf '{"success":false,"aggregated":false,"error":"probe_failed"}\n'
  fi
}
trap on_exit EXIT
[[ $# -eq 4 ]] || fail 'exactly RELEASE_ID OBSERVE_SECONDS TARGET_IMAGE EXPECTED_REVISION is required'
release_id=$1
seconds=$2
target_image=$3
expected_revision=$4
[[ $release_id =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,80}$ && $release_id != *..* ]] || fail 'invalid release id'
[[ $seconds =~ ^[1-9][0-9]{0,4}$ ]] || fail 'invalid observation window'
[[ $target_image =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ && $expected_revision =~ ^[0-9a-f]{40}$ ]] || fail 'invalid target identity'

cfg=${OBSERVE_CONFIG:-${RELEASE_PROBE_CONFIG:-}}
[[ $cfg == /* && -f $cfg ]] || fail 'absolute observe config is required'
declare -A C=()
while IFS= read -r line || [[ -n $line ]]; do
  [[ $line != *$'\r'* ]] || fail 'invalid config line'
  [[ -z $line || $line == \#* ]] && continue
  [[ $line =~ ^([A-Z][A-Z0-9_]*)=(.*)$ ]] || fail 'malformed config'
  key=${BASH_REMATCH[1]}; value=${BASH_REMATCH[2]}
  [[ -z ${C[$key]+x} ]] || fail 'duplicate config key'
  case $key in
    OBSERVE_EVIDENCE_RUNNER|OBSERVE_SLEEP_BIN|JQ_BIN|DOCKER_BIN|CURL_BIN|CONTAINER_NAME|POSTGRES_CONTAINER|DB_USER|DB_NAME|HEALTH_URL|RUNTIME_STATS_URL|RUNTIME_CURL_CONFIG|OBSERVE_STATE_DIR|OBSERVE_MAX_HEALTH_FAILURES|OBSERVE_MAX_PANICS|OBSERVE_MAX_LOCK_WAIT_REGRESSION|OBSERVE_MAX_WRITE_LOAD_REGRESSION) ;;
    *) fail 'unknown config key' ;;
  esac
  [[ "$value" != *'$('* && "$value" != *'`'* && "$value" != *';'* && "$value" != *'|'* && "$value" != *'&'* ]] || fail 'unsafe config value'
  C[$key]=$value
done < "$cfg"
get() { printf '%s' "${C[$1]-}"; }
for key in OBSERVE_EVIDENCE_RUNNER OBSERVE_SLEEP_BIN JQ_BIN DOCKER_BIN CURL_BIN; do
  value=$(get "$key")
  [[ $value == /* && -x $value ]] || fail "$key must be an absolute executable"
done
for key in OBSERVE_MAX_HEALTH_FAILURES OBSERVE_MAX_PANICS OBSERVE_MAX_LOCK_WAIT_REGRESSION OBSERVE_MAX_WRITE_LOAD_REGRESSION; do
  [[ $(get "$key") =~ ^[0-9]+$ ]] || fail 'invalid observe threshold'
done
runner=$(get OBSERVE_EVIDENCE_RUNNER)
jq_bin=$(get JQ_BIN)
before=$(OBSERVE_PHASE=baseline PROBE_CONFIG="$cfg" "$runner" "$release_id" "$target_image" "$expected_revision" 2>/dev/null) || fail 'baseline collection failed'
[[ $before != *$'\n'* ]] || fail 'baseline returned multiple lines'
"$jq_bin" -e --arg image "$target_image" --arg revision "$expected_revision" '.success == true and .digest == $image and .revision == $revision' <<<"$before" >/dev/null 2>&1 || fail 'baseline identity mismatch'
$(get OBSERVE_SLEEP_BIN) "$seconds" >/dev/null 2>&1 || fail 'observation wait failed'
after=$(OBSERVE_PHASE=window PROBE_CONFIG="$cfg" "$runner" "$release_id" "$target_image" "$expected_revision" 2>/dev/null) || fail 'window collection failed'
[[ $after != *$'\n'* ]] || fail 'window returned multiple lines'
"$jq_bin" -e --arg image "$target_image" --arg revision "$expected_revision" --argjson seconds "$seconds" 'type == "object" and .success == true and .digest == $image and .revision == $revision and .window_seconds >= $seconds and .digest_before == .digest_after and .revision_before == .revision_after and .health_failures == 0 and .credit_valuation_state_missing == 0 and .credit_valuation_state_mismatch == 0 and .unsupported_fx_errors == 0 and .panic_count == 0 and .abnormal_restarts == 0 and .unknown_credit_regression == false and .postgres_lock_wait_regression == false and .write_load_regression == false' <<<"$after" >/dev/null 2>&1 || fail 'observation thresholds or invariants failed'
printf '%s\n' "$after" | "$jq_bin" -c '{success:true,aggregated:true,digest:.digest,revision:.revision,window_seconds:.window_seconds,health_failures:.health_failures,credit_valuation_state_missing:.credit_valuation_state_missing,credit_valuation_state_mismatch:.credit_valuation_state_mismatch,unsupported_fx_errors:.unsupported_fx_errors,panic_count:.panic_count,abnormal_restarts:.abnormal_restarts,unknown_credit_growth:.unknown_credit_growth,unknown_credit_regression:.unknown_credit_regression,postgres_lock_waits_before:.postgres_lock_waits_before,postgres_lock_waits_after:.postgres_lock_waits_after,postgres_lock_wait_delta:.postgres_lock_wait_delta,postgres_lock_wait_regression:.postgres_lock_wait_regression,write_load_regression:.write_load_regression,postgres_write_delta:.postgres_write_delta,postgres_connection_delta:.postgres_connection_delta,postgres_connections_after:.postgres_connections_after,http_request_count:.http_request_count,http_error_count:.http_error_count,http_error_rate_ppm:.http_error_rate_ppm,settlement_replay_count:.settlement_replay_count,settlement_replay_growth:.settlement_replay_growth,settlement_max_latency_seconds:.settlement_max_latency_seconds,http_active_current:.http_active_current,batch_update_pending_total:.batch_update_pending_total,resource_snapshot:.resource_snapshot}'
out_done=1
