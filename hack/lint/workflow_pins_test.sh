#!/usr/bin/env bash
# REQ-AUD-S09-01 — supply-chain pins across .github/workflows/** are PRESENT and
# the checks that assert them CAN FAIL.
#
# Findings closed here:
#   * SEC-04 — `go install …/task/v3/cmd/task@latest` in verify.yaml (two sites)
#     let a mutable upstream release change gate behaviour under CI silently.
#     The version is now pinned once, at workflow level, as `env: TASK_VERSION`,
#     and both install sites interpolate it — so the two jobs cannot skew.
#
# Spec reconciliation (AUD-S09): the story's acceptance criterion reads "every
# hit carries the pinned version", while its Goal mandates single-sourcing the
# version through a workflow-level `env`. Those pull in opposite directions —
# with `task@"${TASK_VERSION}"` the install line does not literally carry a
# semver. This gate implements the Goal and asserts the AC's intent instead:
#   (a) zero `@latest` anywhere under .github/workflows/**,
#   (b) every `cmd/task@` site interpolates `${TASK_VERSION}`, and
#   (c) `TASK_VERSION` is defined EXACTLY ONCE, at workflow level, as a literal
#       vX.Y.Z — so the pin is a real version and there is only one of it.
#
# ANTI-VACUITY DISCIPLINE. This gate is almost entirely negative assertions
# ("no @latest", "no unpinned checkout"), which is the exact shape that passes
# green while testing nothing — a pattern that matches nothing looks identical
# to a clean tree. Countermeasures, all enforced below on every run:
#
#   * Every check is a FUNCTION over a workflows directory, never a straight-line
#     assertion. Each function is run twice: once against the real tree (must be
#     GREEN) and once against a temp copy carrying the very violation it exists
#     to catch (must be RED). A check that cannot fail fails this script.
#   * Every extraction is positive-controlled — non-empty, and at or above a
#     known-present count — so a pattern that silently stopped matching fails
#     loudly instead of vacuously passing.
#   * Portable ERE only: no `\t`, `\s`, `\b`, no `grep -P`. GNU grep does not
#     honour `\t` in an ERE where BSD grep does, and as a NEGATIVE assertion
#     that divergence passes OPEN on Linux while looking green on macOS.
#   * No `grep -q` on the read end of a pipe: under `set -o pipefail` the early
#     close raises SIGPIPE and the pipeline exits 141 intermittently. Every
#     match goes to a file first and the file is inspected.
#   * No `git` and no `sed -i`: this script must run in a bare container
#     (`debian:stable-slim`) and inside a git worktree, where `.git` is a file
#     and `git` may be absent. Repo root comes from ${BASH_SOURCE}.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

WORKFLOWS="$ROOT/.github/workflows"

WORK="$(mktemp -d)"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

[[ -d "$WORKFLOWS" ]] || fail "missing $WORKFLOWS"

# ---------------------------------------------------------------- inventory --
# Positive control on the inventory itself: if this glob stops matching, every
# check below would sweep an empty set and report success.
WORKFLOW_FILES=()
while IFS= read -r f; do
  WORKFLOW_FILES+=("$f")
done < <(find "$WORKFLOWS" -type f \( -name '*.yml' -o -name '*.yaml' \) | sort)

((${#WORKFLOW_FILES[@]} >= 8)) ||
  fail "found only ${#WORKFLOW_FILES[@]} workflow files under $WORKFLOWS — the inventory glob stopped matching, so every check below would pass vacuously"

for expected in actionlint.yaml codeql.yaml docs.yaml release.yaml schemas.yml scorecard.yaml verify.yaml vulncheck.yaml; do
  [[ -f "$WORKFLOWS/$expected" ]] ||
    fail "expected workflow $expected is missing — the inventory this gate pins has changed shape"
done
echo "OK: inventory — ${#WORKFLOW_FILES[@]} workflow files, all 8 pinned names present"

# ------------------------------------------------------------------ checks --
# Contract for every check_* function: takes a workflows DIRECTORY, prints the
# offending lines to stderr, returns 0 (clean) or 1 (violation). They must never
# call fail() — the mutation harness below needs to observe their return code.

# SEC-04(a): no mutable `@latest` install anywhere in CI.
check_no_latest() {
  local dir="$1"
  local hits="$WORK/hits.no_latest"
  if grep -REn -- '@latest' "$dir" >"$hits" 2>/dev/null; then
    echo "  unpinned '@latest' reference(s) — a mutable upstream release can change CI behaviour silently (SEC-04):" >&2
    sed 's/^/    /' "$hits" >&2
    return 1
  fi
  return 0
}

# SEC-04(b): every Task install interpolates the single-sourced version, and
# that version is defined exactly once, at workflow level, as a literal vX.Y.Z.
check_task_pinned() {
  local dir="$1"
  local sites="$WORK/hits.task_sites"
  local rc=0

  # Install sites only: the first non-blank character must not be `#`, so the
  # prose above the `env:` block that names this very check is not mistaken for
  # an unpinned install. Grepped per-file (not `-R`) so `^` anchors the LINE
  # rather than grep's `file:line:` prefix.
  : >"$sites"
  local f
  for f in "$dir"/*.yml "$dir"/*.yaml; do
    [[ -f "$f" ]] || continue
    grep -En '^[[:space:]]*[^#[:space:]].*cmd/task@' "$f" | sed "s|^|${f##*/}:|" >>"$sites" || true
  done
  local n_sites
  n_sites="$(wc -l <"$sites" | tr -d '[:space:]')"
  if ((n_sites == 0)); then
    echo "  no 'cmd/task@' install site found under $dir — this check has nothing to assert (extraction broke, or the install moved)" >&2
    return 1
  fi
  ((n_sites >= 2)) || {
    echo "  found only $n_sites 'cmd/task@' site(s); verify.yaml has two jobs that install Task — extraction is under-counting" >&2
    rc=1
  }

  # Every site must interpolate ${TASK_VERSION}; anything else (an @latest, or a
  # hard-coded version) is an ad-hoc pin the two jobs can skew on. Fixed-string
  # match: `${...}` in an ERE is brace-interval territory where GNU and BSD grep
  # disagree, and this is a negative assertion — divergence here passes OPEN.
  local bad="$WORK/hits.task_unpinned"
  grep -Fv -- 'cmd/task@"${TASK_VERSION}"' "$sites" >"$bad" || true
  if [[ -s "$bad" ]]; then
    echo "  Task install site(s) not single-sourced through \${TASK_VERSION} (SEC-04):" >&2
    sed 's/^/    /' "$bad" >&2
    rc=1
  fi

  # Exactly one workflow-level definition. Two-space indent == workflow scope;
  # a job-level env would be indented six, a step-level one ten. Grepped
  # per-file (not `-R`) so `^` anchors the LINE, not grep's `file:line:` prefix.
  local defs="$WORK/hits.task_defs"
  local good="$WORK/hits.task_defs_literal"
  : >"$defs"
  for f in "$dir"/*.yml "$dir"/*.yaml; do
    [[ -f "$f" ]] || continue
    grep -En '^  TASK_VERSION:' "$f" | sed "s|^|${f##*/}:|" >>"$defs" || true
  done
  grep -E 'TASK_VERSION: v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?[[:space:]]*(#.*)?$' "$defs" >"$good" || true

  local n_defs n_good
  n_defs="$(wc -l <"$defs" | tr -d '[:space:]')"
  n_good="$(wc -l <"$good" | tr -d '[:space:]')"
  if ((n_defs != 1)) || ((n_good != 1)); then
    echo "  expected exactly ONE workflow-level 'TASK_VERSION: vX.Y.Z' definition; found $n_defs definition(s), $n_good of them a literal version (SEC-04: the pin must be single-sourced AND a real version)" >&2
    sed 's/^/    /' "$defs" >&2
    rc=1
  fi
  return "$rc"
}

# ------------------------------------------------------- mutation harness --
# The reason this file is a gate and not a comment. Each check is proven RED
# against a tree carrying its violation, on every run — so a check that stops
# matching is caught here, not six months later by the vulnerability it was
# supposed to prevent.

mutant() { # <name> -> prints a fresh copy of the real workflows dir
  local name="$1"
  local dir="$WORK/mutant-$name"
  rm -rf "$dir"
  mkdir -p "$dir"
  cp -R "$WORKFLOWS/." "$dir/"
  printf '%s\n' "$dir"
}

# Rewrite a file in place without `sed -i` (BSD needs an argument, GNU must not
# have one) and ASSERT THE MUTATION LANDED — a no-op sed would otherwise make
# the mutant identical to the clean tree and "prove" the check fires when the
# only thing proven is that nothing changed.
mutate() { # <file> <sed-program> <string that must appear afterwards>
  local file="$1" program="$2" witness="$3"
  local before after
  before="$(cat "$file")"
  sed "$program" "$file" >"$file.mut"
  mv "$file.mut" "$file"
  after="$(cat "$file")"
  [[ "$before" != "$after" ]] ||
    fail "mutation harness: sed program '$program' did not change $file — the mutant is identical to the clean tree, so any 'the check fires' conclusion is false"
  grep -Fq -- "$witness" "$file" ||
    fail "mutation harness: expected '$witness' in $file after mutation, but it is absent"
}

expect_green() { # <check-fn> <dir> <label>
  local fn="$1" dir="$2" label="$3"
  "$fn" "$dir" || fail "$label — the real tree violates $fn (see the offending lines above)"
  echo "OK: $label"
}

expect_red() { # <check-fn> <dir> <label>
  local fn="$1" dir="$2" label="$3"
  if "$fn" "$dir" 2>/dev/null; then
    fail "mutation control: $fn accepted a tree that DOES carry the violation ($label) — this check cannot fail and is therefore not a gate"
  fi
  echo "OK: mutation control — $fn goes red on: $label"
}

# ------------------------------------------------ 1. SEC-04: no '@latest' --

echo "== 1. SEC-04: no mutable '@latest' install under .github/workflows/** =="
expect_green check_no_latest "$WORKFLOWS" "zero '@latest' references in the real workflows"

m="$(mutant no-latest)"
mutate "$m/verify.yaml" \
  's|cmd/task@"\${TASK_VERSION}"|cmd/task@latest|' \
  'cmd/task@latest'
expect_red check_no_latest "$m" "a Task install reverted to @latest"

# ------------------------------------------- 2. SEC-04: Task pin sourcing --

echo "== 2. SEC-04: Task version pinned once, at workflow level, and interpolated =="
expect_green check_task_pinned "$WORKFLOWS" "both Task installs interpolate a single workflow-level TASK_VERSION"

m="$(mutant task-latest)"
mutate "$m/verify.yaml" \
  's|cmd/task@"\${TASK_VERSION}"|cmd/task@latest|' \
  'cmd/task@latest'
expect_red check_task_pinned "$m" "an install site stopped interpolating \${TASK_VERSION}"

m="$(mutant task-skew)"
mutate "$m/verify.yaml" \
  's|cmd/task@"\${TASK_VERSION}"|cmd/task@v3.0.0|' \
  'cmd/task@v3.0.0'
expect_red check_task_pinned "$m" "an install site hard-codes its own version (the two jobs can skew)"

m="$(mutant task-unset)"
mutate "$m/verify.yaml" \
  's|^  TASK_VERSION: v[0-9].*$|  TASK_VERSION: latest|' \
  'TASK_VERSION: latest'
expect_red check_task_pinned "$m" "TASK_VERSION is defined but is not a literal vX.Y.Z"

m="$(mutant task-removed)"
mutate "$m/verify.yaml" \
  '/^  TASK_VERSION: v[0-9]/d' \
  'cmd/task@"${TASK_VERSION}"'
expect_red check_task_pinned "$m" "the workflow-level TASK_VERSION definition was deleted"

# --------------------------------------------------------------- CI wiring --
# `task check` runs this script (proven non-self-referentially by
# hack/release/changelog_gate_test.sh, which re-runs its wiring assertion
# against a Taskfile copy with the line deleted). The verify workflow must run
# it too, so a PR that unpins a workflow is caught before merge and not only on
# the author's machine.

echo "== 3. wiring: the verify workflow runs this gate =="
if ! grep -Fqs 'bash hack/lint/workflow_pins_test.sh' "$WORKFLOWS/verify.yaml"; then
  fail "verify.yaml does not run 'bash hack/lint/workflow_pins_test.sh' — the workflow-pin gate would only ever run locally"
fi
echo "OK: verify.yaml runs hack/lint/workflow_pins_test.sh"

echo
echo "workflow_pins_test.sh: PASS"
