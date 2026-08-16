#!/usr/bin/env bash
# REQ-EX-S08-02/03 — dogfood wiring pin, adversarially proven (both polarities).
#
#   REQ-EX-S08-02: Taskfile.yml's dogfood-examples task AND verify.yaml's
#   dogfood step call the SHARED hack/dogfood-examples.sh discovery script —
#   neither may re-hardcode its own `for pack in <names>` loop.
#   REQ-EX-S08-03: `task check` runs dogfood-examples (after build); deleting
#   that line from check: must redden this pin.
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

echo "PASS: dogfood wiring (REQ-EX-S08-02, REQ-EX-S08-03) — task check runs dogfood-examples after build; Taskfile.yml and verify.yaml both delegate to the shared discovery script"
