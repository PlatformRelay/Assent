#!/usr/bin/env bash
# REQ-EX-S01-01..05 — examples pack/format inventory paper-gate (both polarities).
#
# Enumerates examples/packs/* (every immediate child is a pack), records each
# pack's governed format from class match.paths extensions (.yaml / .json /
# .tfvars / .tf), and asserts examples/README.md names exactly those packs and
# claims exactly those formats. A pack directory without .assent/tests/ is an
# incomplete tree (hard error). Claiming cue / kafka-acl / an unmapped token
# (e.g. "hcl", which pack_formats never emits — the real, mapped token is "tf",
# landed by EX-S05) must go red; omitting a real pack from the README must go
# red the other way.
#
# WIRED: `task docs-gates` runs this script (REQ-EX-S01-05). Deleting that
# invocation reddens the wiring pin here and in truthlag_pins_test.sh.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

fails=0
pass() { echo "PASS  $1"; }
fail() { echo "FAIL  $1" >&2; fails=$((fails + 1)); }

# --- discovery ----------------------------------------------------------------

# Immediate children of examples/packs. Every one is a pack candidate.
list_pack_dirs() {
  local root="$1"
  local d
  for d in "$root"/examples/packs/*; do
    [[ -d "$d" ]] || continue
    basename "$d"
  done | LC_ALL=C sort
}

# Formats governed by class match.paths. .tfvars is not .tf.
# Reads .assent/config.yaml; environment match globs without an extension are ignored.
pack_formats() {
  local pack_dir="$1"
  local cfg="$pack_dir/.assent/config.yaml"
  [[ -f "$cfg" ]] || return 0
  # Prefer the classes: block so env path globs cannot contribute a false extension.
  awk '
    $0 ~ /^classes:[[:space:]]*$/ { in_cls = 1; next }
    in_cls && /^[a-zA-Z]/ { in_cls = 0 }
    in_cls { print }
  ' "$cfg" | grep -oE '\*\.[A-Za-z0-9]+' | sed 's/^\*\.//' | while read -r ext; do
    case "$ext" in
      tfvars) echo tfvars ;;
      tf) echo tf ;;
      yaml|yml) echo yaml ;;
      json) echo json ;;
    esac
  done | LC_ALL=C sort -u
}

all_pack_formats() {
  local root="$1"
  local name
  while IFS= read -r name; do
    [[ -z "$name" ]] && continue
    pack_formats "$root/examples/packs/$name"
  done < <(list_pack_dirs "$root") | LC_ALL=C sort -u
}

# Backtick pack names on the [`packs/`] list item (and its wrapped continuation).
readme_packs() {
  local readme="$1"
  awk '
    /\[`packs\/`\]/ { in_packs = 1 }
    in_packs { print }
    in_packs && /^[[:space:]]*- \[`/ && !/\[`packs\/`\]/ { in_packs = 0 }
    in_packs && /^$/ { in_packs = 0 }
  ' "$readme" | grep -oE '`[a-z0-9-]+`' | tr -d '`' | grep -vx 'packs' | LC_ALL=C sort -u
}

# ALL comma-separated tokens on the dedicated "Input formats:" sentence.
# No whitelist: an unrecognised claim (toml, ini, …) must surface as a token so
# the doc-vs-filesystem comparison reddens instead of silently dropping it.
readme_formats() {
  local readme="$1"
  local line
  line="$(grep -E '^[[:space:]]*Input formats:' "$readme" || true)"
  [[ -n "$line" ]] || return 0
  printf '%s\n' "$line" \
    | sed -E 's/^[[:space:]]*Input formats:[[:space:]]*//; s/\.?[[:space:]]*$//' \
    | tr ',' '\n' \
    | sed -E 's/^[[:space:]]+//; s/[[:space:]]+$//; s/^yml$/yaml/' \
    | grep -v '^$' | LC_ALL=C sort -u
}

# Returns 0 when README and filesystem agree. Prints a PACKS=/FORMATS= report.
inventory_ok() {
  local root="$1"
  local readme="$2"
  local name
  local incomplete=0
  local packs_fs packs_doc fmts_fs fmts_doc

  packs_fs="$(list_pack_dirs "$root" | tr '\n' ' ' | sed 's/[[:space:]]*$//')"
  while IFS= read -r name; do
    [[ -z "$name" ]] && continue
    if [[ ! -d "$root/examples/packs/$name/.assent/tests" ]]; then
      echo "incomplete pack (no .assent/tests/): $name" >&2
      incomplete=1
    fi
  done < <(list_pack_dirs "$root")
  if [[ "$incomplete" -ne 0 ]]; then
    return 1
  fi

  packs_doc="$(readme_packs "$readme" | tr '\n' ' ' | sed 's/[[:space:]]*$//')"
  fmts_fs="$(all_pack_formats "$root" | tr '\n' ' ' | sed 's/[[:space:]]*$//')"
  fmts_doc="$(readme_formats "$readme" | tr '\n' ' ' | sed 's/[[:space:]]*$//')"

  echo "PACKS: $packs_fs"
  echo "FORMATS: $fmts_fs"

  if [[ -z "$packs_fs" ]]; then
    echo "no example packs discovered" >&2
    return 1
  fi
  if [[ "$packs_doc" != "$packs_fs" ]]; then
    echo "README packs [$packs_doc] != filesystem [$packs_fs]" >&2
    return 1
  fi
  if [[ -z "$fmts_doc" ]]; then
    echo "README has no 'Input formats:' sentence (or no tokens on it)" >&2
    return 1
  fi
  local tok unmapped=""
  for tok in $fmts_doc; do
    case " $fmts_fs " in
      *" $tok "*) ;;
      *) unmapped="$unmapped $tok" ;;
    esac
  done
  if [[ -n "$unmapped" ]]; then
    echo "README claims format(s) with no pack class match.paths extension:$unmapped" >&2
    return 1
  fi
  if [[ "$fmts_doc" != "$fmts_fs" ]]; then
    echo "README formats [$fmts_doc] != pack class match.paths [$fmts_fs]" >&2
    return 1
  fi
  return 0
}

docs_gates_invokes_inventory() {
  local taskfile="$1"
  awk '
    $0 == "  docs-gates:" { inblk = 1; next }
    inblk && /^  [A-Za-z0-9_.:-]+:[[:space:]]*$/ { inblk = 0 }
    inblk { print }
  ' "$taskfile" | grep -q 'hack/docs/example_format_inventory_test.sh'
}

# --- polarity mutations (must go red) -----------------------------------------

GOOD_README="$WORK/readme.good.md"
cat >"$GOOD_README" <<'EOF'
# Examples
- [`packs/`](packs/) — complete adopter policy trees (`topic-registry`, `service-catalog`,
  `infra-vars`).
Input formats: yaml, json, tfvars, tf.
EOF

if inventory_ok "$ROOT" "$GOOD_README" >"$WORK/good.out" 2>"$WORK/good.err"; then
  pass "synthetic README matching the three shipped packs is green"
else
  fail "synthetic matching README should be green: $(cat "$WORK/good.err")"
fi

EXTRA="$WORK/readme.extra.md"
sed 's/`infra-vars`/`infra-vars`, `kafka-acl`/' "$GOOD_README" >"$EXTRA"
if inventory_ok "$ROOT" "$EXTRA" >"$WORK/extra.out" 2>"$WORK/extra.err"; then
  fail "claimed-but-missing pack kafka-acl stayed green (REQ-EX-S01-02 vacuous)"
else
  pass "REQ-EX-S01-02: extra pack name kafka-acl reddens"
fi

OMIT="$WORK/readme.omit.md"
sed 's/`topic-registry`, //' "$GOOD_README" >"$OMIT"
if inventory_ok "$ROOT" "$OMIT" >"$WORK/omit.out" 2>"$WORK/omit.err"; then
  fail "omitting topic-registry from README stayed green (REQ-EX-S01-03 vacuous)"
else
  pass "REQ-EX-S01-03: omitting topic-registry reddens"
fi

CUE="$WORK/readme.cue.md"
sed 's/tf\./tf, cue./' "$GOOD_README" >"$CUE"
if inventory_ok "$ROOT" "$CUE" >"$WORK/cue.out" 2>"$WORK/cue.err"; then
  fail "claiming cue stayed green (REQ-EX-S01-04 vacuous)"
else
  pass "REQ-EX-S01-04: claiming cue reddens"
fi

TOML="$WORK/readme.toml.md"
sed 's/tf\./tf, toml./' "$GOOD_README" >"$TOML"
if inventory_ok "$ROOT" "$TOML" >"$WORK/toml.out" 2>"$WORK/toml.err"; then
  fail "claiming toml stayed green (unrecognised token silently dropped — REQ-EX-S01-04 fail-open)"
else
  pass "REQ-EX-S01-04: claiming toml (token outside any whitelist) reddens"
fi

HCL="$WORK/readme.hcl.md"
sed 's/tf\./tf, hcl./' "$GOOD_README" >"$HCL"
if inventory_ok "$ROOT" "$HCL" >"$WORK/hcl.out" 2>"$WORK/hcl.err"; then
  fail "claiming an unrecognised hcl token stayed green"
else
  pass "REQ-EX-S01-04: claiming an unmapped hcl token reddens (S05 landed: tf is now a real, mapped token)"
fi

# Incomplete tree: a fourth pack dir with .assent/ but no tests. The README
# variant DOES list `orphan` (and its yaml format is already claimed), so the
# missing-tests check is the ONLY thing that can redden this — deleting that
# check must flip this assertion, not the packs-list comparison.
INCOMPLETE="$WORK/incomplete-root"
mkdir -p "$INCOMPLETE/examples/packs"
for name in infra-vars service-catalog topic-registry; do
  mkdir -p "$INCOMPLETE/examples/packs/$name/.assent/tests"
  cp "$ROOT/examples/packs/$name/.assent/config.yaml" "$INCOMPLETE/examples/packs/$name/.assent/config.yaml"
done
mkdir -p "$INCOMPLETE/examples/packs/orphan/.assent"
printf 'classes:\n  - name: x\n    match: { paths: ["x/**/*.yaml"] }\n' >"$INCOMPLETE/examples/packs/orphan/.assent/config.yaml"
INCOMPLETE_README="$WORK/readme.incomplete.md"
sed 's/`infra-vars`/`infra-vars`, `orphan`/' "$GOOD_README" >"$INCOMPLETE_README"
if inventory_ok "$INCOMPLETE" "$INCOMPLETE_README" >"$WORK/inc.out" 2>"$WORK/inc.err"; then
  fail "pack without .assent/tests/ stayed green"
else
  if grep -q 'incomplete pack (no .assent/tests/): orphan' "$WORK/inc.err"; then
    pass "pack directory without .assent/tests/ reddens"
  else
    fail "orphan mutation reddened for the wrong reason (confounded): $(cat "$WORK/inc.err")"
  fi
fi

# --- happy path against the real tree (REQ-EX-S01-01) -------------------------

if inventory_ok "$ROOT" "$ROOT/examples/README.md" >"$WORK/real.out" 2>"$WORK/real.err"; then
  report="$(tr '\n' ' ' <"$WORK/real.out")"
  echo "$report"
  if echo "$report" | grep -q 'PACKS: infra-vars service-catalog topic-registry' \
    && echo "$report" | grep -q 'FORMATS: json tf tfvars yaml'; then
    pass "REQ-EX-S01-01: real tree reports the three packs and yaml/json/tf/tfvars"
  else
    fail "REQ-EX-S01-01: unexpected report: $report"
  fi
else
  fail "REQ-EX-S01-01: real examples/README.md vs packs is red: $(cat "$WORK/real.err")"
fi

# --- wiring (REQ-EX-S01-05) ---------------------------------------------------

if docs_gates_invokes_inventory "$ROOT/Taskfile.yml"; then
  pass "REQ-EX-S01-05: task docs-gates invokes example_format_inventory_test.sh"
else
  fail "REQ-EX-S01-05: Taskfile.yml docs-gates does not run hack/docs/example_format_inventory_test.sh"
fi

MUT_TF="$WORK/Taskfile.no-inventory.yml"
# Drop only the inventory invocation; prove the assertion goes red.
grep -v 'hack/docs/example_format_inventory_test.sh' "$ROOT/Taskfile.yml" >"$MUT_TF"
if grep -q 'hack/docs/example_format_inventory_test.sh' "$MUT_TF"; then
  fail "REQ-EX-S01-05: mutation did not remove the inventory invocation"
elif docs_gates_invokes_inventory "$MUT_TF"; then
  fail "REQ-EX-S01-05: wiring assertion stayed green after deleting the invocation (vacuous)"
else
  pass "REQ-EX-S01-05: deleting the docs-gates inventory line reddens the wiring pin"
fi

if [[ "$fails" -ne 0 ]]; then
  echo "FAILED: $fails example-format inventory check(s)" >&2
  exit 1
fi
echo "OK: example format inventory (EX-S01)"
