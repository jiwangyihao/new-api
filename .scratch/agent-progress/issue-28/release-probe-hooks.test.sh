#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

root=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/issue28-probes.XXXXXX")
trap 'rm -rf -- "$tmp"' EXIT
fail() {
  printf 'FAIL: %s\n' "$1" >&2
  [[ -n ${2:-} ]] && printf 'rc=%s\n' "$2" >&2
  for err_file in "$tmp"/*.err; do
    [[ -f "$err_file" ]] || continue
    printf '%s:\n' "$err_file" >&2
    while IFS= read -r err_line; do printf '  %s\n' "$err_line" >&2; done <"$err_file"
  done
  exit 1
}
pass() { printf 'PASS: %s\n' "$1"; }

mkdir -p "$tmp/bin" "$tmp/work"
cat >"$tmp/bin/docker" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
[[ ${1:-} == inspect ]] || exit 2
printf '%s\n' '{"State":{"Running":true,"Health":{"Status":"healthy"}},"Config":{"Image":"registry.example/new-api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","Labels":{"org.opencontainers.image.revision":"1111111111111111111111111111111111111111"}},"RepoDigests":["registry.example/new-api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]}'
EOF
cat >"$tmp/bin/jq" <<'EOF'
#!/usr/bin/env python
import json
import sys

args = sys.argv[1:]
raw = False
compact = False
exit_status = False
variables = {}
filter_text = None
i = 0
while i < len(args):
    arg = args[i]
    if arg in ('--arg', '--argjson'):
        if i + 2 >= len(args):
            raise SystemExit('missing jq variable arguments')
        name, value = args[i + 1], args[i + 2]
        variables[name] = json.loads(value) if arg == '--argjson' else value
        i += 3
        continue
    if arg.startswith('--'):
        if arg == '--raw-output':
            raw = True
        elif arg == '--compact-output':
            compact = True
        elif arg == '--exit-status':
            exit_status = True
        else:
            raise SystemExit('unsupported jq option: ' + arg)
        i += 1
        continue
    if arg.startswith('-') and arg != '-':
        for flag in arg[1:]:
            if flag == 'r':
                raw = True
            elif flag == 'c':
                compact = True
            elif flag == 'e':
                exit_status = True
            else:
                raise SystemExit('unsupported jq option: -' + flag)
        i += 1
        continue
    filter_text = arg
    i += 1

if filter_text is None:
    raise SystemExit('missing jq filter')
data = json.load(sys.stdin)

def path(value, *parts, default=None):
    for part in parts:
        if isinstance(value, list):
            try:
                value = value[int(part)]
            except (ValueError, IndexError):
                return default
        elif isinstance(value, dict):
            value = value.get(part, default)
        else:
            return default
        if value is default:
            return default
    return value

def number(value):
    return isinstance(value, (int, float)) and not isinstance(value, bool)

def production_contract(value):
    return (
        isinstance(value, dict)
        and value.get('success') is True
        and value.get('read_only') is True
        and value.get('marker_status') == 'ready'
        and number(value.get('migration_version'))
        and value.get('invariants') is True
        and value.get('authenticated_frontend') is True
        and value.get('disabled_plan_existing_consumable') is True
        and value.get('disabled_plan_new_allocations_rejected') is True
        and value.get('model_scope_ignored') is True
    )

def clone_contract(value):
    fixture = value.get('fixture', {}) if isinstance(value, dict) else {}
    return (
        isinstance(value, dict)
        and value.get('success') is True
        and value.get('environment') == 'isolated_clone'
        and value.get('clone_isolated') is True
        and value.get('production_identity_collision') is False
        and isinstance(fixture, dict)
        and value.get('source_backup_sha256') == variables.get('sha')
        and value.get('target_digest') == variables.get('img')
        and value.get('cleanup_complete') is True
        and fixture.get('price_amount_micros') == '40000000'
        and fixture.get('plan_credit') == 1000
        and fixture.get('consumed_credit') == 200
        and fixture.get('available_credit') == 800
        and fixture.get('end_time') == 0
        and fixture.get('exact_cost_micros') == '32000000'
        and fixture.get('currency') == 'CNY'
        and fixture.get('active_paid_subscription_count') == 1
        and fixture.get('estimated_cost_micros') == '0'
        and fixture.get('unknown_credit') == 0
        and fixture.get('five_analytics_endpoints_consistent') is True
    )

if filter_text == 'if type == "array" then .[0] else . end':
    result = data[0] if isinstance(data, list) and data else data
elif filter_text == '(.State.Running // false) | tostring':
    result = 'true' if path(data, 'State', 'Running', default=False) is True else 'false'
elif filter_text == '(.State.Health.Status // "")':
    result = path(data, 'State', 'Health', 'Status', default='')
elif filter_text == '(.Config.Image // "")':
    result = path(data, 'Config', 'Image', default='')
elif filter_text == '.Config.Labels["org.opencontainers.image.revision"] // ""':
    result = path(data, 'Config', 'Labels', 'org.opencontainers.image.revision', default='')
elif filter_text == '(.RepoDigests[0] // "")':
    result = path(data, 'RepoDigests', 0, default='')
elif filter_text == '.digest // ""':
    result = data.get('digest', '')
elif filter_text == '.revision // ""':
    result = data.get('revision', '')
elif filter_text == '.migration_version == $v':
    result = data.get('migration_version') == variables.get('v')
elif 'environment == "isolated_clone"' in filter_text and 'clone_isolated' in filter_text:
    result = (
        isinstance(data, dict)
        and data.get('success') is True
        and data.get('environment') == 'isolated_clone'
        and data.get('clone_isolated') is True
        and data.get('production_identity_collision') is False
        and isinstance(data.get('fixture'), dict)
    )
elif filter_text.startswith('.source_backup_sha256 == $sha'):
    result = clone_contract(data)
elif filter_text.startswith('((type == "object" and .success == true'):
    result = clone_contract(data)
elif filter_text.startswith('type == "object" and .success == true'):
    result = production_contract(data)
elif filter_text == '.success==true and .environment=="production" and .migration_version==7':
    result = isinstance(data, dict) and data.get('success') is True and data.get('environment') == 'production' and data.get('migration_version') == 7
elif filter_text == '.success==false and .environment=="production"':
    result = isinstance(data, dict) and data.get('success') is False and data.get('environment') == 'production'
elif filter_text == '.success==true and .fixture.available_credit==800':
    result = isinstance(data, dict) and data.get('success') is True and path(data, 'fixture', 'available_credit') == 800
elif filter_text.startswith('{success:true,environment:"production"'):
    result = {
        'success': True,
        'environment': 'production',
        **{key: data.get(key) for key in ('read_only', 'digest', 'revision', 'marker_status', 'migration_version', 'invariants', 'authenticated_frontend', 'disabled_plan_existing_consumable', 'disabled_plan_new_allocations_rejected', 'model_scope_ignored')},
    }
elif filter_text.startswith('{success:true,environment:"isolated_clone"'):
    result = {
        'success': True,
        'environment': 'isolated_clone',
        'source_backup_sha256': data.get('source_backup_sha256'),
        'fixture': {key: path(data, 'fixture', key) for key in ('price_amount_micros', 'plan_credit', 'consumed_credit', 'available_credit', 'end_time', 'exact_cost_micros', 'currency', 'active_paid_subscription_count', 'estimated_cost_micros', 'unknown_credit', 'five_analytics_endpoints_consistent')},
    }
else:
    raise SystemExit('unsupported jq fixture filter: ' + filter_text)

if exit_status and not result:
    raise SystemExit(1)
if raw and isinstance(result, str):
    sys.stdout.write(result + '\n')
elif compact:
    sys.stdout.write(json.dumps(result, separators=(',', ':')) + '\n')
else:
    sys.stdout.write(json.dumps(result) + '\n')
EOF
cat >"$tmp/bin/stat" <<'EOF'
#!/usr/bin/env bash
if [[ ${1:-} == -c ]]; then printf '600\n'; else exec /usr/bin/stat "$@"; fi
EOF
cat >"$tmp/bin/sha256sum" <<'EOF'
#!/usr/bin/env bash
printf 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb  %s\n' "${1:?}"
EOF
cat >"$tmp/bin/pg_restore" <<'EOF'
#!/usr/bin/env bash
[[ ${1:-} == --list ]] || exit 2
exit 0
EOF
cat >"$tmp/bin/sleep" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
for f in docker jq stat sha256sum pg_restore sleep; do chmod +x "$tmp/bin/$f"; done
export PATH="$tmp/bin:$PATH"
printf secret >"$tmp/credentials"; chmod 600 "$tmp/credentials"
printf backup >"$tmp/backup.dump"; chmod 600 "$tmp/backup.dump"
target='registry.example/new-api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa'
rev='1111111111111111111111111111111111111111'
cat >"$tmp/production-runner" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
[[ ${PROBE_READ_ONLY:-} == 1 && ${PRODUCTION_DSN:-} == 'postgres://readonly:secret@example.invalid/db' ]] || exit 9
printf '%s\n' '{"success":true,"read_only":true,"digest":"registry.example/new-api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","revision":"1111111111111111111111111111111111111111","marker_status":"ready","migration_version":7,"invariants":true,"authenticated_frontend":true,"disabled_plan_existing_consumable":true,"disabled_plan_new_allocations_rejected":true,"model_scope_ignored":true}'
EOF
cat >"$tmp/clone-runner" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail
printf '%s\n' '{"success":true,"environment":"isolated_clone","clone_isolated":true,"production_identity_collision":false,"source_backup_sha256":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","target_digest":"registry.example/new-api@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","cleanup_complete":true,"fixture":{"price_amount_micros":"40000000","plan_credit":1000,"consumed_credit":200,"available_credit":800,"end_time":0,"exact_cost_micros":"32000000","currency":"CNY","active_paid_subscription_count":1,"estimated_cost_micros":"0","unknown_credit":0,"five_analytics_endpoints_consistent":true}}'
EOF
chmod +x "$tmp/production-runner" "$tmp/clone-runner"
cat >"$tmp/production.env" <<EOF
DOCKER_BIN=$tmp/bin/docker
JQ_BIN=$tmp/bin/jq
CONTAINER_NAME=new-api
PRODUCTION_EVIDENCE_RUNNER=$tmp/production-runner
AUTH_CREDENTIALS_FILE=$tmp/credentials
PRODUCTION_API_URL=https://api.example.invalid
PRODUCTION_FRONTEND_URL=https://frontend.example.invalid
PRODUCTION_DSN=postgres://readonly:secret@example.invalid/db
PRODUCTION_TIMEOUT_SECONDS=10
EOF
cat >"$tmp/clone.env" <<EOF
SHA256_BIN=$tmp/bin/sha256sum
PG_RESTORE_BIN=$tmp/bin/pg_restore
CLONE_FIXTURE_RUNNER=$tmp/clone-runner
JQ_BIN=$tmp/bin/jq
TMP_ROOT=$tmp/work
PRODUCTION_CONTAINER_NAME=new-api
PRODUCTION_NETWORK=production-network
EOF
set +e
out=$(PATH="$tmp/bin:$PATH" PRODUCTION_PROBE_CONFIG="$tmp/production.env" "$root/production-probe-hook.sh" release-1 "$target" "$rev" 7 2>"$tmp/prod.err"); rc=$?
set -e
(( rc == 0 )) || fail production-success "$rc"
[[ $out != *$'\n'* && -n $out ]] || fail production-multiline
[[ $out != *secret* && $out != *postgres://* ]] || fail production-secret-leak
printf '%s' "$out" | "$tmp/bin/jq" -e '.success==true and .environment=="production" and .migration_version==7' >/dev/null || fail production-contract
pass production-success-and-no-secret
sed -i 's/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc/g' "$tmp/bin/docker"
set +e
bad=$(PRODUCTION_PROBE_CONFIG="$tmp/production.env" "$root/production-probe-hook.sh" release-1 "$target" "$rev" 7 2>"$tmp/prod-identity-fail.err"); rc=$?
set -e
(( rc != 0 )) || fail production-identity-mismatch-rc
[[ $bad != *$'\n'* && -n $bad ]] || fail production-identity-mismatch-multiline
[[ $bad != *secret* && $bad != *postgres://* ]] || fail production-identity-mismatch-secret-leak
printf '%s' "$bad" | "$tmp/bin/jq" -e '.success==false and .environment=="production"' >/dev/null || fail production-identity-mismatch-json
pass production-identity-mismatch
sed -i 's#PRODUCTION_EVIDENCE_RUNNER=.*#PRODUCTION_EVIDENCE_RUNNER=/nonexistent#' "$tmp/production.env"
set +e
bad=$(PRODUCTION_PROBE_CONFIG="$tmp/production.env" "$root/production-probe-hook.sh" release-1 "$target" "$rev" 7 2>"$tmp/prod-runner-fail.err"); rc=$?
set -e
(( rc != 0 )) || fail production-runner-failure-rc
[[ $bad != *$'\n'* && -n $bad ]] || fail production-runner-failure-multiline
[[ $bad != *secret* && $bad != *postgres://* ]] || fail production-runner-failure-secret-leak
printf '%s' "$bad" | "$tmp/bin/jq" -e '.success==false and .environment=="production"' >/dev/null || fail production-runner-failure-json
pass production-runner-failure
out=$(CLONE_PROBE_CONFIG="$tmp/clone.env" "$root/clone-probe-hook.sh" release-1 "$tmp/backup.dump" bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb "$target" 2>"$tmp/clone.err") || fail clone-success
[[ $out != *$'\n'* && -n $out ]] || fail clone-multiline
printf '%s' "$out" | "$tmp/bin/jq" -e '.success==true and .fixture.available_credit==800' >/dev/null || fail clone-contract
pass clone-success
set +e
bad=$(CLONE_PROBE_CONFIG="$tmp/clone.env" "$root/clone-probe-hook.sh" release-1 "$tmp/backup.dump" cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc "$target" 2>"$tmp/clone-fail.err"); rc=$?
set -e
(( rc != 0 )) || fail clone-checksum-failure-rc
[[ $bad != *secret* ]] || fail clone-failure-secret-leak
pass clone-checksum-failure
printf 'PASS: release probe adapters executed offline fixtures\n'
