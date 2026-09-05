#!/usr/bin/env bash
# commit_subject_gate.sh — REDMAIN-N1 / D-168.
#
# A commit subject must not START with a non-ASCII character.
#
# WHY THIS IS A GATE AND NOT A STYLE PREFERENCE
#   `GUIDELINES.md` § Repository discipline mandates the ASCII gitmoji shortcode
#   (`:construction_worker:`), and `cliff.toml`'s commit_parsers key on exactly
#   that spelling. A subject that leads with a LITERAL emoji (`👷 ci(docs): …`)
#   matches none of them and falls through the `.*` catch-all into `### Other` on
#   the published GitHub Release page — the REL-14 / D-137 defect reached through
#   a different door. `dfdae69` is the commit where the human-attention
#   enforcement failed; this file is the replacement for that attention.
#
# THE RULE, STATED NARROWLY ON PURPOSE
#   "first character is not ASCII" — NOT "the subject matches the full convention".
#   Two legitimate subject shapes in this repository's history do not carry a
#   shortcode at all and must not be rejected: Dependabot's `build(deps): bump …`
#   and GitHub's `Merge pull request #N from …`. Over all of this repo's history
#   the narrow rule has exactly one hit, `dfdae69`, which is the defect itself.
#   Widening this gate to the full convention is a separate decision with its own
#   evidence; do not do it by editing the pattern here.
#
# PUBLISHED HISTORY IS TOLERATED BY SHA, NEVER BY SHAPE
#   Hard rule 2 forbids rewriting `dfdae69`, so it is exempted — by full commit
#   SHA, in LEGACY_ALLOW_SHAS below. A SHA cannot be inherited by a future commit,
#   which a pattern-shaped exemption could be. Three self-checks keep the
#   exemption honest, all in self mode:
#     * every listed SHA must RESOLVE here and be an ancestor of HEAD;
#     * every listed SHA must ITSELF be a detection — an exempt commit that is not
#       a violation is stale scaffolding and reds the gate;
#     * the detector is re-proved on every run against a fabricated known-bad and
#       known-good subject, so a mistyped pattern fails loudly instead of matching
#       nothing and reporting success (the vacuity mode this repo has shipped
#       twice).
#   The list's LENGTH AND CONTENT are pinned from outside this file, in
#   changelog_gate_test.sh §9d — otherwise a lane could land a literal-emoji
#   commit and append its SHA in a later commit of the same PR, and all three
#   self-checks above would still pass (it resolves, it is an ancestor, it IS a
#   real detection). Growing this list is a two-file change on purpose.
#
# WHAT THIS LIST IS *NOT* (P2-1 — read this before deleting an entry)
#   Its predicate is "this COMMIT SUBJECT is a literal emoji and cannot be
#   rewritten". That fact is PERMANENT: `dfdae69`'s subject stays a literal emoji
#   for the life of the repository, so this entry is permanent too.
#   `changelog_gate_test.sh` §8 keeps a SEPARATE list, OTHER_EXEMPT_SHAS, whose
#   predicate is the different and TEMPORARY one "this commit's RENDERED entry is
#   still mis-filed under `### Other`". When REDMAIN-N3 teaches `cliff.toml` to
#   file the entry correctly, §8's list retires and THIS one does not. The two
#   were briefly derived from one another; that coupling had no green state after
#   N3 (keep the entry and §8 reds as stale; drop it and this gate reds on
#   `dfdae69`), so they are deliberately independent, linked only by the one-way
#   subset invariant §8 asserts through `--legacy-shas`.
#
# USAGE
#   commit_subject_gate.sh                 scan this repository (self mode)
#   commit_subject_gate.sh <repo-dir>      scan another repository (foreign mode —
#                                          used by changelog_gate_test.sh §9 to
#                                          drive both polarities over a sandbox;
#                                          the exemption self-checks are skipped
#                                          because the SHAs do not exist there)
#   commit_subject_gate.sh --legacy-shas   print the exempt SHAs, one per line
#                                          (self mode only). §8 reads it to assert
#                                          its own OTHER_EXEMPT_SHAS is a SUBSET
#                                          of this list, and §9d reads it to pin
#                                          the list's exact content.
#
# Deliberately bash-3.2 clean (no associative arrays, no mapfile) — see
# hack/lint/bash_version_guard_test.sh for why that matters in this tree.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

# Full 40-hex SHAs of commits in PUBLISHED history that violate the rule and
# cannot be rewritten. Append only with a recorded reason; never a prefix, never
# a pattern. Adding one here ALSO requires updating LEGACY_EXPECTED in
# changelog_gate_test.sh §9d, which is what makes it a deliberate decision.
#   dfdae69 — `👷 ci(docs): stop uploading the Pages artifact on pull requests`.
#             Landed before any gate existed; REDMAIN-N1. Rewriting it would
#             rewrite published history (hard rule 2). PERMANENT — see "WHAT THIS
#             LIST IS NOT" above; REDMAIN-N3 does not retire it.
LEGACY_ALLOW_SHAS=(
  dfdae69143c3bd5b4819df106bf6fbbad18eb4fc
)

MODE_SHAS=0
REPO="$ROOT"
case "${1:-}" in
  --legacy-shas)
    MODE_SHAS=1
    ;;
  "") ;;
  -*)
    echo "usage: $(basename "$0") [--legacy-shas | <repo-dir>]" >&2
    exit 2
    ;;
  *)
    REPO="$(cd "$1" && pwd)"
    ;;
esac
SELF=0
[[ "$REPO" == "$ROOT" ]] && SELF=1

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

TAB=$'\t'
# The detector. One definition, used for the real scan AND for the self-proof
# below, so the two can never diverge. LC_ALL=C makes `[^ -~]` a BYTE class —
# every byte outside printable ASCII, which is every lead byte of a UTF-8
# emoji — instead of a locale-dependent character class (`[:ascii:]` is a PCRE
# extension GNU grep does not have).
SUBJECT_VIOLATION_RE="^[0-9a-f]{7,40}${TAB}[^ -~]"

# detect <log-file> <out-file> — write the violating `<sha>\t<subject>` lines.
# Returns 0 whether or not it found any; callers test the output file, so a
# grep-exit-1 cannot be confused with a scan that did not run.
detect() {
  LC_ALL=C grep -E "$SUBJECT_VIOLATION_RE" "$1" >"$2" || true
}

# ------------------------------------------------------ detector self-proof --
#
# Runs on EVERY invocation, before anything is concluded from a clean scan. A
# clean result is only evidence if the detector still detects.
printf '%s\n' \
  "1111111111111111111111111111111111111111${TAB}👷 ci(docs): a literal-emoji subject" \
  "2222222222222222222222222222222222222222${TAB}:construction_worker: ci(docs): the same subject, spelled correctly" \
  "3333333333333333333333333333333333333333${TAB}build(deps): bump something from 1 to 2" \
  "4444444444444444444444444444444444444444${TAB}Merge pull request #1 from org/branch" \
  >"$WORK/selfproof.log"
detect "$WORK/selfproof.log" "$WORK/selfproof.hits"
[[ "$(wc -l <"$WORK/selfproof.hits" | tr -d ' ')" -eq 1 ]] \
  || fail "detector self-proof: the fabricated known-bad subject is not the ONE hit ($(wc -l <"$WORK/selfproof.hits" | tr -d ' ') hit(s)) — SUBJECT_VIOLATION_RE is broken, so a clean scan below would prove nothing"
grep -q '^1111111111111111111111111111111111111111' "$WORK/selfproof.hits" \
  || fail "detector self-proof: the single hit is not the known-bad line — SUBJECT_VIOLATION_RE matches the wrong thing"

# ------------------------------------------------------------- the real scan --

git -C "$REPO" log --format="%H${TAB}%s" HEAD >"$WORK/log" 2>"$WORK/log.err" || {
  cat "$WORK/log.err" >&2
  fail "could not read commit subjects from $REPO"
}
[[ -s "$WORK/log" ]] \
  || fail "git log listed NO commits in $REPO, so the scan below would pass vacuously. Either the repository is empty, or HEAD is unborn, or the checkout has no history to walk (CI checks out with fetch-depth: 0 for exactly this reason — a truncated clone must not be read as a clean history)"

detect "$WORK/log" "$WORK/raw"

# Resolve the exemptions against THIS repository's history. The length guard is
# not decoration: `"${arr[@]}"` on an EMPTY array is an unbound-variable error
# under `set -u` on bash 3.2, and an empty allowlist is a legitimate future state.
: >"$WORK/exempt.shas"
resolved=0
if ((${#LEGACY_ALLOW_SHAS[@]} > 0)); then
  for sha in "${LEGACY_ALLOW_SHAS[@]}"; do
    full="$(git -C "$REPO" rev-parse --verify --quiet "${sha}^{commit}" || true)"
    if [[ -z "$full" ]]; then
      # Foreign mode: the sandbox repositories §9 builds do not contain this
      # project's commits, so an unresolvable exemption is expected there and is
      # simply inert. In self mode it is a hard error (below).
      continue
    fi
    git -C "$REPO" merge-base --is-ancestor "$full" HEAD \
      || fail "exempt commit $sha resolves in $REPO but is NOT an ancestor of HEAD — an exemption must name published history, not a dangling object"
    subject="$(git -C "$REPO" log -1 --format=%s "$full")"
    [[ -n "$subject" ]] || fail "exempt commit $sha has an empty subject — refusing to exempt an unreadable commit"
    printf '%s\n' "$full" >>"$WORK/exempt.shas"
    resolved=$((resolved + 1))
  done
fi

if ((SELF == 1)); then
  ((resolved == ${#LEGACY_ALLOW_SHAS[@]})) \
    || fail "only $resolved of ${#LEGACY_ALLOW_SHAS[@]} exempt SHA(s) resolve in this repository. Either an exemption names a commit that is not here — which exempts nothing and hides what it was for — or this checkout does not reach far enough back to contain it (CI uses fetch-depth: 0; a truncated clone must fail here rather than silently scan a shorter history)"
  # ANTI-ROT, and simultaneously the live positive control: each exempt commit
  # must itself be a detection. If a listed SHA stops being a violation, the
  # exemption is dead scaffolding and must go.
  while read -r full; do
    grep -q "^${full}${TAB}" "$WORK/raw" \
      || fail "exempt commit $full is NOT a violation — its subject no longer leads with a non-ASCII character, so the exemption is stale; delete it from LEGACY_ALLOW_SHAS here AND from LEGACY_EXPECTED in changelog_gate_test.sh §9d. (Note this can only happen if history was rewritten. Re-FILING the rendered changelog entry, which is what REDMAIN-N3 does, does not change any commit subject and must NOT be answered by editing this list — see 'WHAT THIS LIST IS NOT' in the header.)"
  done <"$WORK/exempt.shas"
fi

# Subtract the exemptions. `grep -v -F -f` with an EMPTY pattern file drops
# nothing on GNU grep but is a portability trap, so the empty case is explicit.
if [[ -s "$WORK/exempt.shas" ]]; then
  sed 's/$/'"$TAB"'/' "$WORK/exempt.shas" >"$WORK/exempt.prefixes"
  grep -v -F -f "$WORK/exempt.prefixes" "$WORK/raw" >"$WORK/violations" || true
else
  cp "$WORK/raw" "$WORK/violations"
fi

if ((SELF == 1)) && ((MODE_SHAS == 1)); then
  cat "$WORK/exempt.shas"
  exit 0
fi

if [[ -s "$WORK/violations" ]]; then
  echo "FAIL: commit subject(s) start with a non-ASCII character (REDMAIN-N1 / D-168):" >&2
  sed 's/^/  /' "$WORK/violations" >&2
  cat >&2 <<'MSG'

  GUIDELINES.md § Repository discipline requires the ASCII gitmoji SHORTCODE:
      :construction_worker: ci(docs): stop uploading the Pages artifact
  not the literal emoji. cliff.toml's commit_parsers key on the shortcode, so a
  literal-emoji subject matches none of them and is published under "### Other"
  on the GitHub Release page (REL-14 / D-137).

  Fix an UNPUBLISHED commit by rewording it (git rebase -i / git commit --amend).
  A commit that is already on origin/main must NOT be rewritten (hard rule 2) —
  add its full SHA to LEGACY_ALLOW_SHAS in this file with the reason, and record
  the exemption in docs/decisions/decisions.md.
MSG
  exit 1
fi

n="$(wc -l <"$WORK/log" | tr -d ' ')"
if ((SELF == 1)); then
  echo "OK: $n commit subject(s) scanned, all ASCII-leading; $resolved published-history exemption(s), each verified to still be a real detection"
else
  echo "OK: $n commit subject(s) scanned in $REPO, all ASCII-leading"
fi
