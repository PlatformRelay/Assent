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
# human would regress it.
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
# Deliberately NOT listed: `test` (whole-tree `go test -race ./...` — in-process
# subjects, honest cache, and a real time saver) and `coverage` (in-process
# subjects; `-coverprofile` results do cache but the profile is reproduced
# faithfully). `determinism` needs no pin: `-count=2` is uncacheable by
# construction.
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

echo "PASS: dogfood wiring (REQ-EX-S08-02, REQ-EX-S08-03) — task check runs dogfood-examples after build; Taskfile.yml and verify.yaml both delegate to the shared discovery script; the cache-blind go test gates (${COUNT1_TASKS[*]}) carry -count=1"
