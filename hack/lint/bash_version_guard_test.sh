#!/usr/bin/env bash
# bash_version_guard_test.sh — BASH32-F01 / D-154.
#
# A hack/** gate script that uses a bash 4+ feature and does NOT declare a version
# floor is a gate that can certify nothing while reporting success. Measured, on
# stock macOS /bin/bash 3.2.57: `hack/docs/truthlag_pins_test.sh` printed 20 PASS
# lines, died at its `declare -A` (3.2 has no associative arrays, so under `set -u`
# it evaluates the subscript arithmetically and hits "unbound variable"), never
# printed its final `OK:` banner — and EXITED 0. `task docs-gates`, and therefore
# `task check`, read that as green. CI is ubuntu/bash 5, so nothing merges through
# this path; it is a LOCAL trust hole, which is exactly why it survived.
#
# WHAT THIS GATE ASSERTS
#   (1) every hack/**/*.sh using a command-position bash 4+ construct declares a
#       floor, that floor is at least the feature's own minimum, and the call
#       carries `|| exit 1` (the callers run `set -u` WITHOUT `-e`, so a failed
#       source or a missing function would otherwise just carry on);
#   (2) the scan still SEES the known population — a typo in a detection pattern
#       must red, not silently match nothing and pass — and every assertion that
#       THE SCAN DETECTED NOTHING about a given file (the PATTERN_NONPROBES four
#       and MUST_BE_CLEAN; not the controls that assert an empty grade_root
#       output, which have mutant partners of their own) names a file the SAME
#       `find` is shown to have ENUMERATED, so "no pattern detected it" can never
#       be a statement about a file that was never in the population (N5/D-169);
#   (3) negative control: real copies of the guarded scripts, guard lines stripped,
#       ARE flagged; a floor lowered below its feature's minimum IS flagged; a
#       dropped `|| exit 1` IS flagged;
#   (4) the D-154 fail-open control: under a real bash 3.2, guard-stripped
#       `truthlag_pins_test.sh` exits 0 with its banner ABSENT after printing PASS
#       lines, and the guarded original exits NON-zero. Skips LOUDLY where no bash
#       3.2 exists, which on CI is every run.
#
# WHAT IT CAN AND CANNOT SEE — stated plainly, because an honest narrow gate is
# worth more than an overreaching one:
#
#   CAN SEE: `declare -A` / `typeset -A` / `local -A`, `declare -n` / `local -n`,
#            `mapfile` and `readarray` (which are the SAME builtin under two
#            names, so `mapfile -d` and `readarray -d` are both graded 4.4 rather
#            than 4.0 — controls (f) and (g)), `wait -n` and `coproc` — each ONLY
#            when it is the first word on its line (optional leading whitespace).
#            Every real occurrence in this repo has that shape, and anchoring at a
#            command position is what lets this file carry the patterns without
#            matching itself.
#
#   CANNOT SEE: the same constructs after `&&`, `||`, `;`, `then`, in an `if`/
#            `while` head, behind `eval`, or assembled from a variable; `mapfile
#            -d` / `readarray -d` when an option BEFORE it takes an argument
#            (`mapfile -u 3 -d ''` reads as plain 4.0 `mapfile`, so a 4.0 floor on
#            a 4.4 construct would pass green there; `-d` in a later option
#            position with no intervening argument, e.g. `mapfile -t -d '' arr`,
#            IS caught — controls (f) and (g)); bash 4
#            EXPANSIONS — `${v^^}`, `${v,,}`, `${!prefix@}`, `**` globstar,
#            `&>>` — because detecting those needs unanchored patterns that this
#            file could not carry without flagging itself; non-`.sh` files;
#            anything outside `hack/`; and the numeric floor of a hand-rolled
#            `BASH_VERSINFO` guard (such a guard is accepted as PRESENT, but its
#            number is not read, so `hack/audit/aud2_exitgate_test.sh` is graded
#            on having a guard, not on its value).
#
#   GUARD SHAPES NOT RECOGNISED — a hand-rolled guard is recognised in a narrow
#            spelling: `BASH_VERSINFO` on the LEFT of a comparison, on ONE line,
#            with NO `#` anywhere earlier on that line, with nothing between the
#            token and the operator but an optional numeric subscript, a closing
#            brace, a closing quote and whitespace, and — for the symbolic
#            operators — a literal digit on the right. The reasoning for each of
#            those restrictions is recorded at has_versinfo_guard. Every other
#            spelling FAILS CLOSED: a file written another way reds with
#            "declares NO bash version floor" even though it plainly has a
#            guard, and the fix is to add the shape to the recognised set with a
#            control in (e), or to spell the guard a recognised way.
#
#            THAT PARAGRAPH IS PROSE RESTATING A REGEXP, and it does not get to
#            certify itself. It is written as a rule rather than a list because
#            the REJECTED set cannot be enumerated — but a rule stated in English
#            can still be narrower than the predicate, and this one WAS: it said
#            "on ONE line" and omitted the `#` clause, so an argument-checking
#            prologue on the same line — `[ $# -gt 0 ] && [ "${BASH_VERSINFO[0]}"
#            -lt 4 ] && exit 1` — satisfied every word of it and was rejected
#            anyway (measured; found by review of the very change that replaced
#            the old list, i.e. the N6 species committed again one corner
#            smaller). The regexp at has_versinfo_guard is the authority. What is
#            kept honest by a TEST is not this paragraph but the array below.
#
#            The individual shapes that have been MEASURED failing closed live in
#            GUARD_SHAPES_UNRECOGNISED below, where control (i) grades every one
#            of them as rejected. They are a sample, NOT a catalogue, and this
#            paragraph deliberately no longer says otherwise. The wording it
#            replaces — "If you hit that message on a file you know is guarded,
#            this list is why" — read as exhaustive and was not: every claim the
#            old list made was true, but `case "${BASH_VERSINFO[0]}" in
#            0|1|2|3) exit 1;; esac` is a perfectly good guard, is rejected
#            (measured), and was absent from it, as are guards reading
#            `$BASH_VERSION`, guards split across lines, and guards that go
#            through a variable. The rejected set is *every spelling that is not
#            the accepted one*, so no list of it can be complete — which is why
#            the fix here was to state the rule and GRADE the samples rather
#            than to lengthen the list (D-169/N6).
#
# THIS FILE IS BASH-3.2-CLEAN ON PURPOSE and asserts that about itself below: a
# gate about bash floors must still be able to run, and report, on the very shell
# whose behaviour it describes.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT" || exit 1

WORK="$(mktemp -d)"
FINISHED=0

# The defect this gate exists to catch, applied to the gate itself: an early death
# under `set -u` leaves the exit status at 0. Nothing may read this script as green
# unless it reached its last line.
on_exit() {
  rc=$?
  rm -rf "$WORK"
  if [ "$FINISHED" -eq 0 ] && [ "$rc" -eq 0 ]; then
    echo "FAIL: bash_version_guard_test.sh terminated before its final line yet exited 0 — refusing to report success" >&2
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

# --- feature table -----------------------------------------------------------
# `<extended-regexp>;<minimum bash version>;<description>`. The regexp is matched
# with `^[[:space:]]*` prepended, i.e. at a command position only. The minimum is
# per FEATURE, never per repo: a blanket floor would refuse shells that can run
# the 4.0 gates and would hide which construct actually binds.
#
# The field separator is `;`, NOT `|`: several of these regexps need alternation,
# and splitting on `|` silently truncated them to unbalanced parentheses that
# grep rejected — a scan that errors per file and matches nothing, which is the
# vacuity this gate is supposed to be immune to. `;` cannot appear in a pattern
# here; assertion (2) below is what makes a repeat of that mistake red.
FEATURES=(
  'declare[[:space:]]+-A;4.0;declare -A (associative array)'
  'typeset[[:space:]]+-A;4.0;typeset -A (associative array)'
  'local[[:space:]]+-A;4.0;local -A (associative array)'
  'declare[[:space:]]+-n;4.3;declare -n (nameref)'
  'local[[:space:]]+-n;4.3;local -n (nameref)'
  'mapfile([[:space:]]+-[a-zA-Z]+)*[[:space:]]+-[a-zA-Z]*d;4.4;mapfile -d (alternate delimiter)'
  'mapfile([[:space:]]|$);4.0;mapfile'
  'readarray([[:space:]]+-[a-zA-Z]+)*[[:space:]]+-[a-zA-Z]*d;4.4;readarray -d (alternate delimiter)'
  'readarray([[:space:]]|$);4.0;readarray'
  'wait[[:space:]]+-n;4.3;wait -n'
  'coproc([[:space:]]|$);4.0;coproc'
)

# --- the population this gate must keep seeing (anti-vacuity, assertion 2) ----
# If a detection pattern is mistyped the scan finds nothing and every assertion
# below passes. These four files are the entire measured population as of D-154;
# the gate reds if any of them stops being detected, so the patterns cannot rot
# silently. Removing a file from this list is a deliberate act, like CHECK_STAGES.
KNOWN_FEATURE_USERS=(
  hack/docs/truthlag_pins_test.sh
  hack/release/verify-artifacts.sh
  hack/validate-schemas-stock.sh
  hack/audit/aud2_exitgate_test.sh
)

# Files that must NOT be detected: the guard machinery itself has to run under the
# shell it protects against, so a bash 4 construct appearing in either of these is
# the defect reintroduced inside the fix.
MUST_BE_CLEAN=(
  hack/lib/require-bash.sh
  hack/lint/bash_version_guard_test.sh
)

# --- per-pattern anti-vacuity pins (assertion 2, second half) -----------------
# KNOWN_FEATURE_USERS above pins only the patterns the tree HAPPENS to exercise —
# measured, four of eleven (declare -A, local -n, mapfile, mapfile -d). A typo in
# any of the other seven matches nothing, reds nothing, and leaves that construct
# silently ungraded forever: the same vacuity the population check exists to
# prevent, one level down, and the reason the readarray -d floor could be wrong
# from the day it was written without a single assertion noticing.
#
# `<a line the patterns MUST detect>;<the version it must be graded>;<the feature
# description it must be attributed to>`. Same `;` separator and same reason as
# FEATURES. Every FEATURES description is required to appear here, so a pattern
# added without a probe reds rather than shipping unpinned.
PATTERN_PROBES=(
  'declare -A probe=();4.0;declare -A (associative array)'
  'typeset -A probe=();4.0;typeset -A (associative array)'
  'local -A probe=();4.0;local -A (associative array)'
  'declare -n ref=probe;4.3;declare -n (nameref)'
  'local -n ref=probe;4.3;local -n (nameref)'
  'mapfile -t probe < /dev/null;4.0;mapfile'
  'mapfile -d "" -t probe < /dev/null;4.4;mapfile -d (alternate delimiter)'
  'readarray -t probe < /dev/null;4.0;readarray'
  'readarray -d "" -t probe < /dev/null;4.4;readarray -d (alternate delimiter)'
  'wait -n;4.3;wait -n'
  'coproc probe true;4.0;coproc'
)

# The other side of the same pin. A pattern degraded to something that matches
# every line would satisfy every probe above; these lines put the construct
# somewhere OTHER than a command position and must therefore be detected by
# nothing at all. They are also the CANNOT-SEE list, made executable.
PATTERN_NONPROBES=(
  'echo "declare -A is how you get an associative array"'
  'true && declare -A late=()'
  'if declare -n ref=probe; then :; fi'
  'printf "%s\n" "coproc wait -n readarray typeset -A"'
)

# --- guard shapes documented as unrecognised (N6, D-169) ---------------------
# `has_versinfo_guard` accepts one spelling; everything else fails closed. These
# are the shapes measured doing so, and control (i) asserts each is STILL
# rejected — so the header paragraph cannot drift away from the predicate the
# way a hand-maintained prose list does. A shape that later becomes recognised
# reds here instead of leaving a comment quietly wrong.
#
# This list is deliberately NOT complete and cannot be made complete: the
# rejected set is every spelling that is not the one accepted spelling. That is
# the whole point of N6 — the block above used to imply otherwise, and the
# `case` entry is the guard that implication was wrong about.
GUARD_SHAPES_UNRECOGNISED=(
  # BASH_VERSINFO must be on the LEFT. The reversed form was written into the
  # predicate once and REMOVED: with the digit on the left and nothing required
  # on the right it also accepted `2> "$BASH_VERSINFO_LOG"`.
  '[ 4 -gt "${BASH_VERSINFO[0]}" ]'
  # Bare `=` and `!=` are not in the operator set — `=` is what let
  # `BASH_VERSINFO_NOTE=1` through, and `!=` was never separated from it.
  # `==` IS recognised.
  '[ "${BASH_VERSINFO[0]}" = 3 ]'
  '[[ "${BASH_VERSINFO[0]}" != 4 ]]'
  '(( "${BASH_VERSINFO[0]}" != 4 ))'
  # A SYMBOLIC operator is recognised only against a literal digit: a variable
  # right-hand side is indistinguishable from a redirection target. The word
  # operators carry no such restriction, so
  # `[ "${BASH_VERSINFO[0]}" -gt "$want_major" ]` IS recognised.
  '(( BASH_VERSINFO[0] < want_major ))'
  # The entry the prose list was missing, and the reason this array exists: a
  # `case` head is a comparison containing no operator character at all.
  'case "${BASH_VERSINFO[0]}" in 0|1|2|3) exit 1;; esac'
  # A whole FAMILY the predicate never looks for: guards that read the
  # `$BASH_VERSION` string rather than the `BASH_VERSINFO` array.
  'case $BASH_VERSION in 3.*) exit 1;; esac'
  # A regex test and a substring expansion — neither puts a recognised operator
  # adjacent to the token.
  '[[ "${BASH_VERSINFO[0]}" =~ ^[0-3]$ ]] && exit 1'
  '[ "${BASH_VERSINFO:0:1}" -lt 4 ]'
  # The `^[^#]*` prefix. It was written to skip COMMENT lines and it does more
  # than that: it rejects a guard with ANY `#` earlier on the line, and `$#` is
  # how an argument-checking prologue is spelled. Both of these are real guards,
  # in the recognised operator shapes, on one line, with BASH_VERSINFO on the
  # left — and both fail closed purely because of the `$#` to their left
  # (measured). `${#arr[@]}` and a `${var#pfx}` expansion do the same. A comment
  # AFTER the token is harmless, which is why this went unnoticed.
  '[ $# -gt 0 ] && [ "${BASH_VERSINFO[0]}" -lt 4 ] && exit 1'
  'if [ $# -eq 0 ] || ((BASH_VERSINFO[0] < 4)); then exit 1; fi'
)

# --- version arithmetic (3.2-safe) -------------------------------------------
ver_ge() { # <a> <b> — true when a >= b
  local a="$1" b="$2" amaj amin bmaj bmin
  amaj="${a%%.*}"
  if [ "$a" = "$amaj" ]; then amin=0; else amin="${a#*.}"; fi
  bmaj="${b%%.*}"
  if [ "$b" = "$bmaj" ]; then bmin=0; else bmin="${b#*.}"; fi
  if [ "$amaj" -gt "$bmaj" ]; then return 0; fi
  if [ "$amaj" -lt "$bmaj" ]; then return 1; fi
  [ "$amin" -ge "$bmin" ]
}

# --- the scan ----------------------------------------------------------------
# enumerate_scripts <root> — the POPULATION scan_features grades, one path
# relative to <root> per line.
#
# Factored out of the scan on purpose (N5, D-169). An assertion that the scan
# reported NOTHING about a fixture is satisfied two ways that look identical in
# the scan's output: the fixture was graded and cleared, or the fixture was never
# in the population at all. `find` is the single point where a file drops out of
# the population, and it drops out for two reasons — living outside `hack/`, or
# not ending in `.sh`. Measured on a scratch copy of this gate: writing ONLY the
# nonprobe fixture one directory outside the walked root, and separately naming
# it `probe.txt`, EACH left all four absence assertions below green, 0 FAILs, the
# gate at exit 0, with a PASS count identical to the honest run. A sentinel
# beside the fixture does not close that: it proves the ROOT was walked, which is
# a strictly weaker claim than "this file was enumerated".
enumerate_scripts() { # <root>
  local root="$1" f
  while IFS= read -r f; do
    printf '%s\n' "${f#"$root"/}"
  done < <(find "$root/hack" -type f -name '*.sh' | sort)
  return 0
}

# scan_features <root> — one line per feature-using script:
#   <path relative to root>|<highest minimum required>|<features found>
scan_features() { # <root>
  local root="$1" f rel spec pat min desc need feats
  while IFS= read -r rel; do
    f="$root/$rel"
    need=""
    feats=""
    for spec in "${FEATURES[@]}"; do
      pat="${spec%%;*}"
      min="${spec#*;}"
      min="${min%%;*}"
      desc="${spec##*;}"
      if grep -Eq "^[[:space:]]*${pat}" "$f"; then
        if [ -z "$feats" ]; then feats="$desc"; else feats="${feats}, ${desc}"; fi
        if [ -z "$need" ] || ! ver_ge "$need" "$min"; then need="$min"; fi
      fi
    done
    if [ -n "$need" ]; then
      printf '%s|%s|%s\n' "$rel" "$need" "$feats"
    fi
  done < <(enumerate_scripts "$root")
  return 0
}

# declared_floor <file> — highest version passed to require_bash, empty if none.
declared_floor() {
  local f="$1" v best=""
  while IFS= read -r v; do
    [ -n "$v" ] || continue
    if [ -z "$best" ] || ! ver_ge "$best" "$v"; then best="$v"; fi
  done < <(grep -Eo '^[[:space:]]*require_bash[[:space:]]+[0-9]+(\.[0-9]+)?' "$f" 2>/dev/null |
    grep -Eo '[0-9]+(\.[0-9]+)?$')
  printf '%s' "$best"
}

# A hand-rolled guard (the hack/audit/aud2_exitgate_test.sh:71 precedent) counts as
# present — but it must be a COMPARISON, not a mention. Accepting any non-comment
# occurrence of the token was itself the defect this gate exists to catch: a file
# containing `echo "note: BASH_VERSINFO is nice to have"` beside an unguarded
# `declare -A` passed the whole gate at exit 0.
#
# Requiring "an operator character somewhere after the token" did not fix that, it
# narrowed it: `>` is an operator character AND a redirection, so
# `echo "…BASH_VERSINFO…" >&2` — exactly what a hand-rolled guard looks like after
# its `if` has been deleted — was still accepted, as were `> file`, `>>log`, and
# `BASH_VERSINFO_NOTE=1`. Measured, all four. So three things are required now:
#
#   ADJACENCY — nothing may sit between the token and the operator except an
#     optional numeric subscript, a closing brace, a closing quote and whitespace.
#     That is what rejects `BASH_VERSINFO_NOTE=` and `"BASH_VERSINFO note"; x=1`.
#   A VERSION ON THE OTHER SIDE — the symbolic operators (`<`, `>`, `<=`, `>=`,
#     `==`) are accepted only against a literal digit, which is what separates
#     `((BASH_VERSINFO[0] < 4))` from `>&2`, `> /tmp/x` and `>>log`. The word
#     operators (`-lt` … `-ne`) are never redirections, so their right-hand side
#     stays free — `[ "${BASH_VERSINFO[0]}" -gt "$want_major" ]`, the shape in
#     hack/lib/require-bash.sh, is a guard.
#   NO `#` BEFORE THE TOKEN — the `^[^#]*` prefix. It is here to skip COMMENT
#     lines, which is what control (e)'s `# TODO: add a BASH_VERSINFO check`
#     mutant pins, but it is spelled as "no `#` earlier on the line" and that is
#     strictly more than it was meant to say. `$#` is how an argument-checking
#     prologue is written, so `[ $# -gt 0 ] && [ "${BASH_VERSINFO[0]}" -lt 4 ]
#     && exit 1` and `if [ $# -eq 0 ] || ((BASH_VERSINFO[0] < 4)); then exit 1;
#     fi` are real guards in recognised operator shapes and BOTH fail closed
#     (measured); `${#arr[@]}` and a `${var#pfx}` expansion do the same. A `#`
#     AFTER the token — an end-of-line comment — is harmless, which is why the
#     over-reach went unnoticed. Not narrowed to a leading-`#` anchor here: a
#     comment can be indented or trail a line of code, so distinguishing "this
#     line is a comment" from "this line contains a `#`" is the quoting/parsing
#     problem that produced this file's other scars. It stays a documented,
#     GRADED fail-closed limit instead (GUARD_SHAPES_UNRECOGNISED, control (i)).
#
# BASH_VERSINFO MUST BE ON THE LEFT. A second pattern accepting the reversed
# spelling (`[ 4 -gt "${BASH_VERSINFO[0]}" ]`) was written here and REMOVED: with
# the digit required on the left and nothing required on the right, it accepted
# `command foo 2> "$BASH_VERSINFO_LOG"`, `some_cmd 2>${BASH_VERSINFO_TRACE}`,
# `printf "%s\n" 4 > "$BASH_VERSINFO_FILE"` and `echo "bash 5 > $BASH_VERSINFO"` —
# a fresh fail-open, opened by the very commit that closed the forward one, and
# one the predicate it replaced had rejected. Anchoring the digit to a word
# boundary does NOT rescue it: `2>` is a digit at a word boundary, measured. So
# the reversed spelling is simply not recognised. It fails CLOSED — a file
# written that way reds with "declares NO bash version floor" rather than passing
# silently — no file in this tree uses it, and it is listed in CANNOT-SEE.
#
# Still deliberately loose about which comparison it is and blind to the number,
# as documented in CANNOT-SEE: this branch grades that a version comparison
# EXISTS, never that its floor is the right one — and it grades shape, not
# semantics, so a contrived `echo "$BASH_VERSINFO" > 4` would still pass.
# Control (e) below pins both directions: a comment, a bare mention, four
# non-guard operator shapes and the four reversed-form redirections above are
# rejected; four real comparison shapes, the first of them verbatim from
# aud2_exitgate_test.sh, are accepted.
has_versinfo_guard() {
  grep -Eq '^[^#]*BASH_VERSINFO(\[[0-9]+\])?\}?"?[[:space:]]*((-lt|-le|-gt|-ge|-eq|-ne)[[:space:]]|(<=|>=|==|<|>)[[:space:]]*[0-9])' "$1" 2>/dev/null
}

# Lines of the guard idiom that are missing their `|| exit 1`. The callers run
# `set -u` with no `-e`: without it a failed source returns non-zero and execution
# continues into the guarded construct, and an undefined require_bash returns 127
# and does the same. That is the original fail-open, one line up.
#
# `.` and `source` are the SAME builtin, so both spellings of the include have to
# be recognised. Matching only `.` left `source "…/require-bash.sh"` — with no
# `|| exit 1` — reading as no guard line at all, and the tree writes `.` in all
# three of its guarded scripts, so no population check could ever have caught it.
# Control (h) pins both spellings, in both directions.
unterminated_guard_lines() { # <file>
  grep -E '^[[:space:]]*(require_bash[[:space:]]|(\.|source)[[:space:]].*require-bash\.sh)' "$1" |
    grep -Ev '\|\|[[:space:]]*exit[[:space:]]+1[[:space:]]*$' || true
}

# grade_root <root> <label> — print "<file>: <problem>" for every violation found
# under <root>. Used on the real tree AND on mutated copies, which is what makes
# the mutation controls compare like with like.
grade_root() { # <root> <label>
  local root="$1" line rel need feats floor bad
  while IFS= read -r line; do
    rel="${line%%|*}"
    need="${line#*|}"
    need="${need%%|*}"
    feats="${line##*|}"
    floor="$(declared_floor "$root/$rel")"
    if [ -z "$floor" ]; then
      if has_versinfo_guard "$root/$rel"; then
        continue # hand-rolled guard: present, floor not readable (documented above)
      fi
      echo "$rel: uses ${feats} but declares NO bash version floor (needs >= ${need})"
      continue
    fi
    if ! ver_ge "$floor" "$need"; then
      echo "$rel: declares bash >= ${floor} but uses ${feats}, which needs >= ${need}"
    fi
    bad="$(unterminated_guard_lines "$root/$rel")"
    if [ -n "$bad" ]; then
      echo "$rel: guard line without '|| exit 1': $(echo "$bad" | head -1 | sed 's/^[[:space:]]*//')"
    fi
  done < <(scan_features "$root")
  return 0
}

echo "== (1) every hack/** script using a bash 4+ construct declares a sufficient floor =="

# Before anything is scanned: every pattern must be a regexp grep accepts. A
# malformed one makes grep exit 2 on every file — it matches nothing and the scan
# stays green, which is how the `|`-vs-`;` separator bug above hid itself. Checked
# against /dev/null, so the only outcomes are "no match" (1) and "bad regexp" (2).
for spec in "${FEATURES[@]}"; do
  pat="${spec%%;*}"
  grep -Eq "^[[:space:]]*${pat}" /dev/null 2>/dev/null
  if [ $? -ge 2 ]; then
    fail "detection pattern '${pat}' is not a regexp grep accepts — it would error on every file and match nothing"
  fi
done
if [ "$fails" -eq 0 ]; then
  pass "all ${#FEATURES[@]} detection patterns compile"
fi

SCAN="$WORK/scan.txt"
scan_features "$ROOT" >"$SCAN"
scanned="$(wc -l <"$SCAN" | tr -d '[:space:]')"

# The population itself, kept beside the scan output because they answer
# different questions: this one says which files were LOOKED AT, the scan says
# which of them were flagged. MUST_BE_CLEAN below needs the first, and asserting
# it against the second is the N5 defect.
REAL_ENUM="$WORK/enum.txt"
enumerate_scripts "$ROOT" >"$REAL_ENUM"
if [ ! -s "$REAL_ENUM" ]; then
  fail "enumerate_scripts listed NO *.sh file under hack/ at all — the scan has no population, so every assertion in this gate is about a set of files nobody opened"
else
  pass "the population under hack/ is non-empty: $(wc -l <"$REAL_ENUM" | tr -d '[:space:]') script(s) enumerated for grading"
fi

if [ "$scanned" -eq 0 ]; then
  fail "the scan found NO bash 4+ feature user anywhere under hack/ — ${#FEATURES[@]} patterns matched nothing, so every assertion in this gate would be vacuous. A mistyped pattern looks exactly like this."
else
  pass "scan reports $scanned feature-using script(s) under hack/"
fi

for want in "${KNOWN_FEATURE_USERS[@]}"; do
  if grep -q "^${want}|" "$SCAN"; then
    pass "detected the known feature user $want"
  else
    fail "the scan no longer detects $want as a bash 4+ feature user — either the file changed deliberately (update KNOWN_FEATURE_USERS) or a detection pattern broke, which would make this whole gate vacuous"
  fi
done

# Every pattern, including the seven no file in this tree exercises, must be shown
# to detect something and to grade it at the right floor.
probe_descs="|"
probe_i=0
for spec in "${PATTERN_PROBES[@]}"; do
  probe_i=$((probe_i + 1))
  probe_line="${spec%%;*}"
  probe_min="${spec#*;}"
  probe_min="${probe_min%%;*}"
  probe_desc="${spec##*;}"
  probe_descs="${probe_descs}${probe_desc}|"
  proot="$WORK/probe$probe_i"
  mkdir -p "$proot/hack"
  printf '%s\n' '#!/usr/bin/env bash' "$probe_line" >"$proot/hack/probe.sh"
  got="$(scan_features "$proot")"
  got_min="${got#*|}"
  got_min="${got_min%%|*}"
  got_feats="${got##*|}"
  if [ -z "$got" ]; then
    fail "detection probe: '${probe_line}' was detected by NO pattern — the '${probe_desc}' pattern matches nothing, so every script using that construct is graded as using no bash 4 feature at all"
  elif [ "$got_min" != "$probe_min" ]; then
    fail "detection probe: '${probe_line}' was graded ${got_min}, not ${probe_min} — the floor for '${probe_desc}' is wrong, so a script declaring ${got_min} would pass while breaking on every bash below ${probe_min}. scan said: ${got}"
  else
    case "$got_feats" in
    *"$probe_desc"*)
      pass "detection probe: '${probe_line}' is detected as '${probe_desc}' and graded ${probe_min}"
      ;;
    *)
      fail "detection probe: '${probe_line}' was graded ${probe_min} but attributed to '${got_feats}', not '${probe_desc}' — the right answer for the wrong reason, so the '${probe_desc}' pattern is still unpinned"
      ;;
    esac
  fi
done

for spec in "${FEATURES[@]}"; do
  desc="${spec##*;}"
  case "$probe_descs" in
  *"|${desc}|"*) ;;
  *)
    fail "the detection pattern for '${desc}' has no entry in PATTERN_PROBES — nothing proves it matches anything, which is exactly the shape a mistyped pattern hides in"
    ;;
  esac
done

probe_i=0
for probe_line in "${PATTERN_NONPROBES[@]}"; do
  probe_i=$((probe_i + 1))
  proot="$WORK/nonprobe$probe_i"
  mkdir -p "$proot/hack"
  nonprobe_rel="hack/probe.sh"
  nonprobe_path="$proot/$nonprobe_rel"
  printf '%s\n' '#!/usr/bin/env bash' "$probe_line" >"$nonprobe_path"
  # These are assertions of ABSENCE, and absence needs TWO anti-vacuity partners,
  # because it has two innocent-looking causes and they are not the same cause:
  #
  #   ENUMERATION — the fixture ITSELF must be in the list `scan_features`
  #     grades, asserted by exact match against `enumerate_scripts`. A sentinel
  #     alone proves only that the fixture ROOT was walked, which is strictly
  #     weaker: measured (N5/D-169), moving JUST the probe write one directory
  #     outside the walked root, and separately naming the fixture `probe.txt`
  #     so `find -name '*.sh'` never lists it, each left all four of these
  #     assertions green with 0 FAILs and the gate at exit 0 — the sentinel
  #     sitting exactly where it always sat, reporting exactly what it always
  #     reported. A sentinel proves the room was entered, not that anyone looked
  #     at this file.
  #   GRADING — the sentinel stays, and still earns its keep: enumeration proves
  #     the file was offered to the scan, not that the scan can still report
  #     anything at all. A scan degraded to print nothing would enumerate the
  #     probe and "clear" it too.
  #
  # Both sides carry a non-empty guard, and the fixture's existence on disk is
  # checked first, so a mistyped path reds loudly instead of passing vacuously.
  printf '%s\n' '#!/usr/bin/env bash' 'declare -A sentinel=()' >"$proot/hack/sentinel.sh"
  enum="$(enumerate_scripts "$proot")"
  got="$(scan_features "$proot")"
  if [ ! -f "$nonprobe_path" ]; then
    fail "detection probe: the fixture for '${probe_line}' was not written to ${nonprobe_path} — everything below would be asserted about a file that is not there"
  elif [ -z "$nonprobe_rel" ] || [ -z "$enum" ]; then
    fail "detection probe: the fixture path is empty, or enumerate_scripts listed NOTHING under ${proot} — either way the absence asserted below is vacuous. enumeration was: ${enum:-<nothing>}"
  elif ! echo "$enum" | grep -Fqx "$nonprobe_rel"; then
    fail "detection probe: the fixture carrying '${probe_line}' was NOT enumerated as '${nonprobe_rel}' by the same find scan_features uses — it is outside the walked root or not named *.sh, so 'no pattern detected it' is a statement about a file nobody opened. enumeration was: ${enum}"
  elif ! echo "$got" | grep -q '^hack/sentinel\.sh|'; then
    fail "detection probe: the scan did not report the sentinel beside '${probe_line}' — this root is enumerated but nothing in it is being graded, so the absence asserted below would be vacuous rather than a finding. scan said: ${got:-<nothing>}"
  elif echo "$got" | grep -q "^${nonprobe_rel}|"; then
    fail "detection probe: '${probe_line}' does not put the construct at a command position, so nothing may detect it, but the scan reported it — a pattern has lost its anchor and now matches prose, which would make every probe above pass for the wrong reason. scan said: ${got}"
  else
    pass "detection probe: '${probe_line}' is enumerated as '${nonprobe_rel}' and seen by no pattern, in a root the sentinel proves is still being graded"
  fi
done

# The enumeration assertion's OWN control: it must be able to fail, and it must
# fail on each of the two misdirections that the sentinel-only version passed at
# exit 0. Graded in both directions so "was enumerated" is a discrimination and
# not a function that says yes to everything (N5/D-169).
ectl="$WORK/enumctl"
mkdir -p "$ectl/hack" "$ectl/outside"
printf '%s\n' '#!/usr/bin/env bash' 'declare -A sentinel=()' >"$ectl/hack/sentinel.sh"
printf '%s\n' '#!/usr/bin/env bash' 'true' >"$ectl/outside/probe.sh"
printf '%s\n' '#!/usr/bin/env bash' 'true' >"$ectl/hack/probe.txt"
ectl_out="$(enumerate_scripts "$ectl")"
if [ -z "$ectl_out" ]; then
  fail "enumeration control: enumerate_scripts listed nothing for a root that certainly contains hack/sentinel.sh — the enumeration assertions above can only ever pass vacuously"
elif ! echo "$ectl_out" | grep -Fqx 'hack/sentinel.sh'; then
  fail "enumeration control: enumerate_scripts did not list hack/sentinel.sh, a file directly under the walked root — the enumeration assertions above would red on every honest fixture. enumeration was: ${ectl_out}"
elif echo "$ectl_out" | grep -Fqx 'outside/probe.sh'; then
  fail "enumeration control: enumerate_scripts listed a file OUTSIDE hack/, so it is not the population scan_features grades and 'was enumerated' would prove nothing about being scanned. enumeration was: ${ectl_out}"
elif echo "$ectl_out" | grep -Fq 'probe.txt'; then
  fail "enumeration control: enumerate_scripts listed a non-'.sh' file, so it is not the population scan_features grades. enumeration was: ${ectl_out}"
else
  pass "enumeration control: enumerate_scripts lists hack/sentinel.sh and lists NEITHER a file outside hack/ NOR a non-.sh file — both misdirections that left the sentinel-only absence check green are visible to it"
fi

# The coupling between the two. Everything above assumes `scan_features` grades
# the population `enumerate_scripts` defines; today it does so by construction,
# because it reads from it. That is exactly the kind of "holds by construction"
# an edit dissolves without noticing: reintroducing a divergent `find` inside
# scan_features would leave every enumeration assertion above TRUE about a set of
# files that is no longer the set being graded — the N5 defect restored through
# the one door the N5 fix does not watch. Asserted in BOTH directions, because
# each catches a different divergence and neither catches the other's:
#   SUBSET   — nothing may be graded that was not enumerated (a WIDER find).
#   SUPERSET — every enumerated file that uses a feature must be graded (a
#              NARROWER find, which a subset check cannot see).
#
# The fixture is built so BOTH directions are reachable. Three feature-using
# `.sh` files at three depths make a narrower walk (a `-maxdepth`, say) visible;
# two further feature-using files that are deliberately OUT of the population —
# one non-`.sh`, one outside `hack/` — make a wider walk visible. Without those
# two the subset half would be unfalsifiable on this fixture, which is the shape
# of vacuity this whole lane is about.
sroot="$WORK/scanpop"
mkdir -p "$sroot/hack/a/b" "$sroot/hack/c" "$sroot/outside"
printf '%s\n' '#!/usr/bin/env bash' 'declare -A one=()' >"$sroot/hack/top.sh"
printf '%s\n' '#!/usr/bin/env bash' 'mapfile -t two < /dev/null' >"$sroot/hack/a/b/deep.sh"
printf '%s\n' '#!/usr/bin/env bash' 'coproc three true' >"$sroot/hack/c/mid.sh"
printf '%s\n' '#!/usr/bin/env bash' 'declare -A four=()' >"$sroot/hack/notscript.txt"
printf '%s\n' '#!/usr/bin/env bash' 'declare -A five=()' >"$sroot/outside/away.sh"
# Every fixture here uses a feature, so an honest scan reports ALL of them and
# the two lists must be equal, in the same order (both come from the same sorted
# walk). Equality catches both divergences at once; the diagnosis below then says
# which way they went, per file, so a failure names the file rather than the set.
spop_enum="$(enumerate_scripts "$sroot")"
spop_scan="$(scan_features "$sroot" | cut -d'|' -f1)"
if [ -z "$spop_enum" ] || [ -z "$spop_scan" ]; then
  fail "population coupling: enumerate_scripts or scan_features reported NOTHING for a root holding three feature-using scripts — the comparison below would be two empty sets trivially agreeing. enumerated: ${spop_enum:-<nothing>}; graded: ${spop_scan:-<nothing>}"
elif [ "$spop_enum" = "$spop_scan" ]; then
  pass "population coupling: scan_features grades exactly the files enumerate_scripts lists, at three directory depths — neither a wider nor a narrower find survives here"
else
  for spop_f in $spop_scan; do
    echo "$spop_enum" | grep -Fqx "$spop_f" ||
      fail "population coupling: scan_features graded '${spop_f}', which enumerate_scripts never listed — the two have diverged (a WIDER find inside the scan), so the enumeration assertions above are true about a population that is no longer the one being graded"
  done
  for spop_f in $spop_enum; do
    echo "$spop_scan" | grep -Fqx "$spop_f" ||
      fail "population coupling: enumerate_scripts listed '${spop_f}' and scan_features did not grade it, though it uses a bash 4+ feature — the scan's population is NARROWER (a divergent find), so a file can be proven enumerated and still never be looked at: the N5 defect through the one door the enumeration check cannot see"
  done
fi

for clean in "${MUST_BE_CLEAN[@]}"; do
  # An assertion of ABSENCE from the scan passes for two indistinguishable
  # reasons: the file really is clean, or the path no longer names a file the
  # scan ever looks at (renamed, moved, deleted). Every other check here has an
  # anti-vacuity partner; these are its two. Existence is the weaker of them and
  # is NOT sufficient on its own — a file that exists at `hack/lib/require-bash`
  # or at `tools/require-bash.sh` exists and is still outside the population, so
  # membership in `enumerate_scripts` is what actually has to hold (N5/D-169).
  if [ ! -f "$ROOT/$clean" ]; then
    fail "$clean does not exist — its 'is bash-3.2-clean' assertion would pass forever about a file that is not there"
    continue
  fi
  if ! grep -Fqx "$clean" "$REAL_ENUM"; then
    fail "$clean exists but is NOT in the population scan_features grades — it is outside hack/ or not named *.sh, so its absence from the scan is a statement about a file nobody opened, not a finding"
    continue
  fi
  if grep -q "^${clean}|" "$SCAN"; then
    fail "$clean uses a bash 4+ construct — the guard machinery must run under the shell it protects against, or it cannot refuse cleanly on bash 3.2"
  else
    pass "$clean is bash-3.2-clean"
  fi
done

violations="$WORK/violations.txt"
grade_root "$ROOT" real >"$violations"
if [ -s "$violations" ]; then
  while IFS= read -r v; do
    fail "$v"
  done <"$violations"
else
  pass "every detected feature user declares a floor at least as high as the feature it uses, with '|| exit 1' on each guard line"
fi

echo
echo "== (2) negative controls — the checks above can actually fail =="

# The controls mutate REAL copies of the shipped scripts, not synthetic fixtures:
# a fixture proves the regexp works on a fixture. Copies live under mktemp -d;
# `git checkout --` is never used (it reverts to HEAD, which can be behind the
# working tree, so a "mutation" could silently be a no-op) — the aud2 idiom.
MUT="$WORK/mut"
mkdir -p "$MUT/hack/docs" "$MUT/hack/release" "$MUT/hack/lib"
GUARDED=(
  hack/docs/truthlag_pins_test.sh
  hack/release/verify-artifacts.sh
  hack/validate-schemas-stock.sh
)

# (a) the whole guard block deleted -> every one of them must be reported
#     unguarded. The block is deleted as a RANGE (`_assent_lib=` … `require_bash`)
#     rather than by matching individual lines: a line-wise delete would strip the
#     `[ -r … ] || {` head and leave its `exit 1; }` tail behind, and the mutant
#     would then fail to parse rather than fail to guard.
strip_guard() { # <src> <dst>
  sed -E '/^_assent_lib=/,/^require_bash[[:space:]]/d' "$1" >"$2"
}
for g in "${GUARDED[@]}"; do
  strip_guard "$ROOT/$g" "$MUT/$g"
  if grep -q 'require_bash' "$MUT/$g"; then
    fail "mutant control: the guard block was not removed from the $g mutant — the control below would prove nothing"
  fi
  if ! bash -n "$MUT/$g" 2>/dev/null; then
    fail "mutant control: the $g mutant does not parse after guard removal — it would fail for the wrong reason"
  fi
done
out="$(grade_root "$MUT" mutant)"
for g in "${GUARDED[@]}"; do
  if echo "$out" | grep -q "^${g}: uses .* declares NO bash version floor"; then
    pass "mutant control: $g with its guard stripped is reported unguarded"
  else
    fail "mutant control: $g with its guard stripped was NOT reported — the guard check cannot fail, so its green above means nothing. grade_root said: ${out:-<nothing>}"
  fi
done

# (b) pristine copies -> reported clean, so (a) is a real discrimination and not
#     an artefact of grading a copy.
for g in "${GUARDED[@]}"; do
  cp "$ROOT/$g" "$MUT/$g"
done
out="$(grade_root "$MUT" pristine)"
if [ -z "$out" ]; then
  pass "mutant control: unmutated copies of the same three scripts are reported clean"
else
  fail "mutant control: unmutated copies were reported as violations, so the mutant result above is an artefact of copying, not of the mutation: $out"
fi

# (c) floor lowered below the feature's minimum -> reported. This is the assertion
#     that a single blanket floor would have made impossible.
sed -E 's/^([[:space:]]*require_bash[[:space:]]+)4\.4/\14.0/' \
  "$ROOT/hack/validate-schemas-stock.sh" >"$MUT/hack/validate-schemas-stock.sh"
if grep -q '^require_bash 4.0' "$MUT/hack/validate-schemas-stock.sh"; then
  out="$(grade_root "$MUT" lowfloor)"
  if echo "$out" | grep -q '^hack/validate-schemas-stock.sh: declares bash >= 4.0 but uses .*needs >= 4.4'; then
    pass "mutant control: a 4.0 floor on a script using mapfile -d is reported as too low"
  else
    fail "mutant control: lowering validate-schemas-stock.sh's floor from 4.4 to 4.0 was NOT reported — the per-feature minimum is not actually enforced. grade_root said: ${out:-<nothing>}"
  fi
else
  fail "mutant control: could not lower validate-schemas-stock.sh's require_bash floor (its guard shape changed?) — the too-low-floor check was not exercised"
fi
cp "$ROOT/hack/validate-schemas-stock.sh" "$MUT/hack/validate-schemas-stock.sh"

# (d) `|| exit 1` dropped from the require_bash call -> reported. Without it the
#     no-`-e` callers continue past a failed guard.
sed -E 's/^([[:space:]]*require_bash[[:space:]].*)[[:space:]]\|\|[[:space:]]*exit[[:space:]]+1[[:space:]]*$/\1/' \
  "$ROOT/hack/docs/truthlag_pins_test.sh" >"$MUT/hack/docs/truthlag_pins_test.sh"
if grep -Eq '^require_bash[[:space:]]+[0-9.]+[^|]*$' "$MUT/hack/docs/truthlag_pins_test.sh"; then
  out="$(grade_root "$MUT" noexit)"
  if echo "$out" | grep -q "^hack/docs/truthlag_pins_test.sh: guard line without '|| exit 1'"; then
    pass "mutant control: a require_bash call without '|| exit 1' is reported"
  else
    fail "mutant control: dropping '|| exit 1' from the require_bash call was NOT reported — a failed guard would be allowed to continue. grade_root said: ${out:-<nothing>}"
  fi
else
  fail "mutant control: could not strip '|| exit 1' from truthlag_pins_test.sh (its guard shape changed?) — that check was not exercised"
fi
cp "$ROOT/hack/docs/truthlag_pins_test.sh" "$MUT/hack/docs/truthlag_pins_test.sh"

# (h) the SOURCE line, in both of its spellings. `.` and `source` are the same
#     builtin; the pattern recognised only `.`, so an include written
#     `source "…/require-bash.sh"` with no `|| exit 1` was invisible — and that
#     line is the original fail-open itself: without `-e`, a failed include just
#     returns non-zero and execution carries straight on into the construct being
#     guarded. Every file in the tree writes `.` today, so the population check
#     could never have noticed; only this probe can.
for inc in '.' 'source'; do
  case "$inc" in
  '.') inc_re='\.' ;;
  *) inc_re="$inc" ;;
  esac
  sed -E 's#^\. (".*require-bash\.sh") \|\| exit 1$#'"$inc"' \1#' \
    "$ROOT/hack/docs/truthlag_pins_test.sh" >"$MUT/hack/docs/truthlag_pins_test.sh"
  if ! grep -Eq "^${inc_re}[[:space:]]+\".*require-bash\.sh\"\$" "$MUT/hack/docs/truthlag_pins_test.sh"; then
    fail "mutant control: could not rewrite truthlag_pins_test.sh's include as '${inc} …' without '|| exit 1' (its guard shape changed?) — the ${inc}-spelling check was not exercised"
    continue
  fi
  out="$(grade_root "$MUT" "noexit-${inc}")"
  if echo "$out" | grep -q "^hack/docs/truthlag_pins_test.sh: guard line without '|| exit 1'"; then
    pass "mutant control: a '${inc}' include of require-bash.sh without '|| exit 1' is reported"
  else
    fail "mutant control: a '${inc}' include of require-bash.sh without '|| exit 1' was NOT reported — a failed include returns non-zero and, with no '-e', execution continues into the guarded construct. grade_root said: ${out:-<nothing>}"
  fi
  #   The acceptance direction, so the report above is a discrimination and not
  #   the include spelling being rejected outright.
  sed -E 's#^\. (".*require-bash\.sh") \|\| exit 1$#'"$inc"' \1 || exit 1#' \
    "$ROOT/hack/docs/truthlag_pins_test.sh" >"$MUT/hack/docs/truthlag_pins_test.sh"
  out="$(grade_root "$MUT" "ok-${inc}")"
  if [ -z "$out" ]; then
    pass "mutant control: a '${inc}' include WITH '|| exit 1' is accepted"
  else
    fail "mutant control: a '${inc}' include carrying its '|| exit 1' was reported as a violation, so the report above is about the spelling, not the missing exit: $out"
  fi
done
cp "$ROOT/hack/docs/truthlag_pins_test.sh" "$MUT/hack/docs/truthlag_pins_test.sh"

# (e) the hand-rolled-guard acceptance path. `hack/audit/aud2_exitgate_test.sh`
#     is accepted on the strength of a BASH_VERSINFO expression rather than a
#     require_bash call, and that is the ONLY branch of grade_root the controls
#     above never touch. Here the guard block is replaced by a bare COMMENT
#     mentioning BASH_VERSINFO: a mention is not a guard, and it must still be
#     flagged. Also asserted in the other direction, so the acceptance itself is
#     shown to work rather than merely to be lenient.
strip_guard "$ROOT/hack/docs/truthlag_pins_test.sh" "$WORK/nogua.sh"
{
  echo "# TODO: add a BASH_VERSINFO check here one day"
  cat "$WORK/nogua.sh"
} >"$MUT/hack/docs/truthlag_pins_test.sh"
out="$(grade_root "$MUT" versinfo-comment)"
if echo "$out" | grep -q "^hack/docs/truthlag_pins_test.sh: uses .* declares NO bash version floor"; then
  pass "mutant control: a COMMENT mentioning BASH_VERSINFO is not accepted as a guard"
else
  fail "mutant control: a bare '# … BASH_VERSINFO …' comment was accepted as a version guard — the hand-rolled-guard branch cannot fail, so aud2_exitgate_test.sh's green above means nothing. grade_root said: ${out:-<nothing>}"
fi
#     The half that was missing, and the one that mattered: a NON-COMMENT line
#     merely naming the token is not a guard either. Rejecting only comments left
#     `echo "note: BASH_VERSINFO is nice to have"` beside an unguarded
#     `declare -A` passing the entire gate at exit 0 — the D-154 shape walking
#     straight past the gate built to stop it.
{
  echo 'echo "note: BASH_VERSINFO is nice to have"'
  cat "$WORK/nogua.sh"
} >"$MUT/hack/docs/truthlag_pins_test.sh"
out="$(grade_root "$MUT" versinfo-mention)"
if echo "$out" | grep -q "^hack/docs/truthlag_pins_test.sh: uses .* declares NO bash version floor"; then
  pass "mutant control: a non-comment MENTION of BASH_VERSINFO is not accepted as a guard"
else
  fail "mutant control: a non-comment line merely naming BASH_VERSINFO was accepted as a version guard — any file that so much as echoes the token can carry an unguarded bash 4 construct past this gate. grade_root said: ${out:-<nothing>}"
fi
#     The rest of the shapes that are not guards. Rejecting only a bare mention
#     left the predicate matching any comparison-ish CHARACTER anywhere to the
#     right of the token, and `>` is such a character: `echo "…BASH_VERSINFO…"
#     >&2` — precisely what a hand-rolled guard looks like once its `if` has been
#     deleted, i.e. the mutation this branch exists to catch — was accepted, as
#     were a redirect to a file, an append, and a variable merely NAMED after the
#     token. Each is pinned here by shape, since each passed before.
#
#     The last four are the reversed-form regression: a pattern accepting
#     `[ 4 -gt "${BASH_VERSINFO[0]}" ]` also accepted a redirection whose
#     DESTINATION is a variable named after the token (`2> "$BASH_VERSINFO_LOG"`),
#     because `2>` is a digit followed by an operator. They are pinned as
#     rejections so that spelling cannot be reintroduced without a control.
for nonguard in \
  'echo "note: BASH_VERSINFO is nice to have" >&2' \
  'echo "${BASH_VERSINFO[0]}" > /tmp/versinfo.txt' \
  'printf "%s\n" "${BASH_VERSINFO[0]}" >>/tmp/versinfo.log' \
  'BASH_VERSINFO_NOTE=1' \
  'msg="BASH_VERSINFO note"; other=1' \
  'command foo 2> "$BASH_VERSINFO_LOG"' \
  'some_cmd 2>${BASH_VERSINFO_TRACE}' \
  'printf "%s\n" 4 > "$BASH_VERSINFO_FILE"' \
  'echo "bash 5 > $BASH_VERSINFO"'; do
  {
    echo "$nonguard"
    cat "$WORK/nogua.sh"
  } >"$MUT/hack/docs/truthlag_pins_test.sh"
  out="$(grade_root "$MUT" versinfo-nonguard)"
  if echo "$out" | grep -q "^hack/docs/truthlag_pins_test.sh: uses .* declares NO bash version floor"; then
    pass "mutant control: '${nonguard}' is not accepted as a version guard"
  else
    fail "mutant control: '${nonguard}' was accepted as a version guard — it contains no comparison against a version, so the hand-rolled-guard branch grades a character, not a guard. grade_root said: ${out:-<nothing>}"
  fi
done
#     And the acceptance direction, for every guard SHAPE the tree actually uses
#     or plausibly could. The first line is verbatim hack/audit/aud2_exitgate_test.sh:71,
#     the file whose green depends entirely on this branch.
for realguard in \
  'if ((BASH_VERSINFO[0] < 4 || (BASH_VERSINFO[0] == 4 && BASH_VERSINFO[1] < 3))); then exit 1; fi' \
  'if ((BASH_VERSINFO[0] < 4)); then exit 1; fi' \
  '[ "${BASH_VERSINFO[0]}" -lt 4 ] && exit 1' \
  '[ "${BASH_VERSINFO[0]}" -ge 4 ] || exit 1'; do
  {
    echo "$realguard"
    cat "$WORK/nogua.sh"
  } >"$MUT/hack/docs/truthlag_pins_test.sh"
  out="$(grade_root "$MUT" versinfo-code)"
  if [ -z "$out" ]; then
    pass "mutant control: a real BASH_VERSINFO comparison IS accepted as a guard: ${realguard}"
  else
    fail "mutant control: a real BASH_VERSINFO guard was rejected, so hack/audit/aud2_exitgate_test.sh would red for having the guard it actually has. Shape: ${realguard} — grade_root said: $out"
  fi
done

# (i) the shapes this file DOCUMENTS as unrecognised are graded, not asserted in
#     prose. The header block used to carry a hand-maintained list introduced by
#     "if you hit that message on a file you know is guarded, this list is why" —
#     a completeness claim with nothing behind it. Every entry in it was true;
#     the framing was not, and `case "${BASH_VERSINFO[0]}" in …` — a real guard,
#     really rejected — was missing from it for as long as the list was prose
#     (N6/D-169). Prose cannot be kept honest by review alone, so each entry is
#     now asserted rejected END-TO-END through grade_root, the same path
#     aud2_exitgate_test.sh's green depends on. The acceptance loop directly
#     above is this loop's partner: without it, a has_versinfo_guard that
#     rejected everything would satisfy every assertion here.
for unrec in "${GUARD_SHAPES_UNRECOGNISED[@]}"; do
  {
    echo "$unrec"
    cat "$WORK/nogua.sh"
  } >"$MUT/hack/docs/truthlag_pins_test.sh"
  if ! bash -n "$MUT/hack/docs/truthlag_pins_test.sh" 2>/dev/null; then
    fail "documented-unrecognised control: the mutant carrying '${unrec}' does not parse, so whatever grade_root says about it is for the wrong reason"
    continue
  fi
  out="$(grade_root "$MUT" versinfo-unrecognised)"
  if echo "$out" | grep -q "^hack/docs/truthlag_pins_test.sh: uses .* declares NO bash version floor"; then
    pass "documented-unrecognised control: '${unrec}' fails closed, exactly as the header says it does"
  else
    fail "documented-unrecognised control: '${unrec}' IS accepted as a version guard, but this file documents it as unrecognised — the documentation and the predicate have drifted apart, which is the N6 defect reopened. grade_root said: ${out:-<nothing>}"
  fi
done
cp "$ROOT/hack/docs/truthlag_pins_test.sh" "$MUT/hack/docs/truthlag_pins_test.sh"

# (g) `readarray` IS `mapfile` — the same builtin under two names — so `-d` needs
#     the same 4.4 floor under either spelling. Graded 4.0, a script writing
#     `readarray -d ''` behind a declared 4.0 floor passes this gate green and
#     still dies on bash 4.0-4.3. Both option positions are pinned because the
#     tree has no `readarray -d` occurrence to pin them for us.
for probe_line in "readarray -d '' -t arr" "readarray -t -d '' arr"; do
  {
    echo '#!/usr/bin/env bash'
    echo 'set -uo pipefail'
    echo "$probe_line"
  } >"$MUT/hack/n1probe.sh"
  out="$(grade_root "$MUT" readarray-d)"
  if echo "$out" | grep -q '^hack/n1probe.sh: uses .*needs >= 4.4'; then
    pass "mutant control: '${probe_line}' is graded 4.4, not 4.0 (readarray is mapfile)"
  else
    fail "mutant control: '${probe_line}' was NOT graded 4.4 — readarray -d is being graded on mapfile's 4.0 floor, so a 4.0 floor on a 4.4 construct passes green. grade_root said: ${out:-<nothing>}"
  fi
  rm -f "$MUT/hack/n1probe.sh"
done

# (f) `-d` in a LATER option position must still read as 4.4. The tree writes
#     `mapfile -d '' -t refs`; `mapfile -t -d '' refs` means exactly the same
#     thing, and an option-order-sensitive pattern grades it plain 4.0 — a 4.0
#     floor on a 4.4 construct, passing green. (`mapfile -u 3 -d ''`, where an
#     earlier option takes an argument, is still missed; that is in CANNOT-SEE.)
{
  echo '#!/usr/bin/env bash'
  echo 'set -uo pipefail'
  echo "mapfile -t -d '' arr"
} >"$MUT/hack/f3probe.sh"
out="$(grade_root "$MUT" mapfile-d-late)"
if echo "$out" | grep -q '^hack/f3probe.sh: uses .*needs >= 4.4'; then
  pass "mutant control: 'mapfile -t -d' (option-order variant) is graded 4.4, not 4.0"
else
  fail "mutant control: 'mapfile -t -d' was NOT graded 4.4 — the -d pattern is option-order sensitive, so a 4.0 floor on a 4.4 construct passes green. grade_root said: ${out:-<nothing>}"
fi
rm -f "$MUT/hack/f3probe.sh"

echo
echo "== (2b) hack/lib/require-bash.sh BEHAVES — host-independent, so CI runs it too =="

# Sections (1) and (2) grade TEXT; they never call the function. Without the
# probes below the library was exercised behaviourally ONLY by section (3), which
# is macOS-only — so on CI, the one place this runs on every PR, gutting
# `require_bash()` to `return 0` left this gate at exit 0 and the guards in the
# tree decorative. These probes need no particular bash: they assert the two ends
# of the comparison against whatever shell is running them.
# shellcheck source=hack/lib/require-bash.sh
if . "$ROOT/hack/lib/require-bash.sh"; then
  pass "hack/lib/require-bash.sh sources cleanly under bash ${BASH_VERSION}"
else
  fail "hack/lib/require-bash.sh could not be sourced — every guard in the tree resolves to it"
fi

if command -v require_bash >/dev/null 2>&1; then
  rb_err="$WORK/rb.err"

  require_bash 99.0 "a floor no bash satisfies" 2>"$rb_err"
  rb_rc=$?
  if [ "$rb_rc" -ne 0 ]; then
    pass "require_bash 99.0 returns non-zero on bash ${BASH_VERSION} — the refusal path is live"
  else
    fail "require_bash 99.0 returned 0 — the library accepts a floor NO bash satisfies, i.e. it is a no-op and every guard in the tree is decorative"
  fi
  if grep -q 'requires bash >= 99.0' "$rb_err"; then
    pass "require_bash 99.0 names the floor it refused, on stderr"
  else
    fail "require_bash 99.0 refused silently — a guard that fails without naming its floor sends every reader hunting. stderr was: $(cat "$rb_err")"
  fi

  require_bash 1.0 "a floor every bash satisfies" 2>/dev/null
  rb_rc=$?
  if [ "$rb_rc" -eq 0 ]; then
    pass "require_bash 1.0 returns 0 on bash ${BASH_VERSION} — the accept path is live, so the refusal above is a discrimination and not a stuck answer"
  else
    fail "require_bash 1.0 returned $rb_rc — the library refuses everything, which would red every guarded gate on every host"
  fi

  require_bash "" "" 2>/dev/null
  rb_rc=$?
  if [ "$rb_rc" -ne 0 ]; then
    pass "require_bash with no arguments returns non-zero — a mis-called guard must not silently pass"
  else
    fail "require_bash '' '' returned 0 — a guard whose arguments were lost in an edit would pass silently"
  fi
else
  fail "require_bash is not defined after sourcing hack/lib/require-bash.sh — the guard idiom's '|| exit 1' is the only thing standing between that and an unguarded run"
fi

echo
echo "== (3) D-154 fail-open control — guard removed => EXIT 0 with the banner absent =="

# This is the crux. A control asserting merely "the unguarded script fails" would
# pass against the two scripts that fail CLOSED and prove nothing. What has to be
# reproduced is the specific pathology: terminated early, banner never printed,
# exit status 0. It needs a REAL bash 3.2 — no modern bash can emulate it.
BASH32="${ASSENT_BASH32:-}"
if [ -z "$BASH32" ] && [ -x /bin/bash ]; then
  if [ "$(/bin/bash -c 'echo ${BASH_VERSINFO[0]}')" -lt 4 ]; then
    BASH32=/bin/bash
  fi
fi

if [ -z "$BASH32" ]; then
  skips=$((skips + 1))
  {
    echo "SKIP  ================================================================"
    echo "SKIP  D-154 fail-open control DID NOT RUN: no bash < 4 on this host."
    echo "SKIP  This is the ONLY assertion that reproduces the exit-0 pathology"
    echo "SKIP  itself; sections (1) and (2) grade text, not behaviour. On CI"
    echo "SKIP  (ubuntu, bash 5) this skip is EXPECTED and permanent — which is"
    echo "SKIP  precisely why the defect it covers survived: it is macOS-only."
    echo "SKIP  Run it on macOS, where /bin/bash is 3.2, or point ASSENT_BASH32"
    echo "SKIP  at a bash 3.2 binary. A green run of this gate on CI therefore"
    echo "SKIP  carries NO evidence about section (3)."
    echo "SKIP  ================================================================"
  } >&2
else
  echo "-- using $BASH32 ($("$BASH32" -c 'echo $BASH_VERSION'))"
  TREE="$WORK/tree"
  mkdir -p "$TREE"
  # A faithful control needs the real script in a real tree: it resolves its root
  # from ${BASH_SOURCE[0]}/../.. and reads the whole repo. bin/ and .git are
  # excluded for cost; the script rebuilds the binary it needs.
  if ! tar -cf - --exclude=./bin --exclude=./.git --exclude=./.claude . 2>/dev/null | tar -xf - -C "$TREE"; then
    fail "could not copy the tree for the bash 3.2 control"
  else
    MUTSCRIPT="$TREE/hack/docs/truthlag_pins_test.sh"
    sed -E '/^_assent_lib=/,/^require_bash[[:space:]]/d' \
      "$ROOT/hack/docs/truthlag_pins_test.sh" >"$MUTSCRIPT"
    if grep -q 'require_bash' "$MUTSCRIPT"; then
      fail "the bash 3.2 control's mutation did not remove the guard — the run below would prove nothing"
    else
      mo="$WORK/mut.out"
      me="$WORK/mut.err"
      (cd "$TREE" && "$BASH32" hack/docs/truthlag_pins_test.sh) >"$mo" 2>"$me"
      mrc=$?
      npass="$(grep -c '^PASS' "$mo" | tr -d '[:space:]')"
      banner=0
      grep -q 'OK: all truth-lag pins green' "$mo" && banner=1

      if [ "$mrc" -eq 0 ]; then
        pass "guard removed, bash 3.2: exit status is 0 (the fail-open reproduced)"
      else
        fail "guard removed, bash 3.2: expected exit 0 (the fail-open), got $mrc — this control no longer reproduces D-154's pathology, so the fix's value is unmeasured. stderr tail: $(tail -2 "$me")"
      fi
      if [ "$banner" -eq 0 ]; then
        pass "guard removed, bash 3.2: the final 'OK:' banner is ABSENT — it exited 0 without finishing"
      else
        fail "guard removed, bash 3.2: the final banner WAS printed, so the script ran to completion and the exit 0 above is honest — the control is measuring the wrong thing"
      fi
      if [ "$npass" -ge 1 ]; then
        pass "guard removed, bash 3.2: $npass PASS line(s) printed before it died — 'terminated early yet reported success', measured"
      else
        fail "guard removed, bash 3.2: no PASS line was printed, so the script died at the very start for some unrelated reason and the exit-0 assertions above are satisfied trivially"
      fi
      if grep -q 'unbound variable' "$me"; then
        pass "guard removed, bash 3.2: stderr names the unbound-variable death (the associative-array subscript, D-154's mechanism)"
      else
        fail "guard removed, bash 3.2: stderr does not mention 'unbound variable' — it died of something other than the mechanism this gate is about. stderr tail: $(tail -2 "$me")"
      fi
    fi

    # The other half: with the guard in place the same shell refuses, loudly, and
    # non-zero. Asserted for all three scripts, because only truthlag exhibits the
    # exit-0 shape and the fix has to hold for the other two as well.
    for g in "${GUARDED[@]}"; do
      go="$WORK/guarded.out"
      "$BASH32" "$ROOT/$g" >"$go" 2>&1
      grc=$?
      if [ "$grc" -ne 0 ] && grep -q 'requires bash >=' "$go"; then
        pass "guard present, bash 3.2: $g refuses (exit $grc) and names its floor"
      else
        fail "guard present, bash 3.2: $g exited $grc without a 'requires bash >=' refusal — the guard did not fire"
      fi
    done
  fi
fi

echo
if [ "$skips" -ne 0 ]; then
  echo "NOTE: $skips control(s) skipped — see the SKIP block on stderr; this run does not cover them." >&2
fi
if [ "$fails" -ne 0 ]; then
  echo "FAILED: $fails bash-version-guard check(s)" >&2
  FINISHED=1
  exit 1
fi
if [ "$skips" -ne 0 ]; then
  # The success line names the gap: a green stdout must never be readable as
  # "the fail-open control passed" when it never ran.
  echo "OK: bash version guards present and sufficient — but $skips control(s) SKIPPED; the D-154 exit-0 reproduction did NOT run here"
else
  echo "OK: bash version guards present, sufficient, and proven able to fail"
fi
FINISHED=1
