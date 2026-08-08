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

# Guarded directories: the depguard `files:` globs, which are the only
# `- "**/<path>/**"` lines in the config.
GUARDED=()
while IFS= read -r line; do
  GUARDED+=("$line")
done < <(sed -nE 's|^[[:space:]]*- "\*\*/(.+)/\*\*"[[:space:]]*$|\1|p' "$CONFIG")

# Denied packages: the depguard `- pkg: "<path>"` lines.
DENIED=()
while IFS= read -r line; do
  DENIED+=("$line")
done < <(sed -nE 's|^[[:space:]]*- pkg: "(.+)"[[:space:]]*$|\1|p' "$CONFIG")

# --- positive controls on the extraction itself ------------------------------
# Without these, a sed pattern that matches nothing would leave both arrays
# empty and every assertion below would vacuously "pass".
(( ${#GUARDED[@]} >= 8 )) ||
  fail "extracted only ${#GUARDED[@]} guarded paths from $CONFIG — the files: glob pattern stopped matching"
(( ${#DENIED[@]} >= 4 )) ||
  fail "extracted only ${#DENIED[@]} denied packages from $CONFIG — the deny pkg: pattern stopped matching"

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

echo "OK: ${#GUARDED[@]} guarded paths, ${#DENIED[@]} denied packages extracted"
printf '     guarded: %s\n' "${GUARDED[@]}"
printf '     denied:  %s\n' "${DENIED[@]}"

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
echo "== polarity 3: the real repository tree must be lint-clean =="
golangci-lint run ./... >"$WORK/repo.out" 2>&1 || {
  cat "$WORK/repo.out" >&2
  fail "golangci-lint is not clean at HEAD"
}
echo "OK: golangci-lint run ./... clean at HEAD"

echo
echo "== ARCH-02 (REQ-AUD-S15-01): cmd/assent names only CONSTRUCTION symbols from the gitlab adapter =="

# The only gitlab.<Exported> symbols cmd/assent may name. `New`/`WithSleeper`
# are construction; `SyntheticDigest` is the documented E10 residue (the merge
# -digest SCHEME is adapter-owned — E10 collapses both call-sites onto
# Snapshot.Heads.MergeResultDigest, see
# docs/planning/design-notes/e10-forge-port-lift.md step 4). Anything else —
# notably MRInfo and ErrNotFound, the two types AUD-S15 lifted onto the port —
# is a regression. Tighten this list; never widen it without an ADR.
ALLOWED_GITLAB_SYMBOLS=(New WithSleeper SyntheticDigest)

# scan_gitlab_symbols <dir> prints "<relpath>:<line>:<symbol>" for every
# gitlab.<Exported> reference in Go CODE under <dir>.
#
# Whole-line comments are BLANKED first (blanked, not deleted, so the reported
# line numbers stay true to the file): the tree carries prose references to
# `*gitlab.Client` that are not call-sites. Only WHOLE-line comments go — never
# a trailing `// …` — because stripping from the first `//` would also truncate
# a line containing a `"http://…"` literal and could hide a real reference after
# it. Erring toward scanning MORE text is the fail-closed direction. Block
# comments are refused outright by the guard below.
scan_gitlab_symbols() {
  local dir="$1" f rel
  while IFS= read -r f; do
    rel="${f#"$dir"/}"
    # `|| true` on the grep: a file with none of the pattern is the NORMAL case
    # and must not trip `set -o pipefail`.
    sed -E 's|^[[:space:]]*//.*||' "$f" |
      { grep -nEo 'gitlab\.[A-Z][A-Za-z0-9_]*' || true; } |
      while IFS=: read -r ln sym; do
        printf '%s:%s:%s\n' "$rel" "$ln" "${sym#gitlab.}"
      done
  done < <(find "$dir" -name '*.go' | sort)
}

# gitlab_symbol_violations reads scan output on stdin, prints the not-allowed ones.
gitlab_symbol_violations() {
  local file ln sym
  while IFS=: read -r file ln sym; do
    contains "$sym" "${ALLOWED_GITLAB_SYMBOLS[@]}" || printf '%s:%s: gitlab.%s\n' "$file" "$ln" "$sym"
  done
}

# aliased_gitlab_imports <dir> prints "<relpath>:<line>:<text>" for every ALIASED
# import of the adapter (`gl "…/gitlab"`, `_ "…/gitlab"`, `. "…/gitlab"`).
#
# This closes the one way to make scan_gitlab_symbols sweep an empty set: under
# an alias, every reference reads `gl.MRInfo`, the `gitlab\.` pattern matches
# nothing, and the allowlist above would pass OPEN. Requiring the bare import
# path is what makes the symbol scan complete rather than merely true.
aliased_gitlab_imports() {
  local dir="$1" f rel hit
  while IFS= read -r f; do
    rel="${f#"$dir"/}"
    # Strip a leading `import` keyword first (sed preserves the line count, so
    # grep -n still reports true line numbers). That reduces the single-line
    # form `import gl "…"` to the grouped form `gl "…"`, so ONE rule covers
    # both: after the strip, any token before the quoted path is an alias, and
    # a plain `import "…"` has none.
    sed -E 's|^([[:space:]]*)import[[:space:]]+|\1|' "$f" |
      { grep -nE '^[[:space:]]*[A-Za-z_.][A-Za-z0-9_]*[[:space:]]+"'"$MODULE"'/internal/forge/gitlab"' || true; } |
      while IFS= read -r hit; do
        printf '%s:%s\n' "$rel" "$hit"
      done
  done < <(find "$dir" -name '*.go' | sort)
}

CMD_DIR="$ROOT/cmd/assent"
[[ -d "$CMD_DIR" ]] || fail "missing $CMD_DIR"

# The comment-stripping above assumes line comments only.
if grep -rn '^[[:space:]]*/\*' "$CMD_DIR" --include='*.go'; then
  fail "cmd/assent now uses BLOCK comments — scan_gitlab_symbols only strips line comments; extend it before this gate can be trusted"
fi

# --- positive control: the scanner must find the real, allowed call-sites -----
# A scanner whose regex silently stopped matching would report zero violations
# on any tree, violating or not.
REAL_SCAN="$WORK/arch02-real.txt"
scan_gitlab_symbols "$CMD_DIR" >"$REAL_SCAN"
REAL_SCAN_N="$(wc -l <"$REAL_SCAN" | tr -d ' ')"
(( REAL_SCAN_N >= 4 )) ||
  fail "scan_gitlab_symbols found only $REAL_SCAN_N gitlab.<Exported> references in cmd/assent — the scanner stopped matching real code, so its silence would prove nothing"
for expected in New SyntheticDigest; do
  cut -d: -f3 "$REAL_SCAN" | grep -qx "$expected" ||
    fail "scan_gitlab_symbols did not see the known gitlab.$expected call-site in cmd/assent — the scanner is broken"
done
echo "OK: scanner sees $REAL_SCAN_N real gitlab.<Exported> references in cmd/assent"

# --- polarity A: a violating COPY of cmd/assent must be reported --------------
# The copy lives in $WORK; the repository working tree is never written to.
PROBE="$WORK/arch02"
mkdir -p "$PROBE"
cp "$CMD_DIR"/*.go "$PROBE/"
cat >"$PROBE/arch02_probe.go" <<'PROBEEOF'
package main

import "github.com/PlatformRelay/assent/internal/forge/gitlab"

func arch02Probe() (gitlab.MRInfo, error) { return gitlab.MRInfo{}, gitlab.ErrNotFound }
PROBEEOF

# A second probe file for the alias evasion: under `gl`, the symbol scanner is
# blind by construction, so only the import-form check can catch this one.
cat >"$PROBE/arch02_probe_alias.go" <<'PROBEEOF'
package main

import gl "github.com/PlatformRelay/assent/internal/forge/gitlab"

func arch02ProbeAliased() gl.MRInfo { return gl.MRInfo{} }
PROBEEOF

PROBE_VIOL="$WORK/arch02-probe-violations.txt"
scan_gitlab_symbols "$PROBE" | gitlab_symbol_violations >"$PROBE_VIOL"
for expected in MRInfo ErrNotFound; do
  grep -Fq "gitlab.$expected" "$PROBE_VIOL" ||
    fail "the ARCH-02 scanner did NOT report gitlab.$expected in a deliberately violating tree — the gate cannot fire"
done
grep -Fq 'arch02_probe.go' "$PROBE_VIOL" ||
  fail "the ARCH-02 scanner reported violations but never named the violating file"
echo "OK: violating copy reported $(wc -l <"$PROBE_VIOL" | tr -d ' ') disallowed gitlab.<Exported> references"

PROBE_ALIAS="$WORK/arch02-probe-alias.txt"
aliased_gitlab_imports "$PROBE" >"$PROBE_ALIAS"
grep -Fq 'arch02_probe_alias.go' "$PROBE_ALIAS" ||
  fail "the ARCH-02 alias check did NOT report an aliased gitlab import in a deliberately violating tree — an alias would make the symbol scan sweep an empty set and pass open"
# The unaliased probe must NOT be reported: otherwise the check flags every
# import and its silence on the real tree would mean nothing.
if grep -Fq 'arch02_probe.go:' "$PROBE_ALIAS"; then
  cat "$PROBE_ALIAS" >&2
  fail "the ARCH-02 alias check reported an UNALIASED import — it cannot distinguish the two forms"
fi
echo "OK: violating copy's aliased gitlab import reported; its unaliased import not reported"

# --- polarity B: the real cmd/assent must be clean ----------------------------
REAL_VIOL="$WORK/arch02-real-violations.txt"
gitlab_symbol_violations <"$REAL_SCAN" >"$REAL_VIOL"
if [[ -s "$REAL_VIOL" ]]; then
  cat "$REAL_VIOL" >&2
  fail "cmd/assent names gitlab adapter symbols outside the construction allowlist (${ALLOWED_GITLAB_SYMBOLS[*]}) — ARCH-02: the orchestration read port must speak forge.* types"
fi

REAL_ALIAS="$WORK/arch02-real-alias.txt"
aliased_gitlab_imports "$CMD_DIR" >"$REAL_ALIAS"
if [[ -s "$REAL_ALIAS" ]]; then
  cat "$REAL_ALIAS" >&2
  fail "cmd/assent imports the gitlab adapter under an ALIAS — the symbol allowlist above cannot see through it. Import the bare path."
fi
echo "OK: cmd/assent names only ${ALLOWED_GITLAB_SYMBOLS[*]} from the gitlab adapter, and imports it unaliased"

echo
echo "PASS: D-123 depguard boundary rules proven at both polarities (REQ-AUD-S07-01)"
echo "PASS: ARCH-02 cmd/assent adapter-symbol allowlist proven at both polarities (REQ-AUD-S15-01)"
