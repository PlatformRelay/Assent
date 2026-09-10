#!/usr/bin/env bash
# locale_pin_test.sh — LOCALE-F01 / D-179.
#
# Every `sort` / `comm` / `join` / `uniq` in a hack/** script runs under a PINNED
# collation, spelled `LC_ALL=C <command>` at the call site. This gate enforces that,
# proves the scan can fail, and reproduces on the running host the defect the pin
# exists for.
#
# WHY THIS EXISTS. `task check` is the gate AGENTS.md hard rule 4 puts in front of
# every commit, and on a developer machine with `LANG=en_US.UTF-8` it was RED on a
# clean `main` (e556a45, measured 2026-09-10): the `release-changelog-gate-test`
# stage died in §7a with
#
#     comm: file 1 is not in sorted order
#
# while `LC_ALL=C` made the same tree green. CI never saw it: GitHub's runners use
# C.UTF-8, whose collation is plain code-point order. The mechanism, measured on
# Ubuntu 26.04: its coreutils are uutils 0.8.0, whose `sort` collates by
# LC_COLLATE (en_US ignores punctuation and case at the first level, so
# `lane/e10-…` sorts before `lane-e9-…`) while its `comm` checks order BYTEWISE —
# the two halves of one pipeline disagreeing about what "sorted" means. GNU
# coreutils honour LC_COLLATE in both, so the same pipeline is consistent there —
# but GNU has the mirror-image hazard, already present in this tree:
# hack/docs/truthlag_pins_test.sh fed `LC_ALL=C sort` output into an UNPINNED
# `comm`, which under GNU + en_US checks C-ordered input against en_US rules. One
# pin at each call, on every call, is the only spelling that is right on both.
#
# WHY PER-CALL `LC_ALL=C`, AND NOT ONE `export LC_ALL=C` PER SCRIPT. An export
# leaks into every child the script starts. hack/audit/exitgate_test.sh runs a
# whole `task check`; exporting C there would make every stage beneath it run
# under C and hide a missing pin in any of them — the gate would neutralise the
# very condition it needs to observe. An export also moves LC_CTYPE to ASCII for
# every other command in the script (byte-wise `.`, `${#v}`, bracket classes),
# which is a behaviour change nobody asked for. The per-call form pins exactly
# the comparisons and nothing else, and was already this repo's majority spelling
# (example_format_inventory_test.sh, ex_exitgate_test.sh, seed-sample-repo.sh).
#
# WHY `LC_ALL` AND NOT `LC_COLLATE`. LC_ALL overrides every LC_* variable, so a
# developer who exports `LC_ALL=en_US.UTF-8` defeats an `LC_COLLATE=C` pin.
# WHY `C` AND NOT `C.UTF-8`. macOS has no C.UTF-8 locale; an unknown locale falls
# back to C silently on some platforms and not on others. Plain `C` exists
# everywhere and means byte order everywhere.
#
# WHAT THIS GATE ASSERTS
#   (1) every command-position sort/comm/join/uniq in hack/**/*.sh carries an
#       `LC_ALL=C` assignment prefix;
#   (2) the scan still SEES the known population — the comm sites in
#       changelog_gate_test.sh and truthlag_pins_test.sh and a `find … | sort`
#       site — so a broken pattern that matches nothing reds instead of passing;
#   (3) negative controls: real copies of those scripts with the pin stripped ARE
#       flagged, and a fixture of every recognised call shape is flagged unpinned,
#       clean pinned, and a fixture of look-alikes (comments, a variable named
#       `uniq`, the word in a string) is not flagged at all;
#   (4) behavioural control: under a UTF-8 locale whose collation is NOT byte
#       order, the pipeline shape that broke — sort then comm, over the real
#       `git log --merges` subjects §7a compares — misbehaves on this host with
#       one or both pins removed, and is correct with both. SKIPS LOUDLY where no
#       such locale exists; on CI (GITHUB_ACTIONS set) that skip is a FAILURE,
#       because a runner without the locale means no machine anywhere is running
#       the control.
#
# WHAT IT CANNOT SEE — stated so nobody has to discover it:
#   * the command behind a variable (`"$SORT"`), `eval`, or a function wrapper;
#   * anything on a line after a ` #` (treated as a comment — a `'#'` inside a
#     quoted string ends the scan of that line early);
#   * heredoc bodies are scanned as code: a heredoc that PRINTS `… | sort` is
#     flagged, and the fix is to pin it anyway or build the word from a variable;
#   * `jq '… | sort'` would be flagged although jq sorts by code point — pin or
#     reword it; there is none today;
#   * ordering that does not go through these four commands: glob expansion order
#     and `[[ a < b ]]` both follow LC_COLLATE. Measured 2026-09-10: no check stage
#     compares a glob's ORDER against anything (every glob loop is per-item) and no
#     script uses `[[ < ]]`/`[[ > ]]` on strings. That is a measurement, not a gate.
#
# THIS FILE IS BASH-3.2-CLEAN (no associative arrays, no mapfile), so
# hack/lint/bash_version_guard_test.sh has nothing to require of it.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT" || exit 1

WORK="$(mktemp -d)"
FINISHED=0

# An early death under `set -u` leaves the exit status at 0 (the D-154 lesson).
# Nothing may read this script as green unless it reached its last line.
on_exit() {
  rc=$?
  rm -rf "$WORK"
  if [ "$FINISHED" -eq 0 ] && [ "$rc" -eq 0 ]; then
    echo "FAIL: locale_pin_test.sh terminated before its final line yet exited 0 — refusing to report success" >&2
    exit 1
  fi
  exit "$rc"
}
trap on_exit EXIT

fails=0
skips=0
pass() { echo "PASS  $1"; }
fail() {
  echo "FAIL  $1" >&2
  fails=$((fails + 1))
}

# --- the scan ----------------------------------------------------------------
# A line is first stripped of a trailing ` # …` comment (and skipped entirely when
# it IS a comment), then every shell separator is padded with spaces so that
# `a|sort|uniq` and `$(comm …)` read like `a | sort | uniq` and `$ ( comm …`. A
# command position is then: the start of the line, a separator token, or a
# keyword that takes a command after it. Any number of `NAME=value` assignment
# prefixes may sit between that position and the command word.
# Assembled from a separator variable: a literal `sort|comm` here would be padded
# by the scan into `sort | comm` and read as a call in this very file.
_alt='|'
ORDER_CMDS="sort${_alt}comm${_alt}join${_alt}uniq"
OCC_RE='(^[[:space:]]*|[|;&({!][[:space:]]+|(^|[[:space:]])(if|then|do|else|elif|while|until|xargs|exec|env|time|command)[[:space:]]+)([A-Za-z_][A-Za-z0-9_]*=[^[:space:]]*[[:space:]]+)*('"$ORDER_CMDS"')([[:space:]]|$)'
PIN_RE='(^|[[:space:]])LC_ALL=C[[:space:]]'

# A deliberate unpinned call carries this marker on its line. Only THIS file may
# use it (section (1) reds on a marker anywhere else): section (4) has to run
# sort and comm under a chosen locale to show what the pin prevents.
EXEMPT_MARK='D-179-exempt:'
SELF_REL='hack/lint/locale_pin_test.sh'

# occurrences <file> — one `<line>:<unpinned|pinned|exempt>:<text>` row per
# command-position ordering command in <file>. After a hit, scanning resumes
# behind it with a sentinel byte prepended, so the rest of the line can never
# re-match the start-of-line alternative (`sort uniq` is one call, not two).
occurrences() {
  LC_ALL=C awk -v occ="$OCC_RE" -v pin="$PIN_RE" -v mark="$EXEMPT_MARK" '
    /^[[:space:]]*#/ { next }
    {
      exempt = index($0, mark) > 0
      line = $0
      sub(/[[:space:]]#.*$/, "", line)
      gsub(/[|;&(){}]/, " & ", line)
      while (match(line, occ)) {
        hit = substr(line, RSTART, RLENGTH)
        kind = (hit ~ pin) ? "pinned" : (exempt ? "exempt" : "unpinned")
        sub(/^[[:space:]]+/, "", hit)
        sub(/[[:space:]]+$/, "", hit)
        print NR ":" kind ":" hit
        line = "\001" substr(line, RSTART + RLENGTH)
      }
    }
  ' "$1"
}

# enumerate <root> — every hack/**/*.sh under <root>, relative, byte-ordered.
enumerate() {
  (cd "$1" && find hack -type f -name '*.sh' | LC_ALL=C sort)
}

# scan_root <root> — `<relpath>:<line>:<unpinned|pinned>:<text>` for every
# occurrence under <root>/hack.
scan_root() {
  enumerate "$1" | while IFS= read -r rel; do
    occurrences "$1/$rel" | sed "s|^|$rel:|"
  done
}

# violations <root> — the unpinned subset of scan_root, `<relpath>:<line>`.
violations() {
  scan_root "$1" | LC_ALL=C awk -F: '$3 == "unpinned" { print $1 ":" $2 }'
}

# --- (1) the real tree -------------------------------------------------------
echo "== (1) every sort/comm/join/uniq in hack/**/*.sh is LC_ALL=C-pinned =="
scan_root "$ROOT" >"$WORK/real.scan"
violations "$ROOT" >"$WORK/real.viol"
if [ -s "$WORK/real.viol" ]; then
  while IFS= read -r v; do
    fail "$v runs an ordering command under the caller's locale — prefix it with LC_ALL=C (D-179): $(sed -n "${v##*:}p" "$ROOT/${v%%:*}" | sed 's/^[[:space:]]*//')"
  done <"$WORK/real.viol"
else
  pass "$(wc -l <"$WORK/real.scan" | tr -d ' ') ordering call(s) across $(cut -d: -f1 "$WORK/real.scan" | LC_ALL=C sort -u | wc -l | tr -d ' ') script(s), every one pinned or (in this file only) exempt"
fi
# The exemption is this file's alone, and its count is pinned so that a new
# exempt line has to be added here on purpose: the locale probe, the two sorts
# and the comm of section (4).
LC_ALL=C awk -F: -v self="$SELF_REL" '$3 == "exempt" && $1 != self { print $1 ":" $2 }' "$WORK/real.scan" >"$WORK/exempt.foreign"
if [ -s "$WORK/exempt.foreign" ]; then
  fail "the $EXEMPT_MARK marker is used outside $SELF_REL: $(tr '\n' ' ' <"$WORK/exempt.foreign") — pin the call instead; the exemption exists only for this gate's own behavioural control"
else
  pass "no $EXEMPT_MARK marker outside $SELF_REL"
fi
EXEMPT_EXPECTED=4
n_exempt=$(LC_ALL=C awk -F: -v self="$SELF_REL" '$3 == "exempt" && $1 == self' "$WORK/real.scan" | wc -l | tr -d ' ')
if [ "$n_exempt" -eq "$EXEMPT_EXPECTED" ]; then
  pass "$SELF_REL carries exactly $EXEMPT_EXPECTED exempt call(s) (section (4)'s probe, two sorts, one comm)"
else
  fail "$SELF_REL carries $n_exempt exempt call(s), pinned at $EXEMPT_EXPECTED — change EXEMPT_EXPECTED deliberately, with a reason, or pin the new call"
fi

# --- (2) population controls -------------------------------------------------
# A pattern typo would make (1) pass by seeing nothing. These are the sites that
# motivated the gate; the scan must see each of them as an occurrence at all.
echo "== (2) the scan sees the known population =="
see() { # <relpath> <command> <why>
  if LC_ALL=C grep -qE "^$1:[0-9]+:[a-z]+:(.*[[:space:]])?$2\$" "$WORK/real.scan"; then
    pass "scan sees a '$2' call in $1 ($3)"
  else
    fail "scan sees NO '$2' call in $1 — either the call was removed (then update this control on purpose) or the occurrence pattern broke and (1) is vacuous"
  fi
}
see hack/release/changelog_gate_test.sh comm "the §7a site that redded task check under en_US.UTF-8"
see hack/docs/truthlag_pins_test.sh comm "LC_ALL=C sort feeding an unpinned comm — the GNU-coreutils shape"
see hack/lint/bash_version_guard_test.sh sort "find piped into it, inside a process substitution"
see hack/audit/exitgate_test.sh sort "a sort at the start of a continuation line"

# --- (3) negative controls ---------------------------------------------------
echo "== (3) the scan can fail =="

# (3a) real copies, pin stripped. The mutation is asserted to have LANDED before
# its red is believed.
strip_pin() { # <relpath> <command>
  mroot="$WORK/strip-$(basename "$1" .sh)-$2"
  mkdir -p "$mroot/$(dirname "$1")"
  sed "s/LC_ALL=C $2/$2/g" "$ROOT/$1" >"$mroot/$1"
  if cmp -s "$ROOT/$1" "$mroot/$1"; then
    fail "mutation did not land: no 'LC_ALL=C $2' to strip in $1"
    return
  fi
  violations "$mroot" >"$mroot.viol"
  if LC_ALL=C grep -q "^$1:" "$mroot.viol"; then
    pass "pin stripped from every '$2' in $1: flagged at $(tr '\n' ' ' <"$mroot.viol")"
  else
    fail "pin stripped from every '$2' in $1 and the scan did NOT flag it — (1) cannot fail for this file"
  fi
}
strip_pin hack/release/changelog_gate_test.sh comm
strip_pin hack/release/changelog_gate_test.sh sort
strip_pin hack/docs/truthlag_pins_test.sh comm
strip_pin hack/docs/example_format_inventory_test.sh sort

# (3b) every recognised call shape. The command words are spliced in from a
# placeholder so that THIS file carries no unpinned call for (1) to find.
fx="$WORK/fixture"
mkdir -p "$fx/hack"
cat >"$WORK/shapes.tmpl" <<'EOF'
@C@ a b
x | @C@ -u >out
x|@C@
y="$(@C@ -3 a b)"
while read -r l; do :; done < <(find . | @C@)
if ! @C@ -12 a b >/dev/null; then :; fi
a && @C@ a
FOO=bar @C@ a
LC_ALL=en_US.UTF-8 @C@ a
LC_COLLATE=C @C@ a
LC_ALL=C.UTF-8 @C@ a
env @C@ a
    @C@ -u
EOF
SHAPE_COUNT=$(wc -l <"$WORK/shapes.tmpl" | tr -d ' ')
for cmd in sort comm join uniq; do
  sed "s/@C@/$cmd/g" "$WORK/shapes.tmpl" >"$fx/hack/unpinned-$cmd.sh"
  # The pinned twin: LC_ALL=C inserted directly before the command word, after
  # any other assignment (a trailing LC_ALL=C is the one that takes effect).
  sed "s/@C@/LC_ALL=C $cmd/g" "$WORK/shapes.tmpl" >"$fx/hack/pinned-$cmd.sh"
done
violations "$fx" >"$WORK/fixture.viol"
for cmd in sort comm join uniq; do
  n=$(LC_ALL=C grep -c "^hack/unpinned-$cmd.sh:" "$WORK/fixture.viol" || true)
  if [ "$n" -eq "$SHAPE_COUNT" ]; then
    pass "all $SHAPE_COUNT unpinned '$cmd' shapes flagged (pipe, no-space pipe, \$( ), < <( ), if !, &&, other assignments, en_US/LC_COLLATE/C.UTF-8 non-pins, env, continuation)"
  else
    fail "only $n of $SHAPE_COUNT unpinned '$cmd' shapes flagged: $(LC_ALL=C grep "^hack/unpinned-$cmd.sh:" "$WORK/fixture.viol" | tr '\n' ' ')"
  fi
  if LC_ALL=C grep -q "^hack/pinned-$cmd.sh:" "$WORK/fixture.viol"; then
    fail "pinned '$cmd' twin flagged: $(LC_ALL=C grep "^hack/pinned-$cmd.sh:" "$WORK/fixture.viol" | tr '\n' ' ')"
  else
    pass "pinned '$cmd' twin: 0 of $SHAPE_COUNT flagged"
  fi
done

# (3c) look-alikes that are not calls. A false positive teaches people to
# rephrase the gate away; each of these must stay silent.
cat >"$WORK/lookalike.tmpl" <<'EOF'
# @C@ the list first
x=1 # then @C@ it
local uniq="$WORK/formats.uniq"
[[ -s "$uniq" ]] && echo ok
echo "cannot @C@ this"
echo "the @C@ order"
resorted=1
disjoin_x=2
EOF
sed "s/@C@/sort/g" "$WORK/lookalike.tmpl" >"$fx/hack/lookalike.sh"
scan_root "$fx" >"$WORK/fixture.scan"
if LC_ALL=C grep -q '^hack/lookalike.sh:' "$WORK/fixture.scan"; then
  fail "look-alike lines were read as calls: $(LC_ALL=C grep '^hack/lookalike.sh:' "$WORK/fixture.scan" | tr '\n' ' ')"
else
  pass "look-alikes (comment, trailing comment, variable named uniq, the word in strings, substrings) not flagged"
fi

# --- (4) behavioural control -------------------------------------------------
echo "== (4) the hazard is real on this host, and the pin removes it =="

# A candidate counts only if it is installed AND collates differently from byte
# order: in C, 'B' (0x42) sorts before 'a' (0x61); in any natural-language UTF-8
# locale 'a' comes first. An absent locale silently falls back to C, which this
# probe also rejects.
LOCALE_CANDIDATES="${ASSENT_LOCALE_CANDIDATES:-en_US.UTF-8 en_US.utf8 en_GB.UTF-8 en_GB.utf8 de_DE.UTF-8 de_DE.utf8 fr_FR.UTF-8 fr_FR.utf8}"
LOC=""
for cand in $LOCALE_CANDIDATES; do
  first="$(printf 'B\na\n' | LC_ALL="$cand" sort 2>/dev/null | head -n 1)" # D-179-exempt: the probe must sort under the candidate
  if [ "$first" = "a" ]; then
    LOC="$cand"
    break
  fi
done

if [ -z "$LOC" ]; then
  if [ -n "${GITHUB_ACTIONS:-}" ]; then
    fail "no UTF-8 locale with non-byte collation among: $LOCALE_CANDIDATES — on CI that is a FAILURE, not a skip: ubuntu runners ship en_US.UTF-8, and if the image dropped it, generate one in the workflow (locale-gen) rather than letting the only behavioural control of D-179 go dark"
  else
    skips=$((skips + 1))
    {
      echo "SKIP  ================================================================"
      echo "SKIP  D-179 behavioural control DID NOT RUN: no installed UTF-8 locale"
      echo "SKIP  with non-byte collation among: $LOCALE_CANDIDATES"
      echo "SKIP  Sections (1)-(3) grade TEXT; only this section shows the hazard"
      echo "SKIP  exists and that the pin removes it. Install one (e.g."
      echo "SKIP  locale-gen en_US.UTF-8) or set ASSENT_LOCALE_CANDIDATES."
      echo "SKIP  ================================================================"
    } >&2
  fi
else
  # The data §7a compares: real merge-commit subjects, plus two lines that make
  # the divergence independent of what history happens to contain (a case pair
  # and a punctuation pair, the two things natural-language collation ignores at
  # its first level).
  git -C "$ROOT" log --merges --format=%s >"$WORK/subjects.raw" 2>/dev/null || true
  printf '%s\n' 'Zulu probe' 'alpha probe' 'lane/e10 probe' 'lane-e9 probe' >>"$WORK/subjects.raw"
  LC_ALL=C awk '!seen[$0]++' "$WORK/subjects.raw" >"$WORK/A"
  # B = every other line of A; A \ B is then known without sort or comm.
  LC_ALL=C awk 'NR % 2 == 0' "$WORK/A" >"$WORK/B"
  LC_ALL=C awk 'NR % 2 == 1' "$WORK/A" | LC_ALL=C sort >"$WORK/oracle"
  [ -s "$WORK/oracle" ] || fail "behavioural oracle is empty — the comparison below would be vacuous"

  # run_shape <name> <sort-locale> <comm-locale> — sort A and B under the first
  # locale, `comm -23` them under the second; `misbehaved` when comm exits
  # non-zero, writes to stderr, or returns a set different from the oracle.
  run_shape() {
    LC_ALL="$2" sort "$WORK/A" >"$WORK/$1.a" # D-179-exempt: the locale under test
    LC_ALL="$2" sort "$WORK/B" >"$WORK/$1.b" # D-179-exempt: the locale under test
    LC_ALL="$3" comm -23 "$WORK/$1.a" "$WORK/$1.b" >"$WORK/$1.out" 2>"$WORK/$1.err" # D-179-exempt: the locale under test
    rc=$?
    LC_ALL=C sort "$WORK/$1.out" >"$WORK/$1.set"
    if [ "$rc" -ne 0 ] || [ -s "$WORK/$1.err" ] || ! cmp -s "$WORK/$1.set" "$WORK/oracle"; then
      echo "misbehaved (rc=$rc$([ -s "$WORK/$1.err" ] && printf ', %s' "$(head -n 1 "$WORK/$1.err")"), $(wc -l <"$WORK/$1.set" | tr -d ' ') of $(wc -l <"$WORK/oracle" | tr -d ' ') expected lines)"
    else
      echo "correct"
    fi
  }

  both="$(run_shape pinned C C)"
  if [ "$both" = "correct" ]; then
    pass "under $LOC, LC_ALL=C sort + LC_ALL=C comm over $(wc -l <"$WORK/A" | tr -d ' ') real subjects: correct, silent, exit 0"
  else
    fail "under $LOC, the PINNED pipeline $both — the pin does not do what D-179 says it does"
  fi

  # The three pin-removed mutants of that pipeline. Which of them breaks depends
  # on the host's coreutils (uutils: an unpinned sort; GNU: a pinned sort feeding
  # an unpinned comm, or the reverse), so the claim is that AT LEAST ONE does —
  # i.e. that on this host a missing pin is not harmless.
  broke=0
  for shape in "neither:$LOC:$LOC" "sort-only:C:$LOC" "comm-only:$LOC:C"; do
    name="${shape%%:*}"
    rest="${shape#*:}"
    verdict="$(run_shape "$name" "${rest%%:*}" "${rest#*:}")"
    echo "      pins on $name: $verdict"
    case "$verdict" in misbehaved*) broke=$((broke + 1)) ;; esac
  done
  if [ "$broke" -ge 1 ]; then
    pass "under $LOC, $broke of 3 pin-removed variants of the §7a pipeline misbehave on this host — a missing pin is a live defect here, not a style point"
  else
    fail "under $LOC, every pin-removed variant behaved correctly — either this host's sort and comm share one collation AND the data no longer straddles a punctuation/case boundary, or the probe is broken; the behavioural half of D-179 is vacuous until that is understood"
  fi
fi

echo
if [ "$fails" -ne 0 ]; then
  echo "FAILED: $fails locale-pin check(s)" >&2
  FINISHED=1
  exit 1
fi
if [ "$skips" -ne 0 ]; then
  echo "OK: every ordering call in hack/ is LC_ALL=C-pinned and the scan can fail — but the behavioural control SKIPPED (no suitable locale); this run carries no evidence that the hazard exists here"
else
  echo "OK: every ordering call in hack/ is LC_ALL=C-pinned, the scan can fail, and under $LOC the unpinned pipeline misbehaves while the pinned one is correct"
fi
FINISHED=1
