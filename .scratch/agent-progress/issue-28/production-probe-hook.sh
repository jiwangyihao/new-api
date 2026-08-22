#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

out_done=0
fail() {
  local message=${1:-probe failed}
  if (( out_done == 0 )); then
    printf '{"success":false,"environment":"production","error":"probe_failed"}\n'
    out_done=1
  fi
  printf 'production probe: %s\n' "$message" >&2
  exit 1
}
on_exit() {
  local rc=$?
  if (( rc != 0 && out_done == 0 )); then
    printf '{"success":false,"environment":"production","error":"probe_failed"}\n'
  fi
}
trap on_exit EXIT
[[ $# -eq 4 ]] || fail 'exactly RELEASE_ID TARGET_IMAGE EXPECTED_REVISION MIGRATION_VERSION is required'
release_id=$1
target_image=$2
expected_revision=$3
migration_version=$4
[[ $release_id =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,80}$ && $release_id != *..* ]] || fail 'invalid release id'
[[ $target_image =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ ]] || fail 'target image must be immutable digest'
[[ $expected_revision =~ ^[0-9a-f]{40}$ ]] || fail 'invalid revision'
[[ $migration_version =~ ^[1-9][0-9]*$ ]] || fail 'invalid migration version'

cfg=${PRODUCTION_PROBE_CONFIG:-${RELEASE_PROBE_CONFIG:-}}
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
for key in DOCKER_BIN JQ_BIN CURL_BIN PRODUCTION_EVIDENCE_RUNNER; do
  value=$(get "$key")
  [[ $value == /* && -x $value ]] || fail "$key must be an absolute executable"
done
container=$(get CONTAINER_NAME)
[[ $container =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$ ]] || fail 'invalid container name'
credentials=$(get AUTH_CREDENTIALS_FILE)
[[ $credentials == /* && -f $credentials ]] || fail 'credentials file is required'
[[ $(stat -c '%a' "$credentials" 2>/dev/null || printf 000) == 600 ]] || fail 'credentials file must have mode 0600'
api=$(get PRODUCTION_API_URL)
[[ $api == http://127.0.0.1:13080 ]] || fail 'production API must be the audited loopback origin'

docker_bin=$(get DOCKER_BIN)
jq_bin=$(get JQ_BIN)
inspect=$($docker_bin inspect --format '{{json .}}' "$container" 2>/dev/null) || fail 'container inspection failed'
"$jq_bin" -e --arg image "$target_image" --arg revision "$expected_revision" '.State.Running == true and .State.Health.Status == "healthy" and ((.Config.Image == $image) or (.RepoDigests | index($image) != null)) and .Config.Labels["org.opencontainers.image.revision"] == $revision' <<<"$inspect" >/dev/null 2>&1 || fail 'running container identity mismatch'

runner=$(get PRODUCTION_EVIDENCE_RUNNER)
set +e
report=$(PROBE_READ_ONLY=1 PROBE_CONFIG="$cfg" PRODUCTION_API_URL="$api" PRODUCTION_SNAPSHOT_AT="${PRODUCTION_SNAPSHOT_AT:-}" AUTH_CREDENTIALS_FILE="$credentials" "$runner" "$release_id" "$target_image" "$expected_revision" "$migration_version" 2>/dev/null)
rc=$?
set -e
(( rc == 0 )) || fail 'production evidence runner failed'
[[ $report != *$'\n'* ]] || fail 'production evidence runner returned multiple lines'
"$jq_bin" -e --arg image "$target_image" --arg revision "$expected_revision" --argjson version "$migration_version" 'type == "object" and .success == true and .read_only == true and .digest == $image and .revision == $revision and .marker_status == "ready" and .migration_version == $version and .invariants == true and .authenticated_api == true and .administrator_identity == true and .paid_value_endpoints_structurally_consistent == true and (.snapshot_at | type == "number" and . > 0) and .api_origin == "http://127.0.0.1:13080" and .browser_evidence_required == true' <<<"$report" >/dev/null 2>&1 || fail 'production evidence contract failed'
printf '%s\n' "$report" | "$jq_bin" -c '{success:true,environment:"production",read_only:true,digest:.digest,revision:.revision,marker_status:.marker_status,migration_version:.migration_version,invariants:.invariants,authenticated_api:.authenticated_api,administrator_identity:.administrator_identity,paid_value_endpoints_structurally_consistent:.paid_value_endpoints_structurally_consistent,snapshot_at:.snapshot_at,api_origin:.api_origin,browser_evidence_required:.browser_evidence_required}'
out_done=1
