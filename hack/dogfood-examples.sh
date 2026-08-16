#!/usr/bin/env bash
# hack/dogfood-examples.sh — EX-S08 shared discovery script.
#
# Before this script existed, "run every example pack through `assent test`"
# was a hardcoded `for pack in service-catalog infra-vars topic-registry` loop
# duplicated in THREE places: Taskfile.yml's dogfood-examples task,
# .github/workflows/verify.yaml's dogfood step, and the greenExamplePacks Go
# slice in cmd/assent/test_corpus_test.go. Adding a pack meant editing all
# three in sync — a proven skew risk (a pack green in one, silently absent in
# another). Taskfile.yml and verify.yaml now both call THIS script; the Go
# corpus test discovers packs the same way (filepath.Glob over
# examples/packs/*/.assent/tests), so all three halves walk the same
# filesystem contract instead of carrying independent copies of a name list.
#
# Discovery rule (matches examples/README.md's inventory gate,
# hack/docs/example_format_inventory_test.sh, EX-S01): every immediate child
# of examples/packs/ that has a .assent/tests/ subdirectory is a pack to
# dogfood. A pack directory WITHOUT .assent/tests/ is not silently skipped —
# hack/docs/example_format_inventory_test.sh treats that as a hard error
# (incomplete tree) — so this script only ever sees complete packs, and an
# incomplete/red one that DOES have .assent/tests/ fails loudly below instead
# of being filtered out by name.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

BIN="${ASSENT_BIN:-bin/assent}"
if [[ ! -x "$BIN" ]]; then
  echo "== building $BIN =="
  CGO_ENABLED=0 go build -o "$BIN" ./cmd/assent
fi

packs=()
for dir in examples/packs/*/; do
  [[ -d "$dir" ]] || continue
  name="$(basename "$dir")"
  if [[ -d "${dir}.assent/tests" ]]; then
    packs+=("$name")
  fi
done

if [[ "${#packs[@]}" -eq 0 ]]; then
  echo "no example packs with .assent/tests/ discovered under examples/packs/" >&2
  exit 1
fi

echo "discovered packs: ${packs[*]}"
for pack in "${packs[@]}"; do
  echo "== assent test examples/packs/$pack =="
  "$BIN" test "examples/packs/$pack"
  "$BIN" test --coverage "examples/packs/$pack"
done

echo "OK: dogfooded ${#packs[@]} example pack(s): ${packs[*]}"
