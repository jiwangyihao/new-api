#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

out_done=0
fail() { local msg=${1:-probe failed}; if (( out_done == 0 )); then printf '{"success":false,"environment":"production","error":"probe_failed"}\n'; out_done=1; fi; printf 'production probe: %s\n' "$msg" >&2; exit 1; }
on_exit() {
  local rc=$?
  if (( rc != 0 && out_done == 0 )); then
    printf '{"success":false,"environment":"production","error":"probe_failed"}\n'
  fi
}
trap on_exit EXIT
[[ $# -eq 4 ]] || fail 'exactly RELEASE_ID TARGET_IMAGE EXPECTED_REVISION MIGRATION_VERSION is required'
release_id=$1; target_image=$2; expected_revision=$3; migration_version=$4
[[ $release_id =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,80}$ && $release_id != *..* ]] || fail 'invalid release id'
[[ $target_image =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ ]] || fail 'target image must be immutable digest'
[[ $expected_revision =~ ^[0-9a-f]{40}$ ]] || fail 'invalid revision'
[[ $migration_version =~ ^[0-9]+$ ]] || fail 'invalid migration version'

cfg=${PRODUCTION_PROBE_CONFIG:-${RELEASE_PROBE_CONFIG:-}}
[[ -n $cfg && $cfg == /* && -f $cfg ]] || fail 'absolute probe config is required'
# The adapter reads, but never sources, the config.  Reject shell syntax and unknown/duplicate keys.
declare -A C=()
while IFS= read -r line || [[ -n $line ]]; do
  [[ $line != *$'\r'* ]] || fail 'invalid config line'
  [[ -z $line || $line == \#* ]] && continue
  [[ $line =~ ^([A-Z][A-Z0-9_]*)=(.*)$ ]] || fail 'malformed config'
  key=${BASH_REMATCH[1]}; val=${BASH_REMATCH[2]}
  if [[ "$val" == *'$('* || "$val" == *$'\x60'* || "$val" == *';'* || "$val" == *'|'* || "$val" == *'&'* ]]; then fail 'unsafe config value'; fi
  case $key in DOCKER_BIN|JQ_BIN|CONTAINER_NAME|PRODUCTION_EVIDENCE_RUNNER|AUTH_CREDENTIALS_FILE|PRODUCTION_API_URL|PRODUCTION_FRONTEND_URL|PRODUCTION_DSN|PRODUCTION_TIMEOUT_SECONDS) ;; *) fail 'unknown config key' ;; esac
  [[ -z ${C[$key]+x} ]] || fail 'duplicate config key'
  C[$key]=$val
done < "$cfg"
get() { printf '%s' "${C[$1]-}"; }
for key in DOCKER_BIN JQ_BIN PRODUCTION_EVIDENCE_RUNNER; do v=$(get "$key"); [[ -n $v && $v == /* && -x $v ]] || fail "$key must be an absolute executable/path"; done
container=$(get CONTAINER_NAME); [[ $container =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$ ]] || fail 'invalid container name'
cred=$(get AUTH_CREDENTIALS_FILE); [[ -f $cred && $cred == /* ]] || fail 'authorized credentials file is required'
mode=$(stat -c '%a' "$cred" 2>/dev/null || printf 000); [[ $mode == 600 ]] || fail 'credentials file must have mode 0600'
api=$(get PRODUCTION_API_URL); front=$(get PRODUCTION_FRONTEND_URL); dsn=$(get PRODUCTION_DSN)
[[ -n $api && $api =~ ^https://[^[:space:]]+$ ]] || fail 'authorized production API URL is required'
[[ -n $front && $front =~ ^https://[^[:space:]]+$ ]] || fail 'authorized frontend URL is required'
[[ -n $dsn ]] || fail 'authorized read-only DSN is required'

# Inspect the running container without allowing the evidence runner to invent identity.
docker_bin=$(get DOCKER_BIN)
jq_bin=$(get JQ_BIN)
inspect=$("$docker_bin" inspect --format '{{json .}}' "$container" 2>/dev/null) || fail 'container inspection failed'
identity=$("$jq_bin" -cer 'if type == "array" then .[0] else . end' <<<"$inspect" 2>/dev/null) || fail 'invalid container inspection JSON'
running=$("$jq_bin" -r '(.State.Running // false) | tostring' <<<"$identity")
health=$("$jq_bin" -r '(.State.Health.Status // "")' <<<"$identity")
image=$("$jq_bin" -r '(.Config.Image // "")' <<<"$identity")
revision=$("$jq_bin" -r '.Config.Labels["org.opencontainers.image.revision"] // ""' <<<"$identity")
repo_digest=$("$jq_bin" -r '(.RepoDigests[0] // "")' <<<"$identity")
[[ $running == true && $health == healthy ]] || fail 'container is not running and healthy'
[[ $image == "$target_image" || $repo_digest == "$target_image" ]] || fail 'running digest does not match target'
[[ $revision == "$expected_revision" ]] || fail 'running revision does not match target'

runner=$(get PRODUCTION_EVIDENCE_RUNNER)
set +e
report=$(PROBE_READ_ONLY=1 PROBE_CONFIG="$cfg" PRODUCTION_API_URL="$api" PRODUCTION_FRONTEND_URL="$front" PRODUCTION_DSN="$dsn" AUTH_CREDENTIALS_FILE="$cred" "$runner" "$release_id" "$target_image" "$expected_revision" "$migration_version" 2>/dev/null)
rc=$?
set -e
(( rc == 0 )) || fail 'evidence runner failed'
[[ $report != *$'\n'* ]] || fail 'runner returned multiple lines'
"$jq_bin" -e 'type == "object" and .success == true and .read_only == true and .marker_status == "ready" and (.migration_version|type == "number") and .invariants == true and .authenticated_frontend == true and .disabled_plan_existing_consumable == true and .disabled_plan_new_allocations_rejected == true and .model_scope_ignored == true' <<<"$report" >/dev/null 2>&1 || fail 'runner evidence does not satisfy frozen contract'
runner_digest=$("$jq_bin" -r '.digest // ""' <<<"$report"); runner_revision=$("$jq_bin" -r '.revision // ""' <<<"$report")
[[ $runner_digest == "$target_image" && $runner_revision == "$expected_revision" ]] || fail 'runner identity mismatch'
"$jq_bin" -e --argjson v "$migration_version" '.migration_version == $v' <<<"$report" >/dev/null 2>&1 || fail 'runner migration marker mismatch'
# Emit only the frozen public fields; no DSN, credential, user, or runner diagnostics cross stdout.
printf '%s\n' "$report" | "$jq_bin" -c '{success:true,environment:"production",read_only:true,digest:.digest,revision:.revision,marker_status:.marker_status,migration_version:.migration_version,invariants:.invariants,authenticated_frontend:.authenticated_frontend,disabled_plan_existing_consumable:.disabled_plan_existing_consumable,disabled_plan_new_allocations_rejected:.disabled_plan_new_allocations_rejected,model_scope_ignored:.model_scope_ignored}'
out_done=1
