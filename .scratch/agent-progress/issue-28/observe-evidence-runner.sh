#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

fail() { printf 'observe evidence runner: %s\n' "${1:-failed}" >&2; exit 1; }
[[ $# -eq 3 ]] || fail 'exactly RELEASE_ID TARGET_IMAGE EXPECTED_REVISION is required'
release_id=$1
target_image=$2
expected_revision=$3
[[ $release_id =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,80}$ && $release_id != *..* ]] || fail 'invalid release id'
[[ $target_image =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ ]] || fail 'target image must be immutable'
[[ $expected_revision =~ ^[0-9a-f]{40}$ ]] || fail 'invalid revision'
[[ ${OBSERVE_PHASE:-} == baseline || ${OBSERVE_PHASE:-} == window ]] || fail 'OBSERVE_PHASE must be baseline or window'

cfg=${PROBE_CONFIG:-${OBSERVE_CONFIG:-}}
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
for key in JQ_BIN DOCKER_BIN CURL_BIN; do
  value=$(get "$key")
  [[ $value == /* && -x $value ]] || fail "$key must be an absolute executable"
done
container=$(get CONTAINER_NAME)
postgres=$(get POSTGRES_CONTAINER)
db_user=$(get DB_USER)
db_name=$(get DB_NAME)
[[ $container =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$ && $postgres =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$ ]] || fail 'invalid container identity'
[[ $db_user =~ ^[A-Za-z_][A-Za-z0-9_]*$ && $db_name =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || fail 'invalid database identity'
health_url=$(get HEALTH_URL)
runtime_url=$(get RUNTIME_STATS_URL)
[[ $health_url =~ ^http://127\.0\.0\.1(:[0-9]{1,5})?/ && $runtime_url =~ ^http://127\.0\.0\.1(:[0-9]{1,5})?/ ]] || fail 'observe URLs must be loopback'
curl_config=$(get RUNTIME_CURL_CONFIG)
[[ $curl_config == /* && -f $curl_config ]] || fail 'runtime curl config is required'
[[ $(stat -c '%a' "$curl_config" 2>/dev/null || printf 000) == 600 ]] || fail 'runtime curl config must have mode 0600'
state_dir=$(get OBSERVE_STATE_DIR)
[[ $state_dir == /* ]] || fail 'observe state directory must be absolute'
mkdir -p "$state_dir"
chmod 700 "$state_dir"
state_file="$state_dir/${release_id}.baseline.json"
for key in OBSERVE_MAX_HEALTH_FAILURES OBSERVE_MAX_PANICS OBSERVE_MAX_LOCK_WAIT_REGRESSION OBSERVE_MAX_WRITE_LOAD_REGRESSION; do
  value=$(get "$key")
  [[ $value =~ ^[0-9]+$ ]] || fail 'observe thresholds must be non-negative integers'
done

docker_bin=$(get DOCKER_BIN)
curl_bin=$(get CURL_BIN)
jq_bin=$(get JQ_BIN)

collect() {
  local observed inspect runtime db_values health_ok=0 stats
  observed=$(date +%s)
  inspect=$($docker_bin inspect --format '{{json .}}' "$container" 2>/dev/null) || fail 'application inspect failed'
  "$jq_bin" -e --arg image "$target_image" --arg revision "$expected_revision" '.State.Running == true and .State.Health.Status == "healthy" and ((.Config.Image == $image) or (.RepoDigests | index($image) != null)) and .Config.Labels["org.opencontainers.image.revision"] == $revision' <<<"$inspect" >/dev/null 2>&1 || fail 'application identity or health mismatch'
  if "$curl_bin" --config "$curl_config" "$health_url" 2>/dev/null | "$jq_bin" -e '.success == true' >/dev/null 2>&1; then
    health_ok=1
  fi
  runtime=$($curl_bin --config "$curl_config" "$runtime_url" 2>/dev/null) || fail 'runtime endpoint failed'
  "$jq_bin" -e 'type == "object" and (.http_active_current | type == "number" and . >= 0) and (.batch_update_pending_total | type == "number" and . >= 0) and .batch_update.status == "ok"' <<<"$runtime" >/dev/null 2>&1 || fail 'runtime contract failed'
  db_values=$($docker_bin exec "$postgres" psql -AtX -F '|' -U "$db_user" -d "$db_name" -c "WITH marker AS (SELECT version, valuation_currency FROM credit_valuation_migrations WHERE status = 'ready' ORDER BY version DESC LIMIT 1), credit AS (SELECT COUNT(*) FILTER (WHERE v.user_subscription_id IS NULL) AS missing, COUNT(*) FILTER (WHERE v.user_subscription_id IS NOT NULL AND (v.user_id <> s.user_id OR v.available_credit <> GREATEST(s.token_limit - s.token_used, 0) OR v.exact_cost_micros < 0 OR v.estimated_cost_micros < 0 OR v.unknown_credit < 0 OR v.unknown_credit > GREATEST(s.token_limit - s.token_used, 0) OR v.currency <> (SELECT valuation_currency FROM marker) OR v.rule_version <> 1 OR v.migration_version <> (SELECT version FROM marker) OR v.state_version < 0)) + (SELECT COUNT(*) FROM credit_valuation_states orphan LEFT JOIN user_subscriptions owner ON owner.id = orphan.user_subscription_id WHERE owner.id IS NULL) AS mismatch, COALESCE(SUM(v.unknown_credit), 0) AS unknown_credit FROM user_subscriptions s LEFT JOIN credit_valuation_states v ON v.user_subscription_id = s.id WHERE s.entitlement_type = 'credit_balance'), locks AS (SELECT COUNT(*) AS waiting FROM pg_locks WHERE NOT granted), activity AS (SELECT COALESCE(xact_commit + xact_rollback, 0) AS transactions, COALESCE(tup_inserted + tup_updated + tup_deleted, 0) AS writes, numbackends AS connections FROM pg_stat_database WHERE datname = current_database()), settlement AS (SELECT COUNT(*) FILTER (WHERE settlement_version > 1) AS replays, COALESCE(MAX(CASE WHEN finalized_at > 0 THEN finalized_at - created_at ELSE 0 END), 0) AS max_latency FROM subscription_pre_consume_records) SELECT credit.missing, credit.mismatch, credit.unknown_credit, locks.waiting, activity.transactions, activity.writes, activity.connections, settlement.replays, settlement.max_latency FROM credit, locks, activity, settlement;") || fail 'observe database snapshot failed'
  IFS='|' read -r missing mismatch unknown lock_waits transactions writes connections settlement_replays settlement_latency <<<"$db_values"
  for value in "$missing" "$mismatch" "$unknown" "$lock_waits" "$transactions" "$writes" "$connections" "$settlement_replays" "$settlement_latency"; do
    [[ $value =~ ^[0-9]+$ ]] || fail 'observe database snapshot is invalid'
  done
  stats=$($docker_bin stats --no-stream --format '{{json .}}' "$container" 2>/dev/null) || fail 'resource snapshot failed'
  "$jq_bin" -e 'type == "object" and (.CPUPerc | type == "string" and length > 0) and (.MemUsage | type == "string" and length > 0) and (.MemPerc | type == "string" and length > 0) and (.BlockIO | type == "string" and length > 0)' <<<"$stats" >/dev/null 2>&1 || fail 'resource snapshot contract failed'
  "$jq_bin" -cn \
    --arg digest "$target_image" --arg revision "$expected_revision" --argjson observed "$observed" \
    --argjson restart_count "$("$jq_bin" -r '.RestartCount // 0' <<<"$inspect")" --argjson health_ok "$health_ok" \
    --argjson missing "$missing" --argjson mismatch "$mismatch" --argjson unknown "$unknown" \
    --argjson lock_waits "$lock_waits" --argjson transactions "$transactions" --argjson writes "$writes" --argjson connections "$connections" \
    --argjson settlement_replays "$settlement_replays" --argjson settlement_latency "$settlement_latency" \
    --argjson http_active "$("$jq_bin" -r '.http_active_current' <<<"$runtime")" --argjson batch_pending "$("$jq_bin" -r '.batch_update_pending_total' <<<"$runtime")" --argjson stats "$stats" \
    '{success:true,digest:$digest,revision:$revision,observed_at:$observed,restart_count:$restart_count,health_ok:$health_ok,credit_valuation_state_missing:$missing,credit_valuation_state_mismatch:$mismatch,unknown_credit:$unknown,postgres_lock_waits:$lock_waits,postgres_transactions:$transactions,postgres_writes:$writes,postgres_connections:$connections,settlement_replay_count:$settlement_replays,settlement_max_latency_seconds:$settlement_latency,http_active_current:$http_active,batch_update_pending_total:$batch_pending,resource_snapshot:$stats}'
}

snapshot=$(collect)
if [[ $OBSERVE_PHASE == baseline ]]; then
  temporary="${state_file}.tmp.$$"
  printf '%s\n' "$snapshot" > "$temporary"
  chmod 600 "$temporary"
  mv -f "$temporary" "$state_file"
  printf '%s\n' "$snapshot"
  exit 0
fi

[[ -f $state_file && $(stat -c '%a' "$state_file" 2>/dev/null || printf 000) == 600 ]] || fail 'baseline state is missing or unsafe'
before=$(cat "$state_file")
"$jq_bin" -e --arg image "$target_image" --arg revision "$expected_revision" '.success == true and .digest == $image and .revision == $revision' <<<"$before" >/dev/null 2>&1 || fail 'baseline identity mismatch'
start=$("$jq_bin" -r '.observed_at' <<<"$before")
end=$("$jq_bin" -r '.observed_at' <<<"$snapshot")
window=$((end - start))
(( window > 0 )) || fail 'observation window did not advance'
logs=$($docker_bin logs --since "$start" "$container" 2>/dev/null) || fail 'container logs collection failed'
unsupported=$(awk 'BEGIN{IGNORECASE=1} index($0,"credit_valuation_unsupported_currency") || index($0,"credit_valuation_invalid_fx") {n++} END{print n+0}' <<<"$logs")
panics=$(awk 'BEGIN{IGNORECASE=1} index($0,"panic detected") {n++} END{print n+0}' <<<"$logs")
state_errors=$(awk 'BEGIN{IGNORECASE=1} index($0,"credit_valuation_state_missing") || index($0,"credit_valuation_state_mismatch") {n++} END{print n+0}' <<<"$logs")
http_total=$(awk 'BEGIN{IGNORECASE=1} /^\[GIN\]/ {n++} END{print n+0}' <<<"$logs")
http_errors=$(awk 'BEGIN{IGNORECASE=1} /^\[GIN\]/ { for (i=1;i<=NF;i++) if ($i ~ /^[45][0-9][0-9]$/) {n++; break} } END{print n+0}' <<<"$logs")
error_rate_ppm=0
(( http_total == 0 )) || error_rate_ppm=$((http_errors * 1000000 / http_total))
unknown_growth=$(( $("$jq_bin" -r '.unknown_credit' <<<"$snapshot") - $("$jq_bin" -r '.unknown_credit' <<<"$before") ))
write_delta=$(( $("$jq_bin" -r '.postgres_writes' <<<"$snapshot") - $("$jq_bin" -r '.postgres_writes' <<<"$before") ))
(( write_delta >= 0 )) || fail 'PostgreSQL write counter decreased'
lock_waits_before="$("$jq_bin" -r '.postgres_lock_waits' <<<"$before")"
lock_waits_after="$("$jq_bin" -r '.postgres_lock_waits' <<<"$snapshot")"
lock_growth=$((lock_waits_after - lock_waits_before))
(( lock_waits_before >= 0 && lock_waits_after >= 0 )) || fail 'PostgreSQL lock-wait gauge is invalid'
restart_delta=$(( $("$jq_bin" -r '.restart_count' <<<"$snapshot") - $("$jq_bin" -r '.restart_count' <<<"$before") ))
(( restart_delta >= 0 )) || fail 'restart counter decreased'
connection_delta=$(( $("$jq_bin" -r '.postgres_connections' <<<"$snapshot") - $("$jq_bin" -r '.postgres_connections' <<<"$before") ))
settlement_replay_growth=$(( $("$jq_bin" -r '.settlement_replay_count' <<<"$snapshot") - $("$jq_bin" -r '.settlement_replay_count' <<<"$before") ))
(( settlement_replay_growth >= 0 )) || fail 'settlement replay counter decreased'
health_failures=0
(( $("$jq_bin" -r '.health_ok' <<<"$before") == 1 && $("$jq_bin" -r '.health_ok' <<<"$snapshot") == 1 )) || health_failures=1
(( health_failures <= $(get OBSERVE_MAX_HEALTH_FAILURES) )) || fail 'health failure threshold exceeded'
(( panics <= $(get OBSERVE_MAX_PANICS) )) || fail 'panic threshold exceeded'
unknown_regression=false
lock_regression=false
write_regression=false
# pg_locks is an instantaneous gauge: falling to zero is healthy. Only an
# end-of-window increase beyond tolerance is a regression.
(( lock_growth <= $(get OBSERVE_MAX_LOCK_WAIT_REGRESSION) )) || lock_regression=true
(( write_delta <= $(get OBSERVE_MAX_WRITE_LOAD_REGRESSION) )) || write_regression=true
"$jq_bin" -cn --argjson before "$before" --argjson after "$snapshot" --argjson window "$window" \
  --argjson health_failures "$health_failures" --argjson unsupported "$unsupported" --argjson panics "$panics" \
  --argjson state_errors "$state_errors" --argjson restart_delta "$restart_delta" --argjson unknown_growth "$unknown_growth" \
  --argjson write_delta "$write_delta" --argjson lock_waits_before "$lock_waits_before" --argjson lock_waits_after "$lock_waits_after" --argjson lock_growth "$lock_growth" --argjson connection_delta "$connection_delta" \
  --argjson http_total "$http_total" --argjson http_errors "$http_errors" --argjson error_rate_ppm "$error_rate_ppm" --argjson settlement_replay_growth "$settlement_replay_growth" \
  --argjson unknown_regression "$unknown_regression" --argjson lock_regression "$lock_regression" --argjson write_regression "$write_regression" \
  '{success:true,digest:$after.digest,revision:$after.revision,digest_before:$before.digest,digest_after:$after.digest,revision_before:$before.revision,revision_after:$after.revision,window_seconds:$window,health_failures:$health_failures,credit_valuation_state_missing:$after.credit_valuation_state_missing,credit_valuation_state_mismatch:($after.credit_valuation_state_mismatch + $state_errors),unsupported_fx_errors:$unsupported,panic_count:$panics,abnormal_restarts:$restart_delta,unknown_credit_before:$before.unknown_credit,unknown_credit_after:$after.unknown_credit,unknown_credit_growth:$unknown_growth,unknown_credit_regression:$unknown_regression,postgres_lock_waits_before:$lock_waits_before,postgres_lock_waits_after:$lock_waits_after,postgres_lock_wait_delta:$lock_growth,postgres_write_delta:$write_delta,postgres_connection_delta:$connection_delta,postgres_connections_after:$after.postgres_connections,postgres_lock_wait_regression:$lock_regression,write_load_regression:$write_regression,http_request_count:$http_total,http_error_count:$http_errors,http_error_rate_ppm:$error_rate_ppm,settlement_replay_count:$after.settlement_replay_count,settlement_replay_growth:$settlement_replay_growth,settlement_max_latency_seconds:$after.settlement_max_latency_seconds,http_active_current:$after.http_active_current,batch_update_pending_total:$after.batch_update_pending_total,resource_snapshot:$after.resource_snapshot}'
