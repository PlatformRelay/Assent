#!/usr/bin/env bash
# REQ-AUD-S06-01 (DOC-07) — the README quick-start is EXECUTED, not eyeballed.
#
# Extracts every fenced ```bash block from README.md's "## Quick start" section and
# runs each `assent …` line verbatim against a freshly built binary, in a throwaway
# copy of the sample repo the README itself names. A quick-start command that exits
# non-zero fails this gate — that is the whole point: `assent lint .assent/` shipped
# in the README for the entire pre-release period and exits 2 for every reader.
#
# Two command families are DELIBERATELY skipped, loudly (never silently dropped):
#   go install …  — needs the network and the module proxy; and what it produces is
#                   pinned separately by the DOC-11 caveat pin in truthlag_pins_test.sh.
#   task …        — this script RUNS inside `task check` (via `task docs-gates`, D-124),
#                   so invoking `task check` from here would recurse without terminating.
#                   The skip is load-bearing, not a convenience: if this case ever stops
#                   matching, `task check` calls itself forever.
# Every skip is printed with its reason and counted, so deleting the executed lines
# cannot leave the script trivially green (see the "no assent command" fail below).
#
# WIRED (D-124/D-125): `task docs-gates` runs this and truthlag_pins_test.sh, and
# `task check` runs `docs-gates`. A README edit that reopens DOC-07 reds the gate every
# developer runs before every commit.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

BIN="$WORK/bin/assent"
mkdir -p "$WORK/bin"
echo "building $BIN"
go build -o "$BIN" ./cmd/assent

# The fixture is whatever the README names — single source of truth, so retitling the
# sample repo in the README cannot leave this script testing a stale path.
FIXTURE_REL="$(grep -o 'examples/packs/[a-z0-9-]*' README.md | head -1 || true)"
if [[ -z "$FIXTURE_REL" ]]; then
  echo "FAIL: README.md quick-start names no examples/packs/<repo> fixture to run against" >&2
  exit 1
fi
if [[ ! -d "$ROOT/$FIXTURE_REL/.assent" ]]; then
  echo "FAIL: README names fixture $FIXTURE_REL, which has no .assent/ tree" >&2
  exit 1
fi
FIXTURE="$WORK/repo"
mkdir -p "$FIXTURE"
cp -R "$ROOT/$FIXTURE_REL/." "$FIXTURE/"
echo "fixture: $FIXTURE_REL -> $FIXTURE"

# Quick-start bash blocks: from "## Quick start" to the next "## " heading, keeping
# only the contents of ```bash fences.
BLOCKS="$WORK/quickstart.sh"
awk '
  /^## Quick start/ { inqs = 1; next }
  inqs && /^## / { inqs = 0 }
  inqs && /^```bash/ { infence = 1; next }
  inqs && infence && /^```/ { infence = 0; next }
  inqs && infence { print }
' README.md > "$BLOCKS"

if [[ ! -s "$BLOCKS" ]]; then
  echo "FAIL: no \`\`\`bash blocks found under README.md '## Quick start'" >&2
  exit 1
fi

ran=0
skipped=0
failed=0
while IFS= read -r line; do
  # Strip trailing inline comments (`task check   # fmt + vet …`) and whitespace.
  cmd="${line%%#*}"
  cmd="$(printf '%s' "$cmd" | sed -e 's/[[:space:]]*$//' -e 's/^[[:space:]]*//')"
  [[ -z "$cmd" ]] && continue

  case "$cmd" in
    "go install"*)
      echo "SKIP  $cmd  (network + module proxy; its output is pinned by the DOC-11 caveat pin)"
      skipped=$((skipped + 1))
      continue
      ;;
    task*)
      echo "SKIP  $cmd  (this script runs inside \`task check\` via \`task docs-gates\`; invoking it here would recurse — D-124)"
      skipped=$((skipped + 1))
      continue
      ;;
    assent*)
      run="$BIN${cmd#assent}"
      ;;
    *)
      echo "FAIL: quick-start line is neither an assent command nor a known skip: $cmd" >&2
      failed=$((failed + 1))
      continue
      ;;
  esac

  echo "RUN   ($FIXTURE_REL) $cmd"
  if (cd "$FIXTURE" && eval "$run"); then
    ran=$((ran + 1))
  else
    rc=$?
    echo "FAIL: README quick-start command exited $rc: $cmd" >&2
    failed=$((failed + 1))
  fi
done < "$BLOCKS"

if [[ "$ran" -eq 0 ]]; then
  echo "FAIL: no assent command in the README quick-start was executed ($skipped skipped) — the gate would be vacuous" >&2
  exit 1
fi
if [[ "$failed" -ne 0 ]]; then
  echo "FAIL: $failed README quick-start command(s) failed" >&2
  exit 1
fi

echo "OK: $ran README quick-start command(s) green, $skipped skipped with a stated reason"
