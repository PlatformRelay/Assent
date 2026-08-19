#!/usr/bin/env bash
# REQ-AUD2-S05-01..05 — the P5-AUD2 exit gate for the 2026-08-18 project audit's
# "Next (risk reduction)" wave.
#
# ONE invocation asserts that all four AUD2 remediations are still present, and
# every failure names the audit finding ID it reopens:
#
#   REL-01/02/07 (AUD2-S01) — internal/provider/transport.go's CallExec bounds
#                 child stdout at MaxResponseBytes, sets cmd.WaitDelay, and
#                 captures stderr into the returned error.
#   REL-03       (AUD2-S02) — cmd/assent/provider_host.go's resolveRunFacts
#                 discriminates forge.ErrNotFound from every other FileAtRef
#                 error on the provider-declaration fetch.
#   SEC-03       (AUD2-S03) — hack/install.sh pins the cosign signer identity and
#                 OIDC issuer, byte-identically with SECURITY.md.
#   TEST-02      (AUD2-S04) — internal/compare's isStricterInterventionEffect
#                 still carries EffectChallenge, and the named unit case that
#                 kills the auditor's demonstrated mutant still exists.
#
# WHAT THIS GATE COSTS, corrected. It was a pure-text gate for five rounds and
# that sentence outlived the fact: it now RUNS `go test` twice, against reverted
# copies of the tree, and takes ~50s (measured 52s here, 38.6s by the reviewer) with the Go toolchain and
# git on PATH. `--text-only` is the mode that is still toolchain-free, and it is
# explicitly NOT what `task check` or CI invoke — both are asserted to pass no
# arguments, because that mode certifies nothing about REL-03/REL-07.
#
# WHY IT RUNS IN THE PR-VISIBLE JOB. Not because it is cheap — because the
# remediations must not be revertible behind a green PR. `task test` already
# runs these tests, but what tests do NOT do is survive their own deletion or
# hollowing: TEST-02 is precisely the finding that a fix can be reverted with
# every wired gate still green. So the gate lives in the `verify` job
# (REQ-AUD2-S05-05) rather than only in the push-only release-exitgate job
# (RELSE-08, the blind spot that hid AUD-S18's stale pin for four merges), and
# it is placed AFTER actions/setup-go there, since it needs a Go toolchain.
#
# SCOPING IS THE WHOLE GAME HERE. Three of the four assertions have a decoy in
# the very same file:
#   * MaxResponseBytes also appears in CallHTTP, twenty lines above CallExec;
#   * errors.Is(err, forge.ErrNotFound) also appears in
#     loadResourceOwnerRegistry, the D-130 guard further down provider_host.go;
#   * EffectChallenge also appears throughout the compare test files.
# A file-level grep for any of those stays GREEN with the remediation reverted.
# Every assertion below therefore reads a FUNCTION BODY extracted by name, with
# a positive control (the body is non-empty and contains a known-present anchor)
# and a scoping control (the body does NOT contain the neighbouring decoy
# function's header) so a broken extraction fails loudly instead of vacuously.
#
# ANTI-VACUITY DISCIPLINE (D-124, AUD-S18: this repo has a documented history of
# gates that cannot fail). Every check is a FUNCTION over file arguments, run
# twice: once against the real tree (must be GREEN) and once against a mutant
# copy carrying the very defect it exists to catch (must be RED, and red for its
# STATED reason — expect_red pins the message fragment). The mutations are
# SURGICAL: the REL-03 mutant deletes only resolveRunFacts' guard and the gate
# then asserts the decoy at loadResourceOwnerRegistry is still there, which is
# what distinguishes a scoped assertion from one that merely happens to pass.
#
# Mutants are file COPIES under mktemp -d. `git checkout --` is never used: it
# reverts to HEAD, which can be behind the working tree, so a "mutation" would
# silently be a no-op or, worse, a real edit to the tree.
set -euo pipefail

# BASH >= 4.3 REQUIRED, ASSERTED FIRST — before anything else can partially run.
# This script uses namerefs (`local -n`) to pass array names into check functions.
# Stock macOS /bin/bash is 3.2, which has no namerefs: it aborted at the first
# nameref, after one section header had already printed, and EXITED 0. A tree
# with every AUD2 remediation reverted would then pass `task check` stage 19 for
# anyone whose `bash` resolves to 3.2 — and AGENTS.md rule 4 makes a green local
# `task check` a per-commit precondition, so that is not a theoretical path. CI
# is ubuntu/bash 5, so nothing merges through it; the anti-vacuity gate silently
# certifying nothing on a maintainer's machine is the defect regardless.
if ((BASH_VERSINFO[0] < 4 || (BASH_VERSINFO[0] == 4 && BASH_VERSINFO[1] < 3))); then
  echo "FAIL: hack/audit/aud2_exitgate_test.sh requires bash >= 4.3 (namerefs); this is bash ${BASH_VERSION}." >&2
  echo "  Refusing to run rather than aborting mid-way and exiting 0 — an exit gate that certifies nothing must say so." >&2
  echo "  On macOS: run it under a modern bash (e.g. \`brew install bash\`), which is what 'task check' and CI use." >&2
  exit 1
fi

# MODES. Default: everything, including the behavioural mutation runs, which
# need the Go toolchain and git. `--text-only`: the pure-text layer plus its full
# mutation harness, for exercising the patterns under Linux/GNU grep + mawk in a
# container that has neither. The wiring checks assert that `task check` and
# verify.yaml invoke this script with NO arguments, so the flag cannot be used to
# disarm the gate where it counts (the AUD-S18 idiom, same reason).
TEXT_ONLY=0
while (($# > 0)); do
  case "$1" in
  --text-only) TEXT_ONLY=1 ;;
  *)
    echo "usage: $0 [--text-only]" >&2
    exit 2
    ;;
  esac
  shift
done

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

TRANSPORT="$ROOT/internal/provider/transport.go"
PROVIDER_HOST="$ROOT/cmd/assent/provider_host.go"
INSTALL="$ROOT/hack/install.sh"
SECURITY="$ROOT/SECURITY.md"
CLASSIFY="$ROOT/internal/compare/classify_intervention.go"
CLASSIFY_TEST="$ROOT/internal/compare/classify_intervention_test.go"
TASKFILE="$ROOT/Taskfile.yml"
AUD_GATE="$ROOT/hack/audit/exitgate_test.sh"
WORKFLOW="$ROOT/.github/workflows/verify.yaml"

# This gate's own `task check` stage, and the script the stage must invoke.
STAGE="audit-aud2-exitgate-test"
SELF="hack/audit/aud2_exitgate_test.sh"
# The named unit case AUD2-S04 added; TEST-02 is closed by its existence.
CHALLENGE_TEST="TestClassifyStricterInterventionAddedChallengeEffect"

# THE PRIMARY PIN for REL-03 and REL-07, and the reason this gate stopped trying
# to re-derive those two remediations from unparsed Go source text.
#
# Four rounds of source-shape assertions were each defeated by a fresh reviewer
# within the hour, twice by idiomatic one-liners that need no camouflage at all:
# a condition NARROWING in the guard's own `if`
#     if !errors.Is(err, forge.ErrNotFound) && errors.Is(err, context.Canceled) {
# (no fall-through, return at the right indent), and a bare return whose text
# merely CONTAINS the sink identifier
#     return nil, fmt.Errorf("provider exec failed (stderr suppressed): %w", err)
# The second one is decisive: it is a plain double-quoted string literal, so no
# comment filter and no filter polarity can fix it. Recognising "does this
# function still fail closed" from text means re-implementing Go's type checker
# and data flow; the space of spellings does not shrink as patches accumulate.
#
# So the property is pinned the way TEST-02 already pins its own — BY TEST NAME —
# and the source-shape checks are demoted to a secondary heuristic that names the
# defect precisely on the shapes it does recognise.
#
# WHAT THESE NAME PINS ARE, PRECISELY — corrected after round five claimed more.
# "A revert that keeps the tests is caught by `go test`; one that deletes them is
# caught by the name pin" was presented as a partition and is not one: KEEPING
# the names while emptying the bodies is a third case, and it defeated both
# halves. These pins are therefore a fast, precisely-worded pre-check — they say
# WHICH test or case was lost, which a go-test transcript does not — and the
# behavioural mutation run below is the actual property.
REL03_TEST_FILE="cmd/assent/provider_host_test.go"
REL03_TESTS=(
  TestProviderDeclarationForgeErrorAbortsResolveRunFacts
  TestProviderDeclarationUnauthorizedAbortsResolveRunFacts
)
# Case-level pins, matched VERBATIM (grep -F) rather than auto-quoted, because
# the two files spell their cases differently: the REL-03 subtests are driven by
# `strconv.Itoa(status)` over an int table, the REL-07 ones are literal t.Run
# names. Each entry below is exactly the text that must survive.
REL03_SUBTESTS=('[]int{401, 403}')

REL07_TEST_FILE="internal/provider/transport_test.go"
REL07_TESTS=(TestExecStderrFoldedIntoError)
REL07_SUBTESTS=(
  't.Run("nonzero_exit_reports_stderr"'
  't.Run("stderr_never_merged_into_facts"'
  't.Run("runaway_stderr_is_bounded_and_truncated"'
)

# The reverts the behavioural run applies to a COPY of the tree. Each must
# COMPILE — a mutant that does not build proves nothing about behaviour, and
# check_behavioural_revert refuses a `[build failed]` transcript explicitly.
#
# REL-03: the condition narrowing that beat every source-shape heuristic. One
# line, gofmt/vet/build clean, `context` is already imported by that file.
REL03_PKG="./cmd/assent/"
REL03_SRC="cmd/assent/provider_host.go"
# INDENT-ANCHORED so the revert lands on the provider-declaration guard ONLY.
# The identical guard in loadResourceOwnerRegistry (D-130) sits at one tab; this
# one is inside `for` + `if err != nil` at three. An unanchored program reverts
# both, and the run would then be red partly for a finding this control is not
# about — check_behavioural_revert's "a PINNED test must be named in a --- FAIL:
# line" catches that, but a mutant should mean what it says.
REL03_REVERT=$'s|^\t\t\tif !errors.Is(err, forge.ErrNotFound) {|\t\t\tif !errors.Is(err, forge.ErrNotFound) \&\& errors.Is(err, context.Canceled) {|'
REL03_REVERT_WITNESS='errors.Is(err, context.Canceled)'
REL03_RUN='TestProviderDeclarationForgeErrorAbortsResolveRunFacts|TestProviderDeclarationUnauthorizedAbortsResolveRunFacts'
# THE QUORUM. Every one of these must appear in a `--- FAIL:` line when the
# revert is applied. "At least one pinned test failed" was the previous rule and
# it is weaker than the claim built on it: gutting ONLY
# TestProviderDeclarationUnauthorizedAbortsResolveRunFacts (a no-op body, the
# case table kept as live code so the text pins stay green) left the other test
# failing, satisfied the quorum, and passed the gate at exit 0 with production
# source untouched. Subtests are pinned too, so dropping just the 401 case is
# caught at case granularity rather than only at function granularity.
REL03_MUSTFAIL=(
  TestProviderDeclarationForgeErrorAbortsResolveRunFacts
  TestProviderDeclarationUnauthorizedAbortsResolveRunFacts/401
  TestProviderDeclarationUnauthorizedAbortsResolveRunFacts/403
)

# REL-07: drop the fold. `err` is in scope, so this compiles as written and is
# exactly the pre-fix behaviour — a bare "exit status 1" with no diagnostic.
REL07_PKG="./internal/provider/"
REL07_SRC="internal/provider/transport.go"
REL07_REVERT='s|return nil, execFailure(err, stderr)|return nil, err|'
REL07_REVERT_WITNESS='return nil, err'
REL07_RUN='TestExecStderrFoldedIntoError'
# ASYMMETRY, and it is deliberate: do NOT require all three REL-07 subtests.
# Under the fold-dropping revert, `stderr_never_merged_into_facts` LEGITIMATELY
# PASSES — it asserts stderr does not leak into the fact map, which stays true
# when the fold is removed. Demanding it would make the gate red for a property
# the revert does not break, i.e. an assertion that cannot be satisfied by any
# correct tree. Only the two canaries that DO detect the revert are pinned; the
# third is still held by the case pin in check_named_tests, which requires it to
# exist. Measured, not assumed: see the transcript names in the commit body.
REL07_MUSTFAIL=(
  TestExecStderrFoldedIntoError/nonzero_exit_reports_stderr
  TestExecStderrFoldedIntoError/runaway_stderr_is_bounded_and_truncated
)


WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# --------------------------------------------------------------- helpers --

# extract_func <file> <name> — the body of a top-level Go func (or method),
# from its `func` line to the first column-0 `}`. Column-0 anchoring is what
# keeps a neighbouring function out of the result.
extract_func() {
  awk -v name="$2" '
    !inf && $0 ~ "^func (\\([^)]*\\) )?"name"\\(" { inf = 1; print; next }
    inf && /^}/ { print; exit }
    inf { print }
  ' "$1"
}

# code_only — filter dropping whole-line Go comments from stdin, so an assertion
# can never be satisfied by prose that merely mentions the construct it wants.
#
# Strips BOTH comment forms. The first version stripped `//` lines only, and an
# independent reviewer immediately built a gofmt-clean, compiling half-revert
# that hid the required return inside a `/* ... */` span — so the claim "a
# return quoted in a comment cannot satisfy the assertion" was false as written.
# Block spans carry across lines, hence the state machine rather than a grep.
#
# It does not model Go string literals: a `//` or `/*` inside a quoted string
# would over-strip. That direction is fail-CLOSED (the assertion sees less code,
# never more), and no such literal exists in the region this filter is applied
# to, so the simpler scanner is the safer one.
code_only() {
  awk '
    {
      line = $0
      out = ""
      while (length(line) > 0) {
        if (inblock) {
          p = index(line, "*/")
          if (p == 0) { line = ""; break }
          line = substr(line, p + 2)
          inblock = 0
          continue
        }
        pb = index(line, "/*")
        pl = index(line, "//")
        if (pl > 0 && (pb == 0 || pl < pb)) {
          out = out substr(line, 1, pl - 1)
          line = ""
          break
        }
        if (pb > 0) {
          out = out substr(line, 1, pb - 1)
          line = substr(line, pb + 2)
          inblock = 1
          continue
        }
        out = out line
        line = ""
      }
      print out
    }
  '
}

# code_conservative — the OTHER filter, for ABSENCE assertions.
#
# THE INVERSION, stated plainly because getting it wrong trades one unfailable
# assertion for another. code_only above is tuned for PRESENCE assertions ("a
# return must appear"): over-stripping there hides a return and causes a false
# RED, which is safe. An ABSENCE assertion ("no `continue` may appear") has the
# opposite polarity: over-stripping hides the very token being hunted and causes
# a false GREEN. code_only's own documented weakness — it does not model Go
# string literals, so `u := "http://x"` truncates the line — would therefore
# become a way to smuggle a fall-through past the gate.
#
# So absence assertions use this filter instead, which can only ever keep TOO
# MUCH. It removes a construct only when the construct is unambiguous without
# parsing Go:
#   * a line whose FIRST non-blank characters are `//` — everything after a
#     leading `//` is comment, whatever quotes appear later on that line;
#   * a span whose opening line's FIRST non-blank characters are `/*`, up to and
#     including the line carrying `*/`.
# A TRAILING `// ...` comment is deliberately NOT stripped, and a block comment
# opened mid-line is deliberately NOT recognised: both would require deciding
# whether the marker sits inside a string literal, and guessing wrong in the
# stripping direction is the false-green this filter exists to rule out. The
# cost is false REDs on prose that names a fall-through in a trailing comment —
# fail-closed, and loud.
#
# Residual, named rather than assumed away: a multi-line RAW string literal
# (backticks) whose interior line begins with `//` would still be stripped.
# assert_no_raw_string below refuses any region containing a backtick, so that
# case fails closed instead of silently weakening an absence assertion.
code_conservative() {
  awk '
    inblock { if (index($0, "*/") > 0) { inblock = 0 }; next }
    {
      t = $0
      sub(/^[ \t]+/, "", t)
      if (substr(t, 1, 2) == "//") { next }
      if (substr(t, 1, 2) == "/*") {
        if (index(t, "*/") == 0) { inblock = 1 }
        next
      }
      print
    }
  '
}

# assert_no_raw_string <region-file> <finding> — code_conservative's one
# remaining ambiguity is a multi-line raw string literal. No such literal exists
# in any region this gate reads; if one ever appears, fail closed and say why
# rather than let an absence assertion quietly weaken.
assert_no_raw_string() {
  if grep -q '`' "$1"; then
    echo "  $2: the isolated region contains a backtick — a multi-line raw string literal would let code_conservative strip a line that is not a comment, which is the one way an ABSENCE assertion here can go falsely green. Extend the filter before allowing this shape (AUD2-S05)" >&2
    return 1
  fi
  return 0
}

# block_from <file> <ERE for the opening line> — the brace-matched block a line
# opens: from the first line matching the pattern to the `}` at the SAME
# indentation. gofmt (check stage 1) is what makes indentation load-bearing
# rather than cosmetic, so no brace counting is needed.
block_from() {
  awk -v re="$2" '
    !ing && $0 ~ re {
      match($0, /^[ \t]*/)
      indent = substr($0, 1, RLENGTH)
      ing = 1
      print
      next
    }
    ing {
      print
      if ($0 == indent "}") exit
    }
  ' "$1"
}

# guard_block <body-file> — the `if !errors.Is(err, forge.ErrNotFound) { … }`
# block, from its `if` line to the `}` at the SAME indentation. Brace-matched on
# indentation rather than counting braces: gofmt guarantees the closing brace of
# a block sits at the block's own indent, and the whole tree is gofmt-clean
# (`task fmt` is check stage 1).
guard_block() {
  awk '
    !ing && /if !errors\.Is\(err, forge\.ErrNotFound\)/ {
      match($0, /^[ \t]*/)
      indent = substr($0, 1, RLENGTH)
      ing = 1
      print
      next
    }
    ing {
      print
      if ($0 == indent "}") exit
    }
  ' "$1"
}

# body_of <file> <func> <out> <finding> <anchor> <next-func-header> — extract
# with all three controls. Returns non-zero (naming <finding>) if the extraction
# produced nothing, lost its anchor, or ran past the end of the function.
#
# On the <next-func-header> argument: extract_func only ever reads DOWNWARD from
# its target's `func` line, so a guard naming a function that sits ABOVE the
# target can never fire — a control that cannot fail, which is the same species
# of defect this gate exists to catch. The argument must therefore name the
# function immediately BELOW the target (or be empty when the target is the last
# function in the file, as isStricterInterventionEffect is). The `^func ` count
# below is the direction-independent backstop and is live in every case.
body_of() {
  local file="$1" fn="$2" out="$3" finding="$4" anchor="$5" next_fn="$6"
  if [[ ! -f "$file" ]]; then
    echo "  ${finding}: missing ${file#"$ROOT"/}" >&2
    return 1
  fi
  extract_func "$file" "$fn" >"$out"
  if [[ ! -s "$out" ]]; then
    echo "  ${finding}: func ${fn} was not found in ${file#"$ROOT"/} — it was renamed or deleted, and every assertion over its body would be vacuous" >&2
    return 1
  fi
  if ! grep -Fq -- "$anchor" "$out"; then
    echo "  ${finding}: the extracted ${fn} body does not contain the known-present anchor '${anchor}' — the extraction is wrong, so the assertions below would be vacuous" >&2
    return 1
  fi
  # Direction-independent over-extraction control: a body that swallowed a
  # neighbour carries two `func` headers, whichever side the neighbour is on.
  local n_func
  n_func="$(grep -cE '^func ' "$out" || true)"
  if [[ "$n_func" -ne 1 ]]; then
    echo "  ${finding}: the extracted ${fn} body carries ${n_func} top-level 'func' header(s), not 1 — the extraction ran past the end of the function, so a same-file grep is what is really being graded, not this function" >&2
    return 1
  fi
  if [[ -n "$next_fn" ]] && grep -Fq -- "$next_fn" "$out"; then
    echo "  ${finding}: the extracted ${fn} body ALSO contains '${next_fn}', the function immediately below it — the extraction ran past the closing brace (AUD2-S05: this guard names the function BELOW the target on purpose; extract_func only reads downward, so naming one above it would be a control that cannot fire)" >&2
    return 1
  fi
  return 0
}

# rhs_of <body> <lvalue-regex> — the right-hand side of the first assignment to
# an lvalue, with any leading `&` stripped. Empty when there is no assignment.
rhs_of() {
  sed -nE "s/^[[:space:]]*$2[[:space:]]*=[[:space:]]*&?(.+)$/\1/p" "$1" | head -1
}

# delete_first <file> <ERE> — delete only the FIRST line matching the pattern,
# in place. Surgical by construction: a later decoy occurrence survives, which
# is what makes the mutation a real test of the assertion's scoping.
delete_first() {
  local file="$1" re="$2"
  awk -v re="$re" 'done != 1 && $0 ~ re { done = 1; next } { print }' "$file" >"$file.mut"
  mv "$file.mut" "$file"
}

# downgrade_first_return <file> <ERE> — replace the FIRST line matching the
# pattern with a `continue` at the same indentation. This is the HALF-REVERT
# shape: the guard stays, its body stops stopping the run. It compiles and it is
# gofmt-clean, which is exactly why a body-wide grep could not see it.
downgrade_first_return() {
  local file="$1" re="$2"
  awk -v re="$re" '
    done != 1 && $0 ~ re {
      match($0, /^[ \t]*/)
      print substr($0, 1, RLENGTH) "continue"
      done = 1
      next
    }
    { print }
  ' "$file" >"$file.mut"
  mv "$file.mut" "$file"
}

# splice_first_return <file> <replacement-file> — replace the FIRST
# `return nil, nil, fmt…` line with the replacement file's lines, each prefixed
# with the replaced line's own indentation (so the replacement is written
# RELATIVE, with its own leading tabs for nesting). Used to build the two
# camouflaged half-reverts: a return hidden in a block comment, and a return
# reachable only under a nested condition. Both compile and are gofmt-clean.
splice_first_return() {
  local file="$1" rep="$2"
  awk '
    FNR == NR { line[++n] = $0; next }
    done != 1 && $0 ~ /^[[:space:]]*return nil, nil, fmt/ {
      match($0, /^[ \t]*/)
      ind = substr($0, 1, RLENGTH)
      for (i = 1; i <= n; i++) print ind line[i]
      done = 1
      next
    }
    { print }
  ' "$rep" "$file" >"$file.mut"
  mv "$file.mut" "$file"
}

# mutate <file> <sed-program> <witness> — sed in place, then prove the mutation
# actually landed. A mutant identical to the clean input proves nothing.
mutate() {
  local file="$1" program="$2" witness="$3" before after
  before="$(cat "$file")"
  sed "$program" "$file" >"$file.mut"
  mv "$file.mut" "$file"
  after="$(cat "$file")"
  [[ "$before" != "$after" ]] ||
    fail "mutation harness: sed '$program' did not change ${file#"$WORK"/} — the mutant equals the clean input, so any 'the check fires' conclusion would be false"
  grep -Fq -- "$witness" "$file" ||
    fail "mutation harness: expected '$witness' in ${file#"$WORK"/} after mutation, but it is absent"
}

# ============================================================================
# BEHAVIOURAL MUTATION RUN — the primary evidence for REL-03 and REL-07
# ============================================================================
#
# WHY THIS EXISTS. Round five pinned these two remediations by TEST NAME, on the
# reasoning that `go test` catches a revert that keeps the tests and the name pin
# catches one that deletes them. A reviewer then kept the names and emptied the
# bodies —
#     func TestProviderDeclarationForgeErrorAbortsResolveRunFacts(t *testing.T) {
#         t.Skip("temporarily disabled")
#     }
# — applied the one-line condition narrowing, and got `gofmt` silent, `go build`
# OK, `go vet` OK, `go test ./cmd/assent/` OK and this gate GREEN, with REL-03
# fully reverted. A NAME IS NOT A BEHAVIOUR, and "keeps the tests" vs "deletes
# the tests" was never a partition: gutting them is a third case that both halves
# missed. `t.Skip("flaky")` is a routine event, not a contrivance. There is also
# no third backstop for REL-03: cmd/ sits outside ./internal/..., so the coverage
# floor moves not at all when those tests are gutted.
#
# So the property is measured, not read: copy the tree, apply the revert to the
# COPY, run the pinned tests against the copy, and require them to FAIL. That
# terminates the class — it tests behaviour rather than text or names, and it is
# indifferent to how either the production revert or the test gutting is spelled.
#
# Cost, measured: the tree copy is ~1,200 tracked files / 8.6 MB and takes well
# under a second; each `go test -run <pinned>` over one package takes ~1-2s. Both
# findings together add ~4s to a stage that already runs after `task test`.
#
# NOTHING IS WRITTEN TO THE REAL TREE. Every mutation lands in $WORK.

# mutation_tree <tag> — a throwaway copy of the WORKING TREE's tracked files.
# Tracked-only (git ls-files) to skip .git and build junk, but the CONTENT comes
# from the working tree, so the copy grades what is about to be committed rather
# than what HEAD already holds.
mutation_tree() {
  local dir="$WORK/tree-$1"
  if [[ -d "$dir" ]]; then
    printf '%s\n' "$dir"
    return 0
  fi
  mkdir -p "$dir"
  if [[ ! -s "$WORK/tracked.tar" ]]; then
    (cd "$ROOT" && git ls-files -z | tar -cf - --null -T -) >"$WORK/tracked.tar"
  fi
  tar -xf "$WORK/tracked.tar" -C "$dir"
  printf '%s\n' "$dir"
}

# drop_unused_imports <tree> <pkg> <file-rel> — emptying a test body orphans the
# imports it used, and Go rejects an unused import. The reviewer's own mutant
# "dropped the now-unused imports"; this does the same thing deterministically
# instead of hardcoding a list: compile, read back the `"x" imported and not
# used` diagnostics, delete exactly those import lines, repeat. Bounded, and it
# returns quietly on any other error so the caller still grades the real
# transcript rather than this loop's opinion of it.
drop_unused_imports() {
  local tree="$1" pkg="$2" rel="$3" i out unused imp
  for ((i = 0; i < 12; i++)); do
    out="$WORK/imports.$$.$i"
    (cd "$tree" && go build "$pkg" && go vet "$pkg") >"$out" 2>&1 && return 0
    unused="$(sed -nE 's/.*"([^"]+)" imported and not used.*/\1/p' "$out" | sort -u)"
    [[ -n "$unused" ]] || return 0
    while IFS= read -r imp; do
      [[ -n "$imp" ]] || continue
      awk -v q="\"${imp}\"" '
        {
          t = $0
          sub(/^[ \t]+/, "", t)
          if (t == q) { next }
          if (t ~ ("^[A-Za-z0-9_.]+ " q "$")) { next }
          print
        }
      ' "$tree/$rel" >"$tree/$rel.imp"
      mv "$tree/$rel.imp" "$tree/$rel"
    done <<<"$unused"
  done
  return 0
}

# go_test_named <tree> <run-regex> <pkg> <out> — `go test -v -run` in a copy.
# Returns go test's own exit code; the caller grades the transcript, never the
# exit code alone (a build failure also exits non-zero and proves nothing about
# behaviour).
go_test_named() {
  local tree="$1" run="$2" pkg="$3" out="$4" rc=0
  (cd "$tree" && go test -count=1 -timeout 120s -v -run "$run" "$pkg") >"$out" 2>&1 || rc=$?
  return "$rc"
}

# check_behavioural_revert <finding> <tag> <file-rel> <sed-program> <witness>
#                          <run-regex> <pkg> <fn-list-name> <remediation>
# The pinned tests must actually DETECT the revert. Three graded runs:
#   (1) positive control — the pinned tests PASS by name on an unmutated copy.
#       This is also what catches a gutted body that still returns success only
#       because it does nothing: see the emptiness probe below.
#   (2) the revert applied to a second copy — the pinned tests must FAIL, and
#       fail as TESTS: a `[build failed]` transcript is refused explicitly,
#       because "the mutant does not compile" is not evidence about behaviour.
#   (3) at least one FAIL line must name a PINNED test, not some neighbour.
# Optional 10th argument OVERLAY, "<repo-relative path>=<replacement file>",
# copied into BOTH the control copy and the reverted copy before either runs.
# That is how the gutted-test controls work: they do not touch production source
# at all, they replace the test file and let the run report what that costs.
check_behavioural_revert() {
  local finding="$1" tag="$2" rel="$3" prog="$4" witness="$5" run="$6" pkg="$7" fn_list="$8" remediation="$9"
  local overlay="${10:-}"
  local mustfail_list="${11:?check_behavioural_revert: a must-fail quorum list is required}"
  local -n fns="$fn_list"
  local -n mustfail="$mustfail_list"
  local clean mut out_clean out_mut fn rc=0

  clean="$(mutation_tree "${tag}-control")"
  if [[ -n "$overlay" ]]; then
    cp "${overlay#*=}" "$clean/${overlay%%=*}"
    drop_unused_imports "$clean" "$pkg" "${overlay%%=*}"
  fi
  out_clean="$WORK/gotest.${tag}.clean"
  if go_test_named "$clean" "$run" "$pkg" "$out_clean"; then :; else
    if grep -Fq 'build failed' "$out_clean"; then
      echo "  ${finding}: the pinned tests do not BUILD on an unmutated copy of the tree — this gate cannot measure anything until that is fixed (AUD2-S05 behavioural pin)" >&2
    else
      echo "  ${finding}: the pinned tests are RED on an unmutated copy of the tree — ${remediation} is already broken, or the tests are (AUD2-S05 behavioural pin)" >&2
    fi
    sed 's/^/    /' <(grep -E "^(---|\s+---|FAIL|ok)" "$out_clean" | head -8) >&2
    return 1
  fi
  for fn in "${fns[@]}"; do
    if ! grep -Fq -- "--- PASS: ${fn} " "$out_clean"; then
      echo "  ${finding}: '${fn}' did not report '--- PASS:' on an unmutated copy — it was renamed, deleted or skipped. \`go test\` exits 0 both when a -run pattern matches nothing and when a test calls t.Skip, so grading its exit code alone would treat a disabled test as a passing one (AUD2-S05 behavioural pin)" >&2
      rc=1
    fi
  done
  ((rc == 0)) || return "$rc"

  mut="$(mutation_tree "${tag}-reverted")"
  if [[ -n "$overlay" ]]; then
    cp "${overlay#*=}" "$mut/${overlay%%=*}"
    drop_unused_imports "$mut" "$pkg" "${overlay%%=*}"
  fi
  local target="$mut/$rel"
  sed "$prog" "$target" >"$target.mut" && mv "$target.mut" "$target"
  if ! grep -Fq -- "$witness" "$target"; then
    fail "mutation harness (${finding}): the behavioural revert did not land in ${rel} — expected '${witness}' after applying the sed program, so the run below would grade an unmutated copy and pass for the wrong reason"
  fi

  out_mut="$WORK/gotest.${tag}.mut"
  if go_test_named "$mut" "$run" "$pkg" "$out_mut"; then
    echo "  ${finding}: the pinned tests still PASS with ${remediation} REVERTED in a copy of the tree — they no longer detect the defect they exist to detect. A name that survives with an emptied body (t.Skip, a no-op loop, a deleted assertion) is not a behaviour, and neither \`go test\` nor a name pin can tell the difference. This is the ONLY assertion here that measures the remediation rather than reading it (AUD2-S05 behavioural pin). Revert applied: ${witness}" >&2
    rc=1
  else
    if grep -Fq 'build failed' "$out_mut"; then
      echo "  ${finding}: the reverted copy failed to BUILD rather than failing its tests — that is not evidence about behaviour, and this control must not be allowed to pass on it. Fix the revert program so it produces compiling code (AUD2-S05 behavioural pin)" >&2
      rc=1
    else
      # QUORUM: EVERY entry of the finding's must-fail list, not "at least one".
      # The trailing space matters — `--- FAIL: X ` must not be satisfied by
      # `--- FAIL: X/401`, or a function-level pin would be met by one of its
      # own subtests.
      local missing="$WORK/quorum.${tag}.missing"
      : >"$missing"
      for fn in "${mustfail[@]}"; do
        grep -Fq -- "--- FAIL: ${fn} " "$out_mut" || printf '%s\n' "$fn" >>"$missing"
      done
      if [[ -s "$missing" ]]; then
        echo "  ${finding}: the reverted copy is red, but these pinned tests did NOT report '--- FAIL:' — they no longer detect ${remediation}, and a sibling failing in their place does not close that. Emptying ONE test's body while its siblings still fail is exactly the escape this quorum exists to refuse (AUD2-S05 behavioural pin):" >&2
        sed 's/^/      missing: /' "$missing" >&2
        echo "    what the reverted copy actually reported:" >&2
        sed 's/^/      /' <(grep -E '^[[:space:]]*--- (FAIL|PASS|SKIP):' "$out_mut" | head -8) >&2
        rc=1
      fi
    fi
  fi
  return "$rc"
}

# check_named_tests <finding> <repo-relative test file> <fn-list-name>
#                   <subtest-list-name> <remediation>
# The primary pin: the named behavioural tests still exist, by name, in the named
# file — the same idiom TEST-02 is pinned by, and the half of the composition
# that a revert-plus-delete-the-tests cannot walk around. It deliberately does
# NOT run them: `go test` is check stage 4 and a step of the same PR-visible
# `verify` job, so a revert that keeps the tests is already red before this gate
# runs. What is unique here is catching their DELETION or RENAME, which `go test`
# reports as success.
check_named_tests() {
  local finding="$1" rel="$2" fn_list="$3" sub_list="$4" remediation="$5" rc=0
  # An absolute path is a mutant copy under $WORK; a relative one is the tree.
  local file="$ROOT/$rel"
  [[ "$rel" == /* ]] && file="$rel"
  if [[ ! -f "$file" ]]; then
    echo "  ${finding}: ${rel} does not exist — the behavioural tests that hold ${remediation} closed are gone, and 'go test' reports a deleted test as success (AUD2-S05 primary pin)" >&2
    return 1
  fi
  # There is deliberately NO `grep -cE "^func Test" >= 1` corpus control here.
  # It cannot independently fail: any file satisfying a `func <pinned>(` pin
  # necessarily contains a `^func Test` line, so it would be an assertion that
  # is true whenever the assertions after it are — the D-124 species this epic
  # exists to close. The real defence against an emptied corpus is the
  # behavioural run, which reports what the tests DO.
  local -n fns="$fn_list"
  local -n subs="$sub_list"
  local fn sub
  for fn in "${fns[@]}"; do
    if ! grep -Fq "func ${fn}(" "$file"; then
      echo "  ${finding}: the named behavioural test ${fn} is absent from ${rel} — deleting or renaming it is invisible to 'go test' (a test that does not exist cannot fail), so ${remediation} would then be held closed by nothing but this gate's source-shape heuristics, which are documented as insufficient on their own (AUD2-S05 primary pin)" >&2
      rc=1
    fi
  done
  # F4: the case pins read a COMMENT-STRIPPED view. `[]int{401, 403}` was
  # satisfied out of a Go comment by a gutted test whose body had been replaced
  # with `// coverage note: this used to iterate []int{401, 403}` — every
  # production-source assertion in this script already filters comments, and
  # this one did not. code_conservative (not code_only) because these are
  # PRESENCE assertions over a test file: over-stripping causes a false RED.
  local code_view="$WORK/testfile.code.$(basename "$rel")"
  code_conservative <"$file" >"$code_view"
  if [[ ! -s "$code_view" ]]; then
    echo "  ${finding}: the comment-stripped view of ${rel} is empty — the filter is over-stripping, so the case pins below would be graded against nothing" >&2
    return 1
  fi
  for sub in "${subs[@]}"; do
    if ! grep -Fq -- "$sub" "$code_view"; then
      echo "  ${finding}: the case pin '${sub}' is absent from the CODE of ${rel} — a case can be dropped without touching any test function name, which narrows the coverage that holds ${remediation} closed while every function-level pin stays green. Comments do not count: a gutted body that leaves the table behind in prose satisfies nothing here (AUD2-S05 primary pin)" >&2
      rc=1
    fi
  done
  return "$rc"
}

# assign_count <region-file> <lvalue-ERE> — how many times an lvalue is
# assigned in a region. rhs_of reads the FIRST assignment; at runtime the LAST
# one wins, so "assigned exactly once" is what makes reading the first sound.
assign_count() {
  grep -Ec "^[[:space:]]*$2[[:space:]]*:?=" "$1" || true
}

# assert_assignment_gone <mutant> <lvalue-ERE> <finding> — the mutation really
# removed the ASSIGNMENT, not some prose mention of it. Deleting a doc comment
# also "changes the file", which is why assert_changed alone is not enough.
assert_assignment_gone() {
  ! grep -Eq "^[[:space:]]*$2[[:space:]]*=" "$1" ||
    fail "mutation harness ($3): ${1#"$WORK"/} still assigns $2 — the deletion hit a different line (a comment, most likely) and the control would grade the wrong thing"
}

# assert_changed <clean> <mutant> — the mutant really differs from its source.
assert_changed() {
  ! diff -q "$1" "$2" >/dev/null 2>&1 ||
    fail "mutation harness: ${2#"$WORK"/} is byte-identical to its source — the mutation did not land"
}

expect_green() { # <check-fn> <label> <args...>
  local fn="$1" label="$2"
  shift 2
  "$fn" "$@" || fail "$label — the real tree violates $fn (findings above): an AUD2 remediation is missing"
  echo "OK: $label"
}

# A control that reds is not yet a control that WORKS — it could be red for an
# unrelated reason, which would mask the mutation going undetected. So every
# control pins the finding text it must produce.
expect_red() { # <check-fn> <label> <required stderr fragment> <args...>
  local fn="$1" label="$2" want="$3"
  shift 3
  local err="$WORK/expect_red.err"
  if "$fn" "$@" 2>"$err"; then
    fail "mutation control: $fn ACCEPTED an input that carries the reverted remediation ($label) — this check cannot fail and is therefore not a gate"
  fi
  grep -Fq -- "$want" "$err" || {
    echo "  actual finding was:" >&2
    sed 's/^/    /' "$err" >&2
    fail "mutation control: $fn went red on '$label' but NOT for its stated reason — the finding was expected to mention: $want"
  }
  MUTATIONS_PROVED=$((MUTATIONS_PROVED + 1))
  echo "OK: mutation control — $fn goes red on: $label"
}

MUTATIONS_PROVED=0

# ============================================================================
# (1) REL-01 / REL-02 / REL-07 — AUD2-S01: CallExec containment
# ============================================================================

# CallExec used to collect child stdout into an unlimited bytes.Buffer while
# CallHTTP, in the same file, was bounded fail-closed at MaxResponseBytes
# (REL-01); it set no WaitDelay, so a provider forking a grandchild that
# inherits the stdout pipe hangs Wait past the deadline (REL-02); and it never
# wired cmd.Stderr, so a failing provider yielded "exit status 1" with zero
# diagnostic content (REL-07).
check_exec_containment() { # <transport.go>
  local file="$1" rc=0
  local body="$WORK/body.CallExec"
  # Anchor: cmd.Run() is the call every version of this function has made.
  # Over-extraction guard: execFailure is the function immediately BELOW
  # CallExec. (CallHTTP, which carries the MaxResponseBytes decoy, sits ABOVE —
  # naming it here would have been a guard that can never fire. The scoping
  # proof against that decoy is the REL-01 mutation control, which leaves
  # CallHTTP's MaxResponseBytes intact and still reddens.)
  body_of "$file" CallExec "$body" "REL-01/02/07" 'cmd.Run()' 'func execFailure' || return 1

  # ---- PRIMARY (2): the named tests and their cases still exist -------------
  # PRIMARY (1), the behavioural run, is a separate top-level check — see the
  # note in check_notfound_discrimination for why it is not inlined here.
  check_named_tests "REL-07" "$REL07_TEST_FILE" REL07_TESTS REL07_SUBTESTS \
    "the stderr fold in CallExec" || rc=1

  # SINGLE-ASSIGNMENT DISCIPLINE, before anything reads a right-hand side.
  # rhs_of takes the FIRST assignment; at runtime the LAST one wins. So
  #     cmd.Stdout = stdout          // bounded, and what rhs_of reads
  #     cmd.Stdout = &bytes.Buffer{} // what actually runs
  # would pass every check below while REL-01 is fully reverted — the same
  # "the assertion reads the wrong statement" family as the REL-03 findings.
  # Each of these is assigned exactly once in the remediated function, so
  # requiring exactly one is not a new constraint on the code, only on what this
  # gate is willing to reason about.
  local lv n_assign assign_rc=0
  for lv in 'cmd\.Stdout' 'cmd\.Stderr' 'cmd\.WaitDelay'; do
    n_assign="$(assign_count "$body" "$lv")"
    if [[ "$n_assign" -gt 1 ]]; then
      echo "  REL-01/02/07: CallExec assigns ${lv//\\/} ${n_assign} times — rhs_of reads the FIRST and the LAST is what runs, so a later reassignment reverts the remediation while every check below still reads the good one (AUD2-S01)" >&2
      rc=1
      assign_rc=1
    fi
  done
  # Only the ASSIGNMENT ambiguity short-circuits: rhs_of below would otherwise
  # read a statement that does not decide anything. A failed primary pin must
  # not stop the heuristics from running and naming what else is wrong.
  ((assign_rc == 0)) || return 1

  # -- REL-01: stdout is bounded, and bounded by the SINGLE declared bound.
  local stdout_var
  stdout_var="$(rhs_of "$body" 'cmd\.Stdout')"
  if [[ -z "$stdout_var" ]]; then
    echo "  REL-01: CallExec does not assign cmd.Stdout at all — with no sink the bound cannot exist (AUD2-S01)" >&2
    rc=1
  elif ! grep -Eq "^[[:space:]]*${stdout_var}[[:space:]]*:?=[[:space:]]*newBoundedCapture\(MaxResponseBytes\)" "$body"; then
    echo "  REL-01: CallExec's stdout sink '${stdout_var}' is not created by newBoundedCapture(MaxResponseBytes) — a runaway digest-pinned provider fills memory unbounded while CallHTTP, in the same file, is bounded fail-closed; opts.Timeout bounds wall-clock, not bytes (AUD2-S01)" >&2
    rc=1
  elif [[ "$(assign_count "$body" "$stdout_var")" -gt 1 ]]; then
    echo "  REL-01: CallExec assigns the stdout sink '${stdout_var}' more than once — the bounded capture is created and then replaced, so the check above reads a statement that no longer decides anything (AUD2-S01)" >&2
    rc=1
  fi

  # -- REL-02: killing the child does not close a pipe its grandchildren hold.
  local wait_rhs
  wait_rhs="$(rhs_of "$body" 'cmd\.WaitDelay')"
  if [[ -z "$wait_rhs" ]]; then
    echo "  REL-02: CallExec sets no cmd.WaitDelay — on context timeout cmd.Run() blocks until every writer to the stdout pipe closes, so a provider that forks a background grandchild hangs 'assent run' past its deadline with no decision, no diagnostic and no 'unavailable' classification (AUD2-S01)" >&2
    rc=1
  else
    # A literal `== "0"` compare would ACCEPT `time.Duration(0)` and
    # `0 * time.Second`, both of which are the same wait-forever zero. Rule: if
    # the right-hand side carries digits and every one of them is a zero, the
    # value is zero however the LITERAL is spelled. (A zero reached through a
    # named variable or a helper call is not caught — a residual, not a claim.)
    # `opts.Timeout` carries no digits and
    # is unaffected; `30 * time.Second` carries a non-zero digit and is
    # accepted. A field named e.g. `v0.Timeout` would be refused — fail-closed,
    # and the fix is to name the assertion's exception, not to widen the rule.
    local wait_digits
    wait_digits="$(printf '%s' "$wait_rhs" | tr -cd '0-9')"
    if [[ -n "$wait_digits" && "$wait_digits" =~ ^0+$ ]]; then
      echo "  REL-02: CallExec sets cmd.WaitDelay = ${wait_rhs}, which is Go's 'wait forever' zero however it is spelled (0, time.Duration(0), 0 * time.Second) — the REL-02 hang is reopened while the assignment still looks present (AUD2-S01)" >&2
      rc=1
    fi
  fi

  # -- REL-07: stderr is captured, bounded, and reaches the caller's error.
  local stderr_var
  stderr_var="$(rhs_of "$body" 'cmd\.Stderr')"
  if [[ -z "$stderr_var" ]]; then
    echo "  REL-07: CallExec never wires cmd.Stderr — a failing provider yields a bare 'exit status 1' and an operator debugging a fail-closed REVIEW has nothing to read (AUD2-S01)" >&2
    rc=1
  else
    if ! grep -Eq "^[[:space:]]*${stderr_var}[[:space:]]*:?=[[:space:]]*newBoundedCapture\(" "$body"; then
      echo "  REL-07: CallExec's stderr sink '${stderr_var}' is not a bounded capture — an unbounded stderr sink trades the REL-01 memory hole for the same hole on the other stream (AUD2-S01)" >&2
      rc=1
    fi
    if [[ "$(assign_count "$body" "$stderr_var")" -gt 1 ]]; then
      echo "  REL-07: CallExec assigns the stderr sink '${stderr_var}' more than once — the bounded capture is created and then replaced (AUD2-S01)" >&2
      rc=1
    fi

    # THE FOLD, scoped to the failure branch — not a body-wide grep.
    #
    # "A return mentioning stderr appears somewhere in CallExec" is the same
    # unfailable shape the REL-03 findings were: it survives
    #     if err := cmd.Run(); err != nil {
    #         if errors.Is(err, context.DeadlineExceeded) {
    #             return nil, execFailure(err, stderr)
    #         }
    #         return nil, err
    #     }
    # which reverts REL-07 for every real provider failure (exit status, signal,
    # WaitDelay kill) while leaving one matching line behind. gofmt, vet, lint
    # and build are all clean on it; only `go test` sees it.
    #
    # So: isolate the `cmd.Run()` failure branch, then assert BOTH halves.
    local runblk="$WORK/block.cmdRun"
    # Metacharacter-free pattern: block_from hands it to `awk -v`, which strips
    # one level of escaping, so `\(` would arrive as an unbalanced group and
    # mawk/gawk reject it outright. `cmd.Run` matches exactly one line here.
    block_from "$body" 'cmd.Run' >"$runblk"
    if [[ ! -s "$runblk" ]] || ! head -1 "$runblk" | grep -Fq 'cmd.Run()' ||
      ! head -1 "$runblk" | grep -Fq 'err != nil'; then
      echo "  REL-07: could not isolate CallExec's cmd.Run() failure branch (its opening line must carry both 'cmd.Run()' and 'err != nil') — the brace-matched extraction broke, so the fold assertions below would be vacuous (AUD2-S01)" >&2
      return 1
    fi
    if [[ "$(wc -l <"$runblk")" -ge "$(wc -l <"$body")" ]]; then
      echo "  REL-07: the isolated cmd.Run() failure branch is not smaller than the whole CallExec body — the scoping did not happen, so the fold assertion is body-wide again (AUD2-S01)" >&2
      return 1
    fi

    # (a) PRESENCE, at the branch's body indent: the branch's own terminal path
    # returns the folded error.
    local run_indent run_body_indent
    run_indent="$(sed -n "1s/^\\([[:space:]]*\\).*/\\1/p" "$runblk")"
    run_body_indent="$(printf '%s\t' "$run_indent")"
    if ! awk -v ind="$run_body_indent" -v v="$stderr_var" '
          index($0, ind) == 1 {
            rest = substr($0, length(ind) + 1)
            if (rest ~ /^return / && index(rest, v) > 0) { found = 1 }
          }
          END { exit(found ? 0 : 1) }
        ' "$runblk"; then
      echo "  REL-07: CallExec captures stderr into '${stderr_var}' but its cmd.Run() failure branch does not return it on the branch's own terminal path — collected-and-discarded diagnostics leave REL-07 open with the capture still visible in the source (AUD2-S01)" >&2
      rc=1
    fi

    # (b) ABSENCE, the half that survives a polarity swap: NO return inside that
    # branch may omit the stderr sink. One that does is a path on which the
    # operator gets a bare "exit status 1" again, wherever it sits and however
    # the condition around it is spelled. Conservative view (see
    # code_conservative): this is an absence assertion, so over-stripping would
    # go falsely green.
    local runblk_abs="$WORK/block.cmdRun.conservative"
    code_conservative <"$runblk" >"$runblk_abs"
    if [[ ! -s "$runblk_abs" ]] || ! grep -Fq 'cmd.Run()' "$runblk_abs"; then
      echo "  REL-07: the conservative view of the cmd.Run() failure branch lost its own opening line — the filter is over-stripping, so the absence assertion would be vacuous (AUD2-S01)" >&2
      return 1
    fi
    assert_no_raw_string "$runblk_abs" "REL-07" || return 1
    local bare="$WORK/hits.rel07_bare_returns"
    grep -nE '^[[:space:]]*return ' "$runblk_abs" | grep -vF -- "$stderr_var" >"$bare" || true
    if [[ -s "$bare" ]]; then
      echo "  REL-07: CallExec's cmd.Run() failure branch has a return that does NOT carry '${stderr_var}' — on that path the operator gets a bare 'exit status 1' with no provider diagnostic, which is REL-07 reverted in one of the shapes this HEURISTIC recognises (a nested errors.Is fold, or its polarity swap). It is a SUBSTRING test, not data flow: a bare return whose message merely contains the text 'stderr' passes it, and no comment filter can change that because a string literal is not a comment. The named-test pin above, not this check, is what holds REL-07 closed. A stderr-folding return elsewhere in the branch does not close this either (AUD2-S01):" >&2
      sed 's/^/    /' "$bare" >&2
      rc=1
    fi
  fi
  return "$rc"
}

# ============================================================================
# (2) REL-03 — AUD2-S02: absent declaration vs. broken forge
# ============================================================================

# resolveRunFacts used to `continue` on ANY FileAtRef error for
# providers/<name>.json, so a 503, a throttle or a token scoped away from the
# governance repo was indistinguishable from "this provider declares nothing".
# The fail-safe DIRECTION held; the PATH was invisible — a wrong decision with a
# misleading predicate.error, and a has()-tolerant policy silently taking its
# fallback branch.
check_notfound_discrimination() { # <provider_host.go>
  local file="$1" rc=0
  local body="$WORK/body.resolveRunFacts"
  # ---- PRIMARY (2): the named tests and their cases still exist -------------
  # The behavioural run (PRIMARY (1)) is a SEPARATE top-level check, not part of
  # this function: this one is re-invoked against mutant FILES by a dozen
  # controls, and running a `go test` per invocation took the gate from 4s to
  # 65s. It also has nothing to say about a mutated copy of one source file.
  check_named_tests "REL-03" "$REL03_TEST_FILE" REL03_TESTS REL03_SUBTESTS \
    "the forge-error/absent-file discrimination in resolveRunFacts" || rc=1

  # Anchor: the FileAtRef call itself. Over-extraction guard:
  # loadResourceOwnerRegistry, further down the same file, carries the identical
  # errors.Is guard (D-130) — it is both the next-function-below check and the
  # decoy this assertion must not be reading.
  body_of "$file" resolveRunFacts "$body" "REL-03" 'FileAtRef' 'func loadResourceOwnerRegistry' || return 1
  # rc may already be 1 from the primary pin; the heuristics below only add to it.

  if ! grep -Fq 'errors.Is(err, forge.ErrNotFound)' "$body"; then
    echo "  REL-03: resolveRunFacts does not discriminate forge.ErrNotFound on the provider-declaration fetch — every FileAtRef error (retry-exhausted 5xx, deterministic 401/403) is treated as 'declaration absent' and skipped, so a forge blip silently converts approvable MRs to REVIEW by an invisible path. NOTE: the identical guard in loadResourceOwnerRegistry does NOT close this; the finding is at this call site (AUD2-S02)" >&2
    return 1
  fi

  # Discrimination is only half of it: the non-absent branch must actually STOP
  # the run rather than fall through to the same `continue`.
  #
  # THIS ASSERTION IS SCOPED TO THE GUARD BLOCK, not to the function body, and
  # that is the whole point. resolveRunFacts contains THREE
  # `return nil, nil, fmt.Errorf(` lines — the guard's own, the
  # LoadProviderConfig malformed-declaration path, and the provider-call path.
  # A body-wide grep for that pattern is satisfied on every possible tree,
  # including a half-reverted one where the guard is kept and its body is
  # downgraded back to `continue`: a compiling, gofmt-clean restoration of the
  # exact REL-03 defect that an independent reviewer built and that this gate
  # passed. Reading only between the guard's `if` and its matching `}` is what
  # makes the assertion capable of failing at all.
  local guard="$WORK/guard.resolveRunFacts"
  # Line AND block comments are stripped (code_only): this guard's own prose
  # names both LoadProviderConfig and the `continue` it replaced, so a raw-text
  # control would fire on the documentation instead of the code — and a
  # `return … fmt.Errorf(` quoted in a comment must not satisfy the assertion.
  # Both forms are handled because a reviewer's `/* … */` camouflage mutant got
  # past the line-only version; see the block-comment control in section (5).
  guard_block "$body" | code_only >"$guard"
  if [[ ! -s "$guard" ]]; then
    echo "  REL-03: could not isolate the forge.ErrNotFound guard block inside resolveRunFacts — the brace-matched extraction broke, so the branch assertion below would be vacuous" >&2
    return 1
  fi
  # Positive control: the isolated region is the GUARD and nothing more. It must
  # open with the guard line, and it must NOT have swallowed the two sibling
  # error returns that follow it (LoadProviderConfig's is the first of them) —
  # if it had, this assertion would be body-wide again by another route.
  head -1 "$guard" | grep -Fq 'errors.Is(err, forge.ErrNotFound)' \
    || {
      echo "  REL-03: the isolated region does not START with the forge.ErrNotFound guard — the wrong block was extracted, so the branch assertion would be vacuous" >&2
      return 1
    }
  # Over-extraction control, on CODE: the statement that immediately follows the
  # guard is the LoadProviderConfig load, and it carries the second of the
  # function's three error returns. If it is inside the isolated region, the
  # assertion below is body-wide again by another route.
  if grep -Eq '^[[:space:]]*hostCfg, err :?= ' "$guard" || grep -Eq '^[[:space:]]*[^/]*LoadProviderConfig\(' "$guard"; then
    echo "  REL-03: the isolated guard block ran past its closing brace and swallowed the LoadProviderConfig path — its error return would satisfy this assertion for the wrong reason, exactly the body-wide grep this scoping replaces" >&2
    return 1
  fi
  # The isolated region must also be strictly smaller than the body it came
  # from: a 'guard block' equal to the whole function is the body-wide grep
  # wearing a different name.
  if [[ "$(wc -l <"$guard")" -ge "$(wc -l <"$body")" ]]; then
    echo "  REL-03: the isolated guard block is not smaller than the whole resolveRunFacts body — the scoping did not happen" >&2
    return 1
  fi
  # ---- SECONDARY: source-shape HEURISTICS ----------------------------------
  # Everything from here down is a heuristic, and is documented as one. It
  # recognises the revert shapes it knows — a bare `continue`, a return quoted
  # in a comment, a return behind a nested condition, the polarity swap — and
  # names the defect precisely when it fires, which a test failure does not do.
  # It does NOT decide whether resolveRunFacts still fails closed: a one-line
  # condition narrowing in the guard's own `if`
  #     if !errors.Is(err, forge.ErrNotFound) && errors.Is(err, context.Canceled) {
  # has no fall-through, returns at the correct body indent, and passes every
  # check below while reverting REL-03 in full. The named-test pin above plus
  # `go test` (check stage 4, and a step of the same PR-visible verify job) is
  # what actually holds this closed. Do not re-promote these to "the property".

  # (a) PRESENCE, at the guard's body indent. A return that is only reachable
  # under a further condition sits one level deeper and does not count. This is
  # necessary but — on its own — NOT sufficient: it measures indent POSITION,
  # and the polarity-swapped revert
  #     if !errors.Is(err, context.Canceled) { continue }
  #     return nil, nil, fmt.Errorf(...)
  # puts a real return at exactly this indent while skipping every 503/401/403.
  # Assertion (b) below is the one that discriminates; this one stays because it
  # is what names the defect precisely when (b) fires.
  local guard_indent body_indent
  guard_indent="$(sed -n '1s/^\([[:space:]]*\).*/\1/p' "$guard")"
  body_indent="$(printf '%s\t' "$guard_indent")"
  if ! awk -v ind="$body_indent" '
        index($0, ind) == 1 &&
        substr($0, length(ind) + 1) ~ /^return nil, nil, fmt\.Errorf\(/ { found = 1 }
        END { exit(found ? 0 : 1) }
      ' "$guard"; then
    echo "  REL-03: the forge.ErrNotFound guard in resolveRunFacts does not RETURN on its own terminal path — the discrimination is present but its non-absent branch falls through (a bare continue, a log-and-carry-on, or a return reachable only under a NESTED condition such as context.Canceled, which leaves every 503/401/403 skipped). That is the REL-03 defect restored with the guard left in place as camouflage. The sibling returns further down the function do NOT close this, and neither does a return quoted in a comment (AUD2-S02)" >&2
    rc=1
  fi

  # (b) ABSENCE OF FALL-THROUGH — the assertion that actually discriminates, and
  # the only one of the two that survives a polarity swap.
  #
  # The property is not "where does the return sit" but "can this guard be
  # entered and NOT stop the run". Structurally that is: no `continue`, `break`
  # or `goto` anywhere inside the guard, at any depth. The remediated guard
  # contains none in code — its single mention of `continue` is in the prose
  # explaining what it replaced — while EVERY revert shape needs one, because
  # skipping the provider is the whole point of the defect. Four mutants, one
  # assertion: the plain half-revert, the /* … */ camouflage, the nested
  # `if errors.Is(err, context.Canceled) { return }` fail-open, and its polarity
  # swap `if !errors.Is(err, context.Canceled) { continue }` — which is the more
  # idiomatic guard-clause form, passes gofmt/vet/golangci-lint clean, and
  # defeated assertion (a).
  #
  # This is an ABSENCE assertion, so it reads the CONSERVATIVE view (see
  # code_conservative): over-stripping here would hide the token being hunted
  # and go falsely green, the exact inversion of code_only's fail direction.
  local guard_abs="$WORK/guard.resolveRunFacts.conservative"
  guard_block "$body" | code_conservative >"$guard_abs"
  if [[ ! -s "$guard_abs" ]]; then
    echo "  REL-03: the conservative view of the guard block is empty — the absence assertion below would be vacuously satisfied, which is the one failure mode an absence check cannot tolerate" >&2
    return 1
  fi
  assert_no_raw_string "$guard_abs" "REL-03" || return 1
  # Positive control on the filter: it must still be showing us the guard's code.
  grep -Fq 'errors.Is(err, forge.ErrNotFound)' "$guard_abs" \
    || {
      echo "  REL-03: the conservative view lost the guard line itself — the filter is over-stripping, so the absence assertion would be vacuous" >&2
      return 1
    }
  local fallthrough="$WORK/hits.rel03_fallthrough"
  grep -nE '^[[:space:]]*(continue|break|goto)([[:space:]]|$)' "$guard_abs" >"$fallthrough" || true
  if [[ -s "$fallthrough" ]]; then
    echo "  REL-03: the forge.ErrNotFound guard in resolveRunFacts contains a FALL-THROUGH statement — it can be entered without stopping the run, so a 503, a 401, a 403 or a throttle is skipped exactly as before the fix. This is the REL-03 defect in one of the shapes this HEURISTIC recognises: a bare continue, a return hidden behind a nested condition, or the polarity swap 'if !errors.Is(err, context.Canceled) { continue }' followed by a return at the right indent. It does NOT recognise every spelling — a condition narrowing in the guard's own if has no fall-through at all — which is why the named-test pin above, not this check, is what holds REL-03 closed (AUD2-S02):" >&2
    sed 's/^/    /' "$fallthrough" >&2
    rc=1
  fi
  return "$rc"
}

# ============================================================================
# (3) SEC-03 — AUD2-S03: the cosign signer-identity pin
# ============================================================================

extract_issuer() {
  sed -nE "s/.*--certificate-oidc-issuer[[:space:]]+['\"]?([^'\"[:space:]]+)['\"]?.*/\1/p" "$1" | sort -u
}

extract_identity() {
  sed -nE "s/.*--certificate-identity-regexp[[:space:]]+'([^']*)'.*/\1/p" "$1" | sort -u
}

# hack/install.sh ran `cosign verify-blob --bundle` with NO identity/issuer
# pin while SECURITY.md published both, so --require-signature promised
# something it did not deliver: any Fulcio identity verified clean, and a
# mirror-swapped archive shipped with its own valid bundle installed.
#
# The values are NOT hardcoded here. Both files are read and compared, so this
# gate stays a pin and never becomes a third copy of the published truth (D-128
# in spirit) — a legitimate re-pin needs no edit to this script.
check_cosign_pin() { # <install.sh> <SECURITY.md>
  local inst="$1" sec="$2" rc=0
  for f in "$inst" "$sec"; do
    [[ -f "$f" ]] || {
      echo "  SEC-03: missing ${f#"$ROOT"/}" >&2
      return 1
    }
  done
  # Positive control on the extraction target itself.
  if ! grep -Fq 'cosign verify-blob' "$inst"; then
    echo "  SEC-03: ${inst#"$ROOT"/} has no 'cosign verify-blob' invocation — the signature path this pin guards is gone, so the flag assertions would be vacuous" >&2
    return 1
  fi

  local inst_issuer inst_identity sec_issuer sec_identity
  inst_issuer="$(extract_issuer "$inst")"
  inst_identity="$(extract_identity "$inst")"
  sec_issuer="$(extract_issuer "$sec")"
  sec_identity="$(extract_identity "$sec")"

  if [[ -z "$inst_issuer" ]]; then
    echo "  SEC-03: hack/install.sh's cosign call carries no --certificate-oidc-issuer — keyless verification accepts any issuer (AUD2-S03)" >&2
    rc=1
  fi
  if [[ -z "$inst_identity" ]]; then
    echo "  SEC-03: hack/install.sh's cosign call carries no --certificate-identity-regexp — a mirror-swapped archive shipped with its own validly signed bundle verifies clean and '--require-signature' is a broken promise (AUD2-S03)" >&2
    rc=1
  fi
  if [[ -z "$sec_issuer" || -z "$sec_identity" ]]; then
    echo "  SEC-03: SECURITY.md no longer publishes both the OIDC issuer and the identity regexp — adopters following the manual instructions have nothing to pin against (AUD2-S03)" >&2
    rc=1
  fi
  ((rc == 0)) || return "$rc"

  # One value per file, or the file disagrees with itself.
  local n
  for pair in "install.sh|$inst_issuer" "install.sh|$inst_identity" "SECURITY.md|$sec_issuer" "SECURITY.md|$sec_identity"; do
    n="$(printf '%s' "${pair#*|}" | grep -c . || true)"
    if ((n > 1)); then
      echo "  SEC-03: ${pair%%|*} carries $n DIFFERENT pinned values, so the file disagrees with itself: $(printf '%s' "${pair#*|}" | tr '\n' '|')" >&2
      rc=1
    fi
  done
  ((rc == 0)) || return "$rc"

  if [[ "$inst_issuer" != "$sec_issuer" || "$inst_identity" != "$sec_identity" ]]; then
    echo "  SEC-03: DRIFT — hack/install.sh pins issuer=${inst_issuer} identity=${inst_identity} but SECURITY.md publishes issuer=${sec_issuer} identity=${sec_identity}; adopters running install.sh and adopters following the published instructions would verify against different signers (AUD2-S03)" >&2
    rc=1
  fi
  return "$rc"
}

# ============================================================================
# (4) TEST-02 — AUD2-S04: the demonstrated EffectChallenge mutant stays dead
# ============================================================================

# The 2026-08-18 auditor demonstrated that deleting EffectChallenge from
# isStricterInterventionEffect left ./internal/compare/... AND the comparison
# corpus dogfood green. Both halves are asserted: the term in the classifier and
# the named case that kills the mutant.
check_challenge_classification() { # <classify_intervention.go> <classify_intervention_test.go>
  local src="$1" tst="$2" rc=0
  local body="$WORK/body.isStricterInterventionEffect"
  # Anchor: EffectBlock, present in every version. No next-function argument:
  # isStricterInterventionEffect is the LAST function in the file, so there is
  # nothing below it to swallow and the `^func ` count is the live control here.
  # (Its sibling isMissedInterventionEffect sits ABOVE it, so naming it would be
  # a guard that can never fire.)
  body_of "$src" isStricterInterventionEffect "$body" "TEST-02" 'aggregate.EffectBlock' '' || return 1

  if ! grep -Fq 'aggregate.EffectChallenge' "$body"; then
    echo "  TEST-02: isStricterInterventionEffect no longer classifies aggregate.EffectChallenge as a stricter intervention — 'assent compare' stops grading challenge-effect deltas, which is exactly the mutant the auditor demonstrated merges through every wired gate (AUD2-S04)" >&2
    rc=1
  fi
  if [[ ! -f "$tst" ]]; then
    echo "  TEST-02: missing ${tst#"$ROOT"/} — the named defence of the EffectChallenge term is gone (AUD2-S04)" >&2
    return 1
  fi
  if ! grep -Fq "func ${CHALLENGE_TEST}(" "$tst"; then
    echo "  TEST-02: the named unit case ${CHALLENGE_TEST} is absent from ${tst#"$ROOT"/} — without it, deleting the EffectChallenge term from the classifier is green in 'go test ./internal/compare/...' again (AUD2-S04)" >&2
    rc=1
  fi
  return "$rc"
}

# ============================================================================
# (5) Wiring — REQ-AUD2-S05-03/04/05
# ============================================================================

# extract_block <file> <2-space-indented key> — a Taskfile task body.
extract_block() {
  awk -v name="$2" '
    $0 == "  " name ":" { inblk = 1; next }
    inblk && /^  [A-Za-z0-9_.:-]+:[[:space:]]*$/ { inblk = 0 }
    inblk { print }
  ' "$1"
}

check_task_wiring() { # <taskfile>
  local tf="$1" rc=0
  [[ -f "$tf" ]] || {
    echo "  REQ-AUD2-S05-03: missing $tf" >&2
    return 1
  }
  local block="$WORK/check.block"
  extract_block "$tf" check >"$block"
  if [[ ! -s "$block" ]] || ! grep -qE '^[[:space:]]+- task: build$' "$block"; then
    echo "  REQ-AUD2-S05-03: the Taskfile 'check:' block extracted empty or without the known-present '- task: build' — the wiring assertions would be vacuous" >&2
    return 1
  fi
  if ! grep -qE "^[[:space:]]+- task: ${STAGE}\$" "$block"; then
    echo "  REQ-AUD2-S05-03: 'task check' does not run '${STAGE}' — a gate invoked by nothing is not a gate (D-124), and the AUD2 remediations would be unpinned locally" >&2
    rc=1
  fi
  local def="$WORK/def.stage"
  extract_block "$tf" "$STAGE" >"$def"
  if [[ ! -s "$def" ]]; then
    echo "  REQ-AUD2-S05-03: '${STAGE}' is not defined as a task in Taskfile.yml" >&2
    rc=1
  elif ! grep -Fq "$SELF" "$def"; then
    echo "  REQ-AUD2-S05-03: the '${STAGE}' task does not invoke ${SELF} — the stage name is wired but the gate is not" >&2
    rc=1
  elif ! grep -qE "bash ${SELF//\//\\/}[[:space:]]*\$" "$def"; then
    echo "  REQ-AUD2-S05-03: the '${STAGE}' task invokes ${SELF} WITH ARGUMENTS — '--text-only' skips the behavioural mutation runs, which are the only assertions here that measure REL-03 and REL-07 rather than reading their source text. An argument-carrying invocation is a disarmed gate (the AUD-S18 idiom, same reason):" >&2
    sed 's/^/    /' <(grep -F "$SELF" "$def") >&2
    rc=1
  fi
  return "$rc"
}

# CHECK_STAGES in hack/audit/exitgate_test.sh is the AUD-S18 authority on which
# stages `task check` runs; a stage missing from it is a stage the release exit
# gate does not grade. Adding the stage here in the SAME commit is what the
# AUD-S18 pin exists to force (judgment call (e)) — a stale array is the
# mismatch that reddened main for four merges in the AUD-S18/RELSE-08 incident.
check_stage_pinned() { # <hack/audit/exitgate_test.sh>
  local gate="$1"
  [[ -f "$gate" ]] || {
    echo "  REQ-AUD2-S05-03: missing $gate" >&2
    return 1
  }
  local arr="$WORK/check_stages"
  awk '/^CHECK_STAGES=\(/ { inarr = 1; next } inarr && /^\)/ { exit } inarr { gsub(/[[:space:]]/, ""); if ($0 != "" && $0 !~ /^#/) print }' "$gate" >"$arr"
  if [[ ! -s "$arr" ]] || ! grep -qx 'fmt' "$arr"; then
    echo "  REQ-AUD2-S05-03: the CHECK_STAGES array extracted empty or without the known-present 'fmt' — the pin assertion would be vacuous" >&2
    return 1
  fi
  if ! grep -qx "$STAGE" "$arr"; then
    echo "  REQ-AUD2-S05-03: '${STAGE}' is not in CHECK_STAGES in hack/audit/exitgate_test.sh — the AUD-S18 release exit gate would not grade this stage, and deleting it from the Taskfile would be invisible there ($(wc -l <"$arr" | tr -d ' ') stages pinned)" >&2
    return 1
  fi
  return 0
}

# extract_job <workflow> <job-name> — the body of a 2-space-indented job key.
extract_job() {
  awk -v name="$2" '
    $0 == "  " name ":" { inj = 1; next }
    inj && /^  [A-Za-z0-9_.-]+:[[:space:]]*$/ { inj = 0 }
    inj { print }
  ' "$1"
}

# isolate_step <job-block> <lineno> — the 6-space `- ` step containing a line.
isolate_step() {
  awk -v n="$2" '
    /^      - / { start = NR }
    NR == n { want = start }
    { line[NR] = $0 }
    END {
      if (!want) exit 1
      for (i = want; i <= NR; i++) {
        if (i > want && line[i] ~ /^      - /) break
        print line[i]
      }
    }
  ' "$1"
}

# REQ-AUD2-S05-05: RELSE-08 not reproduced. release-exitgate carries
# `if: github.event_name != 'pull_request'`, so anything that runs ONLY there is
# invisible on pull requests — which is how AUD-S18's stale pin survived four
# merges. This gate must therefore be a step of the `verify` job, which has no
# such job-level guard, undisarmed.
check_pr_wiring() { # <verify.yaml>
  local wf="$1" rc=0
  [[ -f "$wf" ]] || {
    echo "  REQ-AUD2-S05-05: missing $wf" >&2
    return 1
  }
  # The job-level guard is not the only way to lose PR reach: deleting
  # `pull_request` from the workflow's own `on:` block removes it for every job
  # at once, and every assertion below would stay green while the gate's entire
  # pull-request coverage disappeared.
  local trigger="$WORK/on.block"
  awk '/^on:/ { ino = 1; next } ino && /^[A-Za-z]/ { ino = 0 } ino { print }' "$wf" >"$trigger"
  if [[ ! -s "$trigger" ]]; then
    echo "  REQ-AUD2-S05-05: the workflow's 'on:' block extracted empty — the trigger assertion would be vacuous" >&2
    return 1
  fi
  if ! grep -qE '^[[:space:]]+pull_request:' "$trigger"; then
    echo "  REQ-AUD2-S05-05: verify.yaml no longer triggers on pull_request — the gate's step and the job's missing if: are both irrelevant, because the workflow never runs on a PR at all (RELSE-08 by another route)" >&2
    return 1
  fi

  local job="$WORK/job.verify"
  extract_job "$wf" verify >"$job"
  if [[ ! -s "$job" ]] || ! grep -q '^    steps:' "$job"; then
    echo "  REQ-AUD2-S05-05: the 'verify' job block extracted empty or without a 'steps:' key — the PR-visibility assertions would be vacuous" >&2
    return 1
  fi
  # The job must not have grown release-exitgate's push-only guard: a job-level
  # `if:` sits at 4 spaces, a step-level one at 8.
  local jobif="$WORK/hits.jobif"
  grep -nE '^    if:' "$job" >"$jobif" || true
  if [[ -s "$jobif" ]]; then
    echo "  REQ-AUD2-S05-05: the 'verify' job carries a JOB-LEVEL if: — if it skips pull requests, this gate is push-only exactly like release-exitgate and RELSE-08 is reproduced:" >&2
    sed 's/^/    /' "$jobif" >&2
    return 1
  fi
  local hit lineno
  hit="$(grep -nF "$SELF" "$job" | head -1 || true)"
  if [[ -z "$hit" ]]; then
    echo "  REQ-AUD2-S05-05: the 'verify' job — the only job that runs on pull_request — does not invoke ${SELF}. The gate would run only in release-exitgate, which skips PRs (RELSE-08): a reverted AUD2 remediation would merge green and be caught only after landing on main" >&2
    return 1
  fi
  lineno="${hit%%:*}"
  local step="$WORK/step.aud2"
  isolate_step "$job" "$lineno" >"$step" || true
  if [[ ! -s "$step" ]] || ! grep -qE '^      - ' "$step"; then
    echo "  REQ-AUD2-S05-05: could not isolate the AUD2 gate step in the verify job — the extraction broke, so the disarm assertion would be vacuous" >&2
    return 1
  fi
  # Same argument ban as the Taskfile stage: `--text-only` in CI would leave the
  # behavioural runs unexecuted on every pull request.
  if ! grep -qE "bash ${SELF//\//\\/}[[:space:]]*\$" "$step"; then
    echo "  REQ-AUD2-S05-05: the AUD2 gate step invokes ${SELF} WITH ARGUMENTS — '--text-only' skips the behavioural mutation runs, so the step would look wired while certifying nothing about REL-03 and REL-07:" >&2
    sed 's/^/    /' <(grep -F "$SELF" "$step") >&2
    rc=1
  fi
  local disarmed="$WORK/hits.disarmed"
  grep -nE '^[[:space:]]*(if|continue-on-error):' "$step" >"$disarmed" || true
  if [[ -s "$disarmed" ]]; then
    echo "  REQ-AUD2-S05-05: the AUD2 gate step is present but DISARMED — an 'if:' or 'continue-on-error:' means a red gate does not fail the PR:" >&2
    sed 's/^/    /' "$disarmed" >&2
    rc=1
  fi
  return "$rc"
}

# ============================================================================
# Run: real tree GREEN, then every assertion proved capable of going RED
# ============================================================================

echo "== AUD2 exit gate — 2026-08-18 audit, risk-reduction wave (REQ-AUD2-S05-01..05) =="
echo

echo "== (1) REL-01/02/07 — AUD2-S01: CallExec bounds stdout, sets WaitDelay, captures stderr =="
expect_green check_exec_containment "CallExec is contained: bounded stdout, WaitDelay, stderr folded into the error" "$TRANSPORT"

echo "== (2) REL-03 — AUD2-S02: resolveRunFacts discriminates forge.ErrNotFound =="
expect_green check_notfound_discrimination "the provider-declaration fetch skips only on ErrNotFound and returns every other error" "$PROVIDER_HOST"

echo "== (3) SEC-03 — AUD2-S03: hack/install.sh pins the cosign signer identity + issuer =="
expect_green check_cosign_pin "install.sh pins issuer and identity, byte-identically with SECURITY.md" "$INSTALL" "$SECURITY"

echo "== (4) TEST-02 — AUD2-S04: EffectChallenge classified, and named-tested =="
expect_green check_challenge_classification "isStricterInterventionEffect keeps EffectChallenge and ${CHALLENGE_TEST} defends it" "$CLASSIFY" "$CLASSIFY_TEST"

echo
echo "== (4b) REL-03 / REL-07 — the pinned tests still DETECT their revert =="
if ((TEXT_ONLY == 1)); then
  echo "SKIP (--text-only): the behavioural mutation runs need the Go toolchain and git."
  echo "  This mode certifies the PATTERNS and their mutation harness only. It says NOTHING"
  echo "  about whether REL-03 and REL-07 are still held closed — a name pin cannot tell a"
  echo "  live test from an emptied one. 'task check' and verify.yaml invoke this script with"
  echo "  NO arguments, and the wiring checks below refuse an argument-carrying invocation."
else
  expect_green check_behavioural_revert "the REL-03 tests FAIL when the condition-narrowing revert is applied to a copy of the tree" \
    "REL-03" rel03 "$REL03_SRC" "$REL03_REVERT" "$REL03_REVERT_WITNESS" "$REL03_RUN" "$REL03_PKG" REL03_TESTS \
    "the forge-error/absent-file discrimination in resolveRunFacts" "" REL03_MUSTFAIL
  expect_green check_behavioural_revert "the REL-07 tests FAIL when the stderr fold is dropped in a copy of the tree" \
    "REL-07" rel07 "$REL07_SRC" "$REL07_REVERT" "$REL07_REVERT_WITNESS" "$REL07_RUN" "$REL07_PKG" REL07_TESTS \
    "the stderr fold in CallExec" "" REL07_MUSTFAIL
fi

echo "== (5) REQ-AUD2-S05-02 — every assertion above proved capable of going RED =="

# ---- REL-01: stdout back to an unbounded buffer -----------------------------
m="$WORK/transport.rel01.go"
cp "$TRANSPORT" "$m"
mutate "$m" 's|stdout := newBoundedCapture(MaxResponseBytes)|stdout := \&bytes.Buffer{}|' 'stdout := &bytes.Buffer{}'
assert_changed "$TRANSPORT" "$m"
# THE scoping control for REL-01: CallHTTP's own MaxResponseBytes use survives,
# so a file-level grep would still be green on this mutant.
grep -Fq 'MaxResponseBytes' "$m" ||
  fail "mutation harness: the REL-01 mutant lost every MaxResponseBytes occurrence — then a file-level grep would also catch it and this control proves nothing about scoping"
expect_red check_exec_containment "CallExec's stdout is an unlimited bytes.Buffer again (REL-01), while CallHTTP's bound survives in the same file" \
  "REL-01: CallExec's stdout sink" "$m"

# ---- REL-02: no WaitDelay ---------------------------------------------------
m="$WORK/transport.rel02.go"
cp "$TRANSPORT" "$m"
# Anchored on the ASSIGNMENT: an unanchored pattern deletes the doc comment
# twenty lines above, which changes the file without reverting anything.
delete_first "$m" '^[[:space:]]*cmd\.WaitDelay[[:space:]]*='
assert_changed "$TRANSPORT" "$m"
assert_assignment_gone "$m" 'cmd\.WaitDelay' 'REL-02'
expect_red check_exec_containment "CallExec sets no cmd.WaitDelay (REL-02)" \
  "REL-02: CallExec sets no cmd.WaitDelay" "$m"

# A zero WaitDelay is Go's "wait forever": the assignment is present, the hang
# is back. Deletion alone would not have caught this.
m="$WORK/transport.rel02zero.go"
cp "$TRANSPORT" "$m"
mutate "$m" 's|cmd.WaitDelay = opts.Timeout|cmd.WaitDelay = 0|' 'cmd.WaitDelay = 0'
expect_red check_exec_containment "cmd.WaitDelay is assigned Go's wait-forever zero value (REL-02)" \
  "REL-02: CallExec sets cmd.WaitDelay = 0" "$m"

# The same zero, spelled so that a literal string compare would accept it. This
# is the control that keeps the digit rule above honest.
m="$WORK/transport.rel02typedzero.go"
cp "$TRANSPORT" "$m"
mutate "$m" 's|cmd.WaitDelay = opts.Timeout|cmd.WaitDelay = 0 * time.Second|' 'cmd.WaitDelay = 0 * time.Second'
expect_red check_exec_containment "cmd.WaitDelay is assigned a typed wait-forever zero (0 * time.Second) that a literal compare would accept (REL-02)" \
  "REL-02: CallExec sets cmd.WaitDelay = 0 * time.Second" "$m"

# Non-vacuity of the digit rule in the OTHER direction: a real non-zero bound
# must still be accepted, or the rule would red every honest tree.
m="$WORK/transport.rel02nonzero.go"
cp "$TRANSPORT" "$m"
mutate "$m" 's|cmd.WaitDelay = opts.Timeout|cmd.WaitDelay = 30 * time.Second|' 'cmd.WaitDelay = 30 * time.Second'
check_exec_containment "$m" >/dev/null 2>&1 \
  || fail "the zero-value rule REJECTED a real non-zero bound (30 * time.Second) — it is over-broad and would red an honest tree"
echo "OK: the wait-forever rule accepts a real non-zero bound (30 * time.Second) — it refuses zeros, not arithmetic"

# ---- REL-07: stderr unwired, and stderr captured-then-discarded -------------
m="$WORK/transport.rel07.go"
cp "$TRANSPORT" "$m"
delete_first "$m" '^[[:space:]]*cmd\.Stderr[[:space:]]*='
assert_changed "$TRANSPORT" "$m"
assert_assignment_gone "$m" 'cmd\.Stderr' 'REL-07'
expect_red check_exec_containment "CallExec never wires cmd.Stderr (REL-07)" \
  "REL-07: CallExec never wires cmd.Stderr" "$m"

m="$WORK/transport.rel07fold.go"
cp "$TRANSPORT" "$m"
# Witness is the CallExec-scoped absence of the fold, not the presence of
# `return nil, err` — that string already occurs twice above in the same
# function, so the old witness made a no-op sed look like a landed mutation and
# reddened with "mutation harness: sed did not change" instead of naming REL-07.
sed 's|return nil, execFailure(err, stderr)|return nil, err|' "$TRANSPORT" >"$m"
assert_changed "$TRANSPORT" "$m"
if extract_func "$m" CallExec | grep -Fq 'execFailure('; then
  fail "mutation harness (REL-07): CallExec still calls execFailure in ${m#"$WORK"/} — the fold-removal mutation did not land"
fi
expect_red check_exec_containment "stderr is captured but discarded instead of folded into the returned error (REL-07)" \
  "does not return it on the branch's own terminal path" "$m"

# ---- REL-07 NESTED FOLD: the fold survives only for one error kind ----------
# Review round 3, the REL-03 finding's sibling. The old body-wide grep ("a
# return mentioning stderr appears somewhere in CallExec") accepts this: the
# fold is real, and reachable only for context.DeadlineExceeded. Every actual
# provider failure — non-zero exit, signal, the WaitDelay kill — takes
# `return nil, err` and the operator is back to a bare "exit status 1".
# NOTE, corrected: as spliced here this mutant does NOT compile — transport.go
# does not import `errors`, and the control does not add the import because it
# grades TEXT, which is all the assertion under test reads. The equivalent
# revert written by hand (with the import) is gofmt/vet/lint/build clean; that
# was verified separately, on the real tree. Do not read this control as a
# compile check.
m="$WORK/transport.rel07nested.go"
cp "$TRANSPORT" "$m"
cat >"$WORK/rep.rel07nested" <<'GOREP'
if errors.Is(err, context.DeadlineExceeded) {
	return nil, execFailure(err, stderr)
}
return nil, err
GOREP
awk '
  FNR == NR { line[++n] = $0; next }
  done != 1 && $0 ~ /return nil, execFailure/ {
    match($0, /^[ \t]*/); ind = substr($0, 1, RLENGTH)
    for (i = 1; i <= n; i++) print ind line[i]
    done = 1; next
  }
  { print }
' "$WORK/rep.rel07nested" "$TRANSPORT" >"$m"
assert_changed "$TRANSPORT" "$m"
# THE precondition: a body-wide grep for a stderr-carrying return is STILL green
# on this mutant. That survival is what makes the control prove scoping.
grep -Eq "return .*[ ,(]stderr[ ,)]" "$m" ||
  fail "mutation harness: the REL-07 nested-fold mutant has no surviving stderr-carrying return — then the old body-wide grep would also have caught it and this control proves nothing"
echo "OK: the REL-07 nested-fold mutant KEEPS a stderr-carrying return, so the old body-wide grep stays green on it — only the branch-scoped assertions refuse it"
expect_red check_exec_containment "the stderr fold is reachable only for context.DeadlineExceeded; every real provider failure returns a bare error (REL-07 nested fail-open)" \
  "has a return that does NOT carry 'stderr'" "$m"

# ---- REL-07 POLARITY SWAP: the same revert, written as a guard clause -------
# The shape that defeats an indent rule: the stderr fold now sits at the
# branch's own terminal path, and the bare return is the nested one. Assertion
# (a) accepts it; only the absence half — no return in this branch may omit the
# sink — refuses it.
m="$WORK/transport.rel07polarity.go"
cp "$TRANSPORT" "$m"
cat >"$WORK/rep.rel07polarity" <<'GOREP'
if !errors.Is(err, context.DeadlineExceeded) {
	return nil, err
}
return nil, execFailure(err, stderr)
GOREP
awk '
  FNR == NR { line[++n] = $0; next }
  done != 1 && $0 ~ /return nil, execFailure/ {
    match($0, /^[ \t]*/); ind = substr($0, 1, RLENGTH)
    for (i = 1; i <= n; i++) print ind line[i]
    done = 1; next
  }
  { print }
' "$WORK/rep.rel07polarity" "$TRANSPORT" >"$m"
assert_changed "$TRANSPORT" "$m"
grep -Eq "^[[:space:]]+return nil, execFailure\(err, stderr\)\$" "$m" ||
  fail "mutation harness: the REL-07 polarity mutant lost its terminal-path fold — then the presence half would already have caught it and this control does not prove the absence half discriminates"
echo "OK: the REL-07 polarity mutant puts the stderr fold on the branch's terminal path — the presence half accepts it; only the absence half refuses it"
expect_red check_exec_containment "REL-07 polarity swap: 'if !errors.Is(err, context.DeadlineExceeded) { return nil, err }' then the fold — a full revert (text-only mutant; the hand-written equivalent with the errors import is gofmt/vet/lint/build clean)" \
  "has a return that does NOT carry 'stderr'" "$m"

# ---- REL-01 REASSIGNMENT: the good statement stays, a later one overrides ---
# rhs_of reads the FIRST assignment; Go runs the LAST. This mutant leaves the
# bounded capture and `cmd.Stdout = stdout` exactly as written and appends a
# second assignment, so every pre-existing REL-01 check reads a statement that
# no longer decides anything.
m="$WORK/transport.rel01reassign.go"
cp "$TRANSPORT" "$m"
awk '
  done != 1 && /^[[:space:]]*cmd\.Stdout[[:space:]]*=/ {
    print
    match($0, /^[ \t]*/)
    print substr($0, 1, RLENGTH) "cmd.Stdout = &bytes.Buffer{}"
    done = 1
    next
  }
  { print }
' "$TRANSPORT" >"$m"
assert_changed "$TRANSPORT" "$m"
grep -Eq "^[[:space:]]*stdout :?= newBoundedCapture\(MaxResponseBytes\)" "$m" ||
  fail "mutation harness: the REL-01 reassignment mutant lost the bounded-capture statement — then the existing REL-01 check would already have caught it and this control proves nothing about last-write-wins"
echo "OK: the REL-01 reassignment mutant KEEPS 'stdout := newBoundedCapture(MaxResponseBytes)' and 'cmd.Stdout = stdout' intact — every pre-existing REL-01 check still reads the good statement"
expect_red check_exec_containment "cmd.Stdout is assigned a second time after the bounded capture — last write wins and REL-01 is reverted while the good statement remains (REL-01)" \
  "rhs_of reads the FIRST and the LAST is what runs" "$m"

# ---- REL-07 STRING-LITERAL BARE RETURN: why no filter can fix this ----------
# The mutant that settles it. Both halves of the fold assertion are SUBSTRING
# tests over text, not data-flow analysis: presence asks "a return line at this
# indent mentioning `stderr`", absence asks "no return line lacking `stderr`".
# A bare return whose MESSAGE happens to contain the identifier satisfies both:
#     return nil, fmt.Errorf("provider exec failed (stderr suppressed): %w", err)
# It is an ordinary double-quoted string literal — not a comment — so neither
# code_only nor code_conservative nor any third filter polarity touches it, and
# it also invalidates the "keeping too much is fail-closed" argument for THIS
# assertion specifically: the absence half is a NEGATED match, so extra kept
# text is fail-OPEN. What holds REL-07 closed is the named-test pin plus
# `go test`; this control records the blind spot executably.
m="$WORK/transport.rel07strlit.go"
cp "$TRANSPORT" "$m"
mutate "$m" 's|return nil, execFailure(err, stderr)|return nil, fmt.Errorf("provider exec failed (stderr suppressed): %w", err)|' \
  'stderr suppressed'
# Scoped to CallExec: `func execFailure` itself lives below it and always stays.
if extract_func "$m" CallExec | grep -Fq 'execFailure('; then
  fail "mutation harness: the REL-07 string-literal mutant's CallExec still calls execFailure — the fold was not actually removed"
fi
echo "OK: the REL-07 string-literal mutant removes the fold from CallExec entirely and keeps only the identifier's TEXT inside a double-quoted message"
if check_exec_containment "$m" 2>/dev/null; then
  echo "OK: KNOWN BLIND SPOT recorded — a bare return whose message merely contains the identifier passes both halves of the fold heuristic, exactly as documented. No comment filter can fix a string literal; the named-test pin plus 'go test' is what holds REL-07 closed."
else
  fail "the string-literal bare return is now REJECTED by the fold heuristic. That may be an improvement, but this control pins the DOCUMENTED blind spot: update check_exec_containment's comments, hack/audit/README.md and this control before claiming the wider guarantee."
fi

# ---- REL-07 TEST DELETION -----------------------------------------------
m="$WORK/transport_test.renamed.go"
cp "$ROOT/$REL07_TEST_FILE" "$m"
mutate "$m" "s/func ${REL07_TESTS[0]}(/func disabled${REL07_TESTS[0]}(/" "func disabled${REL07_TESTS[0]}("
[[ "$(grep -cE '^func Test' "$m")" -ge 1 ]] ||
  fail "mutation harness: the REL-07 test-rename mutant has no 'func Test' left — it would trip the corpus positive control instead of the name pin"
echo "OK: the REL-07 test-rename mutant still declares other 'func Test's — only the NAME pin sees the loss"
expect_red check_named_tests "the named test ${REL07_TESTS[0]} was renamed away — with the fold heuristic blind to a string-literal revert, this is the pin that remains (REL-07 primary pin)" \
  "is absent from" "REL-07" "$m" REL07_TESTS REL07_SUBTESTS "the stderr fold in CallExec"

m="$WORK/transport_test.nocase.go"
cp "$ROOT/$REL07_TEST_FILE" "$m"
mutate "$m" 's|t.Run("stderr_never_merged_into_facts"|t.Run("disabled_stderr_never_merged_into_facts"|' 'disabled_stderr_never_merged_into_facts'
grep -Fq "func ${REL07_TESTS[0]}(" "$m" ||
  fail "mutation harness: the dropped-case mutant lost the test function too — then the function pin catches it and this control says nothing about case coverage"
echo "OK: the REL-07 dropped-case mutant keeps the pinned test FUNCTION — only the case pin sees the lost subtest"
expect_red check_named_tests "a pinned subtest case was renamed away while the test function survived (REL-07 primary pin)" \
  "is absent from" "REL-07" "$m" REL07_TESTS REL07_SUBTESTS "the stderr fold in CallExec"

# ---- REL-03: the guard deleted at the call site, decoy left intact ----------
m="$WORK/provider_host.rel03.go"
cp "$PROVIDER_HOST" "$m"
# No parentheses in the pattern: awk -v strips one level of escaping, so an
# escaped `\(` would arrive as a capture group and silently match nothing.
delete_first "$m" '^[[:space:]]*if !errors'
assert_changed "$PROVIDER_HOST" "$m"
# THE scoping control for REL-03. loadResourceOwnerRegistry's identical D-130
# guard survives this mutation on purpose: a file-level grep for
# `errors.Is(err, forge.ErrNotFound)` is STILL GREEN on this mutant, so if the
# check below reds it can only be because it read resolveRunFacts' body.
decoys="$(grep -c 'errors.Is(err, forge.ErrNotFound)' "$m" || true)"
[[ "$decoys" -ge 1 ]] ||
  fail "mutation harness: the REL-03 mutant has no surviving errors.Is(err, forge.ErrNotFound) occurrence — the mutation was not surgical and this control cannot demonstrate scoping"
echo "OK: the REL-03 mutant still carries $decoys decoy errors.Is(err, forge.ErrNotFound) occurrence(s) — a file-level grep stays green on it"
expect_red check_notfound_discrimination "resolveRunFacts treats every FileAtRef error as 'declaration absent' again (REL-03), with the D-130 decoy guard still present" \
  "REL-03: resolveRunFacts does not discriminate" "$m"

# ---- REL-03 HALF-REVERT: the guard is kept, its body stops stopping the run --
# The mutation an independent reviewer used to show the previous, body-wide
# version of the branch assertion could not fail on ANY tree: resolveRunFacts
# has three `return nil, nil, fmt.Errorf(` lines, so a body-wide grep is
# satisfied even here — where the guard is intact, compiles, is gofmt-clean, and
# the REL-03 defect is fully restored. Only the guard-scoped assertion sees it.
m="$WORK/provider_host.rel03half.go"
cp "$PROVIDER_HOST" "$m"
# Paren-free pattern: awk -v strips one level of escaping, so an escaped `\(`
# arrives as an unbalanced group ("illegal primary in regular expression").
downgrade_first_return "$m" '^[[:space:]]*return nil, nil, fmt'
assert_changed "$PROVIDER_HOST" "$m"
if ! grep -Fq 'if !errors.Is(err, forge.ErrNotFound)' "$m"; then
  fail "mutation harness: the REL-03 half-revert lost the guard line — then it is the previous mutation again and proves nothing new about the branch assertion"
fi
half_returns="$(grep -c 'return nil, nil, fmt.Errorf(' "$m" || true)"
[[ "$half_returns" -ge 2 ]] ||
  fail "mutation harness: the REL-03 half-revert left only ${half_returns} 'return nil, nil, fmt.Errorf(' line(s) — the point of this control is that the SIBLING returns survive, so a body-wide grep stays green on it"
echo "OK: the REL-03 half-revert keeps the guard AND ${half_returns} sibling 'return nil, nil, fmt.Errorf(' line(s) — a body-wide grep stays green on it"
expect_red check_notfound_discrimination "the forge.ErrNotFound guard is kept but its body is downgraded to 'continue' — REL-03 restored behind an intact-looking guard" \
  "REL-03: the forge.ErrNotFound guard in resolveRunFacts does not RETURN" "$m"

# ---- REL-03 CAMOUFLAGE 1: the return survives only inside a block comment ----
# Review round 2. code_only used to strip `//` lines only, so quoting the return
# inside a `/* … */` span satisfied the assertion while REL-03 was fully
# restored — and it made the script's own "a return in a comment cannot satisfy
# this" claim false. Compiles, gofmt-clean, no behavioural change from the plain
# half-revert.
m="$WORK/provider_host.rel03comment.go"
cp "$PROVIDER_HOST" "$m"
cat >"$WORK/rep.blockcomment" <<'GOREP'
/*
return nil, nil, fmt.Errorf("provider %q declaration %q at ref %q: %w", name, declPath, targetRef, err)
*/
continue
GOREP
splice_first_return "$m" "$WORK/rep.blockcomment"
assert_changed "$PROVIDER_HOST" "$m"
grep -Fq '/*' "$m" || fail "mutation harness: the block-comment camouflage did not land — no /* span in the mutant"
grep -Fq 'return nil, nil, fmt.Errorf("provider %q declaration %q at ref %q: %w"' "$m" ||
  fail "mutation harness: the block-comment camouflage lost the quoted return — then it is the plain half-revert again and proves nothing about comment stripping"
echo "OK: the block-comment mutant still CONTAINS the required return text — only code_only's /* … */ stripping can tell it is not code"
expect_red check_notfound_discrimination "the guard's return survives only inside a /* … */ block comment (REL-03 behind comment camouflage)" \
  "REL-03: the forge.ErrNotFound guard in resolveRunFacts does not RETURN" "$m"

# ---- REL-03 CAMOUFLAGE 2: the return is reachable only under a nested if -----
# The one that needs no comment trick, and the reason the assertion measures the
# guard's TERMINAL path rather than "a return appears in the region". The return
# is real code at a real indent — but only `context.Canceled` reaches it; every
# 503, 401, 403 and throttle falls through to `continue`, which is REL-03 in
# full. Compiles, gofmt-clean, and a naive grep over the guard block is green.
m="$WORK/provider_host.rel03nested.go"
cp "$PROVIDER_HOST" "$m"
printf 'if errors.Is(err, context.Canceled) {\n\treturn nil, nil, fmt.Errorf("provider %%q declaration %%q at ref %%q: %%w", name, declPath, targetRef, err)\n}\ncontinue\n' >"$WORK/rep.nested"
splice_first_return "$m" "$WORK/rep.nested"
assert_changed "$PROVIDER_HOST" "$m"
grep -Fq 'errors.Is(err, context.Canceled)' "$m" || fail "mutation harness: the nested-if camouflage did not land"
guard_probe="$WORK/guard.nested"
extract_func "$m" resolveRunFacts | guard_block /dev/stdin | code_only >"$guard_probe"
grep -Eq '^[[:space:]]*return nil, nil, fmt\.Errorf\(' "$guard_probe" ||
  fail "mutation harness: the nested-if mutant's guard block carries no return at all — then it is the plain half-revert again and proves nothing about terminal-path scoping"
echo "OK: the nested-if mutant's guard block DOES contain a real, uncommented return — only the terminal-path indent check can tell it is unreachable for every forge error"
expect_red check_notfound_discrimination "the guard's return is reachable only under a nested context.Canceled condition, everything else continues (REL-03 nested fail-open)" \
  "REL-03: the forge.ErrNotFound guard in resolveRunFacts does not RETURN" "$m"

# ---- REL-03 CAMOUFLAGE 3: the POLARITY SWAP, the one that beat indent rules ---
# Review round 3. Semantically identical to camouflage 2, but written as the
# more idiomatic guard clause — so the return lands at EXACTLY the guard's body
# indent and assertion (a) accepts it:
#     if !errors.Is(err, context.Canceled) { continue }
#     return nil, nil, fmt.Errorf(...)
# gofmt -l silent, `task fmt` no change, go vet clean, golangci-lint 0 issues,
# go build OK — and every forge error except context.Canceled is skipped
# (`go test ./cmd/assent` fails the forge-error and 401/403 cases). Only the
# absence-of-fall-through assertion (b) sees it.
m="$WORK/provider_host.rel03polarity.go"
cp "$PROVIDER_HOST" "$m"
printf 'if !errors.Is(err, context.Canceled) {\n\tcontinue\n}\nreturn nil, nil, fmt.Errorf("provider %%q declaration %%q at ref %%q: %%w", name, declPath, targetRef, err)\n' >"$WORK/rep.polarity"
splice_first_return "$m" "$WORK/rep.polarity"
assert_changed "$PROVIDER_HOST" "$m"
# THE precondition that makes this control mean something: assertion (a) — the
# terminal-path indent rule — must ACCEPT this mutant. If it did not, the
# control would be re-proving (a) instead of proving (b).
polarity_guard="$WORK/guard.polarity"
extract_func "$m" resolveRunFacts | guard_block /dev/stdin | code_only >"$polarity_guard"
polarity_indent="$(sed -n '1s/^\([[:space:]]*\).*/\1/p' "$polarity_guard")"
awk -v ind="$(printf '%s\t' "$polarity_indent")" '
      index($0, ind) == 1 &&
      substr($0, length(ind) + 1) ~ /^return nil, nil, fmt\.Errorf\(/ { found = 1 }
      END { exit(found ? 0 : 1) }
    ' "$polarity_guard" ||
  fail "mutation harness: the polarity-swap mutant has no return at the guard's body indent — then the terminal-path rule would already have caught it and this control does not prove the absence assertion is what discriminates"
echo "OK: the polarity-swap mutant puts a real, uncommented return at EXACTLY the guard's body indent — the indent rule accepts it; only the absence-of-fall-through assertion refuses it"
expect_red check_notfound_discrimination "the idiomatic polarity swap: 'if !errors.Is(err, context.Canceled) { continue }' then a return at the right indent — a full REL-03 revert that is gofmt/vet/lint clean" \
  "REL-03: the forge.ErrNotFound guard in resolveRunFacts contains a FALL-THROUGH statement" "$m"

# ---- REL-03 TEST DELETION: the half `go test` structurally cannot catch ------
# The reviewer's closing demonstration: revert REL-03 AND delete its behavioural
# tests. `go test ./cmd/assent/` is then GREEN — a test that does not exist
# cannot fail — and every source-shape heuristic can be walked around by
# restructuring production code. This control is why the named-test pin is the
# PRIMARY assertion and not a nicety.
m="$WORK/provider_host_test.renamed.go"
cp "$ROOT/$REL03_TEST_FILE" "$m"
mutate "$m" "s/func ${REL03_TESTS[0]}(/func disabled${REL03_TESTS[0]}(/" "func disabled${REL03_TESTS[0]}("
# It must still be a plausible test file: this control grades the NAME pin, not
# "the file went missing", which the corpus positive control already covers.
[[ "$(grep -cE '^func Test' "$m")" -ge 1 ]] ||
  fail "mutation harness: the REL-03 test-rename mutant has no 'func Test' left — it would trip the corpus positive control instead of the name pin, and prove nothing about renaming"
echo "OK: the REL-03 test-rename mutant still declares other 'func Test's — only the NAME pin can tell that the one holding REL-03 closed is gone"
expect_red check_named_tests "the named test ${REL03_TESTS[0]} was renamed away — 'go test' reports a deleted test as SUCCESS (REL-03 primary pin)" \
  "is absent from" "REL-03" "$m" REL03_TESTS REL03_SUBTESTS "the forge-error/absent-file discrimination in resolveRunFacts"

# A dropped table CASE is the quieter version: every function name survives.
m="$WORK/provider_host_test.nocase.go"
cp "$ROOT/$REL03_TEST_FILE" "$m"
mutate "$m" 's|\[\]int{401, 403}|[]int{403}|' '[]int{403}'
grep -Fq "func ${REL03_TESTS[1]}(" "$m" ||
  fail "mutation harness: the dropped-case mutant lost the test function too — then the function pin catches it and this control says nothing about case coverage"
echo "OK: the dropped-case mutant keeps every pinned test FUNCTION — only the case pin sees that 401 is no longer exercised"
expect_red check_named_tests "the 401 case was dropped from the auth table while every test function name survived (REL-03 primary pin)" \
  "is absent from" "REL-03" "$m" REL03_TESTS REL03_SUBTESTS "the forge-error/absent-file discrimination in resolveRunFacts"

# ---- REL-03 CONDITION NARROWING: the shape heuristics' documented blind spot -
# One line, no camouflage, no fall-through, return at the correct body indent:
#     if !errors.Is(err, forge.ErrNotFound) && errors.Is(err, context.Canceled) {
# gofmt/vet/build clean, and it reverts REL-03 in full. It passes EVERY
# source-shape heuristic below — deliberately NOT patched around, because the
# space of such spellings does not shrink and four rounds of trying proved it.
# It is caught by `go test` (check stage 4, and a step of the same PR-visible
# verify job), whose named tests this gate pins. Recording the blind spot as an
# executable fact rather than a comment means a future "simplification" that
# drops the name pin turns this control red instead of passing quietly.
m="$WORK/provider_host.rel03narrow.go"
cp "$PROVIDER_HOST" "$m"
mutate "$m" 's|if !errors.Is(err, forge.ErrNotFound) {|if !errors.Is(err, forge.ErrNotFound) \&\& errors.Is(err, context.Canceled) {|' \
  'errors.Is(err, context.Canceled)'
if check_notfound_discrimination "$m" 2>/dev/null; then
  echo "OK: KNOWN BLIND SPOT recorded — the condition-narrowing revert passes every source-shape heuristic, exactly as documented. What holds it closed is the named-test pin above plus 'go test', not this function's text analysis."
else
  fail "the condition-narrowing revert is now REJECTED by the source-shape heuristics. That may be an improvement, but this control pins the DOCUMENTED blind spot: update check_notfound_discrimination's comments, hack/audit/README.md and this control before claiming the wider guarantee."
fi

# ---- SEC-03: the identity pin deleted, and the two files drifted apart ------
m="$WORK/install.sec03.sh"
cp "$INSTALL" "$m"
grep -vF -- '--certificate-identity-regexp' "$INSTALL" >"$m"
assert_changed "$INSTALL" "$m"
expect_red check_cosign_pin "hack/install.sh's --certificate-identity-regexp is gone (SEC-03)" \
  "carries no --certificate-identity-regexp" "$m" "$SECURITY"

m="$WORK/SECURITY.sec03.md"
cp "$SECURITY" "$m"
# Flag-relative rewrite, never a pattern built from the pinned value: the
# identity pin is itself a regexp ([Aa], escaped dots), so interpolating it
# would match nothing and the mutation would silently not land.
mutate "$m" 's|\(--certificate-oidc-issuer \)[^[:space:]]*|\1https://accounts.example.invalid|g' 'https://accounts.example.invalid'
expect_red check_cosign_pin "SECURITY.md's published issuer was changed without install.sh following (SEC-03 drift)" \
  "SEC-03: DRIFT" "$INSTALL" "$m"

# ---- TEST-02: the classifier term, and the named case that defends it ------
m="$WORK/classify_intervention.test02.go"
cp "$CLASSIFY" "$m"
mutate "$m" 's/ || e == aggregate.EffectChallenge//' 'aggregate.EffectRequireReview'
assert_changed "$CLASSIFY" "$m"
if grep -Fq 'aggregate.EffectChallenge' "$m"; then
  fail "mutation harness: the TEST-02 mutant still carries aggregate.EffectChallenge — the deletion did not land"
fi
expect_red check_challenge_classification "the EffectChallenge term was deleted from isStricterInterventionEffect (TEST-02 — the auditor's demonstrated mutant)" \
  "TEST-02: isStricterInterventionEffect no longer classifies" "$m" "$CLASSIFY_TEST"

m="$WORK/classify_intervention_test.test02.go"
cp "$CLASSIFY_TEST" "$m"
mutate "$m" "s/func ${CHALLENGE_TEST}(/func disabled${CHALLENGE_TEST}(/" "func disabled${CHALLENGE_TEST}("
expect_red check_challenge_classification "the named EffectChallenge unit case was renamed away (TEST-02)" \
  "is absent from" "$CLASSIFY" "$m"


# ============================================================================
# (6) Wiring — REAL TREE FIRST, then its mutations
# ============================================================================

# Order matters, and it is the same order sections 1-4 use. If the real-tree
# greens ran AFTER the mutants, a genuinely reverted `check:` line / CHECK_STAGES
# entry / verify.yaml step would surface as the harness's own
# "mutation did not land: <file> is byte-identical to its source" — fail-closed,
# but pointing the maintainer at the harness instead of naming the REQ that was
# broken. Green first means the REQ-named message always wins.

echo
# ---- GUTTED TEST BODIES: a name is not a behaviour --------------------------
# The round-five escape, as executable controls. Both keep every pinned NAME and
# every pinned case string, so check_named_tests stays green on them; both leave
# production source untouched until the revert is applied by the run itself.
# They are caught by different halves, which is why both are here:
#   * t.Skip  -> `go test` reports `--- SKIP:`, never `--- PASS:`, so the
#     unmutated control run catches it before the revert is even applied;
#   * a no-op body -> reports `--- PASS:` and keeps passing WITH the revert
#     applied, so only the reverted run catches it.
# `go test` exits 0 for both. So does a name pin. So did this gate, one round ago.
if ((TEXT_ONLY == 0)); then
  gut="$WORK/provider_host_test.skipped.go"
  awk '
    /^func TestProviderDeclarationForgeErrorAbortsResolveRunFacts\(/ ||
    /^func TestProviderDeclarationUnauthorizedAbortsResolveRunFacts\(/ {
      print
      print "\tt.Skip(\"temporarily disabled\")"
      print "\t// coverage note: this used to iterate []int{401, 403}"
      gutting = 1
      depth = 1
      next
    }
    gutting {
      if ($0 == "}") { print "}"; gutting = 0 }
      next
    }
    { print }
  ' "$ROOT/$REL03_TEST_FILE" >"$gut"
  grep -Fq 't.Skip("temporarily disabled")' "$gut" ||
    fail "mutation harness: the t.Skip gutting did not land in the REL-03 test file"
  for fn in "${REL03_TESTS[@]}"; do
    grep -Fq "func ${fn}(" "$gut" ||
      fail "mutation harness: the t.Skip mutant lost the NAME ${fn} — then check_named_tests catches it and this control proves nothing about names surviving"
  done
  grep -Fq '[]int{401, 403}' "$gut" ||
    fail "mutation harness: the t.Skip mutant lost the case pin text — it must survive (in a comment) for this control to show that a case pin can be satisfied by prose"
  check_named_tests "REL-03" "$gut" REL03_TESTS REL03_SUBTESTS "x" 2>/dev/null &&
    echo "OK: the t.Skip mutant keeps every pinned NAME and case string — check_named_tests is GREEN on it, which is exactly the gap the behavioural run closes" ||
    echo "OK: check_named_tests already refuses the t.Skip mutant (the case pin now reads code, not comments) — the behavioural run below is the independent second half"
  expect_red check_behavioural_revert "the pinned REL-03 tests keep their names but t.Skip their bodies — 'go test' exits 0 and reports --- SKIP:, never --- PASS:" \
    "did not report '--- PASS:'" \
    "REL-03" rel03skip "$REL03_SRC" "$REL03_REVERT" "$REL03_REVERT_WITNESS" "$REL03_RUN" "$REL03_PKG" REL03_TESTS \
    "the forge-error/absent-file discrimination in resolveRunFacts" "${REL03_TEST_FILE}=${gut}" REL03_MUSTFAIL

  gut2="$WORK/provider_host_test.noop.go"
  awk '
    /^func TestProviderDeclarationForgeErrorAbortsResolveRunFacts\(/ ||
    /^func TestProviderDeclarationUnauthorizedAbortsResolveRunFacts\(/ {
      print
      print "\t_ = t"
      print "\t// coverage note: this used to iterate []int{401, 403}"
      gutting = 1
      next
    }
    gutting {
      if ($0 == "}") { print "}"; gutting = 0 }
      next
    }
    { print }
  ' "$ROOT/$REL03_TEST_FILE" >"$gut2"
  grep -Fq '_ = t' "$gut2" || fail "mutation harness: the no-op gutting did not land"
  grep -Fq 't.Skip' "$gut2" &&
    fail "mutation harness: the no-op mutant contains t.Skip — then it is the previous control again, and it would be caught by the --- PASS: check rather than by the reverted run"
  echo "OK: the no-op mutant has NO t.Skip — it reports '--- PASS:' like a live test, so only the REVERTED run can tell it is empty"
  expect_red check_behavioural_revert "the pinned REL-03 tests keep their names and PASS, but their bodies assert nothing — the revert no longer makes them fail" \
    "they no longer detect the defect they exist to detect" \
    "REL-03" rel03noop "$REL03_SRC" "$REL03_REVERT" "$REL03_REVERT_WITNESS" "$REL03_RUN" "$REL03_PKG" REL03_TESTS \
    "the forge-error/absent-file discrimination in resolveRunFacts" "${REL03_TEST_FILE}=${gut2}" REL03_MUSTFAIL

  gut3="$WORK/transport_test.noop.go"
  awk '
    /^func TestExecStderrFoldedIntoError\(/ {
      print
      print "\t_ = t"
      gutting = 1
      next
    }
    gutting {
      if ($0 == "}") { print "}"; gutting = 0 }
      next
    }
    { print }
  ' "$ROOT/$REL07_TEST_FILE" >"$gut3"
  grep -Fq '_ = t' "$gut3" || fail "mutation harness: the REL-07 no-op gutting did not land"
  grep -Fq "func ${REL07_TESTS[0]}(" "$gut3" ||
    fail "mutation harness: the REL-07 no-op mutant lost the test NAME — then the name pin catches it and this control says nothing"
  expect_red check_behavioural_revert "the pinned REL-07 test keeps its name and PASSes, but its body asserts nothing — dropping the stderr fold no longer makes it fail" \
    "they no longer detect the defect they exist to detect" \
    "REL-07" rel07noop "$REL07_SRC" "$REL07_REVERT" "$REL07_REVERT_WITNESS" "$REL07_RUN" "$REL07_PKG" REL07_TESTS \
    "the stderr fold in CallExec" "${REL07_TEST_FILE}=${gut3}" REL07_MUSTFAIL

  # ---- QUORUM ESCAPE 1: gut ONE of the two REL-03 tests -------------------
  # Round six. Production source untouched; only
  # TestProviderDeclarationUnauthorizedAbortsResolveRunFacts is emptied, with
  # `_ = []int{401, 403}` left as LIVE CODE so the case pin stays green too.
  # Its sibling still fails under the revert, so an "at least one pinned test
  # failed" quorum was satisfied and the gate exited 0 — the whole 401/403 auth
  # branch silently undefended. Only an ALL-of quorum refuses this.
  gut4="$WORK/provider_host_test.onegutted.go"
  awk '
    /^func TestProviderDeclarationUnauthorizedAbortsResolveRunFacts\(/ {
      print
      print "\t_ = []int{401, 403}"
      print "\t_ = t"
      gutting = 1
      next
    }
    gutting {
      if ($0 == "}") { print "}"; gutting = 0 }
      next
    }
    { print }
  ' "$ROOT/$REL03_TEST_FILE" >"$gut4"
  grep -Fq '_ = []int{401, 403}' "$gut4" ||
    fail "mutation harness: the one-gutted mutant did not land"
  grep -Fq "func ${REL03_TESTS[0]}(" "$gut4" ||
    fail "mutation harness: the one-gutted mutant lost the SIBLING test — the whole point is that the sibling still fails and satisfies a weak quorum"
  check_named_tests "REL-03" "$gut4" REL03_TESTS REL03_SUBTESTS "x" 2>/dev/null ||
    fail "mutation harness: check_named_tests reds on the one-gutted mutant — then this control grades the name/case pins, not the quorum. The case table must stay in LIVE code for it to mean anything"
  echo "OK: the one-gutted mutant passes every NAME and CASE pin (the []int{401, 403} table is live code, not a comment) and its sibling still fails under the revert — only an ALL-of quorum can refuse it"
  expect_red check_behavioural_revert "one of the two REL-03 tests is emptied while its sibling still fails — a weak 'at least one' quorum was satisfied by the sibling" \
    "did NOT report '--- FAIL:'" \
    "REL-03" rel03onegut "$REL03_SRC" "$REL03_REVERT" "$REL03_REVERT_WITNESS" "$REL03_RUN" "$REL03_PKG" REL03_TESTS \
    "the forge-error/absent-file discrimination in resolveRunFacts" "${REL03_TEST_FILE}=${gut4}" REL03_MUSTFAIL

  # ---- QUORUM ESCAPE 2: empty REL-07's actual canary subtest ---------------
  # `nonzero_exit_reports_stderr` IS the behavioural defence of the fold. Empty
  # its body, keep its name: the function-level `--- FAIL:` line still appears
  # (its sibling runaway_stderr_is_bounded_and_truncated still fails), the case
  # pin still matches the t.Run name, and the gate exited 0. The subtest-level
  # quorum is what sees it.
  gut5="$WORK/transport_test.canaryemptied.go"
  awk '
    /t\.Run\("nonzero_exit_reports_stderr"/ {
      print
      print "\t\t_ = q"
      gutting = 1
      next
    }
    gutting {
      if ($0 == "\t})") { print "\t})"; gutting = 0 }
      next
    }
    { print }
  ' "$ROOT/$REL07_TEST_FILE" >"$gut5"
  grep -Fq 't.Run("nonzero_exit_reports_stderr"' "$gut5" ||
    fail "mutation harness: the canary-emptied mutant lost the subtest NAME — then the case pin catches it and this control says nothing about the quorum"
  grep -Fq 'runaway_stderr_is_bounded_and_truncated' "$gut5" ||
    fail "mutation harness: the canary-emptied mutant lost the sibling subtest — the point is that the sibling still fails and produces a function-level --- FAIL: line"
  check_named_tests "REL-07" "$gut5" REL07_TESTS REL07_SUBTESTS "x" 2>/dev/null ||
    fail "mutation harness: check_named_tests reds on the canary-emptied mutant — then this control grades the case pins, not the quorum"
  echo "OK: the canary-emptied mutant keeps every name and case pin, and its sibling still fails — only the SUBTEST-level quorum can refuse it"
  expect_red check_behavioural_revert "REL-07's canary subtest nonzero_exit_reports_stderr is emptied while its sibling still fails — the function-level FAIL line is still emitted" \
    "did NOT report '--- FAIL:'" \
    "REL-07" rel07canary "$REL07_SRC" "$REL07_REVERT" "$REL07_REVERT_WITNESS" "$REL07_RUN" "$REL07_PKG" REL07_TESTS \
    "the stderr fold in CallExec" "${REL07_TEST_FILE}=${gut5}" REL07_MUSTFAIL

  # The harness must also refuse a mutant that merely fails to COMPILE: "the
  # reverted copy is red" is not evidence about behaviour.
  expect_red check_behavioural_revert "the revert program produces code that does not build — red for the wrong reason" \
    "failed to BUILD rather than failing its tests" \
    "REL-07" rel07nobuild "$REL07_SRC" 's|return nil, execFailure(err, stderr)|return nil, notAThing(err)|' \
    'notAThing(err)' "$REL07_RUN" "$REL07_PKG" REL07_TESTS "the stderr fold in CallExec" "" REL07_MUSTFAIL
fi

echo "== (6) REQ-AUD2-S05-03/04/05 — this gate is wired where it can be seen =="
expect_green check_task_wiring "'task check' runs ${STAGE}, which invokes ${SELF}" "$TASKFILE"
expect_green check_stage_pinned "${STAGE} is pinned in CHECK_STAGES (AUD-S18 grades it)" "$AUD_GATE"
expect_green check_pr_wiring "the pull-request-visible 'verify' job runs ${SELF}, undisarmed" "$WORKFLOW"

echo "== (6b) the wiring assertions proved capable of going RED =="
m="$WORK/Taskfile.nostage.yml"
grep -vE "^[[:space:]]+- task: ${STAGE}\$" "$TASKFILE" >"$m"
assert_changed "$TASKFILE" "$m"
expect_red check_task_wiring "'- task: ${STAGE}' was deleted from check: (REQ-AUD2-S05-04)" \
  "'task check' does not run" "$m"

# REQ-AUD2-S05-03's SECOND half, previously asserted but never controlled: the
# stage stays wired and stays named, and its body stops invoking this script.
# `task check` is still green, the stage banner still prints, and nothing is
# graded — the AUD-S18 stage-body lesson (a gutted `coverage:` body kept four
# structural pins green) applied to this gate's own stage.
m="$WORK/Taskfile.gutted.yml"
sed "s|bash ${SELF}|true # gutted|" "$TASKFILE" >"$m"
assert_changed "$TASKFILE" "$m"
grep -qE "^[[:space:]]+- task: ${STAGE}\$" "$m" ||
  fail "mutation harness: the gutted Taskfile also lost the '- task: ${STAGE}' line — then it is the previous mutation again and proves nothing about the stage BODY"
expect_red check_task_wiring "the ${STAGE} stage is still wired and named, but its body no longer runs ${SELF} (REQ-AUD2-S05-03)" \
  "does not invoke" "$m"

# The stage runs the gate, and runs it DISARMED: `--text-only` skips the two
# behavioural mutation runs, which are the only assertions that measure REL-03
# and REL-07 instead of reading their source. Everything else stays green.
m="$WORK/Taskfile.textonly.yml"
sed "s|bash ${SELF}|bash ${SELF} --text-only|" "$TASKFILE" >"$m"
assert_changed "$TASKFILE" "$m"
grep -Fq "$SELF" "$m" ||
  fail "mutation harness: the --text-only mutant lost the gate invocation — this control must keep it, or it grades absence instead of disarmament"
expect_red check_task_wiring "'task check' runs the gate with --text-only, skipping the behavioural runs (REQ-AUD2-S05-03)" \
  "WITH ARGUMENTS" "$m"

m="$WORK/exitgate.nopin.sh"
grep -vE "^[[:space:]]+${STAGE}\$" "$AUD_GATE" >"$m"
assert_changed "$AUD_GATE" "$m"
expect_red check_stage_pinned "the stage was dropped from CHECK_STAGES (AUD-S18 lockstep)" \
  "is not in CHECK_STAGES" "$m"

m="$WORK/verify.nostep.yaml"
grep -vF "$SELF" "$WORKFLOW" >"$m"
assert_changed "$WORKFLOW" "$m"
expect_red check_pr_wiring "the AUD2 gate step was deleted from the verify job (REQ-AUD2-S05-05 — RELSE-08 reproduced)" \
  "does not invoke" "$m"

m="$WORK/verify.pushonly.yaml"
# Give the verify job release-exitgate's own push-only guard: the step is still
# there, and the gate is still invisible on pull requests. This is RELSE-08
# itself, reproduced — the mutation a "the step exists" grep would not catch.
awk '{ print } $0 == "  verify:" { print "    if: github.event_name != '\''pull_request'\''" }' "$WORKFLOW" >"$m"
assert_changed "$WORKFLOW" "$m"
if ! grep -Fq "$SELF" "$m"; then
  fail "mutation harness: the push-only verify mutant lost the ${SELF} step — this control must keep the step present, or it grades absence instead of visibility"
fi
expect_red check_pr_wiring "the verify job grew release-exitgate's push-only if: while still carrying the step (RELSE-08 reproduced)" \
  "JOB-LEVEL if:" "$m"

m="$WORK/verify.nopr.yaml"
# The step stays, the job keeps no if:, and the gate still loses every pull
# request — because the WORKFLOW stopped triggering on them.
grep -vE '^[[:space:]]+pull_request:[[:space:]]*$' "$WORKFLOW" >"$m"
assert_changed "$WORKFLOW" "$m"
if ! grep -Fq "$SELF" "$m"; then
  fail "mutation harness: the no-pull_request mutant lost the ${SELF} step — this control must keep the step present, or it grades absence instead of trigger reach"
fi
expect_red check_pr_wiring "the workflow stopped triggering on pull_request while the step stayed wired" \
  "no longer triggers on pull_request" "$m"

m="$WORK/verify.textonly.yaml"
sed "s|bash ${SELF}|bash ${SELF} --text-only|" "$WORKFLOW" >"$m"
assert_changed "$WORKFLOW" "$m"
grep -Fq "$SELF" "$m" ||
  fail "mutation harness: the --text-only workflow mutant lost the step — it must stay present for this control to grade disarmament"
expect_red check_pr_wiring "the verify job runs the gate with --text-only — wired, PR-visible, and certifying nothing about REL-03/REL-07 (REQ-AUD2-S05-05)" \
  "WITH ARGUMENTS" "$m"

# REQ-AUD2-S05-05's core property, previously asserted but never controlled: the
# step is present, the job runs on PRs, and the step is DISARMED — a red gate
# then does not fail the check. `continue-on-error:` and `if:` are the two
# spellings; both are pinned, because actionlint is green on both.
for disarm in "continue-on-error: true" "if: false"; do
  m="$WORK/verify.disarmed.${disarm%%:*}.yaml"
  awk -v self="$SELF" -v d="        $disarm" '
    { print }
    index($0, self) > 0 { print d }
  ' "$WORKFLOW" >"$m"
  assert_changed "$WORKFLOW" "$m"
  grep -Fq "$SELF" "$m" ||
    fail "mutation harness: the disarm mutant lost the ${SELF} step — this control must keep the step PRESENT, or it grades absence instead of disarmament"
  expect_red check_pr_wiring "the AUD2 gate step is present on PRs but DISARMED with '${disarm}' (REQ-AUD2-S05-05)" \
    "DISARMED" "$m"
done


# The floor is a FLOOR, and the banner below says exactly that. An earlier
# wording claimed the controls proved "every assertion" could fail; this script
# emits roughly forty distinct findings and thirty-eight of them carry a control,
# so that claim overstated the gate's own coverage — the D-124 failure mode this
# epic exists to close, in the gate built to close it. What IS asserted: every
# control that exists ran, each went red for its own pinned message, and every
# REQ-bearing assertion (REQ-AUD2-S05-02/03/04/05 and the four findings) has one.
# Mode-aware floor: --text-only legitimately skips the four behavioural
# controls, and a single number would either pass a truncated full run or red
# every text-only run. Both floors are pinned so neither mode can quietly shrink.
CONTROL_FLOOR=38
((TEXT_ONLY == 0)) || CONTROL_FLOOR=32
((MUTATIONS_PROVED >= CONTROL_FLOOR)) ||
  fail "only ${MUTATIONS_PROVED} mutation controls ran in $([[ $TEXT_ONLY == 1 ]] && echo --text-only || echo full) mode, fewer than the pinned floor of ${CONTROL_FLOOR} — a section was skipped, so REQ-AUD2-S05-02's evidence is incomplete"

# ============================================================================
# REQ-AUD2-S05-01 — the disposition statement
# ============================================================================

echo
echo "== AUD2 findings dispositioned (REQ-AUD2-S05-01) =="
echo "  REL-01  (AUD2-S01) CLOSED — CallExec's stdout is bounded at MaxResponseBytes"
echo "  REL-02  (AUD2-S01) CLOSED — CallExec sets cmd.WaitDelay = opts.Timeout"
echo "  REL-07  (AUD2-S01) CLOSED — CallExec captures stderr; the fold is held by ${REL07_TESTS[0]} (pinned by name) + go test, with a source-shape heuristic as secondary"
echo "  REL-03  (AUD2-S02) CLOSED — resolveRunFacts discriminates ErrNotFound; held by ${#REL03_TESTS[@]} named tests (pinned) + go test, with a source-shape heuristic as secondary"
echo "  SEC-03  (AUD2-S03) CLOSED — hack/install.sh pins the signer identity + OIDC issuer, no drift from SECURITY.md"
echo "  TEST-02 (AUD2-S04) CLOSED — EffectChallenge is classified and ${CHALLENGE_TEST} defends it"
echo
if ((TEXT_ONLY == 1)); then
  echo "PASS (--text-only): the PATTERN layer and its mutation harness only."
  echo "  This run did NOT execute the behavioural mutation runs, so it certifies NOTHING about"
  echo "  whether REL-03 and REL-07 are still held closed: their name pins cannot tell a live"
  echo "  test from an emptied one. Run with no arguments for that. 'task check' and verify.yaml"
  echo "  both invoke this script with no arguments, and the wiring checks above refuse otherwise."
  exit 0
fi
echo "PASS: aud2_exitgate_test.sh — all four AUD2 remediations dispositioned (REL-01/02/07, REL-03, SEC-03, TEST-02)."
echo "  HOW each is held closed, precisely: SEC-03 and REL-01/02 by source pins over hack/install.sh and CallExec; TEST-02 by its named unit case. REL-03 and REL-07 by a BEHAVIOURAL MUTATION RUN — the revert is applied to a throwaway copy of the tree and the pinned tests must FAIL against it — which is indifferent to how the production revert is spelled AND to how the tests might be hollowed out (t.Skip, a no-op body, a deleted assertion all make it red). The name/case pins beside it are a fast, precisely-worded pre-check, not the property; the source-shape checks are secondary heuristics with two recorded blind spots."
echo "  ${MUTATIONS_PROVED} mutation controls ran (floor-asserted), each red for its own stated reason, covering every REQ-bearing assertion here — NOT every finding this script can emit. The gate is wired into 'task check', pinned in CHECK_STAGES, and run by the pull-request-visible verify job."
