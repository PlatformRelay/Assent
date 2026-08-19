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

# extract_on_block <workflow> — the body of the top-level `on:` mapping.
extract_on_block() {
  awk '/^on:/ { ino = 1; next } ino && /^[A-Za-z]/ { ino = 0 } ino { print }' "$1"
}

# extract_job <workflow> <job> — the body of a 2-space-indented jobs: entry.
extract_job() {
  awk -v name="$2" '
    $0 == "  " name ":" { inb = 1; next }
    inb && /^  [A-Za-z0-9_.-]+:[[:space:]]*$/ { inb = 0 }
    inb { print }
  ' "$1"
}

# isolate_step_at <job-block-file> <line no> — the step containing that line,
# from its `- ` marker to the next one.
isolate_step_at() {
  local blockfile="$1" hitline="$2" start end
  start="$(grep -nE '^      - ' "$blockfile" | cut -d: -f1 | awk -v h="$hitline" '$1 <= h { s = $1 } END { print s }')"
  [[ -n "$start" ]] || return 1
  end="$(awk -v s="$start" 'NR > s && /^      - / { print NR; exit }' "$blockfile")"
  [[ -n "$end" ]] || end="$(($(wc -l <"$blockfile") + 1))"
  sed -n "${start},$((end - 1))p" "$blockfile"
}

# SELF_RE — SELF_REL as an ERE (the dot in `.sh` must not be a wildcard).
SELF_RE="${SELF_REL//./\\.}"

# pr_step_pinned <workflow> — 0 when this gate runs, undisarmed, in a
# pull-request-visible job. Distinct codes so a mutation control can prove it
# went red for ITS reason: 2 no pull_request trigger, 3 job block unextractable,
# 4 job-level if:, 5 step absent, 6 step unisolatable, 7 arguments, 8 disarmed.
pr_step_pinned() {
  local wf="$1"
  extract_on_block "$wf" >"$WORK/on.block"
  [[ -s "$WORK/on.block" ]] || return 2
  grep -qE '^[[:space:]]+pull_request:' "$WORK/on.block" || return 2
  extract_job "$wf" verify >"$WORK/job.verify"
  { [[ -s "$WORK/job.verify" ]] && grep -q '^    steps:' "$WORK/job.verify"; } || return 3
  grep -qE '^    if:' "$WORK/job.verify" && return 4
  # Anchored on a `run:` COMMAND, never a mention: verify.yaml's step comments
  # name this script ("Pinned by hack/examples/dogfood_wiring_test.sh"), and a
  # fixed-string search would let those comments satisfy the presence check
  # after the real invocation was deleted. Measured while building 7b.
  local runline
  runline="$(grep -nE "^[[:space:]]*run:[[:space:]]*bash[[:space:]]+${SELF_RE}([[:space:]]|\$)" "$WORK/job.verify" | head -1 | cut -d: -f1)"
  [[ -n "$runline" ]] || return 5
  isolate_step_at "$WORK/job.verify" "$runline" >"$WORK/step.self" || return 6
  { [[ -s "$WORK/step.self" ]] && grep -qE '^      - ' "$WORK/step.self"; } || return 6
  # Isolation bound, the same defect hack/lint/workflow_pins_test.sh:433-440
  # closes with a 1..6 LINE cap. Without a bound, deleting only this step's
  # `- name:` line merges it into the neighbouring step and the isolated region
  # silently becomes that other step's body plus ours — a malformed workflow
  # this function called fine (measured: rc=0). A line cap is the wrong shape
  # here: this step carries a long comment block, so the honest size (14) and
  # the merged size (15) are one line apart and any cap that admits the first
  # admits the second. The structural invariant is what actionlint itself
  # flags — a step has exactly ONE `run:` key — and it holds whatever the
  # comments do. (Also caught independently by workflow_pins_test.sh's own
  # "got 15 lines, expected 1..6" and by actionlint's `key "run" is
  # duplicated`, so this is a bound made explicit, not a hole being closed.)
  local n_run
  n_run="$(grep -cE '^        run:' "$WORK/step.self" || true)"
  ((n_run == 1)) || return 6
  grep -qE "^[[:space:]]*run:[[:space:]]*bash[[:space:]]+${SELF_RE}[[:space:]]*\$" "$WORK/step.self" || return 7
  grep -qE '^[[:space:]]*(if|continue-on-error):' "$WORK/step.self" && return 8
  return 0
}

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
  *) fail "pr_step_pinned returned an unmapped code $rc" ;;
esac

echo "== 7b. every branch of the PR-visibility pin proved capable of going RED =="
# Bound, stated rather than implied: rc=3 (verify job block unextractable —
# reachable by renaming the `verify:` job) has NO control here. rc=6 is
# controlled for the reachable route (a step merged into its neighbour by a
# deleted `- name:`); its other route, a region that is not a step at all,
# needs a workflow actionlint cannot parse and so is unreachable through green
# CI. Every other branch below is controlled.
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

echo "PASS: dogfood wiring (REQ-EX-S08-02, REQ-EX-S08-03) — task check runs dogfood-examples after build; Taskfile.yml and verify.yaml both delegate to the shared discovery script; the cache-blind go test gates (${COUNT1_TASKS[*]}) carry -count=1, in Taskfile.yml AND in verify.yaml; and this gate itself runs undisarmed in the pull-request-visible verify job"
