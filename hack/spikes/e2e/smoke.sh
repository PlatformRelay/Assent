#!/usr/bin/env bash
# Spike B (P2-E2-S02) — product-surface smoke against a booted GitLab instance.
#
# Exercises the exact primitives the Reconcile port needs (ADR-0011/ADR-0015 §2):
#   create project → seed sample repo → open MR → resolvable thread → resolve →
#   approve (project token) → adversarial stale-SHA merge (must be REJECTED) →
#   SHA-pinned merge with the fresh sha (must succeed).
#
# Repeatable: deletes and recreates the sample project on every run. Exits 0 only if
# every assertion passes. No secrets in this file: a throwaway root PAT is minted at
# runtime inside the instance (or supply ASSENT_SPIKE_ROOT_TOKEN to skip minting).
#
# Profile: ASSENT_SPIKE_PROFILE=testcontainer (default) | kind
set -euo pipefail

PROFILE="${ASSENT_SPIKE_PROFILE:-testcontainer}"
case "${PROFILE}" in
testcontainer)
  BASE="http://localhost:${ASSENT_SPIKE_HTTP_PORT:-8980}"
  rails_exec() { local ruby="$1"; docker exec assent-spike-gitlab gitlab-rails runner "$ruby"; }
  ;;
kind)
  BASE="http://localhost:8929"
  rails_exec() { local ruby="$1"; kubectl --context kind-assent exec deploy/gitlab -- gitlab-rails runner "$ruby"; }
  ;;
*)
  echo "ERROR: unknown profile '${PROFILE}'" >&2
  exit 1
  ;;
esac
API="${BASE}/api/v4"
# Reused literals (kept single-sourced): JSON content-type header and the
# curl write-out format that appends the HTTP status on its own line.
readonly CONTENT_TYPE_JSON='Content-Type: application/json'
readonly HTTP_CODE_FMT=$'\n%{http_code}'
PROJECT="topic-registry-smoke"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
TOPIC_DIR="${REPO_ROOT}/examples/repos/topic-registry/topics/prod"

log() { echo "--- $*"; }

# api METHOD PATH EXPECTED_STATUS [curl args...] — body to stdout, dies on status mismatch.
api() {
  local method="$1" path="$2" expect="$3"
  shift 3
  local out status body
  out=$(curl -s -w "${HTTP_CODE_FMT}" -X "${method}" -H "PRIVATE-TOKEN: ${TOKEN}" "$@" "${API}${path}")
  status="${out##*$'\n'}"
  body="${out%$'\n'*}"
  if [[ "${status}" != "${expect}" ]]; then
    echo "ERROR: ${method} ${path} -> ${status} (expected ${expect}): ${body}" >&2
    return 1
  fi
  printf '%s' "${body}"
  return
}

json_escape() {
  python3 -c 'import json,sys; print(json.dumps(sys.stdin.read()), end="")'
  return
}

# --- auth: mint a throwaway root PAT unless one was provided ------------------------------
if [[ -z "${ASSENT_SPIKE_ROOT_TOKEN:-}" ]]; then
  log "minting throwaway root token inside the instance (gitlab-rails runner, ~30s)"
  TOKEN="spike-$(openssl rand -hex 16)"
  rails_exec "t = User.find_by_username('root').personal_access_tokens.create!(scopes: ['api'], name: 'spike-smoke-$(date +%s)', expires_at: 1.day.from_now); t.set_token('${TOKEN}'); t.save!" >/dev/null
else
  TOKEN="${ASSENT_SPIKE_ROOT_TOKEN}"
fi

# --- repeatability: recreate the sample project -------------------------------------------
old_id=$(curl -s -H "PRIVATE-TOKEN: ${TOKEN}" "${API}/projects/root%2F${PROJECT}" | jq -r '.id? // empty')
if [[ -n "${old_id}" ]]; then
  log "deleting previous project ${old_id}"
  api DELETE "/projects/${old_id}" 202 >/dev/null
  for _ in $(seq 1 30); do
    code=$(curl -s -o /dev/null -w '%{http_code}' -H "PRIVATE-TOKEN: ${TOKEN}" "${API}/projects/root%2F${PROJECT}")
    [[ "${code}" == "404" ]] && break
    sleep 2
  done
fi

log "creating project ${PROJECT}"
project=$(api POST "/projects" 201 --data "name=${PROJECT}&initialize_with_readme=false")
pid=$(printf '%s' "${project}" | jq -r .id)

# --- seed from examples/repos/topic-registry/ (P1-E1 sample shape) -------------------------
[[ -d "${TOPIC_DIR}" ]] || {
  echo "ERROR: topic-registry sample missing at ${TOPIC_DIR}" >&2
  exit 1
}
log "seeding sample repo from examples/repos/topic-registry/topics/prod/"
seed_json=$(TOPIC_DIR="${TOPIC_DIR}" python3 - <<'PY'
import json, os, pathlib
topic_dir = pathlib.Path(os.environ["TOPIC_DIR"])
actions = []
for path in sorted(topic_dir.glob("*.yaml")):
    actions.append({
        "action": "create",
        "file_path": f"topics/prod/{path.name}",
        "content": path.read_text(),
    })
print(json.dumps({
    "branch": "main",
    "commit_message": "seed: topic-registry prod topics",
    "actions": actions,
}))
PY
)
api POST "/projects/${pid}/repository/commits" 201 \
  -H "${CONTENT_TYPE_JSON}" \
  --data "${seed_json}" >/dev/null

CHANGE_FILE="topics/prod/orders.events.v1.yaml"
ORDERS_SRC="${TOPIC_DIR}/orders.events.v1.yaml"
# Bump partitions 12 -> 24 on the change branch (matches sample's real shape).
changed_content=$(python3 -c '
import re, sys
p = open(sys.argv[1]).read()
p2, n = re.subn(r"(?m)^partitions:\s*12\s*$", "partitions: 24", p, count=1)
if n != 1:
    sys.exit("expected partitions: 12 once in orders.events.v1.yaml")
sys.stdout.write(p2)
' "${ORDERS_SRC}")
changed_json=$(printf '%s' "${changed_content}" | json_escape)

log "creating change branch + commit (bump orders.events.v1 partitions)"
api POST "/projects/${pid}/repository/branches" 201 \
  --data "branch=change/orders-partitions&ref=main" >/dev/null
api POST "/projects/${pid}/repository/commits" 201 \
  -H "${CONTENT_TYPE_JSON}" \
  --data "{\"branch\":\"change/orders-partitions\",\"commit_message\":\"orders.events.v1: scale partitions 12 -> 24\",\"actions\":[{\"action\":\"update\",\"file_path\":\"${CHANGE_FILE}\",\"content\":${changed_json}}]}" >/dev/null

log "opening MR"
mr=$(api POST "/projects/${pid}/merge_requests" 201 \
  --data "source_branch=change/orders-partitions&target_branch=main&title=orders.events.v1: scale partitions")
iid=$(printf '%s' "${mr}" | jq -r .iid)
sha=$(printf '%s' "${mr}" | jq -r .sha)
log "MR !${iid} at head sha ${sha}"

log "posting resolvable discussion thread"
disc=$(api POST "/projects/${pid}/merge_requests/${iid}/discussions" 201 \
  --data "body=Please confirm orders-team is aware of the partition bump.")
disc_id=$(printf '%s' "${disc}" | jq -r .id)

log "resolving thread ${disc_id}"
resolved=$(api PUT "/projects/${pid}/merge_requests/${iid}/discussions/${disc_id}" 200 \
  --data "resolved=true")
[[ "$(printf '%s' "${resolved}" | jq -r '.notes[0].resolved')" == "true" ]] || {
  echo "ERROR: thread not resolved" >&2
  exit 1
}

log "creating project access token (Maintainer bot) and approving with it"
expires=$(date -v+1d +%Y-%m-%d 2>/dev/null || date -d '+1 day' +%Y-%m-%d)
bot_token_json=$(api POST "/projects/${pid}/access_tokens" 201 \
  -H "${CONTENT_TYPE_JSON}" \
  --data "{\"name\":\"spike-approver-$(date +%s)\",\"scopes\":[\"api\"],\"access_level\":40,\"expires_at\":\"${expires}\"}")
BOT_TOKEN=$(printf '%s' "${bot_token_json}" | jq -r .token)
[[ -n "${BOT_TOKEN}" && "${BOT_TOKEN}" != "null" ]] || {
  echo "ERROR: project access token missing from response: ${bot_token_json}" >&2
  exit 1
}
# Project bots can take a beat to become usable; retry approve briefly.
approve_status=""
approve_body=""
for _ in $(seq 1 10); do
  out=$(curl -s -w "${HTTP_CODE_FMT}" -X POST \
    -H "PRIVATE-TOKEN: ${BOT_TOKEN}" "${API}/projects/${pid}/merge_requests/${iid}/approve")
  approve_status="${out##*$'\n'}"
  approve_body="${out%$'\n'*}"
  [[ "${approve_status}" == "201" ]] && break
  sleep 2
done
[[ "${approve_status}" == "201" ]] || {
  echo "ERROR: approve with project token -> ${approve_status} (expected 201): ${approve_body}" >&2
  exit 1
}
log "approved by project bot (201)"

# --- adversarial: push AFTER approval, then merge with the STALE sha (ADR-0015 §2) --------
adversarial_content=$(python3 -c '
import re, sys
p = open(sys.argv[1]).read()
p = re.sub(r"(?m)^partitions:\s*12\s*$", "partitions: 24", p, count=1)
p2, n = re.subn(r"(?m)^retention_hours:\s*\d+\s*$", "retention_hours: 336", p, count=1)
if n != 1:
    sys.exit("expected retention_hours once")
sys.stdout.write(p2)
' "${ORDERS_SRC}")
adversarial_json=$(printf '%s' "${adversarial_content}" | json_escape)

log "pushing new commit after approval"
api POST "/projects/${pid}/repository/commits" 201 \
  -H "${CONTENT_TYPE_JSON}" \
  --data "{\"branch\":\"change/orders-partitions\",\"commit_message\":\"orders.events.v1: also bump retention\",\"actions\":[{\"action\":\"update\",\"file_path\":\"${CHANGE_FILE}\",\"content\":${adversarial_json}}]}" >/dev/null

# After a push GitLab briefly reports merge_status=checking and PUT /merge can
# return 405 Method Not Allowed — wait until mergeability is computed so the
# SHA-guard response is the real conflict (409), not the transient 405.
log "waiting for mergeability to settle after adversarial push"
for _ in $(seq 1 30); do
  ms=$(api GET "/projects/${pid}/merge_requests/${iid}" 200 | jq -r '.detailed_merge_status // .merge_status')
  [[ "${ms}" != "checking" && "${ms}" != "unchecked" ]] && break
  sleep 2
done
log "mergeability settled as ${ms}"

log "attempting merge with STALE sha ${sha} (must be rejected)"
stale_status=""
stale_body=""
for _ in $(seq 1 6); do
  stale_out=$(curl -s -w "${HTTP_CODE_FMT}" -X PUT -H "PRIVATE-TOKEN: ${TOKEN}" \
    "${API}/projects/${pid}/merge_requests/${iid}/merge?sha=${sha}")
  stale_status="${stale_out##*$'\n'}"
  stale_body="${stale_out%$'\n'*}"
  # 405 = mergeability still settling; retry. 409/406 = SHA guard fired.
  [[ "${stale_status}" == "409" || "${stale_status}" == "406" ]] && break
  [[ "${stale_status}" != "405" ]] && break
  sleep 2
done
if [[ "${stale_status}" == "409" || "${stale_status}" == "406" ]]; then
  log "ASSERT OK: stale-sha merge rejected with HTTP ${stale_status}"
else
  echo "ERROR: stale-sha merge returned HTTP ${stale_status} (expected 409/406): ${stale_body} — MERGE INTEGRITY VIOLATION" >&2
  exit 1
fi

log "merging with fresh sha (SHA-pinned happy path)"
fresh_sha=$(api GET "/projects/${pid}/merge_requests/${iid}" 200 | jq -r .sha)
# GitLab computes mergeability asynchronously after the new push; retry briefly.
merged=""
status=""
for _ in $(seq 1 12); do
  out=$(curl -s -w "${HTTP_CODE_FMT}" -X PUT -H "PRIVATE-TOKEN: ${TOKEN}" \
    "${API}/projects/${pid}/merge_requests/${iid}/merge?sha=${fresh_sha}")
  status="${out##*$'\n'}"
  if [[ "${status}" == "200" ]]; then
    merged=yes
    break
  fi
  sleep 5
done
[[ "${merged}" == "yes" ]] || {
  echo "ERROR: fresh-sha merge did not succeed (last HTTP ${status})" >&2
  exit 1
}
state=$(api GET "/projects/${pid}/merge_requests/${iid}" 200 | jq -r .state)
[[ "${state}" == "merged" ]] || {
  echo "ERROR: MR state is '${state}', expected 'merged'" >&2
  exit 1
}

log "SMOKE PASS: seed, MR, thread, resolve, approve, stale-sha rejection (${stale_status}), sha-pinned merge"
echo "SMOKE_STALE_HTTP=${stale_status}"
