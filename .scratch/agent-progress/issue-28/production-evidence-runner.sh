#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

fail() { printf 'production evidence runner: %s\n' "${1:-failed}" >&2; exit 1; }
[[ ${PROBE_READ_ONLY:-} == 1 ]] || fail 'PROBE_READ_ONLY=1 is required'
[[ $# -eq 4 ]] || fail 'exactly RELEASE_ID TARGET_IMAGE EXPECTED_REVISION MIGRATION_VERSION is required'
release_id=$1
target_image=$2
expected_revision=$3
migration_version=$4
[[ $release_id =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,80}$ && $release_id != *..* ]] || fail 'invalid release id'
[[ $target_image =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ ]] || fail 'target image must be immutable'
[[ $expected_revision =~ ^[0-9a-f]{40}$ ]] || fail 'invalid revision'
[[ $migration_version =~ ^[1-9][0-9]*$ ]] || fail 'invalid migration version'

cfg=${PROBE_CONFIG:-}
[[ $cfg == /* && -f $cfg ]] || fail 'absolute probe config is required'
declare -A C=()
while IFS= read -r line || [[ -n $line ]]; do
  [[ $line != *$'\r'* ]] || fail 'invalid config line'
  [[ -z $line || $line == \#* ]] && continue
  [[ $line =~ ^([A-Z][A-Z0-9_]*)=(.*)$ ]] || fail 'malformed config'
  key=${BASH_REMATCH[1]}; value=${BASH_REMATCH[2]}
  [[ -z ${C[$key]+x} ]] || fail 'duplicate config key'
  case $key in
    DOCKER_BIN|JQ_BIN|CURL_BIN|CONTAINER_NAME|PRODUCTION_EVIDENCE_RUNNER|AUTH_CREDENTIALS_FILE|PRODUCTION_API_URL|PRODUCTION_TIMEOUT_SECONDS) ;;
    *) fail 'unknown config key' ;;
  esac
  [[ "$value" != *'$('* && "$value" != *'`'* && "$value" != *';'* && "$value" != *'|'* && "$value" != *'&'* ]] || fail 'unsafe config value'
  C[$key]=$value
done < "$cfg"
get() { printf '%s' "${C[$1]-}"; }
for key in DOCKER_BIN JQ_BIN CURL_BIN; do
  value=$(get "$key")
  [[ $value == /* && -x $value ]] || fail "$key must be an absolute executable"
done
container=$(get CONTAINER_NAME)
[[ $container =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$ ]] || fail 'invalid container name'
timeout=$(get PRODUCTION_TIMEOUT_SECONDS)
[[ $timeout =~ ^[1-9][0-9]{0,3}$ ]] || fail 'invalid production timeout'
api=${PRODUCTION_API_URL:-$(get PRODUCTION_API_URL)}
[[ $api == http://127.0.0.1:13080 ]] || fail 'production API must be the audited loopback origin'
snapshot=${PRODUCTION_SNAPSHOT_AT:-}
[[ $snapshot =~ ^[1-9][0-9]*$ ]] || fail 'PRODUCTION_SNAPSHOT_AT must be a positive integer'
credentials=${AUTH_CREDENTIALS_FILE:-$(get AUTH_CREDENTIALS_FILE)}
[[ $credentials == /* && -f $credentials ]] || fail 'credentials file is required'
[[ $(stat -c '%a' "$credentials" 2>/dev/null || printf 000) == 600 ]] || fail 'credentials file must have mode 0600'

declare -A A=()
while IFS= read -r line || [[ -n $line ]]; do
  [[ $line != *$'\r'* ]] || fail 'invalid credentials line'
  [[ -z $line || $line == \#* ]] && continue
  [[ $line =~ ^(ACCESS_TOKEN|USER_ID)=(.*)$ ]] || fail 'unknown credentials key'
  key=${BASH_REMATCH[1]}; value=${BASH_REMATCH[2]}
  [[ -z ${A[$key]+x} ]] || fail 'duplicate credentials key'
  A[$key]=$value
done < "$credentials"
access_token=${A[ACCESS_TOKEN]-}
user_id=${A[USER_ID]-}
[[ $access_token =~ ^[A-Za-z0-9_-]{16,128}$ && $user_id =~ ^[1-9][0-9]*$ ]] || fail 'invalid authorized probe identity'

docker_bin=$(get DOCKER_BIN)
jq_bin=$(get JQ_BIN)
curl_bin=$(get CURL_BIN)
verify=$($docker_bin exec "$container" /new-api credit-valuation-migrate --verify --version "$migration_version" 2>/dev/null) || fail 'container verify failed'
"$jq_bin" -e --argjson version "$migration_version" '
  .success == true and .report.version == $version and
  .report.mode == "verify" and .report.status == "ready" and
  .report.read_only == true and .report.changed == false and .report.ready == true and
  (.report.blockers | type == "array" and length == 0) and
  .report.price.rows_invalid == 0 and
  (.report.checksum | type == "string" and test("^[0-9a-f]{64}$"))
' <<<"$verify" >/dev/null 2>&1 || fail 'container verify contract failed'

tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/issue28-production-probe.XXXXXX")
cleanup() {
  local rc=$?
  trap - EXIT HUP INT TERM
  rm -rf -- "$tmpdir"
  exit "$rc"
}
trap cleanup EXIT HUP INT TERM
curl_config="$tmpdir/curl.conf"
{
  printf 'silent\nshow-error\nfail-with-body\nmax-time = "%s"\n' "$timeout"
  printf 'header = "Authorization: Bearer %s"\n' "$access_token"
  printf 'header = "New-Api-User: %s"\n' "$user_id"
} > "$curl_config"
chmod 600 "$curl_config"
request() { "$curl_bin" --config "$curl_config" "$1"; }

status=$(request "$api/api/status") || fail 'status request failed'
"$jq_bin" -e '.success == true' <<<"$status" >/dev/null 2>&1 || fail 'status response failed'
self=$(request "$api/api/user/self") || fail 'authenticated self request failed'
"$jq_bin" -e --argjson user_id "$user_id" '
  .success == true and (.data | type == "object") and
  .data.id == $user_id and (.data.role == 10 or .data.role == 100) and .data.status == 1
' <<<"$self" >/dev/null 2>&1 || fail 'authorized probe identity is not an enabled administrator'

query="snapshot_at=$snapshot&currency=CNY&limit=20"
summary_file="$tmpdir/summary.json"
users_file="$tmpdir/users.json"
subscriptions_file="$tmpdir/subscriptions.json"
plans_file="$tmpdir/plans.json"
sources_file="$tmpdir/sources.json"
request "$api/api/admin-analytics/paid-subscription-value/summary?$query" > "$summary_file" || fail 'paid-value summary request failed'
request "$api/api/admin-analytics/paid-subscription-value/users?$query" > "$users_file" || fail 'paid-value users request failed'
request "$api/api/admin-analytics/paid-subscription-value/subscriptions?$query" > "$subscriptions_file" || fail 'paid-value subscriptions request failed'
request "$api/api/admin-analytics/paid-subscription-value/breakdown/plans?$query" > "$plans_file" || fail 'paid-value plans request failed'
request "$api/api/admin-analytics/paid-subscription-value/breakdown/sources?$query" > "$sources_file" || fail 'paid-value sources request failed'

"$jq_bin" -e --argjson snapshot "$snapshot" '
  def money:
    type == "array" and
    all(.[];
      (.currency | type == "string" and test("^[A-Z]{3}$")) and
      (.amount_micros | type == "string" and test("^-?[0-9]+$"))
    );
  .success == true and .data.range.snapshot_at == $snapshot and
  (.data.data.summary | type == "object") and
  (.data.data.summary.recognized_remaining_value_by_currency | money) and
  (.data.data.summary.token_based_value_by_currency | money) and
  (.data.data.summary.time_based_value_by_currency | money) and
  (.data.data.summary.exact_remaining_value_by_currency | money) and
  (.data.data.summary.estimated_remaining_value_by_currency | money) and
  ([
    .data.data.summary.active_paid_subscription_count,
    .data.data.summary.active_paid_user_count,
    .data.data.summary.token_value_unavailable_count,
    .data.data.summary.unknown_cost_credit,
    .data.data.summary.unknown_timed_subscription_count,
    .data.data.summary.credit_valuation_state_missing_count
  ] | all(type == "number" and . >= 0 and floor == .)) and
  .data.data.summary.credit_valuation_state_missing_count == 0
' "$summary_file" >/dev/null || fail 'paid-value summary contract failed'

"$jq_bin" -e --argjson snapshot "$snapshot" '
  def money:
    type == "array" and
    all(.[];
      (.currency | type == "string" and test("^[A-Z]{3}$")) and
      (.amount_micros | type == "string" and test("^-?[0-9]+$"))
    );
  def page:
    type == "object" and (.limit | type == "number") and
    (.offset | type == "number") and (.total | type == "number" and . >= 0) and
    (.has_more | type == "boolean");
  .success == true and .data.range.snapshot_at == $snapshot and
  (.data.data.users.page | page) and
  (.data.data.users.items | type == "array" and all(.[];
    (.user_id | type == "number" and . > 0) and
    (.recognized_remaining_value_by_currency | money) and
    (.exact_remaining_value_by_currency | money) and
    (.estimated_remaining_value_by_currency | money) and
    (.unknown_cost_credit | type == "number" and . >= 0)
  ))
' "$users_file" >/dev/null || fail 'paid-value users contract failed'

"$jq_bin" -e --argjson snapshot "$snapshot" '
  def page:
    type == "object" and (.limit | type == "number") and
    (.offset | type == "number") and (.total | type == "number" and . >= 0) and
    (.has_more | type == "boolean");
  .success == true and .data.range.snapshot_at == $snapshot and
  (.data.data.subscriptions.page | page) and
  (.data.data.subscriptions.items | type == "array" and all(.[];
    (.subscription_id | type == "number" and . > 0) and
    (.user_id | type == "number" and . > 0) and
    (.plan_id | type == "number" and . > 0) and
    (.end_time | type == "number") and
    (.available_credit | type == "number" and . >= 0) and
    (.unknown_cost_credit | type == "number" and . >= 0) and
    (.exact_remaining_value.amount_micros | type == "string" and test("^-?[0-9]+$")) and
    (.estimated_remaining_value.amount_micros | type == "string" and test("^-?[0-9]+$")) and
    (if .entitlement_type == "credit_balance" then .end_time == 0 and .time_based_value == null else true end)
  ))
' "$subscriptions_file" >/dev/null || fail 'paid-value subscriptions contract failed'

"$jq_bin" -e --argjson snapshot "$snapshot" '
  def money:
    type == "array" and
    all(.[];
      (.currency | type == "string" and test("^[A-Z]{3}$")) and
      (.amount_micros | type == "string" and test("^-?[0-9]+$"))
    );
  def page:
    type == "object" and (.limit | type == "number") and
    (.offset | type == "number") and (.total | type == "number" and . >= 0) and
    (.has_more | type == "boolean");
  .success == true and .data.range.snapshot_at == $snapshot and
  (.data.data.plans.page | page) and
  (.data.data.plans.items | type == "array" and all(.[];
    (.plan_id | type == "number" and . > 0) and
    (.recognized_remaining_value_by_currency | money) and
    (.exact_remaining_value_by_currency | money) and
    (.estimated_remaining_value_by_currency | money) and
    (.unknown_cost_credit | type == "number" and . >= 0)
  ))
' "$plans_file" >/dev/null || fail 'paid-value plans contract failed'

"$jq_bin" -e --argjson snapshot "$snapshot" '
  def money:
    type == "array" and
    all(.[];
      (.currency | type == "string" and test("^[A-Z]{3}$")) and
      (.amount_micros | type == "string" and test("^-?[0-9]+$"))
    );
  def page:
    type == "object" and (.limit | type == "number") and
    (.offset | type == "number") and (.total | type == "number" and . >= 0) and
    (.has_more | type == "boolean");
  .success == true and .data.range.snapshot_at == $snapshot and
  (.data.data.sources.page | page) and
  (.data.data.sources.items | type == "array" and all(.[];
    (.source | type == "string" and length > 0) and
    (.recognized_remaining_value_by_currency | money) and
    (.exact_remaining_value_by_currency | money) and
    (.estimated_remaining_value_by_currency | money) and
    (.unknown_cost_credit | type == "number" and . >= 0)
  ))
' "$sources_file" >/dev/null || fail 'paid-value sources contract failed'

printf '{"success":true,"read_only":true,"digest":"%s","revision":"%s","marker_status":"ready","migration_version":%s,"invariants":true,"authenticated_api":true,"administrator_identity":true,"paid_value_endpoints_structurally_consistent":true,"snapshot_at":%s,"api_origin":"http://127.0.0.1:13080","browser_evidence_required":true}\n' \
  "$target_image" "$expected_revision" "$migration_version" "$snapshot"
