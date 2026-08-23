#!/usr/bin/env bash
# REQ-EX-S08-02/03 — dogfood wiring pin, adversarially proven (both polarities).
#
#   REQ-EX-S08-02: Taskfile.yml's dogfood-examples task AND verify.yaml's
#   dogfood step call the SHARED hack/dogfood-examples.sh discovery script —
#   neither may re-hardcode its own `for pack in <names>` loop.
#   REQ-EX-S08-03: `task check` runs dogfood-examples (after build); deleting
#   that line from check: must redden this pin.
#
# Section 5 extends the same discipline to a SECOND way a dogfood gate can be
# invoked-by-nothing: `task dogfood-comparison` runs `go test` over an EXTERNAL
# test package whose real subject is a binary the test builds at runtime, so the
# Go test cache key is blind to production changes and the stage can print
# `ok … (cached)` and exit 0 on a tree where the test genuinely fails (measured
# 2026-08-19). `-count=1` is what makes that stage a gate at all — so it is
# pinned here, with a mutation control that strips the flag from the COMMAND
# LINE while leaving the explanatory comment in place, which is exactly how a
# human would regress it. Section 6 pins the SAME flag on verify.yaml's copy of
# that invocation: actions/setup-go restores GOCACHE across commits, so the CI
# half is blind in exactly the same way, and it is the half where a vacuous PASS
# turns a red tree into a green PR.
#
# Follows the hack/release/changelog_gate_test.sh / example_format_inventory_test.sh
# discipline: every "is it wired" assertion is re-run against a mutated copy
# with the wiring deleted, so the assertion is proven capable of failing
# before its green result on the real tree is believed.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

TASKFILE="$ROOT/Taskfile.yml"
WORKFLOW="$ROOT/.github/workflows/verify.yaml"
SCRIPT="hack/dogfood-examples.sh"

# D-157 — the PR-reach and step-wiring checks are SHARED with the other two
# PR-visible text gates (hack/lint/workflow_pins_test.sh,
# hack/audit/aud2_exitgate_test.sh). Sourced, not executed; it returns codes and
# prints nothing, so the wording below stays this script's own. Guarded rather
# than assumed: under `set -e` a missing helper would abort this script
# mid-section, and a gate that stops half-way must say so instead of exiting on
# whatever code the failed source left behind.
PR_REACH_LIB="$ROOT/hack/lib/pr_reach.sh"
if [[ ! -f "$PR_REACH_LIB" ]]; then
  echo "FAIL: missing $PR_REACH_LIB — the shared PR-reach/step-wiring helper (D-157). Refusing to run rather than skipping section 7." >&2
  exit 1
fi
# shellcheck source=../lib/pr_reach.sh
. "$PR_REACH_LIB"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# extract_block <file> <indent-2 key> — body of a top-level 2-space-indented
# mapping key (a Taskfile task), up to the next such key.
extract_block() {
  awk -v name="$2" '
    $0 == "  " name ":" { inblk = 1; next }
    inblk && /^  [A-Za-z0-9_.:-]+:[[:space:]]*$/ { inblk = 0 }
    inblk { print }
  ' "$1"
}

check_lists_task() {
  extract_block "$1" check >"$WORK/check.block"
  grep -qE "^[[:space:]]+- task: $2\$" "$WORK/check.block"
}

# hardcoded_pack_loop <file> — true if the file still contains the OLD
# `for pack in <name1> <name2> ...` shell-loop shape (2+ space-separated
# tokens after "for pack in"), the pattern this script replaces.
hardcoded_pack_loop() {
  grep -qE 'for pack in [A-Za-z0-9_-]+ [A-Za-z0-9_-]+' "$1"
}

echo "== 0. extraction positive control =="
extract_block "$TASKFILE" check >"$WORK/check.control"
[[ -s "$WORK/check.control" ]] || fail "Taskfile check: block extracted EMPTY — the awk range is broken, every assertion below would be vacuous"
grep -qE '^[[:space:]]+- task: build$' "$WORK/check.control" \
  || fail "Taskfile check: block does not contain the known-present '- task: build' — extraction is wrong"
echo "OK: check: block extracted ($(wc -l <"$WORK/check.control" | tr -d ' ') lines), anchored on build"

# --------------------------------------------- 1. REQ-EX-S08-03 (wiring) --

echo "== 1. REQ-EX-S08-03: task check runs dogfood-examples, after build =="
check_lists_task "$TASKFILE" dogfood-examples \
  || fail "'task check' does not run 'dogfood-examples' — the gate is defined but invoked by nothing (REQ-EX-S08-03)"
extract_block "$TASKFILE" dogfood-examples >"$WORK/def.dogfood-examples"
[[ -s "$WORK/def.dogfood-examples" ]] || fail "'dogfood-examples' is listed in check: but not defined in Taskfile.yml"

build_line="$(grep -nE '^[[:space:]]+- task: build$' "$WORK/check.control" | head -1 | cut -d: -f1)"
dogfood_line="$(grep -nE '^[[:space:]]+- task: dogfood-examples$' "$WORK/check.control" | head -1 | cut -d: -f1)"
[[ -n "$build_line" && -n "$dogfood_line" ]] || fail "could not locate build/dogfood-examples lines within check: block"
[[ "$dogfood_line" -gt "$build_line" ]] \
  || fail "dogfood-examples (line $dogfood_line) is not positioned AFTER build (line $build_line) in check: — D-124 requires a sequential stage, not a deps: race with fmt"
echo "OK: check runs dogfood-examples after build (line $dogfood_line > $build_line)"

echo "== 1b. the wiring assertion itself can fail (mutation) =="
mutant="$WORK/Taskfile.no-dogfood.yml"
grep -vE '^[[:space:]]+- task: dogfood-examples$' "$TASKFILE" >"$mutant"
if grep -qE '^[[:space:]]+- task: dogfood-examples$' "$mutant"; then
  fail "mutation did not land: '- task: dogfood-examples' is still in $mutant"
fi
if [[ "$(wc -l <"$mutant")" -eq "$(wc -l <"$TASKFILE")" ]]; then
  fail "mutation did not land: $mutant has the same line count as Taskfile.yml"
fi
if check_lists_task "$mutant" dogfood-examples; then
  fail "check_lists_task reports dogfood-examples wired in a Taskfile with that line deleted — the assertion is vacuous"
fi
echo "OK: deleting '- task: dogfood-examples' from check: turns the assertion red"

# --------------------------------------------- 2. REQ-EX-S08-02 (shared script) --

echo "== 2. REQ-EX-S08-02: Taskfile dogfood-examples calls the shared script =="
grep -qF "$SCRIPT" "$WORK/def.dogfood-examples" \
  || fail "Taskfile.yml dogfood-examples task does not invoke $SCRIPT"
if hardcoded_pack_loop "$WORK/def.dogfood-examples"; then
  fail "Taskfile.yml dogfood-examples re-hardcodes a pack-name loop instead of calling $SCRIPT"
fi
echo "OK: Taskfile dogfood-examples calls $SCRIPT, no hardcoded loop"

echo "== 2b. the shared-script assertion can fail (mutation, positive control) =="
# Prove hardcoded_pack_loop actually detects the OLD shape it replaces.
printf 'cmds:\n  - for pack in service-catalog infra-vars topic-registry; do true; done\n' >"$WORK/old-shape.yml"
if ! hardcoded_pack_loop "$WORK/old-shape.yml"; then
  fail "hardcoded_pack_loop failed to detect the historical three-name loop shape — the detector is broken"
fi
echo "OK: hardcoded_pack_loop detects the historical loop shape (positive control)"

echo "== 3. REQ-EX-S08-02: verify.yaml dogfood step calls the shared script =="
grep -qF "$SCRIPT" "$WORKFLOW" \
  || fail "verify.yaml does not invoke $SCRIPT"
if hardcoded_pack_loop "$WORKFLOW"; then
  fail "verify.yaml re-hardcodes a pack-name loop instead of calling $SCRIPT"
fi
echo "OK: verify.yaml calls $SCRIPT, no hardcoded loop"

echo "== 3b. the workflow assertion itself can fail (mutation) =="
mutant_wf="$WORK/verify.no-script.yaml"
grep -vF "$SCRIPT" "$WORKFLOW" >"$mutant_wf"
if grep -qF "$SCRIPT" "$mutant_wf"; then
  fail "mutation did not land: $mutant_wf still references $SCRIPT"
fi
[[ "$(wc -l <"$mutant_wf")" -lt "$(wc -l <"$WORKFLOW")" ]] || fail "mutation did not land: $mutant_wf has the same line count as verify.yaml"
if grep -qF "$SCRIPT" "$mutant_wf"; then
  fail "the shared-script assertion stayed green after deleting the reference — vacuous"
fi
echo "OK: deleting the $SCRIPT reference from verify.yaml turns the assertion red"

# ------------------------------------------------------------- 4. the script exists --

echo "== 4. hack/dogfood-examples.sh exists and is executable =="
[[ -f "$ROOT/$SCRIPT" ]] || fail "$SCRIPT does not exist"
[[ -x "$ROOT/$SCRIPT" ]] || fail "$SCRIPT is not executable"
echo "OK: $SCRIPT present and executable"

# --------------------------------- 5. -count=1 on cache-blind `go test` gates --
#
# COUNT1_TASKS: Taskfile tasks whose `go test` subject is NOT the test binary the
# cache keys on — it is a binary built (or prebuilt) at runtime and driven as a
# subprocess. For those the cache key is blind to the code under test, so a
# cached PASS is vacuous and `-count=1` is load-bearing:
#
#   dogfood-comparison  examples/comparison/validate_test.go is an EXTERNAL test
#                       package referencing only exported constants from
#                       internal/compare; the compare logic is unreachable from
#                       the test binary (linker strips it) and runs instead in a
#                       `go build -o … ./cmd/assent` subprocess.
#   e2e                 test/e2e drives a PREBUILT bin/assent against a live
#                       GitLab endpoint; neither the binary bytes nor the forge
#                       state is in the cache key.
#
# Deliberately NOT listed, with the true reason rather than a tidy one:
#
#   test      whole-tree `go test -race ./...`. Its subjects are OVERWHELMINGLY
#             in-process, the cache is honest for them, and it is a real time
#             saver — but not uniformly: internal/schemadrift compares local
#             schemas/ (read in-process, so testlog records it) against
#             `git show origin/main:…` run as a SUBPROCESS (invisible), and
#             internal/provider/isolation_test.go `go build`s a fixture at
#             runtime. Tracked as backlog row COUNT1-F01
#             (openspec/specs/backlog.md, "Found in flight" block), NOT fixed
#             here: isolating them means carving packages out of
#             `test`/`coverage` and losing an otherwise-honest whole-tree
#             cache. examples/comparison is also cached here under a separate
#             (-race) entry; that is covered because dogfood-comparison re-runs
#             the same package with -count=1 in the same `task check`.
#   coverage  NOT "in-process subjects" — `./internal/...` contains both
#             packages the `test` bullet just named (COUNT1-F01 applies here
#             verbatim; the same two blind spots, the same reason for leaving
#             them). The verdict rests on something else entirely: the gated
#             number is honest because a cached `-coverprofile` run REGENERATES
#             the profile faithfully (measured: the cached run reprints the
#             identical total), so what this gate reads as evidence is never a
#             stale number even when a cached package result is stale.
#
# `determinism` needs no pin: `-count=2` is uncacheable by construction.
COUNT1_TASKS=(dogfood-comparison e2e)

# count1_pinned <file> <task> — the task's `go test` COMMAND LINE carries
# -count=1. Anchored on the `- go test` command shape and stopping at `#`, so a
# comment that merely mentions -count=1 can never satisfy it.
count1_pinned() {
  extract_block "$1" "$2" \
    | grep -qE '^[[:space:]]*-[[:space:]]+go test[^#]*[[:space:]]-count=1([[:space:]]|$)'
}

echo "== 5. -count=1 pins on cache-blind go test gates =="
for t in "${COUNT1_TASKS[@]}"; do
  extract_block "$TASKFILE" "$t" >"$WORK/def.$t"
  [[ -s "$WORK/def.$t" ]] || fail "Taskfile task '$t' is missing or extracted EMPTY — the -count=1 assertion below would be vacuous"
  grep -qE '^[[:space:]]*-[[:space:]]+go test' "$WORK/def.$t" \
    || fail "Taskfile task '$t' no longer runs a 'go test' command — re-derive whether it is still cache-blind before deleting this pin"
  count1_pinned "$TASKFILE" "$t" \
    || fail "Taskfile task '$t' runs 'go test' WITHOUT -count=1 — its subject is a runtime-built/prebuilt binary the Go test cache key cannot see, so the stage can report 'ok … (cached)' and exit 0 on a tree where the test genuinely fails"
  echo "OK: $t runs go test with -count=1"
done

echo "== 5b. the -count=1 assertion is comment-blind (positive control) =="
cat >"$WORK/comment-only.yml" <<'EOF'
  dogfood-comparison:
    cmds:
      # -count=1 is required here, honest
      - go test ./examples/comparison/...

  next-task:
EOF
if count1_pinned "$WORK/comment-only.yml" dogfood-comparison; then
  fail "count1_pinned accepted a task whose -count=1 appears ONLY in a comment — the assertion is satisfiable by prose and therefore vacuous"
fi
echo "OK: a -count=1 that lives only in a comment does NOT satisfy the pin"

echo "== 5c. the -count=1 assertion can fail (mutation: strip the flag, keep the comment) =="
mutant_c1="$WORK/Taskfile.nocount1.yml"
sed -E 's/^([[:space:]]*-[[:space:]]+go test)[[:space:]]+-count=1/\1/' "$TASKFILE" >"$mutant_c1"
if cmp -s "$TASKFILE" "$mutant_c1"; then
  fail "mutation did not land: $mutant_c1 is byte-identical to Taskfile.yml"
fi
grep -qE '^[[:space:]]*#.*-count=1' "$mutant_c1" \
  || fail "mutation is not the intended one: it removed the explanatory -count=1 COMMENTS too, so 5c would not prove comment-blindness under mutation"
for t in "${COUNT1_TASKS[@]}"; do
  if grep -qE '^[[:space:]]*-[[:space:]]+go test[^#]*[[:space:]]-count=1' <(extract_block "$mutant_c1" "$t"); then
    fail "mutation did not land for '$t': the command line still carries -count=1 in $mutant_c1"
  fi
  if count1_pinned "$mutant_c1" "$t"; then
    fail "count1_pinned reports '$t' pinned in a Taskfile with the flag stripped from its command line — the assertion is vacuous"
  fi
done
echo "OK: stripping -count=1 from the command lines (comments intact) turns the pin red for: ${COUNT1_TASKS[*]}"

echo "== 5d. task check still runs dogfood-comparison (an unwired gate cannot be saved by -count=1) =="
check_lists_task "$TASKFILE" dogfood-comparison \
  || fail "'task check' does not run 'dogfood-comparison' — the gate is defined but invoked by nothing"
mutant_dc="$WORK/Taskfile.no-comparison.yml"
grep -vE '^[[:space:]]+- task: dogfood-comparison$' "$TASKFILE" >"$mutant_dc"
[[ "$(wc -l <"$mutant_dc")" -lt "$(wc -l <"$TASKFILE")" ]] || fail "mutation did not land: $mutant_dc has the same line count as Taskfile.yml"
if check_lists_task "$mutant_dc" dogfood-comparison; then
  fail "check_lists_task reports dogfood-comparison wired in a Taskfile with that line deleted — the assertion is vacuous"
fi
echo "OK: check runs dogfood-comparison, and deleting that line turns the assertion red"

# ------------------------- 6. the CI half of the -count=1 pin (verify.yaml) --
#
# Taskfile.yml is only the local half. verify.yaml runs its OWN
# `go test … ./examples/comparison/…`, and actions/setup-go restores the Go
# build cache (where test results live) across commits, so an uncounted CI
# invocation can serve `ok … (cached)` and turn a genuinely red tree into a
# green PR. Same discipline as section 5: match the COMMAND, never a comment.

# workflow_comparison_go_test_lines <file> — every line that RUNS `go test` over
# examples/comparison. Anchored on the command shape (optional `- `, optional
# `run: `) and stopping at `#`, so both `run:` steps and `run: |` block lines
# match while a YAML comment mentioning the command cannot.
workflow_comparison_go_test_lines() {
  grep -nE '^[[:space:]]*(-[[:space:]]+)?(run:[[:space:]]*)?go test[^#]*\./examples/comparison' "$1" || true
}

# workflow_count1_pinned <file> — 0: every such command carries -count=1.
# 1: at least one does not. 2: NO such command exists (vacuity, not success).
workflow_count1_pinned() {
  local line found=0
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    found=1
    # Anchored on the COMMAND and stopping at `#`, exactly like count1_pinned:
    # a whole-line comment cannot reach this function at all, and a TRAILING
    # `# -count=1` on an uncounted command must not satisfy it either (review
    # finding F2 — the whole-line grep this replaces accepted precisely that).
    grep -qE '(-[[:space:]]+)?(run:[[:space:]]*)?go test[^#]*[[:space:]]-count=1([[:space:]]|$)' \
      <<<"$line" || return 1
  done < <(workflow_comparison_go_test_lines "$1")
  ((found == 1)) || return 2
  return 0
}

echo "== 6. verify.yaml runs the comparison corpus with -count=1 =="
rc=0; workflow_count1_pinned "$WORKFLOW" || rc=$?
case "$rc" in
  0) echo "OK: every 'go test … ./examples/comparison…' in verify.yaml carries -count=1" ;;
  2) fail "verify.yaml contains NO 'go test … ./examples/comparison…' command — either the CI dogfood step was deleted or it was respelled past this matcher; re-derive the pin rather than leaving it silently vacuous" ;;
  *) fail "verify.yaml runs 'go test' over examples/comparison WITHOUT -count=1 — setup-go restores GOCACHE across commits, so that step can report 'ok … (cached)' and pass a PR on a tree where the corpus genuinely fails" ;;
esac

echo "== 6b. the workflow assertion is comment-blind (positive control) =="
cat >"$WORK/wf-comment-only.yaml" <<'EOF'
      # -count=1 is required here, honest
      - name: comparison corpus dogfood
        run: go test ./examples/comparison/...
EOF
rc=0; workflow_count1_pinned "$WORK/wf-comment-only.yaml" || rc=$?
((rc != 0)) || fail "workflow_count1_pinned accepted a workflow whose -count=1 appears ONLY in a comment — the assertion is satisfiable by prose"
((rc == 1)) || fail "the comment-only workflow control failed for the WRONG reason (rc=$rc, want 1 = flag missing from a present command)"
echo "OK: a -count=1 that lives only in a whole-line YAML comment does NOT satisfy the workflow pin"

echo "== 6b2. …and neither does a TRAILING comment on an uncounted command (F2) =="
cat >"$WORK/wf-trailing-comment.yaml" <<'EOF'
      - name: comparison corpus dogfood
        run: go test ./examples/comparison/... # -count=1
EOF
rc=0; workflow_count1_pinned "$WORK/wf-trailing-comment.yaml" || rc=$?
((rc != 0)) || fail "workflow_count1_pinned accepted 'go test ./examples/comparison/... # -count=1' — the flag is in a TRAILING COMMENT and the command in CI is genuinely uncounted (F2)"
((rc == 1)) || fail "the trailing-comment control failed for the WRONG reason (rc=$rc, want 1 = flag missing from a present command)"
# The counterpart: a real flag plus an unrelated trailing comment must still PASS,
# so the fix is comment-BLIND, not comment-HOSTILE.
cat >"$WORK/wf-flag-plus-comment.yaml" <<'EOF'
      - name: comparison corpus dogfood
        run: go test -count=1 ./examples/comparison/... # cache is blind here
EOF
rc=0; workflow_count1_pinned "$WORK/wf-flag-plus-comment.yaml" || rc=$?
((rc == 0)) || fail "workflow_count1_pinned REJECTED a genuinely counted command that carries an unrelated trailing comment (rc=$rc) — the matcher is comment-hostile, not comment-blind"
echo "OK: a trailing '# -count=1' does not satisfy the pin, while a real flag beside a trailing comment still does"

echo "== 6c. the workflow assertion can fail (mutation: strip the flag, keep the comment) =="
mutant_wf_c1="$WORK/verify.nocount1.yaml"
sed -E 's/^([[:space:]]*(-[[:space:]]+)?(run:[[:space:]]*)?go test)[[:space:]]+-count=1/\1/' "$WORKFLOW" >"$mutant_wf_c1"
if cmp -s "$WORKFLOW" "$mutant_wf_c1"; then
  fail "mutation did not land: $mutant_wf_c1 is byte-identical to verify.yaml"
fi
grep -qE '^[[:space:]]*#.*-count=1' "$mutant_wf_c1" \
  || fail "mutation is not the intended one: it stripped the explanatory -count=1 COMMENTS too, so 6c would not prove comment-blindness under mutation"
[[ -n "$(workflow_comparison_go_test_lines "$mutant_wf_c1")" ]] \
  || fail "mutation removed the comparison command entirely — the mutant would go red for vacuity (rc=2), not for the missing flag"
rc=0; workflow_count1_pinned "$mutant_wf_c1" || rc=$?
((rc != 0)) || fail "workflow_count1_pinned reports verify.yaml pinned with the flag stripped from its command line — the assertion is vacuous"
((rc == 1)) || fail "the verify.yaml mutant went red for the WRONG reason (rc=$rc, want 1 = flag missing from a present command)"
echo "OK: stripping -count=1 from verify.yaml's command line (comment intact) turns the pin red, for the flag and not for vacuity"

# --------------------- 7. this gate's OWN PR-visible step (RELSE-08) --
#
# Review finding F1: every assertion above is worthless on a pull request if the
# only path from this script to CI is `task check` -> release-exitgate, which
# carries `if: github.event_name != 'pull_request'`. A PR that strips -count=1
# from verify.yaml would then merge green and redden main afterwards — the
# lane's own thesis failing to apply to the lane. Mirrors the AUD2-S05 pin in
# hack/audit/aud2_exitgate_test.sh: the step must exist in the PR-visible
# `verify` job, be argument-free, and carry no `if:`/`continue-on-error:`; the
# job must carry no job-level `if:`; and the workflow must still trigger on
# pull_request at all.

SELF_REL="hack/examples/dogfood_wiring_test.sh"

# pr_step_pinned <workflow> — 0 when this gate runs, undisarmed, in a
# pull-request-visible job. Distinct codes so a mutation control can prove it
# went red for ITS reason. The whole body now delegates to hack/lib/pr_reach.sh
# (D-157): the on:/job/step readers that used to live here were copied into two
# sibling gates, one of which had already drifted weaker, and the PR-reach test
# was a bare `grep -qE '^[[:space:]]+pull_request:'` that a reviewer defeated
# with a `paths:`-filtered trigger (measured rc=0). The codes are unchanged for
# 2..8 — 9..13 are the ones the old check could not distinguish at all:
#
#   2  no top-level pull_request trigger      9  job-level needs:
#   3  job block unextractable               10  on: shape not evaluable
#   4  job-level if:                         11  pull_request has paths:
#   5  step absent (no run: COMMAND)         12  pull_request types: too narrow
#   6  step unisolatable                     13  pull_request branch filter
#   7  arguments                                 excludes main
#   8  disarmed
pr_step_pinned() {
  local wf="$1" rc=0
  assent_pr_reach "$wf" "$WORK" || rc=$?
  if ((rc != 0)); then
    return "$rc"
  fi
  assent_step_wired "$wf" "$ASSENT_PR_JOB" "$SELF_REL" "$WORK" || rc=$?
  return "$rc"
}

# --------------------------- 7a. the SHARED HELPER itself, against fixtures --
#
# Everything else in sections 7..7d mutates verify.yaml and asks the helper
# about it. None of that can tell a working helper from a stubbed one: replace
# `assent_pr_reach` with `return 0` and every assertion here — and in both
# sibling gates — goes green at once, because hack/lib/pr_reach.sh is now a
# single point of failure for all three. So the helper is also driven against
# workflows written FROM SCRATCH here, minimal and self-evidently
# reaching/not-reaching, where the expected answer does not depend on anything
# in .github/workflows/**. hack/lint/workflow_pins_test.sh carries the same
# fixtures, so deleting this section alone does not remove the property.
helper_fixture() { # <name> <heredoc on stdin> -> prints the path
  local f="$WORK/fixture.$1.yaml"
  cat >"$f"
  printf '%s' "$f"
}

helper_reach_is() { # <want-rc> <file> <label>
  local want="$1" f="$2" label="$3" got=0
  assent_pr_reach "$f" "$WORK" || got=$?
  ((got == want)) ||
    fail "helper self-check '$label': assent_pr_reach returned $got, want $want — hack/lib/pr_reach.sh does not do what every assertion in this script and in both sibling gates delegates to it"
  echo "OK: helper self-check — $label (rc=$want)"
}

echo "== 7a. hack/lib/pr_reach.sh answers correctly on from-scratch fixtures =="
m="$(helper_fixture reach-min <<'FIX'
name: f
on:
  pull_request:
jobs: {}
FIX
)"
helper_reach_is 0 "$m" "a bare 'on: pull_request:' reaches every PR"

m="$(helper_fixture reach-paths <<'FIX'
name: f
on:
  push:
    branches: [main]
  pull_request:
    paths: [internal/**]
jobs: {}
FIX
)"
helper_reach_is 11 "$m" "a paths:-filtered pull_request does NOT — and it is the LAST key in on:, so the filter is not read from a sibling trigger"

m="$(helper_fixture reach-none <<'FIX'
name: f
on:
  push:
    branches: [main]
jobs: {}
FIX
)"
helper_reach_is 2 "$m" "a push-only workflow reaches no PR"

helper_wired_is() { # <want-rc> <file> <script> <label>
  local want="$1" f="$2" s="$3" label="$4" got=0
  assent_step_wired "$f" verify "$s" "$WORK" || got=$?
  ((got == want)) ||
    fail "helper self-check '$label': assent_step_wired returned $got, want $want — the shared step reader does not do what all three gates delegate to it"
  echo "OK: helper self-check — $label (rc=$want)"
}

m="$(helper_fixture wired-min <<'FIX'
name: f
on:
  pull_request:
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - name: gate
        run: bash hack/examples/dogfood_wiring_test.sh
FIX
)"
helper_wired_is 0 "$m" "$SELF_REL" "a minimal wired step is accepted"
helper_wired_is 5 "$m" "hack/lint/workflow_pins_test.sh" "a script the fixture does not run is reported absent"

# G3-01/G3-02 — the codes the original fixture set did NOT cover, which is why
# both P1s survived a 28-row mutant matrix: it was broad in mutant SHAPES and
# narrow in CODE coverage. Every filter code and the disarm code now have a
# host-independent fixture, and the filter ones are written at a NON-4-space
# indent on purpose — the bypass was a grep pinned to exactly four spaces, and
# 6-space is valid YAML resolving to the identical mapping.
m="$(helper_fixture reach-paths-6sp <<'FIX'
name: f
on:
  pull_request:
      paths: [internal/**]
jobs: {}
FIX
)"
helper_reach_is 11 "$m" "paths: at SIX spaces is still a paths: filter (G3-01 — the 4-space grep fell through to 0 here)"

m="$(helper_fixture reach-types-6sp <<'FIX'
name: f
on:
  pull_request:
      types: [closed]
jobs: {}
FIX
)"
helper_reach_is 12 "$m" "types: [closed] at SIX spaces still omits the defaults (G3-01)"

m="$(helper_fixture reach-branches-6sp <<'FIX'
name: f
on:
  pull_request:
      branches: [release]
jobs: {}
FIX
)"
helper_reach_is 13 "$m" "branches: at SIX spaces still excludes main (G3-01)"

m="$(helper_fixture reach-bi-6sp <<'FIX'
name: f
on:
  pull_request:
      branches-ignore: [main]
jobs: {}
FIX
)"
helper_reach_is 13 "$m" "branches-ignore: [main] at SIX spaces still excludes main (G3-01)"

m="$(helper_fixture reach-bi-quoted-hash <<'FIX'
name: f
on:
  pull_request:
    branches-ignore: ['x #y', 'main']
jobs: {}
FIX
)"
helper_reach_is 13 "$m" "a quoted ' #' before 'main' in branches-ignore does not hide it (G3-04 — comment stripping is fail-OPEN for a must-NOT-contain test, so it is off for this key)"

# The property that keeps G3-01's indent-agnostic widening honest, MEASURED
# rather than reasoned: a key's value region must stop at a SAME-INDENT sibling
# key. If it swallowed the sibling, `branches: [release]` followed by
# `branches-ignore: [main]` would merge into one token set, `main` would appear
# among the `branches:` tokens, and a main-excluding filter would look fine.
m="$(helper_fixture reach-sibling-scope <<'FIX'
name: f
on:
  pull_request:
    branches: [release]
    branches-ignore: [main]
jobs: {}
FIX
)"
helper_reach_is 13 "$m" "a key's value region stops at a SAME-INDENT sibling — 'main' in branches-ignore does not leak into the branches: token set (the bound G3-01's widening rests on)"

m="$(helper_fixture reach-legit-6sp <<'FIX'
name: f
on:
  pull_request:
      types: [opened, synchronize, reopened, ready_for_review]
      branches: [main]
jobs: {}
FIX
)"
helper_reach_is 0 "$m" "the legitimate shapes are still accepted at SIX spaces — G3-01's fix widened the match, it did not make the check indent-hostile"

m="$(helper_fixture reach-unknown-key <<'FIX'
name: f
on:
  pull_request:
    only-when: [tuesday]
jobs: {}
FIX
)"
helper_reach_is 10 "$m" "an UNKNOWN key under pull_request: is REFUSED, not accepted — the header's own rule, now honoured for a key it cannot evaluate as well as for an unparseable shape"

m="$(helper_fixture wired-if-first <<'FIX'
name: f
on:
  pull_request:
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - if: ${{ github.event_name != 'pull_request' }}
        name: gate
        run: bash hack/examples/dogfood_wiring_test.sh
FIX
)"
helper_wired_is 8 "$m" "$SELF_REL" "a step whose FIRST key is 'if:' is DISARMED (G3-02 — the sequence-item form '      - if:' was never matched, and this graded WIRED)"

m="$(helper_fixture wired-coe-first <<'FIX'
name: f
on:
  pull_request:
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      - continue-on-error: true
        name: gate
        run: bash hack/examples/dogfood_wiring_test.sh
FIX
)"
helper_wired_is 8 "$m" "$SELF_REL" "a step whose FIRST key is 'continue-on-error:' is DISARMED (G3-02)"

m="$(helper_fixture wired-comment <<'FIX'
name: f
on:
  pull_request:
jobs:
  verify:
    runs-on: ubuntu-latest
    steps:
      # bash hack/examples/dogfood_wiring_test.sh
      - name: gate
        # run: bash hack/examples/dogfood_wiring_test.sh
        run: echo hi
FIX
)"
helper_wired_is 5 "$m" "$SELF_REL" "a step that only MENTIONS the script in comments is reported absent (the residual-3 property, on a fixture)"

echo "== 7. this gate runs in the pull-request-visible verify job, undisarmed =="
rc=0; pr_step_pinned "$WORKFLOW" || rc=$?
case "$rc" in
  0) echo "OK: the 'verify' job (which fires on pull_request) runs $SELF_REL, argument-free and undisarmed" ;;
  2) fail "verify.yaml no longer triggers on pull_request — every pin in this script would reach CI only via release-exitgate, which skips PRs (RELSE-08)" ;;
  3) fail "the 'verify' job block extracted empty or without 'steps:' — the PR-visibility assertions would be vacuous" ;;
  4) fail "the 'verify' job carries a JOB-LEVEL if: — if it skips pull requests this gate is push-only exactly like release-exitgate, and RELSE-08 is reproduced" ;;
  5) fail "the pull-request-visible 'verify' job does not invoke $SELF_REL — this gate would run only via release-exitgate (skipped on PRs), so a PR that strips -count=1 from verify.yaml merges green and reddens main afterwards (RELSE-08)" ;;
  6) fail "could not isolate this gate's step in the verify job — the extraction broke, so the disarm assertions would be vacuous" ;;
  7) fail "the step invokes $SELF_REL WITH ARGUMENTS — an argument-carrying invocation is how a wired-looking step is hollowed out" ;;
  8) fail "the step is present but DISARMED — an 'if:' or 'continue-on-error:' means a red gate does not fail the PR" ;;
  9) fail "the 'verify' job carries a JOB-LEVEL needs: — a dependency that skips on pull requests skips this job with it, so the gate is push-only by proxy while the step still looks wired (RELSE-08 through the back door)" ;;
  10) fail "verify.yaml's pull_request trigger is something hack/lib/pr_reach.sh REFUSES to grade: either a shape it cannot read (a flow mapping, an anchor, an alias) or a filter key outside GitHub's five (types, branches, branches-ignore, paths, paths-ignore). It refuses rather than guess, because a narrowing hidden in either would disarm this gate silently" ;;
  11) fail "verify.yaml's pull_request trigger carries a 'paths:' or 'paths-ignore:' filter — the trigger is present and the gate still loses every PR that does not touch the listed paths, which includes PRs to Taskfile.yml and .github/workflows/** (the two files this gate reads)" ;;
  12) fail "verify.yaml's pull_request trigger carries a 'types:' filter that omits one of GitHub's defaults (opened, synchronize, reopened) — e.g. 'types: [closed]' satisfies a presence grep while the gate never runs on an open PR" ;;
  13) fail "verify.yaml's pull_request trigger carries a branch filter that excludes 'main' — PRs onto the branch this gate protects would not run it" ;;
  *) fail "pr_step_pinned returned an unmapped code $rc" ;;
esac

echo "== 7b. every branch of the PR-visibility pin proved capable of going RED =="
# Bound, stated rather than implied: rc=6 is controlled for the reachable route
# (a step merged into its neighbour by a deleted `- name:`); its other route, a
# region that is not a step at all, needs a workflow actionlint cannot parse and
# so is unreachable through green CI. rc=13's `branches-ignore:` half is
# controlled through its `branches:` sibling only. Every other branch below,
# including the ones D-157 added (9..13) and rc=3, which was uncontrolled until
# cross-pinning made the JOB NAME a single point of failure for all three
# gates, is controlled.
expect_rc() { # <want> <file> <label>
  local want="$1" f="$2" label="$3" got=0
  pr_step_pinned "$f" || got=$?
  ((got == want)) || fail "mutation control '$label': pr_step_pinned returned $got, want $want — the branch is not the one this mutation exercises"
  echo "OK: mutation control — $label (rc=$want)"
}

m="$WORK/verify.nostep.yaml"
grep -vF -- "run: bash $SELF_REL" "$WORKFLOW" >"$m"
[[ "$(wc -l <"$m")" -lt "$(wc -l <"$WORKFLOW")" ]] || fail "mutation did not land: the step's run line is still present"
expect_rc 5 "$m" "the gate's step was deleted from the verify job"

# The subtler half of the same branch, and a bug this control CAUGHT: the run
# line is gone but the step comments still NAME the script. A fixed-string
# presence check calls that wired.
m="$WORK/verify.commentonly.yaml"
sed "s|^\([[:space:]]*\)run: bash ${SELF_REL}\$|\\1# run: bash ${SELF_REL}  # temporarily disabled|" "$WORKFLOW" >"$m"
cmp -s "$WORKFLOW" "$m" && fail "mutation did not land: the run line was not commented out"
grep -qF -- "$SELF_REL" "$m" || fail "mutation is not the intended one: the mutant no longer mentions the script at all, so it does not test comment-vs-command"
expect_rc 5 "$m" "the invocation was COMMENTED OUT while the step comments still name the script"

# R2-03: deleting ONLY the step's `- name:` line merges it into the neighbouring
# step. Before the one-`run:`-key invariant this returned 0 on a malformed
# workflow; it must now be rejected as an isolation failure.
m="$WORK/verify.noname.yaml"
grep -v 'name: dogfood + -count=1 wiring pin' "$WORKFLOW" >"$m"
cmp -s "$WORKFLOW" "$m" && fail "mutation did not land: the step's '- name:' line is still present"
grep -qF -- "run: bash $SELF_REL" "$m" || fail "mutation is not the intended one: it removed the invocation too, which is the rc=5 control again"
expect_rc 6 "$m" "the step's '- name:' was deleted, merging it into the neighbouring step (two run: keys)"

m="$WORK/verify.args.yaml"
sed "s|run: bash ${SELF_REL}\$|run: bash ${SELF_REL} --quick|" "$WORKFLOW" >"$m"
cmp -s "$WORKFLOW" "$m" && fail "mutation did not land: no argument was appended to the gate invocation"
expect_rc 7 "$m" "the step still runs the gate, but with an argument"

m="$WORK/verify.disarmed.yaml"
awk -v self="run: bash $SELF_REL" '{ if (index($0, self) > 0) print "        continue-on-error: true"; print }' "$WORKFLOW" >"$m"
cmp -s "$WORKFLOW" "$m" && fail "mutation did not land: continue-on-error was not inserted"
expect_rc 8 "$m" "the step was made advisory with continue-on-error"

m="$WORK/verify.jobif.yaml"
awk '{ print; if ($0 == "  verify:") print "    if: github.event_name != '"'"'pull_request'"'"'" }' "$WORKFLOW" >"$m"
cmp -s "$WORKFLOW" "$m" && fail "mutation did not land: the job-level if: was not inserted"
expect_rc 4 "$m" "the verify JOB was given release-exitgate's push-only guard"

m="$WORK/verify.nopr.yaml"
grep -vE '^[[:space:]]+pull_request:[[:space:]]*$' "$WORKFLOW" >"$m"
[[ "$(wc -l <"$m")" -lt "$(wc -l <"$WORKFLOW")" ]] || fail "mutation did not land: pull_request: is still in the on: block"
expect_rc 2 "$m" "the workflow stopped triggering on pull_request at all"

# --- D-157: the branches the OLD `grep -qE '^[[:space:]]+pull_request:'` could
# not tell apart. Each of these leaves that grep satisfied. The first two are
# the reviewer's measured bypass (rc=0 before this change).
insert_under_pr() { # <outfile> <lines...> — insert under `  pull_request:`
  local out="$1"
  shift
  local payload
  payload="$(printf '%s\n' "$@")"
  awk -v add="$payload" '{ print } $0 == "  pull_request:" { print add }' "$WORKFLOW" >"$out"
  cmp -s "$WORKFLOW" "$out" && fail "mutation did not land: nothing was inserted under 'pull_request:' in $out"
  grep -qE '^[[:space:]]+pull_request:' "$out" ||
    fail "mutation is not the intended one: $out no longer satisfies even the OLD presence grep, so it does not demonstrate the bypass"
  return 0
}

m="$WORK/verify.prpaths.yaml"
insert_under_pr "$m" "    paths: ['internal/**']"
expect_rc 11 "$m" "the pull_request trigger grew a paths: filter (the reviewer's measured bypass: present, and disarmed for a PR that touches only Taskfile.yml)"

m="$WORK/verify.prpathsignore.yaml"
insert_under_pr "$m" "    paths-ignore: ['docs/**']"
expect_rc 11 "$m" "the pull_request trigger grew a paths-ignore: filter"

m="$WORK/verify.prtypes.yaml"
insert_under_pr "$m" "    types: [closed]"
expect_rc 12 "$m" "the pull_request trigger was narrowed to types: [closed] — it never fires on an open PR"

m="$WORK/verify.prtypespartial.yaml"
insert_under_pr "$m" "    types: [opened, reopened]"
expect_rc 12 "$m" "the pull_request types: list dropped 'synchronize' — the gate would not re-run when a PR is updated"

m="$WORK/verify.prbranches.yaml"
insert_under_pr "$m" "    branches: [release]"
expect_rc 13 "$m" "the pull_request trigger was restricted to branches that do not include main"

m="$WORK/verify.prflowmap.yaml"
awk 'NR == 1 { print; next } { print }' "$WORKFLOW" | sed "s|^  pull_request:\$|  pull_request: {paths: ['internal/**']}|" >"$m"
cmp -s "$WORKFLOW" "$m" && fail "mutation did not land: pull_request: was not rewritten as a flow mapping"
expect_rc 10 "$m" "the pull_request filter was hidden in a YAML flow mapping the reader refuses to guess at"

m="$WORK/verify.jobneeds.yaml"
awk '{ print; if ($0 == "  verify:") print "    needs: release-exitgate" }' "$WORKFLOW" >"$m"
cmp -s "$WORKFLOW" "$m" && fail "mutation did not land: the job-level needs: was not inserted"
grep -qF -- "run: bash $SELF_REL" "$m" || fail "mutation is not the intended one: the needs: mutant lost the gate step"
expect_rc 9 "$m" "the verify JOB was made to depend on the push-only release-exitgate job — skipped dependency, skipped job, step still there"

m="$WORK/verify.jobrenamed.yaml"
sed "s|^  ${ASSENT_PR_JOB}:\$|  ${ASSENT_PR_JOB}-all:|" "$WORKFLOW" >"$m"
cmp -s "$WORKFLOW" "$m" && fail "mutation did not land: the '${ASSENT_PR_JOB}:' job key was not renamed"
grep -qF -- "run: bash $SELF_REL" "$m" || fail "mutation is not the intended one: the job-rename mutant lost the gate step"
expect_rc 3 "$m" "the '${ASSENT_PR_JOB}' job was renamed — with three gates now cross-pinning that job name, an uncontrolled extraction failure here would go vacuous for all of them at once"

m="$WORK/verify.prnested.yaml"
awk '
  { print }
  $0 == "  push:" && !done {
    print "  workflow_call:"
    print "    inputs:"
    print "      pull_request:"
    print "        type: string"
    done = 1
  }
' "$WORKFLOW" | grep -vE '^  pull_request:[[:space:]]*$' >"$m"
grep -qE '^[[:space:]]+pull_request:' "$m" ||
  fail "mutation is not the intended one: the nested-key mutant has no 'pull_request:' line at all, so it does not test indent scoping"
expect_rc 2 "$m" "the only 'pull_request:' left is an INPUT NAME nested under workflow_call — indent-blind grep says yes, GitHub says the workflow never runs on a PR"

# A fail-open found by building this mutant rather than by reasoning about it.
# The block-scalar form is accepted on purpose (see 7's legit-shapes control
# below), and accepting it naively means a BARE `bash <script>` line anywhere in
# the job satisfies the check — including inside some OTHER step's multi-command
# `run: |` body, where `echo skip && exit 0` on the line above would mean this
# gate never runs. Measured at rc=0 before the restriction; the block form is
# now accepted only when it is the block's SOLE command.
# The injected step is written here rather than appended to an existing one, so
# this control depends on NO other step's contents — sibling lanes edit
# verify.yaml's toolchain steps, and a mutation harness that anchors on their
# text reds for their reason instead of its own.
m="$WORK/verify.blockbleed.yaml"
grep -vF "run: bash $SELF_REL" "$WORKFLOW" \
  | awk -v s="$SELF_REL" '
      { print }
      $0 == "    steps:" && !done {
        print "      - name: unrelated"
        print "        run: |"
        print "          echo skip \&\& exit 0"
        print "          bash " s
        done = 1
      }
    ' >"$m"
[[ "$(grep -cF "bash $SELF_REL" "$m")" == "1" ]] ||
  fail "mutation did not land: expected exactly one 'bash $SELF_REL' occurrence in the bleed mutant"
grep -qF -- "run: bash $SELF_REL" "$m" && fail "mutation did not land: the real step's run: line survived in the bleed mutant"
expect_rc 5 "$m" "this gate's own step was deleted and its invocation smuggled into ANOTHER step's multi-command 'run: |' body"

# --- the other polarity. Seventeen red controls above are equally satisfied by a
# pin that always fails, so the shapes a maintainer could legitimately introduce
# must be proved to stay GREEN. These are the narrowings D-157 deliberately does
# NOT make: a types: superset, a branches: list containing main, a
# branches-ignore: that spares main, the inline `on:` forms (which cannot carry
# a filter at all), the quoted `"on":` key, and a block-scalar invocation.
m="$WORK/verify.typessuperset.yaml"
insert_under_pr "$m" "    types: [opened, synchronize, reopened, ready_for_review]"
expect_rc 0 "$m" "a types: list that SUPERSETS GitHub's defaults is still accepted"

m="$WORK/verify.branchesmain.yaml"
insert_under_pr "$m" "    branches: [main]"
expect_rc 0 "$m" "branches: [main] — the branch this gate protects — is still accepted"

m="$WORK/verify.branchesignoreother.yaml"
insert_under_pr "$m" "    branches-ignore: [gh-pages]"
expect_rc 0 "$m" "branches-ignore: on a branch that is not main is still accepted"

m="$WORK/verify.inlineseq.yaml"
awk '{ if ($0 == "on:") { print "on: [push, pull_request]"; skip = 1; next } if (skip && $0 ~ /^  /) next; skip = 0; print }' "$WORKFLOW" >"$m"
cmp -s "$WORKFLOW" "$m" && fail "mutation did not land: the on: block was not replaced by an inline sequence"
expect_rc 0 "$m" "the inline 'on: [push, pull_request]' form — which cannot carry a filter at all — is still accepted"

m="$WORK/verify.quotedon.yaml"
sed 's|^on:$|"on":|' "$WORKFLOW" >"$m"
cmp -s "$WORKFLOW" "$m" && fail "mutation did not land: the on: key was not quoted"
expect_rc 0 "$m" "the quoted '\"on\":' key (YAML 1.1 truthiness) is still accepted"

m="$WORK/verify.blockscalar.yaml"
awk -v s="        run: bash $SELF_REL" '{ if ($0 == s) { print "        run: |"; print "          bash '"$SELF_REL"'" } else print }' "$WORKFLOW" >"$m"
cmp -s "$WORKFLOW" "$m" && fail "mutation did not land: the run: line was not turned into a block scalar"
expect_rc 0 "$m" "a block-scalar 'run: |' invocation of this gate is still accepted"

# ------------------------------- 7c. the OTHER two PR-visible text gates --
#
# D-157 residual 2. Until now each of the three PR-visible text gates pinned
# only its OWN step: workflow_pins_test.sh section 5, aud2_exitgate_test.sh's
# check_pr_wiring, and section 7 above. A reviewer grepped and confirmed none
# named another — so a PR deleting one of those three steps was invisible to
# the gate that still ran on that PR. (`task check` and the push-only
# release-exitgate job caught it afterwards, which is why this was a gap and
# not a hole.) Each gate now asserts all three steps, so deleting ANY ONE of
# them reds on the pull request that does it, twice over.
check_sibling_gates() { # <workflow> — 0 when both siblings are wired
  local wf="$1" g rc=0
  while IFS= read -r g; do
    [[ -n "$g" ]] || continue
    rc=0
    assent_step_wired "$wf" "$ASSENT_PR_JOB" "$g" "$WORK" || rc=$?
    if ((rc != 0)); then
      SIBLING_FINDING="$g (rc=$rc)"
      return "$rc"
    fi
  done < <(assent_pr_gate_others "$SELF_REL")
  return 0
}

echo "== 7c. the OTHER two PR-visible text gates are wired in the same job =="
# Positive control on the shared list first: if this script is not a member,
# every "other" below is the wrong set and the section certifies nothing.
assent_pr_gate_others "$SELF_REL" >"$WORK/siblings" ||
  fail "$SELF_REL is not listed in ASSENT_PR_GATES (hack/lib/pr_reach.sh) — the cross-pin set is not the set this gate belongs to, so section 7c would grade an unrelated pair"
((${#ASSENT_PR_GATES[@]} == 3)) ||
  fail "ASSENT_PR_GATES holds ${#ASSENT_PR_GATES[@]} entries, expected the 3 PR-visible text gates — if a fourth is added, give it its own cross-pin section rather than widening this one silently"
[[ "$(wc -l <"$WORK/siblings" | tr -d ' ')" == "2" ]] ||
  fail "expected exactly 2 sibling gates, got $(wc -l <"$WORK/siblings" | tr -d ' ')"
while IFS= read -r sibling; do
  [[ -f "$ROOT/$sibling" ]] || fail "cross-pinned gate $sibling does not exist on disk — the pin would assert a step that runs nothing"
done <"$WORK/siblings"
SIBLING_FINDING=""
rc=0
check_sibling_gates "$WORKFLOW" || rc=$?
((rc == 0)) ||
  fail "a sibling PR-visible gate is not wired undisarmed into the '${ASSENT_PR_JOB}' job: ${SIBLING_FINDING} — see the rc table at pr_step_pinned. Deleting one of the three gate steps must redden the PR that does it, and this is the half of that guarantee this script owns"
echo "OK: both sibling gates ($(tr '\n' ' ' <"$WORK/siblings"| sed 's/ $//')) are wired, argument-free and undisarmed in the ${ASSENT_PR_JOB} job"

echo "== 7d. the cross-pin can fail — each sibling's step deleted in turn =="
while IFS= read -r sibling; do
  m="$WORK/verify.nosibling.$(basename "$sibling").yaml"
  grep -vF -- "run: bash $sibling" "$WORKFLOW" >"$m"
  [[ "$(wc -l <"$m")" -lt "$(wc -l <"$WORKFLOW")" ]] ||
    fail "mutation did not land: '$sibling' still has its run line in $m"
  grep -qF -- "run: bash $SELF_REL" "$m" ||
    fail "mutation is not the intended one: deleting $sibling's step also removed THIS gate's step, so the control grades self-wiring again"
  rc=0
  check_sibling_gates "$m" || rc=$?
  ((rc == 5)) ||
    fail "cross-pin control: deleting $sibling's step returned rc=$rc, want 5 (no run: command) — the cross-pin either cannot fail or failed for another reason"
  echo "OK: cross-pin control — deleting $sibling's step from the ${ASSENT_PR_JOB} job reds THIS gate (rc=5)"
done <"$WORK/siblings"

echo "PASS: dogfood wiring (REQ-EX-S08-02, REQ-EX-S08-03) — task check runs dogfood-examples after build; Taskfile.yml and verify.yaml both delegate to the shared discovery script; the cache-blind go test gates (${COUNT1_TASKS[*]}) carry -count=1, in Taskfile.yml AND in verify.yaml; and all THREE PR-visible text gates (this one and its two siblings) run undisarmed in a verify job whose pull_request trigger carries no paths:/types:/branches narrowing (D-157)"
