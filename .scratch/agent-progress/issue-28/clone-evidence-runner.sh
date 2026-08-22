#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

fail() { printf 'clone evidence runner: %s\n' "${1:-failed}" >&2; exit 1; }
[[ ${CLONE_READ_ONLY:-} == 1 ]] || fail 'CLONE_READ_ONLY=1 is required'
[[ $# -eq 4 ]] || fail 'exactly RELEASE_ID BACKUP_PATH BACKUP_SHA256 TARGET_IMAGE is required'
release_id=$1
backup=$2
backup_sha=$3
target_image=$4
[[ $release_id =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,80}$ && $release_id != *..* ]] || fail 'invalid release id'
[[ $backup == /* && -f $backup && $backup_sha =~ ^[0-9a-f]{64}$ ]] || fail 'invalid backup identity'
[[ $target_image =~ ^[^[:space:]@]+@sha256:[0-9a-f]{64}$ ]] || fail 'target image must be immutable'
workdir=${CLONE_WORKDIR:-}
[[ $workdir == /* && -d $workdir ]] || fail 'isolated workdir is required'

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
    DOCKER_BIN|JQ_BIN|PG_RESTORE_BIN|CLONE_FIXTURE_RUNNER|CLONE_POSTGRES_IMAGE|CLONE_POSTGRES_USER|CLONE_POSTGRES_DB|CLONE_TIMEOUT_SECONDS|CLONE_CONTAINER_PREFIX|TMP_ROOT|PRODUCTION_CONTAINER_NAME|PRODUCTION_NETWORK) ;;
    *) fail 'unknown config key' ;;
  esac
  [[ "$value" != *'$('* && "$value" != *'`'* && "$value" != *';'* && "$value" != *'|'* && "$value" != *'&'* ]] || fail 'unsafe config value'
  C[$key]=$value
done < "$cfg"
get() { printf '%s' "${C[$1]-}"; }
for key in DOCKER_BIN JQ_BIN; do
  value=$(get "$key")
  [[ $value == /* && -x $value ]] || fail "$key must be an absolute executable"
done
postgres_image=$(get CLONE_POSTGRES_IMAGE)
postgres_user=$(get CLONE_POSTGRES_USER)
postgres_db=$(get CLONE_POSTGRES_DB)
timeout=$(get CLONE_TIMEOUT_SECONDS)
prefix=$(get CLONE_CONTAINER_PREFIX)
[[ $postgres_image =~ ^[^[:space:]]+@sha256:[0-9a-f]{64}$ ]] || fail 'clone PostgreSQL image must be immutable'
[[ $postgres_user =~ ^[A-Za-z_][A-Za-z0-9_]*$ && $postgres_db =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || fail 'invalid clone database identity'
[[ $timeout =~ ^[1-9][0-9]{0,3}$ && $prefix =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,30}$ ]] || fail 'invalid clone runtime config'

docker_bin=$(get DOCKER_BIN)
jq_bin=$(get JQ_BIN)
suffix="${backup_sha:0:12}-$$"
network="${prefix}-${suffix}"
postgres_container="${prefix}-postgres-${suffix}"
app_container="${prefix}-app-${suffix}"
production_container=$(get PRODUCTION_CONTAINER_NAME)
production_network=$(get PRODUCTION_NETWORK)
[[ $production_container =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$ && $production_network =~ ^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$ ]] || fail 'invalid production identity guard'
[[ $app_container != "$production_container" && $postgres_container != "$production_container" && $network != "$production_network" ]] || fail 'clone identity collides with production'
password_file="$workdir/postgres-password"
printf '%s' "$backup_sha" > "$password_file"
chmod 600 "$password_file"
password=$backup_sha
network_id=''
postgres_id=''
app_id=''
container_id() { "$docker_bin" container inspect --format '{{.Id}}' "$1" 2>/dev/null; }
network_id_for() { "$docker_bin" network inspect --format '{{.Id}}' "$1" 2>/dev/null; }
remove_owned_container() {
  local name=$1 expected=$2 actual
  [[ -n $expected ]] || return 0
  actual=$(container_id "$name") || return 0
  [[ $actual == "$expected" ]] || return 1
  "$docker_bin" rm -f "$name" >/dev/null 2>&1
}
remove_owned_network() {
  local name=$1 expected=$2 actual
  [[ -n $expected ]] || return 0
  actual=$(network_id_for "$name") || return 0
  [[ $actual == "$expected" ]] || return 1
  "$docker_bin" network rm "$name" >/dev/null 2>&1
}
cleanup() {
  local rc=$?
  trap - EXIT HUP INT TERM
  set +e
  remove_owned_container "$app_container" "$app_id"
  remove_owned_container "$postgres_container" "$postgres_id"
  remove_owned_network "$network" "$network_id"
  rm -f -- "$password_file"
  exit "$rc"
}
trap cleanup EXIT HUP INT TERM

if network_id_for "$network" >/dev/null || container_id "$postgres_container" >/dev/null || container_id "$app_container" >/dev/null; then
  fail 'clone Docker resource name already exists'
fi
network_id=$("$docker_bin" network create --internal --label "new-api.issue28.release=$release_id" "$network") || fail 'cannot create isolated network'
[[ $(network_id_for "$network") == "$network_id" ]] || fail 'isolated network identity mismatch'
[[ $("$docker_bin" network inspect --format '{{.Internal}}' "$network" 2>/dev/null) == true ]] || fail 'clone network is not internal'
postgres_id=$("$docker_bin" run -d --name "$postgres_container" --network "$network" \
  --label "new-api.issue28.release=$release_id" \
  -e "POSTGRES_USER=$postgres_user" -e "POSTGRES_PASSWORD=$password" -e "POSTGRES_DB=$postgres_db" \
  "$postgres_image") || fail 'cannot start clone PostgreSQL'
[[ $(container_id "$postgres_container") == "$postgres_id" ]] || fail 'clone PostgreSQL identity mismatch'
started=$(date +%s)
while ! "$docker_bin" exec "$postgres_container" pg_isready -U "$postgres_user" -d "$postgres_db" >/dev/null 2>&1; do
  now=$(date +%s)
  (( now - started < timeout )) || fail 'clone PostgreSQL readiness timeout'
  sleep 1
done
"$docker_bin" exec -i "$postgres_container" pg_restore -U "$postgres_user" -d "$postgres_db" --no-owner --no-acl < "$backup" >/dev/null || fail 'backup restore failed'

sql_dsn="postgresql://${postgres_user}:${password}@${postgres_container}:5432/${postgres_db}?sslmode=disable"
app_id=$("$docker_bin" create --name "$app_container" --network "$network" --read-only --tmpfs /tmp:rw,noexec,nosuid,size=64m \
  --label "new-api.issue28.release=$release_id" \
  -e "NEW_API_CLONE_SQL_DSN=$sql_dsn" -e NODE_TYPE=slave -e REDIS_CONN_STRING= -e GIN_MODE=release \
  "$target_image" credit-valuation-probe --clone-tracer --version "${MIGRATION_VERSION:-1}" \
  --backup-sha256 "$backup_sha" --target-image "$target_image") || fail 'cannot create clone tracer container'
[[ $(container_id "$app_container") == "$app_id" ]] || fail 'clone tracer container identity mismatch'
set +e
report=$("$docker_bin" start -a "$app_container" 2>/dev/null)
rc=$?
set -e
(( rc == 0 )) || fail 'clone tracer command failed'
[[ $report != *$'\n'* ]] || fail 'clone tracer returned multiple lines'
"$jq_bin" -e --arg sha "$backup_sha" --arg image "$target_image" '.success == true and .environment == "isolated_clone" and .clone_isolated == true and .production_identity_collision == false and .source_backup_sha256 == $sha and .target_digest == $image and .fixture.disabled_plan_existing_consumable == true and .fixture.disabled_plan_new_allocations_rejected == true and .fixture.model_scope_ignored == true' <<<"$report" >/dev/null 2>&1 || fail 'clone tracer contract failed'

remove_owned_container "$app_container" "$app_id" || fail 'cannot remove owned clone app container'
app_id=''
remove_owned_container "$postgres_container" "$postgres_id" || fail 'cannot remove owned clone PostgreSQL container'
postgres_id=''
remove_owned_network "$network" "$network_id" || fail 'cannot remove owned clone network'
network_id=''
rm -f -- "$password_file"
trap - EXIT HUP INT TERM
printf '%s\n' "$report" | "$jq_bin" -c '. + {cleanup_complete:true}'
