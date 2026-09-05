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

# D-157 — the PR-reach and step-wiring readers are SHARED with the other two
# PR-visible text gates (hack/audit/aud2_exitgate_test.sh,
# hack/examples/dogfood_wiring_test.sh). Sourced, not executed; it returns codes
# and prints nothing, so every finding below stays this script's own wording.
# The helper is held to THIS file's preamble rules (bash 3.2, portable ERE, no
# git, no `grep -q` on a pipe) because this gate is the strictest consumer: it
# runs first in CI, before any toolchain, and must work in `debian:stable-slim`.
PR_REACH_LIB="$ROOT/hack/lib/pr_reach.sh"
[[ -f "$PR_REACH_LIB" ]] || {
  echo "FAIL: missing $PR_REACH_LIB — the shared PR-reach/step-wiring helper (D-157). Refusing to run rather than skipping section 5." >&2
  exit 1
}
# shellcheck source=../lib/pr_reach.sh
. "$PR_REACH_LIB"

SELF_GATE="hack/lint/workflow_pins_test.sh"

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

# Review finding N1: `code_view` alone does NOT close the class it was credited
# with closing. It drops a line only when the FIRST non-blank character is `#`,
# so the flag text hiding in a live line's COMMENT TAIL still satisfied a
# presence grep. Three confirmed bypasses, gate green and actionlint clean:
#
#   fetch-depth: 1  # was: persist-credentials: false
#   run: npm --global install ajv-cli  # was: run: npm ci --ignore-scripts …
#   run: echo skipped  # was: bash hack/lint/workflow_pins_test.sh
#
# The second is the dangerous one — a plausible "just unblock it" edit that
# leaves the entire SEC-01 supply-chain pin inert while the gate reports OK.
#
# So presence checks no longer match raw lines. They match what actually
# EXECUTES: comment tail removed (YAML and shell agree — whitespace then `#`),
# then the list dash and the `run:` key stripped, leaving the command itself.
# Anchoring on that makes a comment tail structurally unable to satisfy a check.
# Verified beforehand that no workflow line carries ` #` inside a quoted value,
# which is the only case where tail-stripping could lose real content.
#
# Emits `<lineno>:<command>`.
command_view() { # <file>
  awk '{
    s = $0
    sub(/^[[:space:]]+/, "", s)
    if (substr(s, 1, 1) == "#") next          # whole-line comment
    sub(/[[:space:]]#.*$/, "", s)             # inline comment tail
    sub(/^-[[:space:]]+/, "", s)              # list item dash
    sub(/^run:[[:space:]]*/, "", s)           # the run: key itself
    sub(/[[:space:]]+$/, "", s)
    printf "%d:%s\n", FNR, s
  }' "$1"
}

command_view_dir() { # <dir>
  local f
  for f in "$1"/*.yml "$1"/*.yaml; do
    [[ -f "$f" ]] || continue
    command_view "$f" | sed "s|^|${f##*/}:|"
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

  # Install sites, read as COMMANDS (N1): the comment tail is stripped, so
  # `…cmd/task@v3.0.0  # cmd/task@"${TASK_VERSION}"` can no longer satisfy the
  # interpolation check below. That bypass was found by extending the review's
  # N1 class to this check — it was not in the reported set, but it is the same
  # defect, and it would have re-opened exactly the skew SEC-04 exists to stop.
  local cmds="$WORK/cmd.workflows"
  local code="$WORK/code.workflows"
  command_view_dir "$dir" >"$cmds"
  code_view_dir "$dir" >"$code"
  grep -F -- 'cmd/task@' "$cmds" >"$sites" || true
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
      /^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*actions\/checkout@/ && !is_comment { is_checkout = 1 }
      # N1: ANCHORED to the start of the line, with only an optional comment
      # tail allowed after the value. `fetch-depth: 1 # was: persist-credentials:
      # false` therefore no longer counts as the flag being set.
      /^[[:space:]]*persist-credentials:[[:space:]]*false[[:space:]]*(#.*)?$/ && !is_comment { has_pc = 1 }
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
  local cmds="$WORK/cmd.schemas"
  code_view "$file" >"$code"
  command_view "$file" >"$cmds"

  # N1: judged on the COMMAND, anchored at its start. An npm invocation is a
  # line whose command begins with `npm`, and it must begin with `npm ci`.
  # `run: npm --global install ajv-cli  # was: run: npm ci …` is now a finding
  # instead of a pass — previously it defeated SEC-01 outright while the gate
  # printed OK, because the comment tail contained the string being searched.
  local npm_lines="$WORK/hits.npm_lines"
  local bad="$WORK/hits.npm_install"
  grep -E '^[0-9]+:npm([[:space:]]|$)' "$cmds" >"$npm_lines" || true
  # Positive control on the extraction. Without it, an anchor that stops
  # matching leaves this sweeping an empty set and reporting clean over any
  # install at all — which is precisely what a wrong prefix anchor did here
  # during development, caught only because a sibling assertion still failed.
  if [[ ! -s "$npm_lines" ]]; then
    echo "  found NO npm command in schemas.yml — the job must install the validator, so the extraction is broken and every npm assertion below would be vacuous (SEC-01)" >&2
    return 1
  fi
  grep -Ev '^[0-9]+:npm[[:space:]]+ci([[:space:]]|$)' "$npm_lines" >"$bad" || true
  if [[ -s "$bad" ]]; then
    echo "  schemas.yml invokes npm in a form other than 'npm ci' — dependencies would resolve at run time instead of from the committed lockfile (SEC-01):" >&2
    sed 's/^/    /' "$bad" >&2
    rc=1
  fi

  # And the install must actually be RUN: a command that BEGINS with `npm ci`
  # and carries --ignore-scripts. Presence anywhere on the line is not enough.
  grep -E '^[0-9]+:npm[[:space:]]+ci([[:space:]].*)?[[:space:]]--ignore-scripts([[:space:]]|$)' "$cmds" >/dev/null || {
    echo "  schemas.yml does not RUN 'npm ci --ignore-scripts' as an actual command — the transitive tree is not lockfile-pinned (SEC-01)" >&2
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
  local rc=0
  [[ -f "$file" ]] || {
    echo "  missing $file" >&2
    return 1
  }

  # D-157, half one: THE WORKFLOW MUST REACH PULL REQUESTS. This gate asserted
  # its step was present and undisarmed and never asked whether the workflow
  # carrying that step runs on a PR at all — so `on: pull_request` deleted
  # outright, or narrowed with `paths:`, left every assertion below green while
  # the gate reached CI only through `task check` -> release-exitgate, whose
  # `if: github.event_name != 'pull_request'` is RELSE-08. The two sibling
  # gates DID ask, with `grep -qE '^[[:space:]]+pull_request:'`, which a
  # reviewer defeated with a paths-filtered trigger (rc=0, measured). The check
  # is now one shared reader, hack/lib/pr_reach.sh, for all three.
  local reach=0
  assent_pr_reach "$file" "$WORK" || reach=$?
  case "$reach" in
    0) : ;;
    2)
      echo "  verify.yaml has no top-level 'pull_request:' trigger — this gate would reach CI only through release-exitgate, which skips pull requests (RELSE-08), so a PR that unpins a workflow merges green and reddens main afterwards" >&2
      return 1
      ;;
    10)
      echo "  verify.yaml's pull_request trigger is something hack/lib/pr_reach.sh refuses to evaluate: either a shape it cannot read (a flow mapping, an anchor or an alias) or a filter key outside GitHub's five (types, branches, branches-ignore, paths, paths-ignore). A narrowing hidden in either would disarm this gate silently, so it fails closed rather than guessing" >&2
      return 1
      ;;
    11)
      echo "  verify.yaml's pull_request trigger carries a 'paths:' or 'paths-ignore:' filter — the trigger is PRESENT and the gate still loses every PR that does not touch the listed paths. A PR editing only .github/workflows/** could then unpin an action with this gate never running" >&2
      return 1
      ;;
    12)
      echo "  verify.yaml's pull_request trigger carries a 'types:' filter that omits one of GitHub's defaults (opened, synchronize, reopened) — 'types: [closed]' satisfies a presence grep and never fires on an open PR" >&2
      return 1
      ;;
    13)
      echo "  verify.yaml's pull_request trigger carries a branch filter that excludes 'main' — PRs onto the branch this gate protects would not run it" >&2
      return 1
      ;;
    *)
      echo "  assent_pr_reach returned an unmapped code $reach" >&2
      return 1
      ;;
  esac

  # D-157, half two: the STEP. The extraction, the disarm scan and the
  # comment-vs-command anchor all moved into assent_step_wired, which is the
  # strongest of the three gates' versions (N1/N2/N5 below were the findings
  # that shaped it) plus two the local copy never had: a job-level `needs:`,
  # and the one-`run:`-key/no-`uses:`-key isolation invariant that replaced the
  # 1..6 LINE cap. The cap could not survive cross-pinning: the aud2 and
  # dogfood steps carry comment blocks a dozen lines long, so no cap admits an
  # honest step of theirs while rejecting a merged one.
  #
  # N1 is preserved by the anchor: the command must BEGIN with the invocation.
  # `run: echo skipped  # was: bash hack/lint/workflow_pins_test.sh` is a
  # finding, not a pass.
  # N2/N5 are preserved by the disarm scan over the WHOLE isolated step, so
  # `continue-on-error:`/`if:` are caught in the conventional position (between
  # `- name:` and `run:`) as well as after `run:`.
  local wired=0
  assent_step_wired "$file" "$ASSENT_PR_JOB" "$SELF_GATE" "$WORK" || wired=$?
  case "$wired" in
    0) : ;;
    3)
      echo "  the '$ASSENT_PR_JOB' job block extracted empty or without a 'steps:' key — the extraction broke, so every assertion about this gate's step would be vacuous" >&2
      rc=1
      ;;
    4)
      echo "  the '$ASSENT_PR_JOB' job carries a JOB-LEVEL if: — if it skips pull requests this gate is push-only exactly like release-exitgate, and RELSE-08 is reproduced with the step still in place" >&2
      rc=1
      ;;
    5)
      echo "  verify.yaml does not RUN 'bash $SELF_GATE' as an actual command — the workflow-pin gate would only ever run locally" >&2
      rc=1
      ;;
    6)
      echo "  could not isolate the workflow-pin gate step in verify.yaml (its region is not exactly one step: either two 'run:' keys or a stray 'uses:' key, the shape a deleted '- name:' produces) — the extraction broke, so the assertions here would be vacuous" >&2
      rc=1
      ;;
    7)
      echo "  the workflow-pin gate step invokes $SELF_GATE WITH ARGUMENTS — this gate takes none, and an argument-carrying invocation is how a wired-looking step is hollowed out" >&2
      rc=1
      ;;
    8)
      echo "  the workflow-pin gate step is present but DISARMED — 'continue-on-error' or 'if:' means a red gate does not fail the job" >&2
      rc=1
      ;;
    9)
      echo "  the '$ASSENT_PR_JOB' job carries a JOB-LEVEL needs: — a dependency that skips on pull requests skips this job with it, so the gate is push-only by proxy while the step still looks wired" >&2
      rc=1
      ;;
    *)
      echo "  assent_step_wired returned an unmapped code $wired" >&2
      rc=1
      ;;
  esac
  return "$rc"
}

# D-157 residual 2 — CROSS-PINNING. Until now each of the three PR-visible text
# gates pinned only its OWN step (this one section 5, hack/audit/
# aud2_exitgate_test.sh's check_pr_wiring, hack/examples/
# dogfood_wiring_test.sh section 7); a reviewer grepped and confirmed none
# named another. So a PR that deleted one of those three steps was invisible to
# the gate still running on that PR — caught locally by `task check` and on
# push by release-exitgate, which is why it was a gap and not a hole. Each gate
# now asserts all three steps, so deleting ANY ONE reds on the pull request
# that does it, twice over.
check_pr_gate_cross_pins() {
  local dir="$1"
  local file="$dir/verify.yaml"
  local rc=0
  [[ -f "$file" ]] || {
    echo "  missing $file" >&2
    return 1
  }
  local others="$WORK/cross.others"
  if ! assent_pr_gate_others "$SELF_GATE" >"$others"; then
    echo "  $SELF_GATE is not listed in ASSENT_PR_GATES (hack/lib/pr_reach.sh) — the cross-pin set is not the set this gate belongs to, so this check would grade an unrelated pair" >&2
    return 1
  fi
  if ((${#ASSENT_PR_GATES[@]} != 3)); then
    echo "  ASSENT_PR_GATES holds ${#ASSENT_PR_GATES[@]} entries, expected the 3 PR-visible text gates — a fourth needs its own cross-pin, not a silently widened one" >&2
    return 1
  fi
  local n
  n="$(wc -l <"$others" | tr -d '[:space:]')"
  if [[ "$n" != "2" ]]; then
    echo "  expected exactly 2 sibling gates to cross-pin, got $n — the positive control on the shared list failed, so the assertions below would be vacuous" >&2
    return 1
  fi
  local g w
  while IFS= read -r g; do
    [[ -n "$g" ]] || continue
    if [[ ! -f "$ROOT/$g" ]]; then
      echo "  cross-pinned gate $g does not exist on disk — the pin would assert a step that runs nothing" >&2
      rc=1
      continue
    fi
    w=0
    assent_step_wired "$file" "$ASSENT_PR_JOB" "$g" "$WORK" || w=$?
    if ((w != 0)); then
      echo "  the sibling PR-visible gate $g is NOT wired undisarmed into the '$ASSENT_PR_JOB' job (assent_step_wired rc=$w: 3 job unextractable, 4 job-level if:, 5 no run: command, 6 step unisolatable, 7 arguments, 8 disarmed, 9 job-level needs:) — deleting one of the three gate steps must redden the PR that does it, and this is the half of that guarantee this script owns" >&2
      rc=1
    fi
  done <"$others"
  return "$rc"
}
# Review finding N6 — `command_view` strips from the leftmost ` #`, which is
# exactly YAML's rule for a PLAIN scalar (verified: `run: echo "step # 1" && …`
# really is truncated by YAML too, and actionlint reds it with SC1072). It is
# NOT the rule inside a BLOCK scalar (`run: |`) or a QUOTED scalar, where ` #`
# is ordinary content. There, stripping discards live code: a body of
# `echo "stage # 2" && npm --global install ajv-cli` reduces to `echo "stage`
# and SEC-01 goes inert with the gate green.
#
# The previous commit documented this as a precondition of the current tree.
# Documentation is not enforcement — the next workflow edit reopens it silently.
# This turns the precondition into a gate: no workflow line may contain ` #`
# preceded by an unbalanced quote. Zero hits across all 8 workflows today, so it
# lands without touching a single workflow file.
check_no_quoted_hash() {
  local dir="$1"
  local hits="$WORK/hits.quoted_hash"
  : >"$hits"
  local f
  for f in "$dir"/*.yml "$dir"/*.yaml; do
    [[ -f "$f" ]] || continue
    # The single quote is built with sprintf so the awk program can stay inside
    # a single-quoted shell string with no escaping games.
    awk -v short="${f##*/}" '
      BEGIN { SQ = sprintf("%c", 39); DQ = sprintf("%c", 34) }
      {
        idx = index($0, " #")
        if (idx == 0) next
        ndq = 0; nsq = 0
        for (i = 1; i < idx; i++) {
          c = substr($0, i, 1)
          if (c == DQ) ndq++
          else if (c == SQ) nsq++
        }
        if (ndq % 2 == 1 || nsq % 2 == 1) printf "%s:%d:%s\n", short, FNR, $0
      }
    ' "$f" >>"$hits"
  done
  if [[ -s "$hits" ]]; then
    echo "  workflow line(s) carry ' #' inside a quoted scalar — command_view treats that as a comment and would silently discard the rest of the line, disarming every check that reads the command (N6):" >&2
    sed 's/^/    /' "$hits" >&2
    return 1
  fi
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

  # Review finding F2. ajv-cli@5.0.0 declares `fast-json-patch: ^2.0.0`, and
  # 2.2.1 carries GHSA-8gh8-hqwg-xf34 (prototype pollution). An npm `overrides`
  # entry lifts it out of that range — which is what `overrides` is for. Both
  # halves are pinned here because either alone is silently reversible: drop the
  # override and the next lockfile regeneration walks straight back to 2.2.1.
  grep -Fq -- '"fast-json-patch": "^3.1.1"' "$pkg" || {
    echo "  package.json no longer overrides fast-json-patch to ^3.1.1 — the next lockfile regeneration resolves ajv-cli's declared ^2.0.0 and reintroduces GHSA-8gh8-hqwg-xf34 (SEC-01)" >&2
    rc=1
  }
  grep -Eq '/fast-json-patch-3\.' "$lock" || {
    echo "  package-lock.json does not resolve fast-json-patch to a 3.x release — the override is declared but not applied to the locked tree (SEC-01)" >&2
    rc=1
  }
  return "$rc"
}

# sonar_step_value <isolated-step-file> <normalised-key> — the VALUE of a step
# key, located by NORMALISED name so `if : `, `"if":` and `'if' :` reach it too,
# then normalised for comparison. _assent_map_keys deliberately returns names
# only; this mirrors its normalisation on the value side rather than duplicating
# a literal pattern, which is the defect it exists to prevent (G5-02).
sonar_step_value() {
  awk -v want="$2" '
    { line = $0
      gsub(/\r/, "", line)
      if (line ~ /^[[:space:]]*$/ || line ~ /^[[:space:]]*#/) next
      sub(/^[ ]*- /, "  ", line)
      match(line, /^[ ]*/)
      rest = substr(line, RLENGTH + 1)
      c = index(rest, ":")
      if (c == 0) next
      k = substr(rest, 1, c - 1)
      sub(/[[:space:]]+$/, "", k); sub(/^[[:space:]]+/, "", k)
      if (k ~ /^".*"$/ || k ~ /^\x27.*\x27$/) k = substr(k, 2, length(k) - 2)
      sub(/[[:space:]]+$/, "", k); sub(/^[[:space:]]+/, "", k)
      if (k != want) next
      v = substr(rest, c + 1)
      gsub(/\$\{\{/, "", v); gsub(/\}\}/, "", v)
      sub(/[ \t]+#.*$/, "", v)
      sub(/^[ \t]+/, "", v); sub(/[ \t]+$/, "", v)
      if (v ~ /^".*"$/ || v ~ /^\x27.*\x27$/) v = substr(v, 2, length(v) - 2)
      sub(/^[ \t]+/, "", v); sub(/[ \t]+$/, "", v)
      print tolower(v)
      exit
    }' "$1"
}

# D-177: the SonarCloud scanner step is PRESENT, SHA-pinned, ARMED, and ORDERED
# after the coverage gate that writes the profile it uploads.
#
# This gate exists because of how the analysis was lost the first time. Automatic
# Analysis was switched off on 2026-08-04 in favour of CI-based analysis, the
# scanner half was never built, and the project sat a month with no analysis at
# all while the backlog kept citing frozen figures. Nothing went red, because
# nothing was watching. Deleting this step would restore exactly that state, so
# the step is pinned here the way every other gate step in this file is.
check_sonar_scan_wired() {
  local dir="$1"
  local wf="$dir/verify.yaml"
  local rc=0

  [[ -f "$wf" ]] || {
    echo "  missing $wf (D-177)" >&2
    return 1
  }

  # Positive control: both anchors must exist before any ordering claim is made.
  # Without this an extraction that silently stopped matching would report
  # "ordered correctly" from two empty results.
  local scan_ln cov_ln
  scan_ln="$(awk '/uses: SonarSource\/sonarqube-scan-action@/ { print FNR; exit }' "$wf")"
  cov_ln="$(awk '/^        run: task coverage$/ { print FNR; exit }' "$wf")"

  if [[ -z "$scan_ln" ]]; then
    echo "  verify.yaml runs no SonarSource/sonarqube-scan-action step — CI-based analysis is the ONLY analysis this project has since Automatic Analysis was disabled (D-150/D-177), so deleting it ends all Sonar coverage silently" >&2
    return 1
  fi
  if [[ -z "$cov_ln" ]]; then
    echo "  verify.yaml has no 'run: task coverage' step — the anchor this check orders the scanner against is gone, so the ordering assertion below would be vacuous (D-177)" >&2
    return 1
  fi

  # The action must be pinned to a full 40-hex commit SHA. A tag is mutable and
  # would let an upstream retag change what runs inside the gate.
  local pinned
  pinned="$(awk '/uses: SonarSource\/sonarqube-scan-action@[0-9a-f]{40}([ \t]|$)/ { n++ } END { print n+0 }' "$wf")"
  if [[ "$pinned" -lt 1 ]]; then
    echo "  the SonarSource/sonarqube-scan-action reference is not pinned to a 40-hex commit SHA (D-177 / SEC-04) — a mutable tag lets an upstream retag change gate behaviour" >&2
    rc=1
  fi

  # cov.out is written by `task coverage`. A scanner step ordered BEFORE it
  # uploads no coverage and does not fail while doing so.
  if [[ "$scan_ln" -lt "$cov_ln" ]]; then
    echo "  the SonarCloud scan step (line $scan_ln) runs BEFORE the coverage gate (line $cov_ln) — sonar.go.coverage.reportPaths reads cov.out, which 'task coverage' writes, so this ordering leaves the report path pointing at a file that does not yet exist, which the scanner treats as a warning — coverage goes silently unreported rather than failing (D-177)" >&2
    rc=1
  fi

  # The step must not be disarmed. `continue-on-error` and a constant `if:` both
  # leave the step present and grep-satisfying while it stops gating anything --
  # the shapes hack/audit/exitgate_test.sh already refuses on its own gate steps.
  local block="$WORK/sonar.block"
  awk -v want="$scan_ln" '
    { line[FNR] = $0 }
    END {
      for (i = 1; i <= FNR; i++) {
        if (line[i] ~ /^      - /) s = i
        if (i == want) { for (j = s; j <= FNR; j++) { if (j > s && line[j] ~ /^      - /) break; print line[j] } }
      }
    }
  ' "$wf" >"$block"

  if [[ ! -s "$block" ]]; then
    echo "  the SonarCloud step block extraction returned nothing — every disarm assertion below would be vacuous (D-177)" >&2
    return 1
  fi

  # DISARM DETECTION IS GRADED BY NORMALISED KEY NAME, NOT BY LITERAL PATTERN.
  # This check first shipped with fresh literal readers (`grep -nE "^[ ]+continue-on-error:"`,
  # `awk '/^[ ]+if:/'`) and independent review measured FIVE spellings walking
  # straight past them while disarming the step for a conformant parser:
  # `if : ${{ false }}`, `"if": false`, `if: ${{ false }}<CR>`,
  # `continue-on-error : true` and `"continue-on-error": true`. That is G5-02,
  # already documented and already SOLVED in hack/lib/pr_reach.sh, which this
  # file sources. Writing a sixth literal reader beside a shared normalising one
  # is precisely what D-157 exists to stop, so the shared reader is used here.
  local keys="$WORK/sonar.stepkeys"
  _assent_step_keys "$block" >"$keys"

  if grep -qx '?unreadable' "$keys"; then
    echo "  the SonarCloud scan step carries a mapping key that cannot be normalised to a plain identifier — refused rather than guessed, because an unreadable key is exactly where a disarm hides (G5-02 / D-177)" >&2
    return 1
  fi

  # Presence is refused, not just a truthy value: a gate step has no legitimate
  # reason to carry continue-on-error at all, and grading the VALUE would re-open
  # the spelling problem one level down.
  if grep -qx 'continue-on-error' "$keys"; then
    echo "  the SonarCloud scan step carries continue-on-error — present, green, and gating nothing, which is the exact silence D-177 exists to end" >&2
    rc=1
  fi

  # `if:` is legitimate here (the fork-PR guard), so the VALUE is graded. It is
  # located by normalised key name so the alternate spellings above reach it, then
  # normalised the way the AUD gate normalises its own: CR stripped, ${{ }}
  # unwrapped, comment tail dropped, quotes stripped, trimmed, lowercased.
  if grep -qx 'if' "$keys"; then
    local ifval
    ifval="$(sonar_step_value "$block" if)"
    if [[ "$ifval" == "false" || "$ifval" == "true" ]]; then
      echo "  the SonarCloud scan step's if: normalises to the constant '$ifval' — a constant condition disarms the step while leaving it present and SUCCESS-reporting (D-177)" >&2
      rc=1
    fi
  fi

  return "$rc"
}

# D-177: the properties file that makes CI-based analysis possible at all is
# present and names the project SonarCloud actually holds. Its presence is also
# the switch that keeps Automatic Analysis off -- the two are mutually exclusive.
check_sonar_properties() {
  local dir="$1"
  local f="$dir/sonar-project.properties"
  local rc=0

  [[ -f "$f" ]] || {
    echo "  missing sonar-project.properties — without it the scanner has no project key and Automatic Analysis silently resumes ownership (D-150/D-177)" >&2
    return 1
  }

  local key org
  key="$(awk -F= '/^sonar\.projectKey=/ { print $2; exit }' "$f")"
  org="$(awk -F= '/^sonar\.organization=/ { print $2; exit }' "$f")"

  # The key is verified live, not guessed: api/components/show reports
  # PlatformRelay_assent (display name "Assent"), and the capitalised key does not
  # exist. Changing this creates a SECOND project and orphans all history.
  if [[ "$key" != "PlatformRelay_assent" ]]; then
    echo "  sonar.projectKey is '$key', expected 'PlatformRelay_assent' — SonarCloud project keys are immutable and independent of the GitHub repo name; a different key silently creates a NEW project and orphans every historical measurement (D-177)" >&2
    rc=1
  fi
  if [[ "$org" != "platformrelay" ]]; then
    echo "  sonar.organization is '$org', expected 'platformrelay' (D-177)" >&2
    rc=1
  fi

  # The coverage wiring is the reason the workflow ordering above is pinned.
  local cov
  cov="$(awk -F= '/^sonar\.go\.coverage\.reportPaths=/ { print $2; exit }' "$f")"
  if [[ "$cov" != "cov.out" ]]; then
    echo "  sonar.go.coverage.reportPaths is '$cov', expected 'cov.out' — that is the profile 'task coverage' writes, and the workflow ordering pin above is meaningless if this points elsewhere (D-177)" >&2
    rc=1
  fi

  # D-150's actual purpose. Losing this block silently re-floods the project with
  # the test-file S3776 smells the decision exists to exempt. No count here: the
  # live figure moves with every analysis (D-177 carries it, dated).
  local rule res
  rule="$(awk -F= '/^sonar\.issue\.ignore\.multicriteria\.e1\.ruleKey=/ { print $2; exit }' "$f")"
  res="$(awk -F= '/^sonar\.issue\.ignore\.multicriteria\.e1\.resourceKey=/ { print $2; exit }' "$f")"
  if [[ "$rule" != "go:S3776" || "$res" != '**/*_test.go' ]]; then
    echo "  the D-150 S3776 test-file exemption is missing or altered (ruleKey='$rule', resourceKey='$res') — that exemption is the ONLY reason this file exists instead of Automatic Analysis (D-150/D-177)" >&2
    rc=1
  fi

  return "$rc"
}

sonar_props_mutant() { # <name> -> prints a fresh dir holding a copy of sonar-project.properties
  local name="$1"
  local dir="$WORK/smutant-$name"
  rm -rf "$dir"
  mkdir -p "$dir"
  cp "$ROOT/sonar-project.properties" "$dir/"
  printf '%s\n' "$dir"
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

# N1 extended to this check by our own audit: the real interpolation is
# replaced by a hard-coded version and the expected text hidden in a comment
# tail, which the pre-N1 substring match accepted.
m="$(mutant task-tail-comment)"
mutate "$m/verify.yaml" \
  's|cmd/task@"\${TASK_VERSION}"|cmd/task@v3.0.0  # cmd/task@"${TASK_VERSION}"|' \
  'cmd/task@v3.0.0  # cmd/task@'
expect_red check_task_pinned "$m" "a hard-coded version hides behind the expected text in a COMMENT TAIL (N1)" \
  'install site(s) not single-sourced through'

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

# Review finding N1 — the flag text survives in a live line's COMMENT TAIL,
# which `code_view` alone did not catch: the line is not a comment, so it was
# kept, and an unanchored substring match found the flag inside it.
m="$(mutant persist-tail-comment)"
mutate "$m/verify.yaml" \
  's|^          persist-credentials: false.*$|          fetch-depth: 1  # was: persist-credentials: false|' \
  'fetch-depth: 1  # was: persist-credentials: false'
expect_red check_persist_credentials "$m" "the flag survives only in a COMMENT TAIL on a live line (N1)" \
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

# Review finding N1, the highest-impact instance: a single plausible
# "just unblock it" edit that left the ENTIRE SEC-01 supply-chain pin inert
# while the gate reported OK, because the searched-for text sat in the tail.
m="$(mutant npm-tail-comment)"
mutate "$m/schemas.yml" \
  's|^        run: npm ci --ignore-scripts --prefix hack/schemas-validator$|        run: npm --global install ajv-cli  # was: run: npm ci --ignore-scripts --prefix hack/schemas-validator|' \
  'run: npm --global install ajv-cli  # was:'
expect_red check_npm_ci_in_workflow "$m" "the lockfile install survives only in a COMMENT TAIL beside a live global install (N1)" \
  'invokes npm in a form other than'

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

v="$(validator_mutant no-override)"
mutate "$v/package.json" \
  '/"fast-json-patch": "\^3.1.1"/d' \
  '"ajv-cli": "5.0.0"'
expect_red check_validator_lockfile "$v" "the fast-json-patch override was removed, letting GHSA-8gh8-hqwg-xf34 back in (F2)" \
  'no longer overrides fast-json-patch'

# Review finding N4: only the package.json half of the F2 guard had a control
# (the mutant above leaves the lock untouched). This is the other half — the
# override stays declared while the LOCKED tree walks back to the vulnerable
# release, which is exactly what a careless regeneration would produce.
v="$(validator_mutant lock-reverted)"
mutate "$v/package-lock.json" \
  's|fast-json-patch-3\.1\.1\.tgz|fast-json-patch-2.2.1.tgz|' \
  'fast-json-patch-2.2.1.tgz'
expect_red check_validator_lockfile "$v" "the lockfile reverted to fast-json-patch 2.2.1 while the override stayed declared (N4)" \
  'does not resolve fast-json-patch to a 3.x release'

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

# N1 again: the invocation survives only in a comment tail on a live line.
m="$(mutant wiring-tail-comment)"
mutate "$m/verify.yaml" \
  's|^        run: bash hack/lint/workflow_pins_test.sh$|        run: echo skipped  # was: bash hack/lint/workflow_pins_test.sh|' \
  'run: echo skipped  # was: bash'
expect_red check_ci_wiring "$m" "the invocation survives only in a COMMENT TAIL beside a live no-op (N1)" \
  'the workflow-pin gate would only ever run locally'

# Review finding N2: present but DISARMED. actionlint stays green on this, which
# is what makes it the realistic "unblock CI" edit rather than an obvious one.
m="$(mutant wiring-continue-on-error)"
mutate_awk "$m/verify.yaml" \
  '{ print } /run: bash hack\/lint\/workflow_pins_test.sh/ { print "        continue-on-error: true" }' \
  'continue-on-error: true'
expect_red check_ci_wiring "$m" "the gate step was neutered with continue-on-error: true AFTER run: (N2)" \
  'present but DISARMED'

m="$(mutant wiring-if-false)"
mutate_awk "$m/verify.yaml" \
  '{ print } /run: bash hack\/lint\/workflow_pins_test.sh/ { print "        if: false" }' \
  'if: false'
expect_red check_ci_wiring "$m" "the gate step was neutered with if: false AFTER run: (N2)" \
  'present but DISARMED'

# Review finding N5. The two controls above insert AFTER `run:`. The
# conventional position — the one GitHub's own docs use, and the one a person
# reaching for `continue-on-error` would actually type — is BEFORE it, between
# `- name:` and `run:`. The old extraction could not see that region at all, so
# these two controls certified the only position that happened to work. Both
# keys are now exercised in the position that previously bypassed the gate
# silently (continue-on-error: gate green AND actionlint green).
m="$(mutant wiring-continue-on-error-before)"
mutate_awk "$m/verify.yaml" \
  '{ if ($0 ~ /^        run: bash hack\/lint\/workflow_pins_test.sh$/) print "        continue-on-error: true"; print }' \
  'continue-on-error: true'
expect_red check_ci_wiring "$m" "continue-on-error inserted BEFORE run:, the conventional position (N5)" \
  'present but DISARMED'

m="$(mutant wiring-if-before)"
mutate_awk "$m/verify.yaml" \
  '{ if ($0 ~ /^        run: bash hack\/lint\/workflow_pins_test.sh$/) print "        if: ${{ false }}"; print }' \
  'if: ${{ false }}'
expect_red check_ci_wiring "$m" "if: inserted BEFORE run:, the conventional position (N5)" \
  'present but DISARMED'

# N5 secondary: the extraction must isolate a STEP, not a slab of file header.
# The old one swallowed a slab of the file header and still reported non-empty.
#
# G3-03: this mutation used to be `sed '/^      - name: workflow supply-chain
# pins/d'` — coupled to the literal TEXT of the step's name line. Any unrelated
# edit that stops that line starting with `      - ` (renaming the step, or
# giving the step a different first key, which is exactly the G3-02 disarm
# shape) made the sed a no-op, and `mutate` then reported "mutation harness:
# sed program … did not change" — a BROKEN-HARNESS message for a workflow that
# was merely different. A real finding elsewhere in the family was masked by
# that misreport once already. The step start is now DERIVED: find the line
# that runs this gate, walk back to the nearest `      - ` step marker, delete
# that marker line. No step name, and no other step's text, appears here.
m="$(mutant wiring-name-removed)"
mutate_awk "$m/verify.yaml" \
  '{ lines[NR] = $0; if ($0 ~ /^      - /) starts[NR] = 1; if ($0 ~ /^[[:space:]]*run: bash hack\/lint\/workflow_pins_test\.sh[[:space:]]*$/) target = NR }
   END { s = 0; for (i = target; i >= 1; i--) if (i in starts) { s = i; break }
         for (i = 1; i <= NR; i++) if (i != s) print lines[i] }' \
  'run: bash hack/lint/workflow_pins_test.sh'
# Positive control on the mutation itself: exactly one step marker fewer, and
# the gate's own run line untouched. Without this the derived deletion could
# silently remove the wrong line (or none) and the expect_red below would be
# grading something else.
before_starts="$WORK/hits.starts_before"
after_starts="$WORK/hits.starts_after"
grep -cE '^      - ' "$WORKFLOWS/verify.yaml" >"$before_starts" || true
grep -cE '^      - ' "$m/verify.yaml" >"$after_starts" || true
[[ "$(cat "$after_starts")" -eq "$(($(cat "$before_starts") - 1))" ]] ||
  fail "mutation control 'wiring-name-removed': expected exactly one fewer step marker, got $(cat "$before_starts") -> $(cat "$after_starts") — the derived step-start deletion did not land"
expect_red check_ci_wiring "$m" "the step's own start marker is gone, so it merged into the checkout step above (N5)" \
  'the extraction broke'

m="$(mutant wiring-arguments)"
mutate "$m/verify.yaml" \
  's|^        run: bash hack/lint/workflow_pins_test.sh$|        run: bash hack/lint/workflow_pins_test.sh --quick|' \
  'workflow_pins_test.sh --quick'
expect_red check_ci_wiring "$m" "the step still names the gate but passes it an argument" \
  'WITH ARGUMENTS'

# ---- D-157: the job-level routes that leave the step untouched --------------
m="$(mutant wiring-job-if)"
mutate_awk "$m/verify.yaml" \
  '{ print; if ($0 == "  verify:") print "    if: github.event_name != '"'"'pull_request'"'"'" }' \
  "if: github.event_name != 'pull_request'"
expect_red check_ci_wiring "$m" "the verify JOB was given release-exitgate's push-only guard, step untouched" \
  'JOB-LEVEL if:'

# A job that `needs:` a job which skips on pull requests is itself skipped —
# every other assertion here stays green and the gate never runs on a PR.
m="$(mutant wiring-job-needs)"
mutate_awk "$m/verify.yaml" \
  '{ print; if ($0 == "  verify:") print "    needs: release-exitgate" }' \
  'needs: release-exitgate'
expect_red check_ci_wiring "$m" "the verify JOB was made to depend on the push-only release-exitgate job" \
  'JOB-LEVEL needs:'

m="$(mutant wiring-job-renamed)"
mutate "$m/verify.yaml" \
  's|^  verify:$|  verify-all:|' \
  'verify-all:'
expect_red check_ci_wiring "$m" "the 'verify' job was renamed, so the job extraction finds nothing" \
  'the extraction broke'

# ------------------- 5b. D-157: the workflow must REACH pull requests --------
#
# This gate never asked. The two sibling gates did, with
# `grep -qE '^[[:space:]]+pull_request:'`, and a reviewer defeated that grep
# with a paths-filtered trigger and measured rc=0. Both halves are controlled
# here now: the ones that satisfy the old grep are marked, because they are the
# whole reason hack/lib/pr_reach.sh exists.

# ---------------- 5a. the SHARED HELPER itself, against fixtures -------------
#
# Every other assertion in sections 5..5c mutates verify.yaml and asks
# hack/lib/pr_reach.sh about it. None of that can tell a working helper from a
# stubbed one: replace `assent_pr_reach` with `return 0` and this gate AND both
# sibling gates go green at once, because the helper is now a single point of
# failure for all three. So it is also driven against workflows written FROM
# SCRATCH, minimal and self-evidently reaching or not, where the right answer
# depends on nothing under .github/workflows/**.
# hack/examples/dogfood_wiring_test.sh carries the same fixtures, so deleting
# either section alone does not remove the property.

echo "== 5a. hack/lib/pr_reach.sh answers correctly on from-scratch fixtures =="
helper_fixture() { # <name> <heredoc on stdin> -> prints the path
  local f="$WORK/fixture.$1.yaml"
  cat >"$f"
  printf '%s' "$f"
}
helper_reach_is() { # <want-rc> <file> <label>
  local want="$1" f="$2" label="$3" got=0
  assent_pr_reach "$f" "$WORK" || got=$?
  ((got == want)) ||
    fail "helper self-check '$label': assent_pr_reach returned $got, want $want — hack/lib/pr_reach.sh does not do what this gate and both sibling gates delegate to it"
  echo "OK: helper self-check — $label (rc=$want)"
}
helper_wired_is() { # <want-rc> <file> <script> <label>
  local want="$1" f="$2" s="$3" label="$4" got=0
  assent_step_wired "$f" "$ASSENT_PR_JOB" "$s" "$WORK" || got=$?
  ((got == want)) ||
    fail "helper self-check '$label': assent_step_wired returned $got, want $want — the shared step reader does not do what all three gates delegate to it"
  echo "OK: helper self-check — $label (rc=$want)"
}

f="$(helper_fixture reach-min <<'FIX'
name: f
on:
  pull_request:
jobs: {}
FIX
)"
helper_reach_is 0 "$f" "a bare 'on: pull_request:' reaches every PR"

f="$(helper_fixture reach-paths <<'FIX'
name: f
on:
  push:
    branches: [main]
  pull_request:
    paths: [internal/**]
jobs: {}
FIX
)"
helper_reach_is 11 "$f" "a paths:-filtered pull_request does NOT — and it is the LAST key in on:, so the filter is not read from a sibling trigger"

f="$(helper_fixture reach-none <<'FIX'
name: f
on:
  push:
    branches: [main]
jobs: {}
FIX
)"
helper_reach_is 2 "$f" "a push-only workflow reaches no PR"

f="$(helper_fixture wired-min <<'FIX'
name: f
on:
  pull_request:
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - name: gate
        run: bash hack/lint/workflow_pins_test.sh
FIX
)"
helper_wired_is 0 "$f" "$SELF_GATE" "a minimal wired step is accepted"
helper_wired_is 5 "$f" "hack/audit/aud2_exitgate_test.sh" "a script the fixture does not run is reported absent"

# G3-01/G3-02 — the codes the original fixture set did not cover. Both P1s
# lived in uncovered codes; the matrix was broad in mutant shapes and narrow in
# code coverage. Filter fixtures are written at a NON-4-space indent on purpose.
f="$(helper_fixture reach-paths-6sp <<'FIX'
name: f
on:
  pull_request:
      paths: [internal/**]
jobs: {}
FIX
)"
helper_reach_is 11 "$f" "paths: at SIX spaces is still a paths: filter (G3-01 — the 4-space grep fell through to 0 here)"

f="$(helper_fixture reach-types-6sp <<'FIX'
name: f
on:
  pull_request:
      types: [closed]
jobs: {}
FIX
)"
helper_reach_is 12 "$f" "types: [closed] at SIX spaces still omits the defaults (G3-01)"

f="$(helper_fixture reach-bi-6sp <<'FIX'
name: f
on:
  pull_request:
      branches-ignore: [main]
jobs: {}
FIX
)"
helper_reach_is 13 "$f" "branches-ignore: [main] at SIX spaces still excludes main (G3-01)"

f="$(helper_fixture reach-bi-quoted-hash <<'FIX'
name: f
on:
  pull_request:
    branches-ignore: ['x #y', 'main']
jobs: {}
FIX
)"
helper_reach_is 13 "$f" "a quoted ' #' before 'main' in branches-ignore does not hide it (G3-04)"

# The property that keeps G3-01's indent-agnostic widening honest, MEASURED
# rather than reasoned: a key's value region must stop at a SAME-INDENT sibling
# key. If it swallowed the sibling, `branches: [release]` followed by
# `branches-ignore: [main]` would merge into one token set, `main` would appear
# among the `branches:` tokens, and a main-excluding filter would look fine.
f="$(helper_fixture reach-sibling-scope <<'FIX'
name: f
on:
  pull_request:
    branches: [release]
    branches-ignore: [main]
jobs: {}
FIX
)"
helper_reach_is 13 "$f" "a key's value region stops at a SAME-INDENT sibling — 'main' in branches-ignore does not leak into the branches: token set (the bound G3-01's widening rests on)"

f="$(helper_fixture reach-legit-6sp <<'FIX'
name: f
on:
  pull_request:
      types: [opened, synchronize, reopened, ready_for_review]
      branches: [main]
jobs: {}
FIX
)"
helper_reach_is 0 "$f" "the legitimate shapes are still accepted at SIX spaces — the fix widened the match, it did not make the check indent-hostile"

f="$(helper_fixture reach-unknown-key <<'FIX'
name: f
on:
  pull_request:
    only-when: [tuesday]
jobs: {}
FIX
)"
helper_reach_is 10 "$f" "an UNKNOWN key under pull_request: is REFUSED rather than accepted"

f="$(helper_fixture wired-if-first <<'FIX'
name: f
on:
  pull_request:
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - if: ${{ github.event_name != 'pull_request' }}
        name: gate
        run: bash hack/lint/workflow_pins_test.sh
FIX
)"
helper_wired_is 8 "$f" "$SELF_GATE" "a step whose FIRST key is 'if:' is DISARMED (G3-02 — the sequence-item form was never matched and this graded WIRED)"

f="$(helper_fixture wired-coe-first <<'FIX'
name: f
on:
  pull_request:
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - continue-on-error: true
        name: gate
        run: bash hack/lint/workflow_pins_test.sh
FIX
)"
helper_wired_is 8 "$f" "$SELF_GATE" "a step whose FIRST key is 'continue-on-error:' is DISARMED (G3-02)"

# G3R-01/G3R-02/G3R-03 — the shapes the round-2 fixture set still could not
# see. Round 2 closed the CODE-coverage complaint (every reach and wired code
# got a fixture) but wrote every one of them at 4 or 6 spaces with sequence
# items indented DEEPER than their key, so it reproduced the same indentation
# blind spot somewhere cheaper. FLUSH block sequences — the canonical GitHub
# Actions style, item at the same indent as the key — are covered here, in both
# polarities, because that is the style a maintainer is most likely to write.
V="$(helper_fixture reach-flush-legit <<'FIX'
name: f
on:
  pull_request:
    types:
    - opened
    - synchronize
    - reopened
    - ready_for_review
    branches:
    - main
jobs: {}
FIX
)"
helper_reach_is 0 "$V" "a FULLY legitimate trigger with both filters as FLUSH block sequences is accepted (G3R-01 — this returned 10, refusing valid structure, and 12 before the allowlist existed)"

V="$(helper_fixture reach-flush-bi-legit <<'FIX'
name: f
on:
  pull_request:
    branches-ignore:
    - gh-pages
jobs: {}
FIX
)"
helper_reach_is 0 "$V" "branches-ignore: as a FLUSH block sequence sparing main is accepted (G3R-01 — the case the key allowlist actively BROKE: 0 before it, 10 after)"

V="$(helper_fixture reach-flush-bad <<'FIX'
name: f
on:
  pull_request:
    branches:
    - release
jobs: {}
FIX
)"
helper_reach_is 13 "$V" "a FLUSH branches: that excludes main still reds — the fix reads flush sequences, it does not wave them through"

V="$(helper_fixture reach-substring-branch <<'FIX'
name: f
on:
  pull_request:
    branches: [main-next]
jobs: {}
FIX
)"
helper_reach_is 13 "$V" "branches: [main-next] excludes main (G3R-02 — the tokenizer split on '-', yielded a 'main' token, and reported 'runs on every PR' for a filter under which no PR onto main runs it)"

V="$(helper_fixture reach-substring-path <<'FIX'
name: f
on:
  pull_request:
    branches: ['release/main']
jobs: {}
FIX
)"
helper_reach_is 13 "$V" "branches: ['release/main'] excludes main — same defect through '/' (G3R-02)"

V="$(helper_fixture reach-maintenance <<'FIX'
name: f
on:
  pull_request:
    branches: [maintenance]
jobs: {}
FIX
)"
helper_reach_is 13 "$V" "the control that kept G3R-02 hidden: [maintenance] reddened correctly all along, because only values containing 'main' as a SUBSTRING broke"

V="$(helper_fixture reach-main-exact <<'FIX'
name: f
on:
  pull_request:
    branches: [main]
jobs: {}
FIX
)"
helper_reach_is 0 "$V" "…and the tokenizer fix did not make the match hostile: an exact [main] is still accepted"

V="$(helper_fixture wired-bare-run <<'FIX'
name: f
on:
  pull_request:
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - run: bash hack/lint/workflow_pins_test.sh
FIX
)"
helper_wired_is 0 "$V" "$SELF_GATE" "a step written as a bare '- run:' with no name: is WIRED (G3R-03 — the sequence-item form verify.yaml already uses three times, which reported rc=5 'deleted, commented out': a false red with a wrong diagnosis)"

V="$(helper_fixture wired-bare-run-args <<'FIX'
name: f
on:
  pull_request:
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - run: bash hack/lint/workflow_pins_test.sh --quick
FIX
)"
helper_wired_is 7 "$V" "$SELF_GATE" "…and the bare '- run:' form is still graded for ARGUMENTS, not merely admitted (G3R-03)"

# G4-01 — a stray CR on a sequence-item line. These fixtures are built with
# printf rather than a heredoc because the CR has to be a REAL byte: that is
# the whole point of the defect. A single committed CR was enough (no
# .gitattributes here, core.autocrlf=false preserves it, no gate scans for it,
# and a GitHub diff renders it invisibly), and it made `branches-ignore:` — the
# ONE must-NOT-contain test in this reader — report "runs on every pull
# request" for a trigger that EXCLUDES main. Measured rc=0 at 54de1a4; both
# earlier readers said 13, so it was a regression the round-3 tokenizer
# introduced. Both polarities are pinned at every consumer, because the CR also
# caused FALSE REDS on `branches:` and `types:` and a fix for one direction is
# not a fix for the other.
cr_fixture() { # <name> <key> <item> — the item line carries a CR
  local f="$WORK/fixture.$1.yaml"
  {
    printf 'name: f\non:\n  pull_request:\n    %s:\n' "$2"
    printf '      - %s\r\n' "$3"
    printf 'jobs: {}\n'
  } >"$f"
  grep -q "$(printf '\r')" "$f" ||
    fail "fixture $1 carries no CR — the case it exists to cover is not present, so the assertion below would be vacuous"
  printf '%s' "$f"
}

V="$(cr_fixture bi-cr-bad branches-ignore main)"
helper_reach_is 13 "$V" "branches-ignore: '- main' with a CR still EXCLUDES main (G4-01 — THE fail-open: rc=0 at 54de1a4, on a trigger under which no PR onto main runs the workflow)"

V="$(cr_fixture bi-cr-legit branches-ignore gh-pages)"
helper_reach_is 0 "$V" "branches-ignore: '- gh-pages' with a CR is still accepted — the CR fix is not comment-hostile in the other direction (G4-01)"

V="$(cr_fixture br-cr-legit branches main)"
helper_reach_is 0 "$V" "branches: '- main' with a CR is still accepted (G4-01 — this was a FALSE RED at 54de1a4, the fail-closed half of the same defect)"

V="$(cr_fixture br-cr-bad branches release)"
helper_reach_is 13 "$V" "branches: '- release' with a CR still excludes main (G4-01, bad polarity)"

V="$WORK/fixture.types-cr.yaml"
{
  printf 'name: f\non:\n  pull_request:\n    types:\n'
  printf '      - opened\r\n'
  printf '      - synchronize\n      - reopened\n'
  printf 'jobs: {}\n'
} >"$V"
helper_reach_is 0 "$V" "types: with a CR on one item still reads the full default set (G4-01 — a false red at 54de1a4)"

# The structural half, and the reason this round is not a third character-class
# patch: on the ONE must-NOT-contain key, a token carrying a byte the reader
# does not understand is REFUSED rather than searched. A future tokenizer
# defect there can then produce a red or a refusal, never a silent accept.
V="$WORK/fixture.bi-unreadable.yaml"
{
  printf 'name: f\non:\n  pull_request:\n    branches-ignore:\n'
  printf '      - ma\001in\n'
  printf 'jobs: {}\n'
} >"$V"
helper_reach_is 10 "$V" "a branches-ignore: token carrying an unreadable byte is REFUSED, not searched — the fail-open direction is now closed by construction rather than by enumerating bytes (G4-01)"

# G5-01 — a GLOB on `branches-ignore:`. Every pattern below EXCLUDES main, so
# no pull request onto main runs the workflow; the reader returned 0, "runs on
# every pull request", for all of them. `_assent_tokens_readable` does not
# catch it and must not: `*` is a legal filter character, so the pattern
# tokenises perfectly and nothing is dropped or mangled. The defect was an
# EXACT-TOKEN predicate applied to a PATTERN list — and the header had already
# reasoned it out one key over, where the same un-evaluability is fail-CLOSED.
bi_fixture() { # <name> <yaml value or block> — branches-ignore in flow form
  local f="$WORK/fixture.$1.yaml"
  printf 'name: f\non:\n  pull_request:\n    branches-ignore: %s\njobs: {}\n' "$2" >"$f"
  printf '%s' "$f"
}
bi_seq_fixture() { # <name> <item> — branches-ignore as a block sequence
  local f="$WORK/fixture.$1.yaml"
  printf 'name: f\non:\n  pull_request:\n    branches-ignore:\n      - %s\njobs: {}\n' "$2" >"$f"
  printf '%s' "$f"
}

V="$(bi_fixture bi-glob-star2 "['**']")"
helper_reach_is 10 "$V" "branches-ignore: ['**'] is REFUSED — it excludes every branch, main included, and an exact-token search finds no literal 'main' (G5-01: rc=0 at 01b2322, on a workflow that runs on NO pull request)"

V="$(bi_fixture bi-glob-prefix "[main*]")"
helper_reach_is 10 "$V" "branches-ignore: [main*] is REFUSED — a pattern that covers main without being the token 'main' (G5-01)"

V="$(bi_fixture bi-glob-suffix "['*ain']")"
helper_reach_is 10 "$V" "branches-ignore: ['*ain'] is REFUSED — glob semantics are not evaluable by a text gate, and refusing is the direction this reader already claims for branches: (G5-01)"

V="$(bi_seq_fixture bi-glob-seq "'**'")"
helper_reach_is 10 "$V" "…and in BLOCK SEQUENCE form too, not only flow form (G5-01)"

V="$(bi_fixture bi-literal-bad "[main]")"
helper_reach_is 13 "$V" "a LITERAL branches-ignore: [main] still reds as excluding main — the refusal is scoped to patterns, it did not swallow the real finding (G5-01, other polarity)"

V="$(bi_fixture bi-literal-legit "[gh-pages]")"
helper_reach_is 0 "$V" "a LITERAL branches-ignore: [gh-pages] is still accepted — not made refusal-happy (G5-01, other polarity)"

# G5-02 — a disarm key that is not spelled bare. `if : `, `"if":` and `'if' :`
# are resolved identically to `if:` by a conformant parser (checked against
# ruby/psych), so actionlint has nothing to flag and the step or job carries
# release-exitgate's exact push-only guard while grading "wired, argument-free
# and undisarmed". Graded now by NORMALISED KEY NAME, mirroring the filter-key
# allowlist in assent_pr_reach that is measurably immune to the same evasions —
# normalisation is the immunity, not a wider pattern.
step_key_fixture() { # <name> <extra key line, 8-space indent assumed>
  local f="$WORK/fixture.$1.yaml"
  {
    printf 'name: f\non:\n  pull_request:\njobs:\n  verify:\n    runs-on: ubuntu-latest\n    steps:\n'
    printf '      - name: gate\n'
    printf '        %s\n' "$2"
    printf '        run: bash %s\n' "$3"
  } >"$f"
  printf '%s' "$f"
}
job_key_fixture() { # <name> <extra job-level key line>
  local f="$WORK/fixture.$1.yaml"
  {
    printf 'name: f\non:\n  pull_request:\njobs:\n  verify:\n'
    printf '    %s\n' "$2"
    printf '    runs-on: ubuntu-latest\n    steps:\n'
    printf '      - name: gate\n        run: bash %s\n' "$3"
  } >"$f"
  printf '%s' "$f"
}

V="$(step_key_fixture step-if-bare 'if: false' hack/lint/workflow_pins_test.sh)"
helper_wired_is 8 "$V" "$SELF_GATE" "a step disarmed with a BARE 'if:' is DISARMED (the control that always worked)"

V="$(step_key_fixture step-if-spaced 'if : false' hack/lint/workflow_pins_test.sh)"
helper_wired_is 8 "$V" "$SELF_GATE" "a step disarmed with 'if : ' — a space before the colon — is DISARMED (G5-02: rc=0 at 01b2322; a conformant parser reads it as 'if')"

V="$(step_key_fixture step-if-quoted '"if": false' hack/lint/workflow_pins_test.sh)"
helper_wired_is 8 "$V" "$SELF_GATE" "a step disarmed with a QUOTED '\"if\":' is DISARMED (G5-02)"

V="$(step_key_fixture step-coe-spaced 'continue-on-error : true' hack/lint/workflow_pins_test.sh)"
helper_wired_is 8 "$V" "$SELF_GATE" "a step neutered with 'continue-on-error : ' is DISARMED (G5-02)"

V="$(job_key_fixture job-if-spaced 'if : false' hack/lint/workflow_pins_test.sh)"
helper_wired_is 4 "$V" "$SELF_GATE" "a JOB carrying 'if : ' has release-exitgate's push-only guard — rc=4, not 0 (G5-02: this is the RELSE-08 shape the whole lane exists to catch)"

V="$(job_key_fixture job-if-quoted '"if": false' hack/lint/workflow_pins_test.sh)"
helper_wired_is 4 "$V" "$SELF_GATE" "a JOB carrying a quoted '\"if\":' is likewise rc=4 (G5-02)"

V="$(job_key_fixture job-needs-quoted '"needs": release-exitgate' hack/lint/workflow_pins_test.sh)"
helper_wired_is 9 "$V" "$SELF_GATE" "a JOB carrying a quoted '\"needs\":' is rc=9 — skipped dependency, skipped job (G5-02)"

# Both polarities: normalisation must not turn ordinary step/job keys into a red.
V="$WORK/fixture.step-ordinary-keys.yaml"
{
  printf 'name: f\non:\n  pull_request:\njobs:\n  verify:\n    runs-on: ubuntu-latest\n    timeout-minutes: 60\n    steps:\n'
  printf '      - name: gate\n        id: g\n        shell: bash\n        env:\n          FOO: bar\n'
  printf '        run: bash hack/lint/workflow_pins_test.sh\n'
} >"$V"
helper_wired_is 0 "$V" "$SELF_GATE" "a step carrying ordinary keys (id:, shell:, env:) in a job with timeout-minutes: is still WIRED — the key allowlist grades disarms, it does not refuse normal structure (G5-02, other polarity)"

f="$(helper_fixture wired-comment <<'FIX'
name: f
on:
  pull_request:
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      # bash hack/lint/workflow_pins_test.sh
      - name: gate
        # run: bash hack/lint/workflow_pins_test.sh
        run: echo hi
FIX
)"
helper_wired_is 5 "$f" "$SELF_GATE" "a step that only MENTIONS the script in comments is reported absent"

echo "== 5b. verify.yaml reaches EVERY pull request, unfiltered =="

# Insert a filter directly under the `pull_request:` trigger key.
pr_filter_mutant() { # <name> <yaml line…>
  local name="$1"
  shift
  local d payload
  d="$(mutant "$name")"
  payload="$(printf '%s\n' "$@")"
  mutate_awk "$d/verify.yaml" \
    "{ print } \$0 == \"  pull_request:\" { print \"$payload\" }" \
    "$payload"
  local kept="$WORK/hits.pr_still_present.$name"
  grep -nE '^[[:space:]]+pull_request:' "$d/verify.yaml" >"$kept" || true
  [[ -s "$kept" ]] ||
    fail "mutation control '$name' is not the intended one: the mutant no longer satisfies even the OLD '^[[:space:]]+pull_request:' grep, so it does not demonstrate the bypass"
  printf '%s' "$d"
}

m="$(pr_filter_mutant pr-paths "    paths: [internal/**]")"
expect_red check_ci_wiring "$m" "the pull_request trigger grew a paths: filter — SATISFIES the old grep, disarms the gate for a PR touching only .github/workflows/**" \
  "carries a 'paths:' or 'paths-ignore:' filter"

m="$(pr_filter_mutant pr-paths-ignore "    paths-ignore: [docs/**]")"
expect_red check_ci_wiring "$m" "the pull_request trigger grew a paths-ignore: filter — SATISFIES the old grep" \
  "carries a 'paths:' or 'paths-ignore:' filter"

m="$(pr_filter_mutant pr-types-closed "    types: [closed]")"
expect_red check_ci_wiring "$m" "the pull_request trigger was narrowed to types: [closed] — SATISFIES the old grep, never fires on an open PR" \
  "omits one of GitHub's defaults"

m="$(pr_filter_mutant pr-types-partial "    types: [opened, reopened]")"
expect_red check_ci_wiring "$m" "the pull_request types: list dropped 'synchronize', so the gate would not re-run when a PR is updated" \
  "omits one of GitHub's defaults"

m="$(pr_filter_mutant pr-branches "    branches: [release]")"
expect_red check_ci_wiring "$m" "the pull_request trigger was restricted to base branches that exclude main" \
  "branch filter that excludes 'main'"

m="$(mutant pr-flow-mapping)"
mutate "$m/verify.yaml" \
  's|^  pull_request:$|  pull_request: {paths: [internal/**]}|' \
  'pull_request: {paths:'
expect_red check_ci_wiring "$m" "the paths: filter was hidden inside a YAML flow mapping — refused, not guessed at" \
  'refuses to evaluate'

m="$(mutant pr-deleted)"
mutate "$m/verify.yaml" \
  '/^  pull_request:$/d' \
  'push:'
expect_red check_ci_wiring "$m" "verify.yaml stopped triggering on pull_request at all (RELSE-08 by another route)" \
  "no top-level 'pull_request:' trigger"

# The indent-scoping half: the ONLY `pull_request:` left is an input NAME nested
# under another trigger. `grep -qE '^[[:space:]]+pull_request:'` says yes.
m="$(mutant pr-nested-elsewhere)"
mutate_awk "$m/verify.yaml" \
  '{ if ($0 == "  pull_request:") { print "  workflow_call:"; print "    inputs:"; print "      pull_request:"; print "        type: string" } else print }' \
  '      pull_request:'
nested_hits="$WORK/hits.pr_nested"
grep -nE '^[[:space:]]+pull_request:' "$m/verify.yaml" >"$nested_hits" || true
[[ -s "$nested_hits" ]] ||
  fail "mutation control 'pr-nested-elsewhere' is not the intended one: no 'pull_request:' line survives, so it does not test indent scoping"
expect_red check_ci_wiring "$m" "the only 'pull_request:' left is a workflow_call INPUT NAME — SATISFIES the old indent-blind grep, and GitHub never runs the workflow on a PR" \
  "no top-level 'pull_request:' trigger"

# The other polarity, so this is not a gate that reds on any filter at all: the
# legitimate shapes must still PASS. Without these, the six controls above are
# equally satisfied by a check that always fails.
echo "== 5b2. legitimate trigger shapes still PASS (the anti-overreach control) =="
m="$(pr_filter_mutant pr-types-superset "    types: [opened, synchronize, reopened, ready_for_review]")"
expect_green check_ci_wiring "$m" "a types: list that SUPERSETS GitHub's defaults is accepted"
m="$(pr_filter_mutant pr-branches-main "    branches: [main]")"
expect_green check_ci_wiring "$m" "branches: [main] — the branch this gate protects — is accepted"
m="$(pr_filter_mutant pr-branches-ignore-other "    branches-ignore: [gh-pages]")"
expect_green check_ci_wiring "$m" "branches-ignore: on a branch that is not main is accepted"
m="$(mutant pr-inline-seq)"
mutate_awk "$m/verify.yaml" \
  '{ if ($0 == "on:") { print "on: [push, pull_request]"; skip = 1; next } if (skip && $0 ~ /^  /) next; skip = 0; print }' \
  'on: [push, pull_request]'
expect_green check_ci_wiring "$m" "the inline 'on: [push, pull_request]' form — which cannot carry a filter at all — is accepted"
m="$(mutant pr-quoted-on)"
mutate "$m/verify.yaml" \
  's|^on:$|"on":|' \
  '"on":'
expect_green check_ci_wiring "$m" "the quoted '\"on\":' key (YAML 1.1 truthiness) is accepted"
# A fail-open found by building the mutant, not by reasoning: the block-scalar
# form below is accepted on purpose, and accepting it naively makes a BARE
# `bash <script>` line anywhere in the job satisfy the check — including inside
# another step's multi-command `run: |` body, where `echo skip && exit 0` a line
# above would mean this gate never runs. Measured at rc=0 before the
# restriction; the block form now counts only as a block's SOLE command.
# The injected step is written here rather than appended to an existing one, so
# this control depends on NO other step's contents — sibling lanes edit
# verify.yaml's toolchain steps, and a mutation harness anchored on their text
# would red for their reason instead of its own.
m="$(mutant wiring-block-bleed)"
mutate_awk "$m/verify.yaml" \
  '{ if ($0 == "        run: bash hack/lint/workflow_pins_test.sh") next; print; if ($0 == "    steps:" && !done) { print "      - name: unrelated"; print "        run: |"; print "          echo skip \&\& exit 0"; print "          bash hack/lint/workflow_pins_test.sh"; done = 1 } }' \
  '          bash hack/lint/workflow_pins_test.sh'
bleed_real="$WORK/hits.bleed_real"
grep -nF -- '        run: bash hack/lint/workflow_pins_test.sh' "$m/verify.yaml" >"$bleed_real" || true
[[ -s "$bleed_real" ]] && fail "mutation control 'wiring-block-bleed' is not the intended one: the real step's run: line survived"
expect_red check_ci_wiring "$m" "the gate's own step was deleted and its invocation smuggled into ANOTHER step's multi-command 'run: |' body" \
  'the workflow-pin gate would only ever run locally'

m="$(mutant wiring-block-scalar)"
mutate_awk "$m/verify.yaml" \
  '{ if ($0 == "        run: bash hack/lint/workflow_pins_test.sh") { print "        run: |"; print "          bash hack/lint/workflow_pins_test.sh" } else print }' \
  '          bash hack/lint/workflow_pins_test.sh'
expect_green check_ci_wiring "$m" "a block-scalar 'run: |' invocation of this gate is accepted"

# ---------------- 5c. D-157 residual 2: the other two PR-visible gates -------

echo "== 5c. the other two PR-visible text gates are wired in the same job =="
expect_green check_pr_gate_cross_pins "$WORKFLOWS" \
  "verify.yaml also runs hack/audit/aud2_exitgate_test.sh and hack/examples/dogfood_wiring_test.sh, undisarmed"

# Six rows across the three gates, two of them here: deleting a sibling's step
# must red THIS gate, because on that PR this gate is the one still running.
while IFS= read -r sibling_gate; do
  [[ -n "$sibling_gate" ]] || continue
  m="$(mutant "cross-pin-$(basename "$sibling_gate" .sh)")"
  mutate "$m/verify.yaml" \
    "\\|^        run: bash ${sibling_gate}\$|d" \
    "run: bash $SELF_GATE"
  expect_red check_pr_gate_cross_pins "$m" "the step running the sibling gate $sibling_gate was deleted from the verify job" \
    "the sibling PR-visible gate $sibling_gate is NOT wired"
done < <(assent_pr_gate_others "$SELF_GATE")

# ------------------------------ 6. N6: quoted-scalar precondition enforced --

echo "== 6. N6: no ' #' inside a quoted scalar (command_view's precondition) =="
expect_green check_no_quoted_hash "$WORKFLOWS" "no workflow line hides code behind a quoted ' #'"

m="$(mutant quoted-hash)"
mutate_awk "$m/schemas.yml" \
  '{ if ($0 ~ /^        run: npm ci --ignore-scripts --prefix hack\/schemas-validator$/) { print "        run: echo \"stage # 2\" \\&\\& npm --global install ajv-cli" } else print }' \
  'stage # 2'
expect_red check_no_quoted_hash "$m" "a quoted ' #' hides a live global install from command_view (N6)" \
  "inside a quoted scalar"

# ------------------------------- 7. D-177: SonarCloud analysis stays wired --

echo "== 7. D-177: the SonarCloud scanner step is present, pinned, armed and ordered =="
expect_green check_sonar_scan_wired "$WORKFLOWS" \
  "verify.yaml runs a SHA-pinned, undisarmed SonarSource scan after the coverage gate"

m="$(mutant sonar-step-deleted)"
mutate "$m/verify.yaml" \
  "\\|^        uses: SonarSource/sonarqube-scan-action@|d" \
  "run: task coverage"
expect_red check_sonar_scan_wired "$m" "the SonarCloud scan step was deleted — the state the project spent a month in" \
  "runs no SonarSource/sonarqube-scan-action step"

m="$(mutant sonar-step-unpinned)"
mutate "$m/verify.yaml" \
  "s|uses: SonarSource/sonarqube-scan-action@[0-9a-f]*|uses: SonarSource/sonarqube-scan-action@v8|" \
  "sonarqube-scan-action@v8"
expect_red check_sonar_scan_wired "$m" "the scan action was repointed at a mutable tag instead of a commit SHA" \
  "not pinned to a 40-hex commit SHA"

m="$(mutant sonar-step-continue-on-error)"
mutate_awk "$m/verify.yaml" \
  '{ print; if ($0 ~ /^        uses: SonarSource\/sonarqube-scan-action@/) print "        continue-on-error: true" }' \
  "continue-on-error: true"
expect_red check_sonar_scan_wired "$m" "the scan step was disarmed with continue-on-error — present, green, gating nothing" \
  "carries continue-on-error"

m="$(mutant sonar-step-if-false)"
mutate "$m/verify.yaml" \
  "s|^        if: github.event_name != .pull_request. .*|        if: \${{ false }}|" \
  "if: \${{ false }}"
expect_red check_sonar_scan_wired "$m" "the scan step was disarmed with a constant if: — a skipped job reports SUCCESS" \
  "normalises to the constant 'false'"

# G5-02 REGRESSION CONTROLS. Every one of these five went GREEN against the
# literal readers this check first shipped with, while disarming the step for a
# conformant YAML parser. They are pinned so the fix cannot silently rot back.
m="$(mutant sonar-step-if-spaced-key)"
mutate "$m/verify.yaml" \
  "s|^        if: github.event_name.*|        if : \${{ false }}|" \
  "if : \${{ false }}"
expect_red check_sonar_scan_wired "$m" "the if: key is spelled 'if : ' — identical to a parser, invisible to a literal pattern (G5-02)" \
  "normalises to the constant 'false'"

m="$(mutant sonar-step-if-quoted-key)"
mutate "$m/verify.yaml" \
  "s|^        if: github.event_name.*|        \"if\": false|" \
  '"if": false'
expect_red check_sonar_scan_wired "$m" "the if: key is quoted as \"if\": — same disarm, different spelling (G5-02)" \
  "normalises to the constant 'false'"

m="$(mutant sonar-step-if-carriage-return)"
mutate_awk "$m/verify.yaml" \
  '{ if ($0 ~ /^        if: github.event_name/) { printf "        if: ${{ false }}\r\n" } else print }' \
  'if: ${{ false }}'
expect_red check_sonar_scan_wired "$m" "a constant if: carrying a trailing CR — the G4-01 shape, which a byte-literal comparison misses" \
  "normalises to the constant 'false'"

m="$(mutant sonar-step-coe-spaced-key)"
mutate_awk "$m/verify.yaml" \
  '{ print; if ($0 ~ /^        uses: SonarSource\/sonarqube-scan-action@/) print "        continue-on-error : true" }' \
  "continue-on-error : true"
expect_red check_sonar_scan_wired "$m" "continue-on-error spelled with a space before the colon (G5-02)" \
  "carries continue-on-error"

m="$(mutant sonar-step-coe-quoted-key)"
mutate_awk "$m/verify.yaml" \
  '{ print; if ($0 ~ /^        uses: SonarSource\/sonarqube-scan-action@/) print "        \"continue-on-error\": true" }' \
  '"continue-on-error": true'
expect_red check_sonar_scan_wired "$m" "continue-on-error with a quoted key (G5-02)" \
  "carries continue-on-error"

m="$(mutant sonar-step-unreadable-key)"
mutate_awk "$m/verify.yaml" \
  '{ print; if ($0 ~ /^        uses: SonarSource\/sonarqube-scan-action@/) print "        <<: *disarm" }' \
  "<<: *disarm"
expect_red check_sonar_scan_wired "$m" "the step carries a merge-key/alias the reader cannot normalise — refused, not guessed" \
  "cannot be normalised to a plain identifier"

m="$(mutant sonar-step-before-coverage)"
mutate_awk "$m/verify.yaml" \
  '{ if ($0 ~ /^        run: task coverage$/) { covline = $0; next } if ($0 ~ /^          SONAR_HOST_URL:/) { print; print "      - name: coverage gate (moved)"; print covline; next } print }' \
  "coverage gate (moved)"
expect_red check_sonar_scan_wired "$m" "the coverage gate was moved AFTER the scan step, inverting the ordering the profile depends on" \
  "runs BEFORE the coverage gate"

echo "== 7b. D-177: sonar-project.properties names the project SonarCloud holds =="
expect_green check_sonar_properties "$ROOT" \
  "sonar-project.properties pins the live project key, org, coverage path and the D-150 S3776 exemption"

sm="$(sonar_props_mutant projectkey-recased)"
mutate "$sm/sonar-project.properties" \
  "s|^sonar.projectKey=PlatformRelay_assent$|sonar.projectKey=PlatformRelay_Assent|" \
  "sonar.projectKey=PlatformRelay_Assent"
expect_red check_sonar_properties "$sm" "the project key was 'corrected' to match the renamed GitHub repo — which creates a SECOND project" \
  "orphans every historical measurement"

sm="$(sonar_props_mutant coverage-path-drift)"
mutate "$sm/sonar-project.properties" \
  "s|^sonar.go.coverage.reportPaths=cov.out$|sonar.go.coverage.reportPaths=coverage.out|" \
  "reportPaths=coverage.out"
expect_red check_sonar_properties "$sm" "the coverage report path drifted away from the profile task coverage writes" \
  "that is the profile 'task coverage' writes"

sm="$(sonar_props_mutant s3776-exemption-dropped)"
mutate "$sm/sonar-project.properties" \
  "s|^sonar.issue.ignore.multicriteria.e1.ruleKey=go:S3776$|sonar.issue.ignore.multicriteria.e1.ruleKey=go:S1234|" \
  "ruleKey=go:S1234"
expect_red check_sonar_properties "$sm" "the D-150 S3776 test-file exemption was altered — the only reason this file exists" \
  "the ONLY reason this file exists"

echo
echo "workflow_pins_test.sh: PASS"
