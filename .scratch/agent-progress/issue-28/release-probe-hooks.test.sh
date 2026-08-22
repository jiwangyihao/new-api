#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

root=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/issue28-probes.XXXXXX")
trap 'rm -rf -- "$tmp"' EXIT
fail() { printf 'FAIL: %s\n' "$1" >&2; [[ -n ${2:-} ]] && printf 'rc=%s\n' "$2" >&2; exit 1; }
pass() { printf 'PASS: %s\n' "$1"; }
mkdir -p "$tmp/bin" "$tmp/work"
log="$tmp/calls.log"; : > "$log"
target="registry.example/new-api@sha256:$(printf 'a%.0s' {1..64})"
revision=$(printf '1%.0s' {1..40})
backup_sha=$(printf 'b%.0s' {1..64})

cat >"$tmp/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'docker %s\n' "$*" >>"$CALL_LOG"
case "${1:-}" in
  inspect)
    printf '%s\n' '{"State":{"Running":true,"Health":{"Status":"healthy"}},"Config":{"Image":"registry.example/new-api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Labels":{"org.opencontainers.image.revision":"1111111111111111111111111111111111111111"}},"RepoDigests":["registry.example/new-api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]}' ;;
  *) exit 2 ;;
esac
EOF
cat >"$tmp/bin/jq" <<'EOF'
#!/usr/bin/env python
import json,sys
args=sys.argv[1:]; compact='-c' in args or '-cn' in args; raw='-r' in args; exit_status='-e' in args
variables={}; expr=''; i=0
while i<len(args):
 a=args[i]
 if a in ('--arg','--argjson'):
  variables[args[i+1]]=json.loads(args[i+2]) if a=='--argjson' else args[i+2]; i+=3; continue
 if not a.startswith('-'): expr=a
 i+=1
source=sys.stdin.read(); data=json.loads(source) if source.strip() else None
def path(v,*ps):
 for p in ps:
  if not isinstance(v,dict): return None
  v=v.get(p)
 return v
if expr.startswith('{success:true,environment:"production"'):
 result={'success':True,'environment':'production',**{k:data.get(k) for k in ('read_only','digest','revision','marker_status','migration_version','invariants','authenticated_api','administrator_identity','paid_value_endpoints_structurally_consistent','snapshot_at','api_origin','browser_evidence_required')}}
elif expr.startswith('{success:true,environment:"isolated_clone"'):
 result={'success':True,'environment':'isolated_clone',**{k:data.get(k) for k in ('clone_isolated','production_identity_collision','source_backup_sha256','target_digest','cleanup_complete')},'fixture':data.get('fixture')}
elif 'authenticated_api' in expr and 'paid_value_endpoints_structurally_consistent' in expr:
 result=(data.get('success') is True and data.get('read_only') is True and data.get('digest')==variables.get('image') and data.get('revision')==variables.get('revision') and data.get('migration_version')==variables.get('version') and data.get('authenticated_api') is True and data.get('administrator_identity') is True and data.get('paid_value_endpoints_structurally_consistent') is True and data.get('api_origin')=='http://127.0.0.1:13080' and data.get('browser_evidence_required') is True and isinstance(data.get('snapshot_at'),int))
elif 'cleanup_complete' in expr and 'target_digest' in expr:
 f=data.get('fixture',{}); result=(data.get('success') is True and data.get('source_backup_sha256')==variables.get('sha') and data.get('target_digest')==variables.get('image') and data.get('clone_isolated') is True and data.get('production_identity_collision') is False and data.get('cleanup_complete') is True and f.get('available_credit')==800 and all(f.get(k) is True for k in ('five_analytics_endpoints_consistent','disabled_plan_existing_consumable','disabled_plan_new_allocations_rejected','model_scope_ignored')))
elif expr=='.success==true and .environment=="production" and .migration_version==7': result=data.get('success') is True and data.get('environment')=='production' and data.get('migration_version')==7
elif expr=='.success==false and .environment=="production"': result=data.get('success') is False and data.get('environment')=='production'
elif expr=='.success==true and .fixture.available_credit==800': result=data.get('success') is True and path(data,'fixture','available_credit')==800
elif '.State.Running == true' in expr:
 result=path(data,'State','Running') is True and path(data,'State','Health','Status')=='healthy' and path(data,'Config','Image')==variables.get('image') and path(data,'Config','Labels','org.opencontainers.image.revision')==variables.get('revision')
elif 'paid_value_endpoints_available' in expr and '.digest == $image' in expr:
 result=data.get('success') is True and data.get('read_only') is True and data.get('digest')==variables.get('image') and data.get('revision')==variables.get('revision') and data.get('migration_version')==variables.get('version') and data.get('paid_value_endpoints_available') is True
elif 'disabled_plan_existing_consumable' in expr:
 f=data.get('fixture',{}); result=data.get('success') is True and data.get('source_backup_sha256')==variables.get('sha') and f.get('available_credit')==800 and all(f.get(k) is True for k in ('five_analytics_endpoints_consistent','disabled_plan_existing_consumable','disabled_plan_new_allocations_rejected','model_scope_ignored'))
else: result=False
if exit_status and not result: sys.exit(1)
if raw and isinstance(result,str): print(result)
elif compact: print(json.dumps(result,separators=(',',':')))
else: print(json.dumps(result))
EOF
cat >"$tmp/bin/curl" <<'EOF'
#!/usr/bin/env bash
printf 'curl %s\n' "$*" >>"$CALL_LOG"
printf '%s\n' '{"success":true}'
EOF
cat >"$tmp/bin/sha256sum" <<EOF
#!/usr/bin/env bash
printf '%s  %s\n' '$backup_sha' "\${1:?}"
EOF
cat >"$tmp/bin/pg_restore" <<'EOF'
#!/usr/bin/env bash
[[ ${1:-} == --list ]]
EOF
chmod +x "$tmp/bin"/*
export CALL_LOG="$log"
printf 'ACCESS_TOKEN=abcdefghijklmnop\nUSER_ID=1\n' >"$tmp/credentials"; chmod 600 "$tmp/credentials"
printf backup >"$tmp/backup.dump"; chmod 600 "$tmp/backup.dump"

cat >"$tmp/production-runner" <<EOF
#!/usr/bin/env bash
printf '%s\n' '{"success":true,"read_only":true,"digest":"$target","revision":"$revision","marker_status":"ready","migration_version":7,"invariants":true,"authenticated_api":true,"administrator_identity":true,"paid_value_endpoints_structurally_consistent":true,"snapshot_at":1700000000,"api_origin":"http://127.0.0.1:13080","browser_evidence_required":true}'
EOF
cat >"$tmp/clone-runner" <<EOF
#!/usr/bin/env bash
printf '%s\n' '{"success":true,"environment":"isolated_clone","clone_isolated":true,"production_identity_collision":false,"source_backup_sha256":"$backup_sha","target_digest":"$target","cleanup_complete":true,"fixture":{"price_amount_micros":"40000000","plan_credit":1000,"consumed_credit":200,"available_credit":800,"end_time":0,"exact_cost_micros":"32000000","currency":"CNY","active_paid_subscription_count":1,"estimated_cost_micros":"0","unknown_credit":0,"five_analytics_endpoints_consistent":true,"disabled_plan_existing_consumable":true,"disabled_plan_new_allocations_rejected":true,"model_scope_ignored":true}}'
EOF
chmod +x "$tmp/production-runner" "$tmp/clone-runner"
cat >"$tmp/production.env" <<EOF
DOCKER_BIN=$tmp/bin/docker
JQ_BIN=$tmp/bin/jq
CURL_BIN=$tmp/bin/curl
CONTAINER_NAME=new-api
PRODUCTION_EVIDENCE_RUNNER=$tmp/production-runner
AUTH_CREDENTIALS_FILE=$tmp/credentials
PRODUCTION_API_URL=http://127.0.0.1:13080
PRODUCTION_TIMEOUT_SECONDS=10
EOF
cat >"$tmp/clone.env" <<EOF
DOCKER_BIN=$tmp/bin/docker
JQ_BIN=$tmp/bin/jq
SHA256_BIN=$tmp/bin/sha256sum
PG_RESTORE_BIN=$tmp/bin/pg_restore
CLONE_FIXTURE_RUNNER=$tmp/clone-runner
CLONE_POSTGRES_IMAGE=postgres@example.invalid@sha256:$(printf 'c%.0s' {1..64})
CLONE_POSTGRES_USER=fixture
CLONE_POSTGRES_DB=fixture
CLONE_TIMEOUT_SECONDS=10
CLONE_CONTAINER_PREFIX=issue28
TMP_ROOT=$tmp/work
PRODUCTION_CONTAINER_NAME=new-api
PRODUCTION_NETWORK=production-network
EOF
out=$(PRODUCTION_SNAPSHOT_AT=1700000000 PRODUCTION_PROBE_CONFIG="$tmp/production.env" "$root/production-probe-hook.sh" release-1 "$target" "$revision" 7) || fail production
[[ $out != *$'\n'* && $out != *abcdefghijklmnop* ]] || fail production-output
printf '%s' "$out" | "$tmp/bin/jq" -e '.success==true and .environment=="production" and .migration_version==7' >/dev/null || fail production-contract
pass production-wrapper
out=$(MIGRATION_VERSION=7 CLONE_PROBE_CONFIG="$tmp/clone.env" "$root/clone-probe-hook.sh" release-1 "$tmp/backup.dump" "$backup_sha" "$target") || fail clone
printf '%s' "$out" | "$tmp/bin/jq" -e '.success==true and .fixture.available_credit==800' >/dev/null || fail clone-contract
pass clone-wrapper
set +e
bad=$(PRODUCTION_PROBE_CONFIG="$tmp/production.env" "$root/production-probe-hook.sh" release-1 "registry.example/new-api@sha256:$(printf 'd%.0s' {1..64})" "$revision" 7 2>/dev/null); rc=$?
set -e
(( rc != 0 )) || fail identity-mismatch
printf '%s' "$bad" | "$tmp/bin/jq" -e '.success==false and .environment=="production"' >/dev/null || fail identity-failure-json
pass fail-closed
printf 'PASS: release probe adapters executed offline fixtures\n'
