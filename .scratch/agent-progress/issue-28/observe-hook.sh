#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
out_done=0
fail(){ if (( out_done == 0 )); then printf '{"success":false,"aggregated":false,"error":"probe_failed"}\n'; out_done=1; fi; printf 'observe probe: %s\n' "${1:-probe failed}" >&2; exit 1; }
trap 'rc=$?; if (( rc != 0 && out_done == 0 )); then printf '\''{"success":false,"aggregated":false,"error":"probe_failed"}\''\n'; fi' EXIT
[[ $# -eq 4 ]] || fail 'exactly RELEASE_ID OBSERVE_SECONDS TARGET_IMAGE EXPECTED_REVISION is required'
release_id=$1; seconds=$2; target_image=$3; expected_revision=$4
[[ $release_id =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,80}$ && $release_id != *..* ]] || fail 'invalid release id'
[[ $seconds =~ ^[1-9][0-9]{0,4}$ ]] || fail 'observe window must be positive'
[[ $target_image =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ ]] || fail 'target image must be immutable digest'
[[ $expected_revision =~ ^[0-9a-f]{40}$ ]] || fail 'invalid revision'
cfg=${OBSERVE_CONFIG:-${RELEASE_PROBE_CONFIG:-}}; [[ $cfg == /* && -f $cfg ]] || fail 'absolute probe config is required'
declare -A C=()
while IFS= read -r line || [[ -n $line ]]; do
 [[ $line != *$'\r'* ]] || fail 'invalid config line'; [[ -z $line || $line == \#* ]] && continue
 [[ $line =~ ^([A-Z][A-Z0-9_]*)=(.*)$ ]] || fail 'malformed config'; key=${BASH_REMATCH[1]}; val=${BASH_REMATCH[2]}
 [[ "$val" != *'$('* && "$val" != *'`'* && "$val" != *';'* && "$val" != *'|'* && "$val" != *'&'* ]] || fail 'unsafe config value'
 case $key in OBSERVE_EVIDENCE_RUNNER|OBSERVE_SLEEP_BIN|JQ_BIN|OBSERVE_MAX_HEALTH_FAILURES|OBSERVE_MAX_PANICS|OBSERVE_MAX_LOCK_WAIT_REGRESSION|OBSERVE_MAX_WRITE_LOAD_REGRESSION) ;; *) fail 'unknown config key' ;; esac
 [[ -z ${C[$key]+x} ]] || fail 'duplicate config key'; C[$key]=$val
done < "$cfg"
get(){ printf '%s' "${C[$1]-}"; }
for key in OBSERVE_EVIDENCE_RUNNER OBSERVE_SLEEP_BIN JQ_BIN; do v=$(get "$key"); [[ $v == /* && -x $v ]] || fail "$key must be an absolute executable"; done
for key in OBSERVE_MAX_HEALTH_FAILURES OBSERVE_MAX_PANICS OBSERVE_MAX_LOCK_WAIT_REGRESSION OBSERVE_MAX_WRITE_LOAD_REGRESSION; do v=$(get "$key"); [[ $v =~ ^[0-9]+$ ]] || fail 'invalid threshold'; done
runner=$(get OBSERVE_EVIDENCE_RUNNER)
before=$(OBSERVE_PHASE=baseline "$runner" "$release_id" "$target_image" "$expected_revision" 2>/dev/null) || fail 'baseline collection failed'
[[ $before != *$'\n'* ]] || fail 'baseline returned multiple lines'
jq_bin=$(get JQ_BIN)
sleep_bin=$(get OBSERVE_SLEEP_BIN)
"$jq_bin" -e --arg img "$target_image" --arg rev "$expected_revision" 'type=="object" and .success==true and .digest==$img and .revision==$rev' <<<"$before" >/dev/null 2>&1 || fail 'baseline identity mismatch'
"$sleep_bin" "$seconds" >/dev/null 2>&1 || fail 'observation wait failed'
after=$(OBSERVE_PHASE=window "$runner" "$release_id" "$target_image" "$expected_revision" 2>/dev/null) || fail 'window collection failed'
[[ $after != *$'\n'* ]] || fail 'window returned multiple lines'
"$jq_bin" -e --arg img "$target_image" --arg rev "$expected_revision" --argjson s "$seconds" 'type=="object" and .success==true and .digest==$img and .revision==$rev and (.window_seconds|type=="number") and .window_seconds >= $s and .digest_before==.digest_after and .revision_before==.revision_after and .health_failures==0 and .credit_valuation_state_missing==0 and .credit_valuation_state_mismatch==0 and .unsupported_fx_errors==0 and .panic_count==0 and .abnormal_restarts==0 and .postgres_lock_wait_regression==false and .write_load_regression==false' <<<"$after" >/dev/null 2>&1 || fail 'observation thresholds or invariants failed'
printf '%s\n' "$after" | "$jq_bin" -c '{success:true,aggregated:true,window_seconds:.window_seconds,health_failures:.health_failures,credit_valuation_state_missing:.credit_valuation_state_missing,credit_valuation_state_mismatch:.credit_valuation_state_mismatch,unsupported_fx_errors:.unsupported_fx_errors,panic_count:.panic_count,abnormal_restarts:.abnormal_restarts,postgres_lock_wait_regression:.postgres_lock_wait_regression,write_load_regression:.write_load_regression}'
out_done=1
