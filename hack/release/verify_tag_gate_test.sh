#!/usr/bin/env bash
# REQ-AUD-S03-01: polarity table for hack/release/verify-tag-gate.sh (success / failure /
#                 pending / missing) driven by a stubbed `gh` on PATH — no live run needed.
# REQ-AUD-S03-02: step-order assertion over .github/workflows/release.yaml — the gate step
#                 precedes every build/sign/publish step in the release job DAG.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

GATE="${ROOT}/hack/release/verify-tag-gate.sh"
WF="${ROOT}/.github/workflows/release.yaml"
REPO_SLUG="PlatformRelay/assent"
SHA="0123456789abcdef0123456789abcdef01234567"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

command -v jq >/dev/null 2>&1 \
  || fail "jq is required by this test (the gh stub applies the script's --jq filter for real)"

[[ -f "${GATE}" ]] || fail "missing hack/release/verify-tag-gate.sh (REQ-AUD-S03-01)"
[[ -f "${WF}" ]] || fail "missing .github/workflows/release.yaml (REQ-AUD-S03-02)"

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT
BIN="${TMP}/bin"
STUB="${TMP}/stub"
mkdir -p "${BIN}" "${STUB}"

# --- stubbed `gh` -------------------------------------------------------------
# Serves fixture JSON per API path and honours the caller's --jq filter with real jq,
# so the script's own projection is exercised rather than faked.
cat >"${BIN}/gh" <<'STUBEOF'
#!/usr/bin/env bash
set -euo pipefail
[[ "${1:-}" == "api" ]] || { echo "stub gh: unsupported command: $*" >&2; exit 2; }
shift
path=""
jqexpr=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --jq) shift; jqexpr="${1:-}" ;;
    -*) : ;;
    *) path="$1" ;;
  esac
  shift
done
printf '%s\n' "${path}" >>"${GH_STUB_DIR}/calls.log"
case "${path}" in
  */commits/*) fixture="${GH_STUB_DIR}/commit.json" ;;
  */actions/workflows/*/runs*) fixture="${GH_STUB_DIR}/runs.json" ;;
  *) echo "stub gh: unexpected api path: ${path}" >&2; exit 2 ;;
esac
if [[ ! -f "${fixture}" ]]; then
  echo "gh: Not Found (HTTP 404) ${path}" >&2
  exit 1
fi
if [[ -n "${jqexpr}" ]]; then
  jq -r "${jqexpr}" "${fixture}"
else
  cat "${fixture}"
fi
STUBEOF
chmod +x "${BIN}/gh"

fixture_commit() { printf '{"sha":"%s"}\n' "${1:-${SHA}}" >"${STUB}/commit.json"; }
fixture_runs() { cat >"${STUB}/runs.json"; }
reset_fixtures() { rm -f "${STUB}/commit.json" "${STUB}/runs.json" "${STUB}/calls.log"; }

run_json() { # run_json <status> <conclusion> <url>
  printf '{"status":"%s","conclusion":%s,"html_url":"%s"}' \
    "$1" "$([[ "$2" == "null" ]] && echo null || printf '"%s"' "$2")" "$3"
}
runs_doc() { # runs_doc <run-json>...
  local joined=""
  local r
  for r in "$@"; do
    [[ -z "${joined}" ]] && joined="${r}" || joined="${joined},${r}"
  done
  printf '{"total_count":%d,"workflow_runs":[%s]}\n' "$#" "${joined}"
}

RC=0
OUT=""
run_gate() { # run_gate <event> <ref_name> <tag_input>
  set +e
  OUT="$(
    PATH="${BIN}:${PATH}" \
    GH_STUB_DIR="${STUB}" \
    GH_TOKEN="stub-token" \
    GITHUB_REPOSITORY="${REPO_SLUG}" \
    GITHUB_EVENT_NAME="$1" \
    GITHUB_REF_NAME="$2" \
    TAG_INPUT="$3" \
    bash "${GATE}" 2>&1
  )"
  RC=$?
  set -e
}

assert_rc() { # assert_rc <expected> <case>
  [[ "${RC}" == "$1" ]] || fail "$2: expected exit ${1}, got ${RC}
--- gate output ---
${OUT}"
}
assert_contains() { # assert_contains <needle> <case>
  grep -qF -- "$1" <<<"${OUT}" || fail "$2: output missing '$1'
--- gate output ---
${OUT}"
}

# --- REQ-AUD-S03-01 polarity table -------------------------------------------
echo "== REQ-AUD-S03-01 polarity table (stubbed gh) =="

# success (push)
reset_fixtures
fixture_commit
fixture_runs <<<"$(runs_doc "$(run_json completed success https://example.invalid/run/1)")"
run_gate push v9.9.9 ""
assert_rc 0 "success/push"
assert_contains "OK" "success/push"
grep -qF "repos/${REPO_SLUG}/commits/v9.9.9" "${STUB}/calls.log" \
  || fail "success/push: gate must resolve the tag's commit SHA via gh api"
grep -qF "head_sha=${SHA}" "${STUB}/calls.log" \
  || fail "success/push: gate must query verify runs for the resolved tag SHA"
grep -qF "verify.yaml/runs" "${STUB}/calls.log" \
  || fail "success/push: gate must query the verify workflow's runs"

# failure (push)
reset_fixtures
fixture_commit
fixture_runs <<<"$(runs_doc "$(run_json completed failure https://example.invalid/run/2)")"
run_gate push v9.9.9 ""
assert_rc 1 "failure/push"
assert_contains "failure" "failure/push"
assert_contains "when verify is green" "failure/push"

# pending — in_progress (push)
reset_fixtures
fixture_commit
fixture_runs <<<"$(runs_doc "$(run_json in_progress null https://example.invalid/run/3)")"
run_gate push v9.9.9 ""
assert_rc 1 "pending/in_progress"
assert_contains "in_progress" "pending/in_progress"
assert_contains "when verify is green" "pending/in_progress"

# pending — queued (push)
reset_fixtures
fixture_commit
fixture_runs <<<"$(runs_doc "$(run_json queued null https://example.invalid/run/4)")"
run_gate push v9.9.9 ""
assert_rc 1 "pending/queued"
assert_contains "queued" "pending/queued"

# missing — no verify run on that SHA
reset_fixtures
fixture_commit
fixture_runs <<<'{"total_count":0,"workflow_runs":[]}'
run_gate push v9.9.9 ""
assert_rc 1 "missing"
assert_contains "no completed" "missing"
assert_contains "when verify is green" "missing"

# v0.1.0 shape: a green PR run (release-exitgate skipped) alongside a red main-push run on the
# SAME ff-merged SHA. Newest-first ordering puts the green one first on purpose — a
# "latest/first run wins" implementation would pass this row, so it must stay fail-closed.
reset_fixtures
fixture_commit
fixture_runs <<<"$(runs_doc \
  "$(run_json completed success https://example.invalid/run/pr)" \
  "$(run_json completed failure https://example.invalid/run/push)")"
run_gate push v9.9.9 ""
assert_rc 1 "mixed/green-pr-run-plus-red-push-run"
assert_contains "https://example.invalid/run/push" "mixed/green-pr-run-plus-red-push-run"

# workflow_dispatch is NOT a bypass: same gate, dispatched tag's SHA
reset_fixtures
fixture_commit
fixture_runs <<<"$(runs_doc "$(run_json completed failure https://example.invalid/run/5)")"
run_gate workflow_dispatch v0.0.0-otherref v9.9.9
assert_rc 1 "dispatch/failure"
assert_contains "when verify is green" "dispatch/failure"
grep -qF "repos/${REPO_SLUG}/commits/v9.9.9" "${STUB}/calls.log" \
  || fail "dispatch/failure: gate must use the dispatched tag input, not GITHUB_REF_NAME"
if grep -qF "v0.0.0-otherref" "${STUB}/calls.log"; then
  fail "dispatch/failure: gate must ignore GITHUB_REF_NAME on the workflow_dispatch path"
fi

reset_fixtures
fixture_commit
fixture_runs <<<"$(runs_doc "$(run_json completed success https://example.invalid/run/6)")"
run_gate workflow_dispatch v0.0.0-otherref v9.9.9
assert_rc 0 "dispatch/success"

# unresolvable tag (gh api 404 on the commits endpoint)
reset_fixtures
fixture_runs <<<"$(runs_doc "$(run_json completed success https://example.invalid/run/7)")"
run_gate push v9.9.9 ""
assert_rc 1 "unresolvable-tag"
assert_contains "v9.9.9" "unresolvable-tag"

# malformed tag must be rejected BEFORE it reaches an API path (injection guard)
for bad in "v9.9.9?head_sha=deadbeef" "../../evil" "not-a-tag" ""; do
  reset_fixtures
  fixture_commit
  fixture_runs <<<"$(runs_doc "$(run_json completed success https://example.invalid/run/8)")"
  run_gate push "${bad}" ""
  assert_rc 1 "malformed-tag[${bad}]"
  [[ ! -s "${STUB}/calls.log" ]] \
    || fail "malformed-tag[${bad}]: gate called gh before validating the tag: $(cat "${STUB}/calls.log")"
done

# unsupported event / missing repo → fail closed
reset_fixtures
fixture_commit
fixture_runs <<<"$(runs_doc "$(run_json completed success https://example.invalid/run/9)")"
run_gate pull_request v9.9.9 ""
assert_rc 1 "unsupported-event"

reset_fixtures
fixture_commit
fixture_runs <<<"$(runs_doc "$(run_json completed success https://example.invalid/run/10)")"
set +e
OUT="$(PATH="${BIN}:${PATH}" GH_STUB_DIR="${STUB}" GH_TOKEN=stub-token \
  GITHUB_REPOSITORY="" GITHUB_EVENT_NAME=push GITHUB_REF_NAME=v9.9.9 TAG_INPUT="" \
  bash "${GATE}" 2>&1)"
RC=$?
set -e
assert_rc 1 "missing-repo"

echo "OK: REQ-AUD-S03-01 — success passes; failure / in_progress / queued / missing / mixed and
     the workflow_dispatch rebuild path all fail closed"

# --- REQ-AUD-S03-02 step order + wiring --------------------------------------
echo "== REQ-AUD-S03-02 release.yaml step order =="

release_block() {
  awk '
    /^  release:/ { inrel = 1 }
    inrel && /^  [A-Za-z0-9_-]+:/ && !/^  release:/ { exit }
    inrel { printf "%d\t%s\n", NR, $0 }
  ' "${WF}"
}

step_index_of() { # 1-based index of the release-job step whose body contains <marker>
  local marker="$1" idx
  idx="$(release_block | awk -v m="${marker}" '
    /^[0-9]+\t      - / { n = n + 1 }
    index($0, m) > 0 { print n; found = 1; exit }
    END { if (!found) exit 0 }
  ')"
  [[ -n "${idx}" ]] \
    || fail "step-order: marker not found in the release job: ${marker} (REQ-AUD-S03-02)"
  printf '%s\n' "${idx}"
}

checkout_idx="$(step_index_of "actions/checkout@")"
gate_idx="$(step_index_of "hack/release/verify-tag-gate.sh")"

[[ "${checkout_idx}" == "1" ]] \
  || fail "step-order: actions/checkout must be step 1 of the release job (got ${checkout_idx})"
# The gate is extracted to hack/release/verify-tag-gate.sh (spec DoD), so checkout necessarily
# precedes it — it is the first step that DOES anything, before any build/sign/publish work.
[[ "${gate_idx}" == "2" ]] \
  || fail "step-order: the verify-tag gate must be the first step after checkout (got ${gate_idx}) (REQ-AUD-S03-02)"

for marker in \
  "Resolve version" \
  "actions/setup-go@" \
  "sigstore/cosign-installer@" \
  "anchore/sbom-action" \
  "goreleaser/goreleaser-action@" \
  "actions/attest@" \
  "orhun/git-cliff-action@" \
  "softprops/action-gh-release@"; do
  idx="$(step_index_of "${marker}")"
  ((gate_idx < idx)) \
    || fail "step-order: gate (step ${gate_idx}) must precede '${marker}' (step ${idx}) (REQ-AUD-S03-02)"
done

step_block() { # the release-job step whose body contains <marker>
  release_block | awk -v m="$1" '
    function flush() { if (matched) { for (i = 1; i <= cnt; i++) print buf[i]; exit } }
    /^[0-9]+\t      - / { flush(); cnt = 0; matched = 0 }
    { cnt = cnt + 1; buf[cnt] = $0; if (index($0, m) > 0) matched = 1 }
    END { flush() }
  '
}

# The gate step must actually invoke the extracted script and carry a token to call the API.
# Scoped to the gate step's own block: TAG_INPUT/GH_TOKEN also appear elsewhere in the job,
# so a job-wide grep would pass even with the gate step's env stripped.
gate_step="$(step_block "hack/release/verify-tag-gate.sh")"
[[ -n "${gate_step}" ]] \
  || fail "could not isolate the gate step in the release job (REQ-AUD-S03-02)"
grep -qF "run: bash hack/release/verify-tag-gate.sh" <<<"${gate_step}" \
  || fail "the gate step must invoke hack/release/verify-tag-gate.sh (REQ-AUD-S03-01)"
grep -qF "GH_TOKEN:" <<<"${gate_step}" \
  || fail "the gate step must pass GH_TOKEN (checkout uses persist-credentials: false)"
grep -qF "TAG_INPUT:" <<<"${gate_step}" \
  || fail "the gate step must pass TAG_INPUT so the workflow_dispatch path is gated too"

# An explicit permissions: block sets unlisted scopes to none — the workflow-runs API needs
# actions: read, otherwise the gate 403s on every real tag.
release_block | grep -qE '[[:space:]]actions: read([[:space:]]|$)' \
  || fail "release job permissions must grant 'actions: read' for the workflow-runs API (REQ-AUD-S03-01)"

# Anti-rot: the workflow filename the gate asserts on must exist, or the gate becomes a
# permanent hard-fail that the next person "fixes" by loosening it.
wf_name="$(awk -F'"' '/^readonly VERIFY_WORKFLOW=/ { print $2 }' "${GATE}")"
[[ -n "${wf_name}" ]] \
  || fail "gate script must pin the verify workflow via 'readonly VERIFY_WORKFLOW=\"...\"'"
[[ -f "${ROOT}/.github/workflows/${wf_name}" ]] \
  || fail "gate pins .github/workflows/${wf_name}, which does not exist (REQ-AUD-S03-02)"

echo "OK: REQ-AUD-S03-02 — gate step precedes every build/sign/publish step; actions: read granted"
echo "OK: AUD-S03 verify-tag gate"
