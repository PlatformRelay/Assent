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
VALIDATOR="$ROOT/hack/schemas-validator"

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

# ------------------------------------------------------- comment-safe view --
# Review finding F1: a check that greps for the PRESENCE of a safety flag and
# does not exclude comments fails OPEN. `# persist-credentials: false` disarms
# the flag while satisfying a naive grep, so the gate printed OK over a checkout
# that leaks the workflow token. The same blindness was confirmed in the npm
# check (F5) and the CI-wiring check (F6): all three are PRESENCE assertions,
# and every presence assertion in this file must now read through code_view.
#
# Direction matters, and the two polarities are treated differently:
#   * PRESENCE assertions ("the flag is set", "the gate is invoked") fail OPEN
#     when comment-blind ⇒ they MUST use code_view.
#   * ABSENCE assertions ("no floating tag anywhere") fail CLOSED when
#     comment-blind — a mention in prose merely reds the gate. check_no_latest
#     keeps its blunt total ban deliberately; see its comment.
#
# Emits `<lineno>:<original line>` for every line whose first non-blank
# character is not `#`, so line numbers in findings still point at the file.
code_view() { # <file>
  awk '{
    s = $0
    sub(/^[[:space:]]+/, "", s)
    if (substr(s, 1, 1) != "#") printf "%d:%s\n", FNR, $0
  }' "$1"
}

# Same, for every workflow in a directory, prefixed `<file>:<lineno>:<line>`.
code_view_dir() { # <dir>
  local f
  for f in "$1"/*.yml "$1"/*.yaml; do
    [[ -f "$f" ]] || continue
    code_view "$f" | sed "s|^|${f##*/}:|"
  done
}

# ------------------------------------------------------------------ checks --
# Contract for every check_* function: takes a workflows DIRECTORY, prints the
# offending lines to stderr, returns 0 (clean) or 1 (violation). They must never
# call fail() — the mutation harness below needs to observe their return code.

# SEC-04(a): no mutable `@latest` install anywhere in CI.
#
# Deliberately NOT comment-aware, unlike the presence checks above: this is an
# ABSENCE assertion, so blindness here fails CLOSED (prose mentioning the token
# reds the gate — which it did during authoring, and the prose was reworded
# rather than the ban weakened). A commented-out floating install is also one
# uncomment away from being real.
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

  # Install sites only: comments are excluded so the prose above the `env:`
  # block that names this very check is not mistaken for an unpinned install.
  local code="$WORK/code.workflows"
  code_view_dir "$dir" >"$code"
  grep -F -- 'cmd/task@' "$code" >"$sites" || true
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

  # Review finding F3: counting only TWO-space definitions left the actual
  # invariant ("the two jobs cannot skew") unchecked — a JOB-level
  # `env: TASK_VERSION: v3.0.0` sits at six-space indent, silently overrides the
  # workflow-level value for that job, and was invisible here. So: count
  # definitions at ANY indent, require exactly one, and require THAT one to be
  # at workflow scope with a literal version. An override cannot hide in the
  # indentation any more.
  local defs="$WORK/hits.task_defs"
  local good="$WORK/hits.task_defs_literal"
  grep -E ':[0-9]+:[[:space:]]*TASK_VERSION:' "$code" >"$defs" || true
  # `<file>:<lineno>:` prefix, then EXACTLY two spaces = workflow scope.
  grep -E ':[0-9]+:  TASK_VERSION: v[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?[[:space:]]*(#.*)?$' \
    "$defs" >"$good" || true

  local n_defs n_good
  n_defs="$(wc -l <"$defs" | tr -d '[:space:]')"
  n_good="$(wc -l <"$good" | tr -d '[:space:]')"
  if ((n_defs != 1)) || ((n_good != 1)); then
    echo "  expected exactly ONE TASK_VERSION definition, at workflow scope, as a literal vX.Y.Z; found $n_defs definition(s) at any scope, $n_good of them workflow-scoped literals (SEC-04: a job-level definition overrides the workflow-level pin and lets the two jobs skew)" >&2
    sed 's/^/    /' "$defs" >&2
    rc=1
  fi
  return "$rc"
}

# SEC-03: every actions/checkout leaves no credentials in .git/config. Without
# it, any later step in the job — or anything it executes, including a
# dependency's build — can push with the workflow token.
#
# Step-scoped, not file-scoped: a job-wide grep would pass as long as ONE
# checkout in the file set the flag (proven by its own mutation control, which
# deletes ONLY the second occurrence in release.yaml — review finding F4: the
# three whole-file deletions previously used could not distinguish a step-scoped
# implementation from a file-scoped one). Steps are delimited by `- ` at list
# indent; a nested list inside a checkout step would split it early and report a
# violation, i.e. the parse fails CLOSED.
#
# Review finding F1: every line test here is guarded by `!is_comment`. This is a
# PRESENCE assertion, so without that guard `# persist-credentials: false`
# satisfied it and the gate reported OK over a checkout that keeps the workflow
# token in .git/config — a fail-OPEN, reproduced under GNU grep + mawk.
check_persist_credentials() {
  local dir="$1"
  local hits="$WORK/hits.persist_creds"
  local seen="$WORK/hits.checkout_steps"
  : >"$hits"
  : >"$seen"
  local f
  for f in "$dir"/*.yml "$dir"/*.yaml; do
    [[ -f "$f" ]] || continue
    awk '
      function flush() {
        if (is_checkout) {
          printf "%s:%d\n", short, start >> seen_out
          if (!has_pc) printf "%s:%d: actions/checkout without an ACTIVE `persist-credentials: false` (a commented-out flag is not a flag)\n", short, start
        }
        is_checkout = 0; has_pc = 0
      }
      {
        stripped = $0
        sub(/^[[:space:]]+/, "", stripped)
        is_comment = (substr(stripped, 1, 1) == "#")
      }
      /^[[:space:]]*- / && !is_comment { flush(); start = FNR }
      /uses:[[:space:]]*actions\/checkout@/ && !is_comment { is_checkout = 1 }
      /persist-credentials:[[:space:]]*false/ && !is_comment { has_pc = 1 }
      END { flush() }
    ' short="${f##*/}" seen_out="$seen" "$f" >>"$hits"
  done

  # Positive control: if the step parse stopped recognising checkouts, the hit
  # list would be empty and this check would pass over a fully unpinned tree.
  local n_checkouts
  n_checkouts="$(wc -l <"$seen" | tr -d '[:space:]')"
  if ((n_checkouts < 10)); then
    echo "  found only $n_checkouts actions/checkout step(s); the repo has at least 10 — the step parse is under-counting, so a clean result here would be vacuous" >&2
    return 1
  fi

  if [[ -s "$hits" ]]; then
    echo "  checkout step(s) leaving the workflow token in .git/config (SEC-03):" >&2
    sed 's/^/    /' "$hits" >&2
    return 1
  fi
  return 0
}

# SEC-01(a): the schemas job installs its validator from a committed lockfile,
# not from a registry range resolved at run time.
#
# Review finding F5, two defects, both fixed here:
#   * comment-blind, like F1 — a commented-out `npm ci` satisfied the presence
#     assertion, so `# run: npm ci …` plus a live global install passed. Every
#     line test now reads through code_view.
#   * the old negative pattern `npm[[:space:]]+install` did not match
#     `npm --global install`, nor `npm i`, nor `npm add`. Enumerating bad forms
#     is a losing game, so the polarity is inverted: EVERY npm invocation must
#     be an `npm ci`, and anything else is a finding by default.
check_npm_ci_in_workflow() {
  local dir="$1"
  local file="$dir/schemas.yml"
  local rc=0
  [[ -f "$file" ]] || {
    echo "  missing $file" >&2
    return 1
  }

  local code="$WORK/code.schemas"
  code_view "$file" >"$code"

  local npm_lines="$WORK/hits.npm_lines"
  local bad="$WORK/hits.npm_install"
  grep -F -- 'npm' "$code" >"$npm_lines" || true
  grep -Fv -- 'npm ci' "$npm_lines" >"$bad" || true
  if [[ -s "$bad" ]]; then
    echo "  schemas.yml invokes npm in a form other than 'npm ci' — dependencies would resolve at run time instead of from the committed lockfile (SEC-01):" >&2
    sed 's/^/    /' "$bad" >&2
    rc=1
  fi

  grep -Fq -- 'npm ci --ignore-scripts' "$code" || {
    echo "  schemas.yml does not RUN 'npm ci --ignore-scripts' on an uncommented line — the transitive tree is not lockfile-pinned (SEC-01)" >&2
    rc=1
  }

  # The paths filters enumerate every input to this job. If the validator
  # directory is not listed, moving the pin does not re-run the job that
  # depends on it — the lockfile would be pinned and never revalidated.
  local n_paths
  n_paths="$(grep -Fc -- '"hack/schemas-validator/**"' "$code" || true)"
  if ((n_paths < 2)); then
    echo "  schemas.yml lists 'hack/schemas-validator/**' in only $n_paths of its 2 paths: filters (pull_request and push) — a lockfile change would not trigger the job that consumes it" >&2
    rc=1
  fi
  return "$rc"
}

# F6: the verify workflow actually RUNS this gate. Previously a straight-line
# `grep -Fqs` with no mutation control — the one shape this file's preamble
# forbids — and comment-blind, so `# run: bash hack/lint/workflow_pins_test.sh`
# satisfied it. Now a check function like every other, with its own control.
check_ci_wiring() {
  local dir="$1"
  local file="$dir/verify.yaml"
  [[ -f "$file" ]] || {
    echo "  missing $file" >&2
    return 1
  }
  local code="$WORK/code.verify"
  code_view "$file" >"$code"
  grep -Fq -- 'bash hack/lint/workflow_pins_test.sh' "$code" || {
    echo "  verify.yaml does not run 'bash hack/lint/workflow_pins_test.sh' on an uncommented line — the workflow-pin gate would only ever run locally" >&2
    return 1
  }
  return 0
}

# SEC-01(b): the committed manifest and lockfile are exact and complete.
check_validator_lockfile() {
  local dir="$1"
  local pkg="$dir/package.json"
  local lock="$dir/package-lock.json"
  local rc=0

  [[ -f "$pkg" ]] || {
    echo "  missing $pkg (SEC-01)" >&2
    return 1
  }
  [[ -f "$lock" ]] || {
    echo "  missing $lock — 'npm ci' cannot run and the transitive tree is unpinned (SEC-01)" >&2
    return 1
  }

  # Exact versions only: a caret or tilde range in the manifest means the
  # lockfile can be regenerated onto different code without a manifest diff.
  local ranges="$WORK/hits.pkg_ranges"
  grep -En '"(ajv-cli|ajv-formats)":[[:space:]]*"[^0-9"]' "$pkg" >"$ranges" || true
  if [[ -s "$ranges" ]]; then
    echo "  package.json pins a RANGE rather than an exact version (SEC-01):" >&2
    sed 's/^/    /' "$ranges" >&2
    rc=1
  fi

  local dep
  for dep in ajv-cli ajv-formats; do
    grep -Fqs -- "\"node_modules/$dep\"" "$lock" || {
      echo "  package-lock.json has no resolved entry for $dep — the lockfile does not cover the validator (SEC-01)" >&2
      rc=1
    }
  done

  # Every resolved package must carry an integrity hash, or `npm ci` has
  # nothing to verify the downloaded tarball against.
  local n_resolved n_integrity
  n_resolved="$(grep -Fc -- '"resolved":' "$lock" || true)"
  n_integrity="$(grep -Fc -- '"integrity":' "$lock" || true)"
  if ((n_resolved < 20)) || ((n_resolved != n_integrity)); then
    echo "  package-lock.json has $n_resolved resolved package(s) and $n_integrity integrity hash(es) — expected them equal and >= 20 (SEC-01: the whole transitive tree must be hash-pinned)" >&2
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

validator_mutant() { # <name> -> prints a fresh copy of hack/schemas-validator
  local name="$1"
  local dir="$WORK/vmutant-$name"
  rm -rf "$dir"
  mkdir -p "$dir"
  cp "$VALIDATOR/package.json" "$VALIDATOR/package-lock.json" "$dir/"
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

# awk variant of mutate(), for mutations sed cannot express portably: inserting
# lines (BSD sed needs a literal newline in the replacement, GNU takes `\n`) and
# "change only the Nth occurrence" (`0,/re/s` is GNU-only). Same landed-mutation
# assertion — an awk program that silently matched nothing would otherwise leave
# the mutant identical to the clean tree.
mutate_awk() { # <file> <awk-program> <string that must appear afterwards>
  local file="$1" program="$2" witness="$3"
  local before after
  before="$(cat "$file")"
  awk "$program" "$file" >"$file.mut"
  mv "$file.mut" "$file"
  after="$(cat "$file")"
  [[ "$before" != "$after" ]] ||
    fail "mutation harness: awk program did not change $file — the mutant is identical to the clean tree, so any 'the check fires' conclusion is false"
  grep -Fq -- "$witness" "$file" ||
    fail "mutation harness: expected '$witness' in $file after mutation, but it is absent"
}

expect_green() { # <check-fn> <dir> <label>
  local fn="$1" dir="$2" label="$3"
  "$fn" "$dir" || fail "$label — the real tree violates $fn (see the offending lines above)"
  echo "OK: $label"
}

# A control that reds is not yet a control that WORKS: it could be red for an
# unrelated reason (a broken extraction, a missing file), which would mask the
# fact that the mutation itself goes undetected. So every control also pins the
# FINDING TEXT it must produce. The independent review arrived at the same
# method by patching this function locally; making it permanent means the
# property is enforced on every run rather than during one review.
expect_red() { # <check-fn> <dir> <label> <required stderr fragment>
  local fn="$1" dir="$2" label="$3" want="$4"
  local err="$WORK/expect_red.err"
  if "$fn" "$dir" 2>"$err"; then
    fail "mutation control: $fn accepted a tree that DOES carry the violation ($label) — this check cannot fail and is therefore not a gate"
  fi
  grep -Fq -- "$want" "$err" || {
    echo "  actual finding was:" >&2
    sed 's/^/    /' "$err" >&2
    fail "mutation control: $fn went red on '$label' but NOT for its stated reason — the finding was expected to mention: $want"
  }
  echo "OK: mutation control — $fn goes red on: $label"
}

# ------------------------------------------------ 1. SEC-04: no '@latest' --

echo "== 1. SEC-04: no mutable '@latest' install under .github/workflows/** =="
expect_green check_no_latest "$WORKFLOWS" "zero '@latest' references in the real workflows"

m="$(mutant no-latest)"
mutate "$m/verify.yaml" \
  's|cmd/task@"\${TASK_VERSION}"|cmd/task@latest|' \
  'cmd/task@latest'
expect_red check_no_latest "$m" "a Task install reverted to @latest" \
  'reference(s) — a mutable upstream release'

# ------------------------------------------- 2. SEC-04: Task pin sourcing --

echo "== 2. SEC-04: Task version pinned once, at workflow level, and interpolated =="
expect_green check_task_pinned "$WORKFLOWS" "both Task installs interpolate a single workflow-level TASK_VERSION"

m="$(mutant task-latest)"
mutate "$m/verify.yaml" \
  's|cmd/task@"\${TASK_VERSION}"|cmd/task@latest|' \
  'cmd/task@latest'
expect_red check_task_pinned "$m" "an install site stopped interpolating \${TASK_VERSION}" \
  'install site(s) not single-sourced through'

m="$(mutant task-skew)"
mutate "$m/verify.yaml" \
  's|cmd/task@"\${TASK_VERSION}"|cmd/task@v3.0.0|' \
  'cmd/task@v3.0.0'
expect_red check_task_pinned "$m" "an install site hard-codes its own version (the two jobs can skew)" \
  'install site(s) not single-sourced through'

m="$(mutant task-unset)"
mutate "$m/verify.yaml" \
  's|^  TASK_VERSION: v[0-9].*$|  TASK_VERSION: latest|' \
  'TASK_VERSION: latest'
expect_red check_task_pinned "$m" "TASK_VERSION is defined but is not a literal vX.Y.Z" \
  'expected exactly ONE TASK_VERSION definition'

m="$(mutant task-removed)"
mutate "$m/verify.yaml" \
  '/^  TASK_VERSION: v[0-9]/d' \
  'cmd/task@"${TASK_VERSION}"'
expect_red check_task_pinned "$m" "the workflow-level TASK_VERSION definition was deleted" \
  'expected exactly ONE TASK_VERSION definition'

# Review finding F3. A job-level env sits at six-space indent and overrides the
# workflow-level pin for that job — the exact skew the story exists to prevent,
# and previously invisible because only two-space definitions were counted.
m="$(mutant task-job-override)"
mutate_awk "$m/verify.yaml" \
  '{ print } /^  verify:$/ { print "    env:"; print "      TASK_VERSION: v3.0.0" }' \
  'TASK_VERSION: v3.0.0'
expect_red check_task_pinned "$m" "a JOB-level TASK_VERSION overrides the workflow-level pin (F3)" \
  'expected exactly ONE TASK_VERSION definition'

# -------------------------------- 3. SEC-03: credential-scrubbed checkouts --

echo "== 3. SEC-03: every actions/checkout sets persist-credentials: false =="
expect_green check_persist_credentials "$WORKFLOWS" "all actions/checkout steps scrub the workflow token"

m="$(mutant persist-verify)"
mutate "$m/verify.yaml" \
  '/persist-credentials: false/d' \
  'actions/checkout@'
expect_red check_persist_credentials "$m" "verify.yaml checkout(s) stopped scrubbing credentials" \
  'leaving the workflow token in .git/config'

m="$(mutant persist-schemas)"
mutate "$m/schemas.yml" \
  '/persist-credentials: false/d' \
  'actions/checkout@'
expect_red check_persist_credentials "$m" "the schemas.yml checkout stopped scrubbing credentials" \
  'leaving the workflow token in .git/config'

m="$(mutant persist-release)"
mutate "$m/release.yaml" \
  '/persist-credentials: false/d' \
  'actions/checkout@'
expect_red check_persist_credentials "$m" "release.yaml checkouts stopped scrubbing credentials (the highest-privilege job)" \
  'leaving the workflow token in .git/config'

# Review finding F1 — the fail-OPEN this check shipped with. A commented-out
# flag is not a flag: the token stays in .git/config while a naive presence
# grep reports OK. Reproduced under GNU grep 3.11 + mawk before the fix.
m="$(mutant persist-commented)"
mutate "$m/verify.yaml" \
  's|^          persist-credentials: false.*$|          # persist-credentials: false  # TEMPORARILY DISABLED|' \
  '# persist-credentials: false  # TEMPORARILY DISABLED'
expect_red check_persist_credentials "$m" "a persist-credentials flag was COMMENTED OUT rather than removed (F1)" \
  'leaving the workflow token in .git/config'

# Review finding F4 — the "step-scoped, not file-scoped" claim had zero
# coverage: the three mutations above delete EVERY occurrence in their file, so
# a file-scoped implementation would red on them identically. Deleting only the
# SECOND occurrence in release.yaml leaves the first intact, which a file-scoped
# check would happily accept. This is the control that proves the property.
m="$(mutant persist-second-only)"
mutate_awk "$m/release.yaml" \
  '/persist-credentials: false/ { n++; if (n == 2) next } { print }' \
  'persist-credentials: false'
expect_red check_persist_credentials "$m" "only the SECOND checkout in release.yaml lost the flag — proves step scoping, not file scoping (F4)" \
  'leaving the workflow token in .git/config'

# ------------------------------ 4. SEC-01: lockfile-pinned schema validator --

echo "== 4. SEC-01: the stock schema validator installs from a committed lockfile =="
expect_green check_npm_ci_in_workflow "$WORKFLOWS" "schemas.yml installs the validator with 'npm ci --ignore-scripts'"

m="$(mutant npm-install)"
mutate "$m/schemas.yml" \
  's|npm ci --ignore-scripts.*|npm install -g --ignore-scripts ajv-cli@5.0.0 ajv-formats@3.0.1|' \
  'npm install -g'
expect_red check_npm_ci_in_workflow "$m" "the job reverted to a run-time 'npm install' (unpinned transitives)" \
  'invokes npm in a form other than'

m="$(mutant npm-paths)"
mutate "$m/schemas.yml" \
  '/"hack\/schemas-validator\/\*\*"/d' \
  'npm ci --ignore-scripts'
expect_red check_npm_ci_in_workflow "$m" "the validator directory dropped out of the paths: filters" \
  'of its 2 paths: filters'

# Review finding F5, both halves in one mutant: the `npm ci` line is COMMENTED
# OUT (which the old presence grep still accepted) and replaced by
# `npm --global install`, a form the old `npm[[:space:]]+install` pattern did
# not match either. Both defects had to be fixed for this to red.
m="$(mutant npm-commented-global)"
mutate_awk "$m/schemas.yml" \
  '{ if ($0 ~ /run: npm ci --ignore-scripts --prefix/) { print "        # run: npm ci --ignore-scripts --prefix hack/schemas-validator"; print "        run: npm --global install ajv-cli@5.0.0 ajv-formats@3.0.1" } else print }' \
  'run: npm --global install'
expect_red check_npm_ci_in_workflow "$m" "'npm ci' was commented out and replaced by 'npm --global install' (F5)" \
  'does not RUN'

echo "== 4b. SEC-01: the committed manifest and lockfile are exact and complete =="
expect_green check_validator_lockfile "$VALIDATOR" "package.json pins exact versions and package-lock.json hash-pins the tree"

v="$(validator_mutant no-lock)"
rm -f "$v/package-lock.json"
expect_red check_validator_lockfile "$v" "package-lock.json was deleted" \
  'cannot run and the transitive tree is unpinned'

v="$(validator_mutant range)"
mutate "$v/package.json" \
  's|"ajv-cli": "5.0.0"|"ajv-cli": "^5.0.0"|' \
  '"ajv-cli": "^5.0.0"'
expect_red check_validator_lockfile "$v" "package.json loosened an exact pin into a caret range" \
  'pins a RANGE rather than an exact version'

v="$(validator_mutant no-integrity)"
mutate "$v/package-lock.json" \
  's|"integrity": "sha512-|"XXintegrityXX": "sha512-|' \
  '"XXintegrityXX"'
expect_red check_validator_lockfile "$v" "a resolved package lost its integrity hash" \
  'integrity hash(es)'

# --------------------------------------------------------------- CI wiring --
# `task check` runs this script (proven non-self-referentially by
# hack/release/changelog_gate_test.sh, which re-runs its wiring assertion
# against a Taskfile copy with the line deleted). The verify workflow must run
# it too, so a PR that unpins a workflow is caught before merge and not only on
# the author's machine.

echo "== 5. wiring: the verify workflow runs this gate =="
expect_green check_ci_wiring "$WORKFLOWS" "verify.yaml runs hack/lint/workflow_pins_test.sh"

m="$(mutant wiring-deleted)"
mutate "$m/verify.yaml" \
  '/run: bash hack\/lint\/workflow_pins_test.sh/d' \
  'actions/checkout@'
expect_red check_ci_wiring "$m" "the CI step invoking this gate was deleted" \
  'the workflow-pin gate would only ever run locally'

# F6: and commented out, which the previous straight-line `grep -Fqs` accepted.
m="$(mutant wiring-commented)"
mutate "$m/verify.yaml" \
  's|^        run: bash hack/lint/workflow_pins_test.sh$|        # run: bash hack/lint/workflow_pins_test.sh|' \
  '# run: bash hack/lint/workflow_pins_test.sh'
expect_red check_ci_wiring "$m" "the CI step invoking this gate was COMMENTED OUT (F6)" \
  'the workflow-pin gate would only ever run locally'

echo
echo "workflow_pins_test.sh: PASS"
