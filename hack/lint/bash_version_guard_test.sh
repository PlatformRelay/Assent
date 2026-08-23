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
#       must red, not silently match nothing and pass;
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
#            `mapfile` (and `mapfile -d`, which is 4.4 rather than 4.0),
#            `readarray`, `wait -n` and `coproc` — each ONLY when it is the first
#            word on its line (optional leading whitespace). Every real occurrence
#            in this repo has that shape, and anchoring at a command position is
#            what lets this file carry the patterns without matching itself.
#
#   CANNOT SEE: the same constructs after `&&`, `||`, `;`, `then`, in an `if`/
#            `while` head, behind `eval`, or assembled from a variable; `mapfile
#            -d` when an option BEFORE it takes an argument (`mapfile -u 3 -d ''`
#            reads as plain 4.0 `mapfile`, so a 4.0 floor on a 4.4 construct would
#            pass green there; `-d` in a later option position with no intervening
#            argument, e.g. `mapfile -t -d '' arr`, IS caught — control (f)); bash 4
#            EXPANSIONS — `${v^^}`, `${v,,}`, `${!prefix@}`, `**` globstar,
#            `&>>` — because detecting those needs unanchored patterns that this
#            file could not carry without flagging itself; non-`.sh` files;
#            anything outside `hack/`; and the numeric floor of a hand-rolled
#            `BASH_VERSINFO` guard (such a guard is accepted as PRESENT, but its
#            number is not read, so `hack/audit/aud2_exitgate_test.sh` is graded
#            on having a guard, not on its value).
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
# scan_features <root> — one line per feature-using script:
#   <path relative to root>|<highest minimum required>|<features found>
scan_features() { # <root>
  local root="$1" f rel spec pat min desc need feats
  while IFS= read -r f; do
    rel="${f#"$root"/}"
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
  done < <(find "$root/hack" -type f -name '*.sh' | sort)
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
# `declare -A` passed the whole gate at exit 0. Both halves of control (e) below
# pin this: a comment is rejected, a non-comment MENTION is rejected, and a real
# comparison is accepted.
#
# Deliberately loose about the SHAPE of the comparison (`((BASH_VERSINFO[0] < 4))`,
# `[ "${BASH_VERSINFO[0]}" -lt 4 ]`, `-ge`, `<`, `>` …) and blind to the number, as
# documented in CANNOT-SEE: this branch grades that a version comparison exists,
# never that its floor is the right one.
has_versinfo_guard() {
  grep -Eq '^[^#]*BASH_VERSINFO[^#]*(-lt|-le|-gt|-ge|-eq|-ne|<|>|=)' "$1" 2>/dev/null
}

# Lines of the guard idiom that are missing their `|| exit 1`. The callers run
# `set -u` with no `-e`: without it a failed source returns non-zero and execution
# continues into the guarded construct, and an undefined require_bash returns 127
# and does the same. That is the original fail-open, one line up.
unterminated_guard_lines() { # <file>
  grep -E '^[[:space:]]*(require_bash[[:space:]]|\.[[:space:]].*require-bash\.sh)' "$1" |
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

for clean in "${MUST_BE_CLEAN[@]}"; do
  # An assertion of ABSENCE from the scan passes for two indistinguishable
  # reasons: the file really is clean, or the path no longer names a file the
  # scan ever looks at (renamed, moved, deleted). Every other check here has an
  # anti-vacuity partner; this is its one.
  if [ ! -f "$ROOT/$clean" ]; then
    fail "$clean does not exist — its 'is bash-3.2-clean' assertion would pass forever about a file that is not there"
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
{
  echo 'if ((BASH_VERSINFO[0] < 4)); then exit 1; fi'
  cat "$WORK/nogua.sh"
} >"$MUT/hack/docs/truthlag_pins_test.sh"
out="$(grade_root "$MUT" versinfo-code)"
if [ -z "$out" ]; then
  pass "mutant control: a real BASH_VERSINFO expression IS accepted as a guard (the aud2 precedent)"
else
  fail "mutant control: a real BASH_VERSINFO guard was rejected, so hack/audit/aud2_exitgate_test.sh would red for having the guard it actually has: $out"
fi
cp "$ROOT/hack/docs/truthlag_pins_test.sh" "$MUT/hack/docs/truthlag_pins_test.sh"

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
