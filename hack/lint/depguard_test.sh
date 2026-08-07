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
echo "PASS: D-123 depguard boundary rules proven at both polarities (REQ-AUD-S07-01)"
