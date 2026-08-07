#!/usr/bin/env bash
# REQ-AUD-S03-01: refuse to build a release unless the `verify` workflow concluded SUCCESS on
# the exact commit the tag points at — on BOTH the tag-push and workflow_dispatch (rebuild)
# paths. v0.1.0 shipped signed, attested artifacts off a commit whose release-exitgate was red;
# this gate makes that impossible.
#
# Deliberately no wait-loop: pending/in_progress fails immediately. Re-dispatching the release
# is cheap, whereas waiting hides red (and would happily wait out a run that ends in failure).
#
# Reads the GitHub Actions event context from the environment:
#   GITHUB_EVENT_NAME  push | workflow_dispatch
#   GITHUB_REF_NAME    tag name on the push path
#   TAG_INPUT          dispatched tag on the workflow_dispatch path (inputs.tag)
#   GITHUB_REPOSITORY  owner/repo
#   GH_TOKEN           token for `gh api` (needs contents: read + actions: read)
set -euo pipefail

# Pinned, not overridable: an env-tunable workflow name would be a bypass (point the gate at a
# trivially-green workflow). A rename fails the gate closed and trips the anti-rot test row.
readonly VERIFY_WORKFLOW="verify.yaml"

fail() {
  echo "FAIL: verify-tag-gate: $*" >&2
  exit 1
}

EVENT_NAME="${GITHUB_EVENT_NAME:-}"
REPO="${GITHUB_REPOSITORY:-}"

case "${EVENT_NAME}" in
  push) TAG="${GITHUB_REF_NAME:-}" ;;
  workflow_dispatch) TAG="${TAG_INPUT:-}" ;;
  "") fail "GITHUB_EVENT_NAME is unset — refusing to release without an event context" ;;
  *) fail "unsupported event '${EVENT_NAME}' — the release job runs on tag push and workflow_dispatch only" ;;
esac

[[ -n "${REPO}" ]] || fail "GITHUB_REPOSITORY is unset — cannot query verify runs"
[[ -n "${TAG}" ]] || fail "could not derive the release tag from event '${EVENT_NAME}'"

# Same accepted set as the workflow's "Resolve version" step; also keeps an attacker-supplied
# workflow_dispatch input from smuggling path or query segments into the API call below.
if [[ ! "${TAG}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]]; then
  fail "tag must look like vX.Y.Z (got '${TAG}')"
fi

# Resolve the tag to its commit SHA via the API on BOTH paths (the commits endpoint
# dereferences annotated tags), so the dispatch rebuild path is gated identically.
if ! SHA="$(gh api "repos/${REPO}/commits/${TAG}" --jq '.sha')"; then
  fail "could not resolve tag '${TAG}' to a commit SHA in ${REPO}"
fi
if [[ ! "${SHA}" =~ ^[0-9a-f]{40}$ ]]; then
  fail "resolved an implausible commit SHA for tag '${TAG}': '${SHA}'"
fi

echo "verify-tag-gate: asserting ${VERIFY_WORKFLOW} is green on ${TAG} (${SHA})"

if ! RUNS="$(gh api --paginate \
  "repos/${REPO}/actions/workflows/${VERIFY_WORKFLOW}/runs?head_sha=${SHA}&per_page=100" \
  --jq '.workflow_runs[] | [.status, (.conclusion // "null"), (.event // "null"), .html_url] | @tsv')"; then
  fail "could not query ${VERIFY_WORKFLOW} runs for ${SHA} (needs actions: read)"
fi

if [[ -z "${RUNS}" ]]; then
  fail "no completed '${VERIFY_WORKFLOW}' run found for ${TAG} (${SHA}) — re-run this release when verify is green on that commit"
fi

# Two conditions, both required.
#
# 1. Every run on that SHA must be completed AND successful. A single ff-merged commit can carry
#    both a PR run (which skips release-exitgate) and a main-push run (which does not); taking
#    only the newest would let the green PR run mask the red push run — the v0.1.0 shape.
# 2. At least one of those green runs must NOT be a pull_request run. `release-exitgate` is
#    guarded by `if: github.event_name != 'pull_request'`, so a PR run concludes success without
#    ever executing the release exit gate. GitHub only creates a push run for the TIP of a push,
#    so every intermediate commit of a multi-commit ff-merged PR carries a lone green PR run —
#    tagging one would otherwise publish signed, attested artifacts from a tree the exit gate
#    never saw. Same untrustworthy conclusion as (1), in its absence direction.
bad=0
green_non_pr=0
while IFS=$'\t' read -r status conclusion event url; do
  [[ -n "${status}" ]] || continue
  if [[ "${status}" != "completed" ]]; then
    echo "verify-tag-gate: run not finished (status=${status}, event=${event}): ${url}" >&2
    bad=$((bad + 1))
  elif [[ "${conclusion}" != "success" ]]; then
    echo "verify-tag-gate: run not green (conclusion=${conclusion}, event=${event}): ${url}" >&2
    bad=$((bad + 1))
  else
    echo "verify-tag-gate: run green (event=${event}): ${url}"
    [[ "${event}" == "pull_request" ]] || green_non_pr=$((green_non_pr + 1))
  fi
done <<<"${RUNS}"

if ((bad > 0)); then
  fail "${bad} '${VERIFY_WORKFLOW}' run(s) on ${TAG} (${SHA}) are not green — re-run the red or cancelled run(s) above, then re-run this release when verify is green on that commit"
fi

if ((green_non_pr == 0)); then
  fail "every green '${VERIFY_WORKFLOW}' run on ${TAG} (${SHA}) is a pull_request run — those skip release-exitgate, so the release exit gate never ran on this commit; tag the tip of a push to main (or re-run verify on this commit from the Actions tab) and re-run this release when verify is green on that commit"
fi

echo "OK: ${VERIFY_WORKFLOW} concluded success on ${TAG} (${SHA}) — release may build"
