#!/usr/bin/env bash
# REQ-AUD-S07-01 — adversarial proof that the D-123 depguard boundary rules FIRE.
#
# `task lint` proves only the CLEAN polarity (the tree at HEAD has no forbidden
# import). A deny-rule that can never fail is worthless, so this gate proves the
# other polarity: it builds a throwaway Go module OUTSIDE the repo, copies the
# repo's REAL .golangci.yml into it, and checks that
#
#   (a) every guarded directory x every denied package is reported by depguard
#       naming the `pure-tree` rule                                (violation), and
#   (b) the same module without the violating imports is depguard-clean, while a
#       package OUTSIDE the guarded tree may import the denied packages freely
#                                                     (clean polarity + scoping).
#
# The guarded directories and the denied packages are EXTRACTED from
# .golangci.yml rather than restated here, so the gate cannot drift away from
# the config it guards; the extraction itself is positive-controlled below
# (non-empty, and containing known-present entries) so a broken pattern fails
# loudly instead of silently asserting nothing.
#
# REQ-AUD-S15-01 extends it with a SECOND boundary (audit finding ARCH-02):
#
#   (c) `cmd/assent` names only CONSTRUCTION symbols from the GitLab adapter —
#       the orchestration read port speaks forge.MRInfo / forge.ErrNotFound, so
#       a GitHub adapter can satisfy it without a line changing in cmd.
#
# That one is SYMBOL-level, not import-level, so depguard cannot express it:
# cmd/assent legitimately imports the adapter to construct it. Hence a grep gate
# — and, since a grep that matches nothing passes open, a positive control that
# proves the scanner fires on a deliberately violating copy of the tree.
#
# Nothing is ever written into the repository working tree.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

CONFIG="$ROOT/.golangci.yml"
MODULE="github.com/PlatformRelay/assent"

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

command -v golangci-lint >/dev/null 2>&1 ||
  fail "golangci-lint is not on PATH — this gate cannot be skipped, it is the boundary control (D-123)"

[[ -f "$CONFIG" ]] || fail "missing $CONFIG"

GO_DIRECTIVE="$(sed -nE 's/^go ([0-9]+\.[0-9]+(\.[0-9]+)?)$/\1/p' "$ROOT/go.mod" | head -1)"
[[ -n "$GO_DIRECTIVE" ]] || fail "could not read the go directive from go.mod"

echo "== extract the D-123 guarded tree + deny list from .golangci.yml =="

# Guarded directories and denied packages are extracted PER RULE: the config
# carries TWO depguard rules since E10-S02 (pure-tree, the D-123 boundary; and
# cmd-adapter-boundary, ADR-0021 §1), and a global extraction would assert every
# guarded dir against every deny of the OTHER rule too. extract_rule_block
# isolates one rule's block (from its `rule-name:` key to the next sibling key
# at the same indent), then sed pulls the globs and pkg lines out of that block.
extract_rule_block() {
  awk -v rule="$1" '
    /^        [A-Za-z][A-Za-z0-9_-]*:$/ { inblock = ($0 ~ "^[[:space:]]{8}" rule ":$"); next }
    inblock { print }
  ' "$CONFIG"
}

GUARDED=()
while IFS= read -r line; do
  GUARDED+=("$line")
done < <(extract_rule_block "pure-tree" | sed -nE 's|^[[:space:]]*- "\*\*/(.+)/\*\*"[[:space:]]*$|\1|p')

DENIED=()
while IFS= read -r line; do
  DENIED+=("$line")
done < <(extract_rule_block pure-tree | sed -nE 's|^[[:space:]]*- pkg: "(.+)"[[:space:]]*$|\1|p')

CMD_GUARDED=()
while IFS= read -r line; do
  CMD_GUARDED+=("$line")
done < <(extract_rule_block cmd-adapter-boundary | sed -nE 's|^[[:space:]]*- "\*\*/(.+)/\*\*"[[:space:]]*$|\1|p')

CMD_DENIED=()
while IFS= read -r line; do
  CMD_DENIED+=("$line")
done < <(extract_rule_block cmd-adapter-boundary | sed -nE 's|^[[:space:]]*- pkg: "(.+)"[[:space:]]*$|\1|p')

# --- positive controls on the extraction itself ------------------------------
# Without these, a sed pattern that matches nothing would leave both arrays
# empty and every assertion below would vacuously "pass".
(( ${#GUARDED[@]} >= 8 )) ||
  fail "extracted only ${#GUARDED[@]} guarded paths from $CONFIG — the files: glob pattern stopped matching"
(( ${#DENIED[@]} >= 4 )) ||
  fail "extracted only ${#DENIED[@]} denied packages from $CONFIG — the deny pkg: pattern stopped matching"
[[ ${#CMD_GUARDED[@]} == 1 && "${CMD_GUARDED[0]:-}" == "cmd/assent" ]] ||
  fail "cmd-adapter-boundary extraction wrong: got ${CMD_GUARDED[*]:-<none>} — the E10-S02 files: glob stopped matching"
(( ${#CMD_DENIED[@]} == 2 )) ||
  fail "extracted only ${#CMD_DENIED[@]} adapter deny entries — the cmd-adapter-boundary deny list stopped matching"

contains() {
  local needle="$1"; shift
  local item
  for item in "$@"; do
    [[ "$item" == "$needle" ]] && return 0
  done
  return 1
}

# The full D-123 guarded tree, restated here on purpose: the probe modules are
# generated FROM the config, so a config that shrinks would otherwise still be
# "self-consistently" proven. This list is what pins the tree to the decision.
for expected in internal/core internal/change internal/glob internal/lint \
  internal/catalogue internal/evaldecode internal/compare schemas; do
  contains "$expected" "${GUARDED[@]}" ||
    fail "guarded tree extracted from $CONFIG is missing $expected (D-123)"
done
for expected in "$MODULE/internal/forge" "$MODULE/internal/render" "$MODULE/cmd" net; do
  contains "$expected" "${DENIED[@]}" ||
    fail "deny list extracted from $CONFIG is missing $expected (D-123)"
done

echo "OK: ${#GUARDED[@]} pure-tree guarded paths, ${#DENIED[@]} pure-tree denied packages, cmd/assent boundary (${CMD_DENIED[*]}) extracted"

# probe_import maps a denied package to a concrete package to import. Local
# packages and stdlib `net` are exercised via a SUBPACKAGE so the probe proves
# prefix matching (the strictly stronger property), not just exact matching.
probe_import() {
  case "$1" in
    net) echo "net/http" ;;
    "$MODULE"/*) echo "$1/depguardport" ;;
    *)
      # A new NON-local deny needs a deliberate probe target: blank-importing
      # the denied path verbatim would only prove exact matching, silently
      # weakening this gate's prefix claim. Add a case above instead.
      fail "no probe import mapped for denied package '$1' — add a case to probe_import() that exercises a SUBPACKAGE of it"
      ;;
  esac
}

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# scaffold <dir> <mode>; mode=violating|clean
scaffold() {
  local mod="$1" mode="$2" dir imp denied
  mkdir -p "$mod"
  printf 'module %s\n\ngo %s\n' "$MODULE" "$GO_DIRECTIVE" >"$mod/go.mod"
  cp "$CONFIG" "$mod/.golangci.yml"

  # Stub the local denied packages so the probe imports resolve.
  for denied in "${DENIED[@]}"; do
    imp="$(probe_import "$denied")"
    case "$imp" in
      "$MODULE"/*)
        mkdir -p "$mod/${imp#"$MODULE"/}"
        printf '// Package depguardport stubs a port implementation.\npackage depguardport\n' \
          >"$mod/${imp#"$MODULE"/}/depguardport.go"
        ;;
    esac
  done

  for dir in "${GUARDED[@]}"; do
    mkdir -p "$mod/$dir/depguardprobe"
    write_probe "$mod/$dir/depguardprobe/probe.go" "$mode"
  done

  # A _test.go probe in the first guarded directory: the config claims test
  # files are in scope, so that claim gets its own assertion.
  mkdir -p "$mod/${GUARDED[0]}/depguardtestprobe"
  write_test_probe "$mod/${GUARDED[0]}/depguardtestprobe/probe_test.go" "$mode"

  # Scoping control: an UNGUARDED package importing every denied package. It
  # must never be reported — that proves `files:` actually scopes the rule
  # instead of applying it module-wide.
  mkdir -p "$mod/internal/depguardunguarded"
  write_probe "$mod/internal/depguardunguarded/probe.go" violating
}

# probe_imports emits the blank-import lines for <mode>.
probe_imports() {
  local mode="$1" denied imp
  printf '\t_ "strings"\n'
  [[ "$mode" == violating ]] || return 0
  for denied in "${DENIED[@]}"; do
    imp="$(probe_import "$denied")"
    printf '\t_ "%s"\n' "$imp"
  done
}

# write_probe emits a `main` package: revive's blank-imports rule exempts main
# and test packages, so the probe's own noise cannot mask a depguard finding.
write_probe() {
  local out="$1" mode="$2"
  {
    echo '// Command depguardprobe is a throwaway boundary probe (REQ-AUD-S07-01).'
    echo 'package main'
    echo
    echo 'import ('
    probe_imports "$mode"
    echo ')'
    echo
    echo 'func main() {}'
  } >"$out"
}

write_test_probe() {
  local out="$1" mode="$2"
  {
    echo 'package depguardtestprobe'
    echo
    echo 'import ('
    probe_imports "$mode"
    echo $'\t"testing"'
    echo ')'
    echo
    echo 'func TestProbe(t *testing.T) { _ = t }'
  } >"$out"
}

run_lint() {
  local mod="$1" out="$2" rc=0
  # max-issues flags defeat golangci-lint's default truncation (50 per linter,
  # 3 per identical message) so every expected violation is actually reported.
  (cd "$mod" && golangci-lint run --max-issues-per-linter=0 --max-same-issues=0 ./...) \
    >"$out" 2>&1 || rc=$?
  return "$rc"
}

echo
echo "== polarity 1: violating tree must FAIL depguard on every guarded dir x denied pkg =="
BAD="$WORK/bad"
scaffold "$BAD" violating
BAD_OUT="$WORK/bad.out"
if run_lint "$BAD" "$BAD_OUT"; then
  cat "$BAD_OUT" >&2
  fail "golangci-lint exited 0 on a deliberately violating tree — the depguard rules are not firing"
fi

grep -q 'depguard' "$BAD_OUT" || {
  cat "$BAD_OUT" >&2
  fail "golangci-lint failed but reported no depguard issue — the failure was NOT the boundary rule (config or typecheck error?)"
}

expected=0
for dir in "${GUARDED[@]}"; do
  for denied in "${DENIED[@]}"; do
    imp="$(probe_import "$denied")"
    grep -Fq "$dir/depguardprobe/probe.go" "$BAD_OUT" ||
      fail "depguard never reported guarded dir $dir — it is in .golangci.yml files: but the glob does not match"
    grep -F "$dir/depguardprobe/probe.go" "$BAD_OUT" |
      grep -Fq "import '$imp' is not allowed from list 'pure-tree'" ||
      fail "depguard did not deny '$imp' in guarded dir $dir (D-123 deny rule for '$denied' is not effective)"
    expected=$((expected + 1))
  done
done

# Test files are in scope too (the config says so; here is the proof).
TEST_PROBE_IMPORT="$(probe_import "${DENIED[0]}")"
grep -F "${GUARDED[0]}/depguardtestprobe/probe_test.go" "$BAD_OUT" |
  grep -Fq "import '$TEST_PROBE_IMPORT' is not allowed from list 'pure-tree'" ||
  fail "depguard did not scan _test.go files in ${GUARDED[0]} — the files: globs exclude tests"
expected=$((expected + ${#DENIED[@]}))

actual="$(grep -Fc '(depguard)' "$BAD_OUT" || true)"
[[ "$actual" == "$expected" ]] ||
  fail "expected exactly $expected depguard issues, got $actual (unguarded control leaked, or extra rules fired)"
if grep -F '(depguard)' "$BAD_OUT" | grep -Fq 'internal/depguardunguarded/probe.go'; then
  grep -F 'internal/depguardunguarded/probe.go' "$BAD_OUT" >&2
  fail "depguard reported a package OUTSIDE the guarded tree — the files: scoping is broken"
fi
echo "OK: $expected depguard violations reported (${#GUARDED[@]} guarded dirs + 1 _test.go probe, x ${#DENIED[@]} denied packages); unguarded control not reported"

echo
echo "== polarity 2: same tree without the forbidden imports must be depguard-clean =="
GOOD="$WORK/good"
scaffold "$GOOD" clean
GOOD_OUT="$WORK/good.out"
if ! run_lint "$GOOD" "$GOOD_OUT"; then
  cat "$GOOD_OUT" >&2
  fail "golangci-lint failed on the clean probe tree — false positive, the gate would burn trust"
fi
if grep -q 'depguard' "$GOOD_OUT"; then
  cat "$GOOD_OUT" >&2
  fail "depguard reported an issue on a clean tree"
fi
echo "OK: clean probe tree green (harness is capable of reporting green)"

echo
echo "== polarity 4: cmd/assent must not import either adapter (E10-S02 / ADR-0021 §1) =="

# The cmd-adapter-boundary rule gets its own polarity pair: a violating
# cmd/assent probe importing both adapters must be reported NAMING the rule,
# and the same tree without them must be clean.
CMD_BAD="$WORK/cmdbad"
mkdir -p "$CMD_BAD/cmd/assent"
printf 'module %s\n\ngo %s\n' "$MODULE" "$GO_DIRECTIVE" >"$CMD_BAD/go.mod"
cp "$CONFIG" "$CMD_BAD/.golangci.yml"
for denied in "${CMD_DENIED[@]}"; do
  imp="$(probe_import "$denied")"
  mkdir -p "$CMD_BAD/${imp#"$MODULE"/}"
  printf 'package depguardport\n' >"$CMD_BAD/${imp#"$MODULE"/}/depguardport.go"
done
{
  echo '// Command probe is a throwaway cmd/assent boundary probe (E10-S02).'
  echo 'package main'
  echo
  echo 'import ('
  for denied in "${CMD_DENIED[@]}"; do
    printf '\t_ "%s/depguardport"\n' "$denied"
  done
  echo ')'
  echo
  echo 'func main() {}'
} >"$CMD_BAD/cmd/assent/probe.go"

CMD_OUT="$WORK/cmdbad.out"
if run_lint "$CMD_BAD" "$CMD_OUT"; then
  cat "$CMD_OUT" >&2
  fail "golangci-lint exited 0 on a cmd/assent tree importing both adapters — the cmd-adapter-boundary rule is not firing"
fi
for denied in "${CMD_DENIED[@]}"; do
  grep -Fq "import '$denied/depguardport' is not allowed from list 'cmd-adapter-boundary'" "$CMD_OUT" ||
    fail "depguard did not deny $denied in cmd/assent naming the cmd-adapter-boundary rule (E10-S02)"
done
echo "OK: cmd/assent adapter imports denied at both polarities (rule cmd-adapter-boundary named)"

echo
echo "== polarity 3: the real repository tree must be lint-clean =="
golangci-lint run ./... >"$WORK/repo.out" 2>&1 || {
  cat "$WORK/repo.out" >&2
  fail "golangci-lint is not clean at HEAD"
}
echo "OK: golangci-lint run ./... clean at HEAD"

echo
echo "== E10-S02 (REQ-E10-S02-02/04/07): cmd/assent names ZERO adapter symbols and declares no forge-method interface =="

# REQ-E10-S02-02: the construction allowlist this gate used to enforce
# (New/WithSleeper/SyntheticDigest) is EMPTIED by the neutral factory —
# cmd/assent references ZERO symbols from EITHER adapter, because it imports
# neither (polarity 4 proves the import boundary; this section proves the
# symbol-level shape depguard cannot express, replacing REQ-AUD-S15-01's
# allowlist).

# scan_adapter_symbols <dir> prints "<relpath>:<line>:<pkg>.<Symbol>" for every
# gitlab./github. exported reference in Go CODE under <dir>.
#
# Whole-line comments are BLANKED first (blanked, not deleted, so the reported
# line numbers stay true to the file). Erring toward scanning MORE text is the
# fail-closed direction. Block comments are refused outright by the guard below.
scan_adapter_symbols() {
  local dir="$1" f rel
  while IFS= read -r f; do
    rel="${f#"$dir"/}"
    sed -E 's|^[[:space:]]*//.*||' "$f" |
      { grep -nEo '(gitlab|github)\.[A-Z][A-Za-z0-9_]*' || true; } |
      while IFS=: read -r ln sym; do
        printf '%s:%s:%s\n' "$rel" "$ln" "$sym"
      done
  done < <(find "$dir" -name '*.go' | LC_ALL=C sort)
}

# aliased_adapter_imports <dir> prints "<relpath>:<line>:<text>" for every ALIASED
# import of either adapter (`gl "…/gitlab"`, `_ "…/gitlab"`, `. "…/gitlab"`).
aliased_adapter_imports() {
  local dir="$1" f rel hit
  while IFS= read -r f; do
    rel="${f#"$dir"/}"
    sed -E 's|^([[:space:]]*)import[[:space:]]+|\1|' "$f" |
      { grep -nE '^[[:space:]]*[A-Za-z_.][A-Za-z0-9_]*[[:space:]]+"'"$MODULE"'/internal/forge/(gitlab|github)"' || true; } |
      while IFS= read -r hit; do
        printf '%s:%s\n' "$rel" "$hit"
      done
  done < <(find "$dir" -name '*.go' | LC_ALL=C sort)
}

# forge_interface_methods <dir> prints "<relpath>:<interface>:<method>" for every method
# of every interface declared in <dir> (REQ-E10-S02-07: no interface declared in
# cmd/assent may carry a forge read or write method — forge.RunPort lives in
# internal/forge and cmd/assent references the named type only). Portable awk
# (no gawk-only match-with-array), because this gate runs on BSD and Linux awk.
FORGE_PORT_METHODS='(Approve|MergeCAS|CreateThread|ResolveThread|ListBotThreads|ListBotNotes|UpsertComment|CurrentHeads|GetMR|FileAtRef|FileAtBase|FileAtHead|Snapshot|Resolve|Identity)'
forge_interface_methods() {
  local dir="$1" f
  for f in "$dir"/*.go; do
    awk -v file="$f" -v methods="$FORGE_PORT_METHODS" '
      /^type [A-Za-z0-9_]+ interface[ {]*$/ {
        # The interface name is the second field of "type NAME interface".
        n = split($0, parts, /[ \t]+/); iface = parts[2]; iniface = 1; next
      }
      iniface && /^\}/ { iniface = 0; next }
      iniface && $0 ~ /^[ \t]*[A-Z][A-Za-z0-9_]*\(/ {
        method = $0
        sub(/^[[:space:]]+/, "", method); sub(/\(.*/, "", method)
        print FILENAME ":" iface ":" method
      }
    ' "$f"
  done | grep -E ":(${FORGE_PORT_METHODS})$" || true
}

CMD_DIR="$ROOT/cmd/assent"
[[ -d "$CMD_DIR" ]] || fail "missing $CMD_DIR"

# The comment-stripping above assumes line comments only.
if grep -rn '^[[:space:]]*/\*' "$CMD_DIR" --include='*.go'; then
  fail "cmd/assent now uses BLOCK comments — scan_adapter_symbols only strips line comments; extend it before this gate can be trusted"
fi

# --- polarity A: a violating COPY of cmd/assent must be reported --------------
# The copy lives in $WORK; the repository working tree is never written to.
# REQ-E10-S02-04: this is the scanner's positive control, REPLACED never
# deleted — the real tree legitimately contains zero adapter symbols now that
# the factory constructs both, so a scanner whose silence could not be
# distinguished from a broken scanner would prove nothing.
PROBE="$WORK/e10"
mkdir -p "$PROBE"
cp "$CMD_DIR"/*.go "$PROBE/"
cat >"$PROBE/e10_probe_gitlab.go" <<'PROBEEOF'
package main

import "github.com/PlatformRelay/assent/internal/forge/gitlab"

func e10ProbeGitlab() (gitlab.MRInfo, error) { return gitlab.MRInfo{}, gitlab.ErrNotFound }
PROBEEOF

cat >"$PROBE/e10_probe_github.go" <<'PROBEEOF'
package main

import "github.com/PlatformRelay/assent/internal/forge/github"

func e10ProbeGitHub() error { return github.ErrNotFound }
PROBEEOF

# The alias evasion: under `gl`, the symbol scanner is blind by construction, so
# only the import-form check can catch this one.
cat >"$PROBE/e10_probe_alias.go" <<'PROBEEOF'
package main

import gl "github.com/PlatformRelay/assent/internal/forge/gitlab"

func e10ProbeAliased() gl.MRInfo { return gl.MRInfo{} }
PROBEEOF

PROBE_VIOL="$WORK/e10-probe-violations.txt"
scan_adapter_symbols "$PROBE" >"$WORK/e10-probe-scan.txt"
for expected in gitlab.MRInfo gitlab.ErrNotFound github.ErrNotFound; do
  grep -Fq "$expected" "$WORK/e10-probe-scan.txt" ||
    fail "the E10-S02 scanner did NOT report $expected in a deliberately violating tree — the gate cannot fire"
done
grep -Fq 'e10_probe_gitlab.go' "$WORK/e10-probe-scan.txt" ||
  fail "the E10-S02 scanner reported violations but never named the violating file"
echo "OK: violating copy reported by the adapter-symbol scanner"

PROBE_ALIAS="$WORK/e10-probe-alias.txt"
aliased_adapter_imports "$PROBE" >"$PROBE_ALIAS"
grep -Fq 'e10_probe_alias.go' "$PROBE_ALIAS" ||
  fail "the alias check did NOT report an aliased adapter import in a deliberately violating tree — an alias would make the symbol scan sweep an empty set and pass open"
if grep -Fq 'e10_probe_gitlab.go:' "$PROBE_ALIAS"; then
  cat "$PROBE_ALIAS" >&2
  fail "the alias check reported an UNALIASED import — it cannot distinguish the two forms"
fi
echo "OK: violating copy's aliased adapter import reported; its unaliased import not reported"

# --- REQ-E10-S02-07: the interface invariant, at both polarities --------------
# A hand-rolled interface carrying forge read/write methods must be reported;
# cmd/assent's legitimate non-forge seams (checkout.go's localCheckout) must NOT.
IFACE_PROBE="$WORK/e10-iface"
mkdir -p "$IFACE_PROBE"
cp "$CMD_DIR"/*.go "$IFACE_PROBE/"
cat >"$IFACE_PROBE/e10_iface_probe.go" <<'IFACEEOF'
package main

type privateForgePort interface {
	FileAtRef(project, path, ref string) ([]byte, error)
}

var _ privateForgePort = privateForgePort(nil)
IFACEEOF

IFACE_VIOL="$WORK/e10-iface-violations.txt"
forge_interface_methods "$IFACE_PROBE" >"$IFACE_VIOL"
grep -Fq 'e10_iface_probe.go:privateForgePort' "$IFACE_VIOL" ||
  fail "the interface-invariant guard did NOT report a hand-rolled forge-method interface in a violating copy (REQ-E10-S02-07)"

IFACE_REAL="$WORK/e10-iface-real.txt"
forge_interface_methods "$CMD_DIR" >"$IFACE_REAL"
if [[ -s "$IFACE_REAL" ]]; then
  cat "$IFACE_REAL" >&2
  fail "cmd/assent declares an interface carrying forge read/write methods — only forge.RunPort (declared in internal/forge) may"
fi
echo "OK: interface invariant enforced at both polarities; local-tree seams stay green"

# --- polarity B: the real cmd/assent must be clean ----------------------------
REAL_SCAN="$WORK/e10-real.txt"
scan_adapter_symbols "$CMD_DIR" >"$REAL_SCAN"
if [[ -s "$REAL_SCAN" ]]; then
  cat "$REAL_SCAN" >&2
  fail "cmd/assent names a concrete adapter symbol — E10-S02: the neutral factory is the only adapter importer; the port speaks forge.* types"
fi

REAL_ALIAS="$WORK/e10-real-alias.txt"
aliased_adapter_imports "$CMD_DIR" >"$REAL_ALIAS"
if [[ -s "$REAL_ALIAS" ]]; then
  cat "$REAL_ALIAS" >&2
  fail "cmd/assent imports an adapter under an ALIAS — the symbol scan cannot see through it. Import nothing adapter-named at all."
fi
echo "OK: cmd/assent names zero adapter symbols from either adapter"

echo
echo "PASS: D-123 depguard boundary rules proven at both polarities (REQ-AUD-S07-01)"
echo "PASS: E10-S02 adapter boundary + interface invariant proven at both polarities (REQ-E10-S02-02/04/07)"
echo "OK: cmd/assent names zero adapter symbols in production or test code (the neutral factory is the only importer)"
