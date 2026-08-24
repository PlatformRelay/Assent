#!/usr/bin/env bash
# REQ-AUD2-S03-01..05 — SEC-03: hack/install.sh pins the cosign signer identity
# and OIDC issuer, and that pin is a real, load-bearing guarantee.
#
# Finding closed here (agent-context/PROJECT-AUDIT-2026-08-18.md, SEC-03):
#   `cosign verify-blob --bundle "$bundle" "$ARCHIVE"` carried NO
#   --certificate-identity-regexp / --certificate-oidc-issuer, while SECURITY.md
#   publishes both in its manual copy-paste instructions. Keyless verification
#   without an identity pin either errors out (cosign v2 requires the flags) or
#   accepts ANY Fulcio identity — so a mirror-swapped archive shipped with its
#   own validly signed bundle verifies clean and `--require-signature` promises
#   something it does not deliver.
#
# ONE PUBLISHED TRUTH, THREE FILES. The issuer/identity pair is published in
# SECURITY.md; hack/install.sh (the adopter path) and
# hack/release/verify-artifacts.sh (the maintainer/CI path, run by `task
# release-verify`) copy it byte-identically. Section 3 below is a drift gate over
# all three: if any one of them is edited alone, this script reddens
# (REQ-AUD2-S03-04, AUD2-F01).
#
# AUD2-F01 — SEC-03's twin on the maintainer path. AUD2-S03 fixed hack/install.sh
# and deliberately left hack/release/verify-artifacts.sh's byte-identical
# unpinned `cosign verify-blob --bundle "$bundle" "$archive"` alone (out of that
# story's scope; D-153 records the residue). So the same hole survived on the
# path maintainers and CI use to check a release before it ships. It is closed
# here against the SAME published value and inside the SAME drift gate, rather
# than by starting a second published truth (D-128).
#
# THE VACUITY TRAP FOR THAT FILE. verify-artifacts.sh only reaches cosign when a
# .sigstore.json bundle sits beside an archive; on the SNAPSHOT path dist/ ships
# no bundles at all, so the cosign branch is skipped entirely and a naive
# end-to-end test would pass green without ever executing the changed line.
# Sections 5d-5g are therefore written so the discriminator is the stub's argv
# LOG, not the script's exit code: 5d requires the log to be non-empty and to
# carry both pinned values, and 5e is the paired control showing that a
# bundle-less dist (i.e. the snapshot shape) leaves that same log EMPTY.
#
# REAL-IDENTITY FIXTURES (section 4c) — the assertion that matters most. The
# first version of this pin copied SECURITY.md's published value faithfully and
# was still WRONG: the repo was renamed PlatformRelay/assent -> PlatformRelay/
# Assent between v0.1.0 and v0.2.0, cosign matches the identity regexp
# case-sensitively, and so the published lowercase pin rejected the project's
# own v0.2.0/v0.3.0 artifacts. Self-made fixtures cannot catch that, so the SANs
# decoded from the REAL published bundles are committed here as test data and
# every one of them must verify.
#
# UNIV-COSIGN / D-160 — EXISTENTIAL WHERE IT HAD TO BE UNIVERSAL. Until D-160
# every flag assertion here asked "does SOME cosign invocation in this file carry
# the pin?". Each one was individually sound and the conjunction still certified
# nothing: a reviewer added a SECOND, UNPINNED `cosign verify-blob` to
# hack/release/verify-artifacts.sh and this gate exited 0. `has_flag` grepped the
# whole folded extraction so a pinned sibling satisfied it; `one_value`'s `sort -u`
# collapsed the agreeing values so the drift gate saw one; section 0 PRINTED the
# invocation count and asserted nothing about it; and 4b/5d's stub-log checks were
# positive-only greps, the same shape at runtime. Measured before the fix on
# scratch copies of all three graded files, each with a second unpinned call:
# rc=0, rc=0, rc=0. SECURITY.md was worse than the other two — it had no
# invocation-level grading at all, only the drift comparison.
#
# The property is now UNIVERSAL — EVERY `cosign verify-blob` invocation in EVERY
# graded file carries both pins, EACH WITH SECURITY.md's published value —
# enforced by pin_violations (sections 1, 2, 2b, 2c) and by log_unpinned_lines at
# runtime (4b, 5d). The value half is not decoration: `--certificate-identity-
# regexp ''` carries the flag and matches every Fulcio identity ever issued, and
# `one_value`'s `sort -u` misses it too because an empty capture is not a line.
# A presence-only universal check would be the same defect one level down.
# "Exactly one invocation per
# file", the cheaper remedy, was rejected as FALSE on this tree: SECURITY.md
# legitimately publishes two, one over the archive and one over checksums.txt.
# This is the AUD2-S05 quorum bug one layer out: not a wrong assertion, a wrong
# quantifier.
#
# SECOND ROUND (UC-01..UC-04) — THE FIRST D-160 FIX REPRODUCED THE DEFECT IT WAS
# WRITTEN TO CLOSE, one level down, and independent review caught it. has_flag
# grepped the whole FILE; the replacement grepped the whole LINE. Both are
# existential; only the scope shrank. One folded line was graded as at most ONE
# invocation, so
#
#   cosign verify-blob <both pins> --bundle a.json a && cosign verify-blob --bundle EVIL.json evil
#
# started with `cosign verify-blob` (not UNCLASSIFIABLE), carried --bundle (not
# FOLD-BROKEN), carried both flag strings (not UNPINNED) and yielded the published
# values (not WRONG-VALUE): GREEN, in same-line, `;`-separated and backslash-folded
# forms, on all three graded files — and the FULL gate exited 0 on SECURITY.md,
# which has no runtime twin because it is a document. The gate printed "EVERY
# cosign verify-blob invocation ... is pinned", which was false as printed.
# Closed by grading per OCCURRENCE (occurrence_count + MULTI-OCCURRENCE), not per
# line. UC-02: extract_issuer/extract_identity anchor on a greedy `.*`, so the LAST
# value on a line won and a hostile `--certificate-identity-regexp ''` placed FIRST
# was masked by a correct value placed second — the extraction direction favoured
# the attacker. Closed by grading EVERY value of every flag (flag_value_tokens).
# UC-03: a CORRECTLY pinned call written with double quotes was refused as
# "identity=<empty/unparsable> ... an empty regexp matches every Fulcio identity" —
# a true refusal with a false reason, and the repair a maintainer reaches for is
# widening the extractor. Both quote styles are now accepted and a BARE value is
# diagnosed as a QUOTING defect (UNQUOTED-VALUE), never as a wrong or empty value.
#
# ANTI-VACUITY DISCIPLINE (this repo has a documented history of gates that
# cannot fail — D-124, AUD-S18). Every assertion here is a FUNCTION over a file
# or a fixture, run twice: once against the real tree (must be GREEN) and once
# against a mutant carrying the very defect it exists to catch (must be RED).
# Every extraction is positive-controlled (non-empty, exactly one distinct
# value) so a pattern that silently stopped matching fails loudly instead of
# passing vacuously.
#
# OFFLINE. No network, no real Fulcio, no real cosign: section 4 puts a stub
# `cosign` first on PATH that parses the flags it was handed and matches them
# against a certificate identity carved into a fake .sigstore.json bundle. The
# stub models the PERMISSIVE pre-fix behaviour (no --certificate-identity-regexp
# => accept any identity), which is the dangerous branch of the finding: it lets
# section 5c show that deleting the flag from install.sh installs a
# foreign-signed archive, i.e. that the pin is what closes the hole.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

INSTALL="$ROOT/hack/install.sh"
SECURITY="$ROOT/SECURITY.md"
VERIFY="$ROOT/hack/release/verify-artifacts.sh"
TASKFILE="$ROOT/Taskfile.yml"
AUDIT_GATE="$ROOT/hack/audit/exitgate_test.sh"
STAGE="release-install-cosign-pin-test"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# --------------------------------------------------------------- helpers --

# cosign_candidates <file> — EVERY `cosign verify-blob …` occurrence in the file,
# one per output line, with backslash continuations folded onto that line.
# Comment lines are skipped so a flag mentioned only in prose can never satisfy an
# assertion.
#
# This is the DENOMINATOR of the universal check below (UNIV-COSIGN, D-160), so it
# is deliberately left un-narrowed: anything it drops is a hole. Occurrences that
# are not really invocations (SECURITY.md's table says "`cosign verify-blob` with
# bundle") are classified out — and ACCOUNTED FOR — by pin_violations, never
# filtered away here.
#
# It was already emitting one line per occurrence before D-160; what was missing
# was any caller that looked at more than the first one.
cosign_candidates() {
  awk '
    /^[[:space:]]*#/ { next }
    !inv && /cosign verify-blob/ { inv = 1 }
    inv {
      line = $0
      sub(/^[[:space:]]+/, "", line)
      sub(/[[:space:]]*\\$/, "", line)
      printf "%s ", line
      if ($0 !~ /\\[[:space:]]*$/) { printf "\n"; inv = 0 }
    }
  ' "$1"
}

# cosign_invocations <file> — the candidates that are actual commands, i.e. those
# whose folded text BEGINS with `cosign verify-blob` (leading whitespace already
# stripped by the fold).
cosign_invocations() {
  cosign_candidates "$1" | grep '^cosign verify-blob' || true
}

# extract_issuer <file> — every distinct --certificate-oidc-issuer value.
extract_issuer() {
  sed -nE "s/.*--certificate-oidc-issuer[[:space:]]+['\"]?([^'\"[:space:]]+)['\"]?.*/\1/p" "$1" | sort -u
}

# extract_identity <file> — every distinct --certificate-identity-regexp value.
# BOTH quote styles are accepted (UC-03). The value is single-quoted in all three
# graded files and it must be quoted somehow — a BARE regexp is glob-expanded and
# word-split by the shell, so `[Aa]ssent` would silently become something else —
# but double quotes are an equally correct spelling for this particular value (it
# contains no $ and no backtick). A gate that reds on a CORRECTLY pinned,
# double-quoted call while blaming "an empty regexp that matches every Fulcio
# identity" teaches the next maintainer to widen the extractor, which is how a
# pin gate erodes. A bare value still extracts as nothing here, and pin_violations
# names QUOTE STYLE as the cause instead of misdiagnosing it as a wrong value.
extract_identity() {
  sed -nE "s/.*--certificate-identity-regexp[[:space:]]+'([^']*)'.*/\1/p;s/.*--certificate-identity-regexp[[:space:]]+\"([^\"]*)\".*/\1/p" "$1" | sort -u
}

# --------------------------- UC-01 / UC-02: per-OCCURRENCE grading ----------
#
# THE DEFECT THIS BLOCK EXISTS TO CLOSE, stated plainly because it is the same
# defect D-160 was opened to fix, reproduced one level down. The first D-160 fix
# replaced has_flag — which grepped the whole FILE, so a pinned sibling anywhere
# satisfied it — with a check that grepped the whole LINE. That is still an
# existential test, just with a smaller scope: one folded line was graded as at
# most ONE invocation, so
#
#   cosign verify-blob <both pins> --bundle a.json a && cosign verify-blob --bundle EVIL.json evil
#
# started with `cosign verify-blob` (not UNCLASSIFIABLE), contained --bundle (not
# FOLD-BROKEN), contained both flag strings (not UNPINNED) and yielded the
# published values (not WRONG-VALUE). Green. Confirmed in same-line, `;`-separated
# and backslash-folded forms, on all three graded files. The universal property
# is per OCCURRENCE, not per line.
#
# occurrence_count <text> — how many verify-blob calls sit on one folded line.
# The line-level TRIGGER stays the literal `cosign verify-blob` (see
# cosign_candidates) because widening it to bare `verify-blob` reds on prose; but
# COUNTING inside an already-triggered line uses the shorter `verify-blob`, so a
# second call spelled `cosign  verify-blob` or `"$COSIGN" verify-blob` and chained
# onto a pinned one is still SEEN and still fails closed.
occurrence_count() {
  # WHOLE WORDS, not substrings. A substring count reds on a perfectly good
  # invocation whose BUNDLE is named `verify-blob-test.sigstore.json` — a
  # false positive on a plausible filename, sitting on the branch that carries the
  # whole UC-01 fix, so the repair a maintainer would reach for is loosening this
  # very function. awk's default field splitting also folds runs of whitespace, so
  # `cosign  verify-blob` and `"$COSIGN" verify-blob` still count as occurrences.
  #
  # UC-05: SHELL QUOTES ARE STRIPPED BEFORE THE COMPARE. A bare field compare finds
  # one whole-word token and concludes SINGULARITY, when all it established is that
  # it did not find a second one — absence of evidence read as evidence of absence.
  # `cosign "verify-blob"`, `cosign 'verify-blob'` and `cosign verify-blob""` are
  # all the same command to the shell, and each hid a chained unpinned call from
  # the count. Measured: without the strip, all three returned 1 and the full gate
  # exited 0 on SECURITY.md.
  #
  # This is a SPELLING PATCH, not a terminator, and it is labelled as one on
  # purpose. It closes three known spellings; it does not prove a fourth does not
  # exist. A structural terminator was considered and ruled out with evidence:
  # refusing any graded line that carries a command separator would red the REAL
  # files, because hack/install.sh and hack/release/verify-artifacts.sh both end
  # their genuine invocation with `|| die "cosign verification failed…"`. Since the
  # count cannot be made complete, the PASS banner is written to report what was
  # classified rather than to claim that nothing else exists (UC-07).
  #
  # Residual, stated: a bundle named exactly `verify-blob` (no extension) counts as
  # an occurrence and fails closed. No file does that, and closed is the safe way
  # to be wrong.
  printf '%s' "$1" | awk '{
    for (i = 1; i <= NF; i++) {
      tok = $i
      gsub(/["\047]/, "", tok)
      if (tok == "verify-blob") n++
    }
  } END { print n + 0 }'
}

# flag_value_tokens <flag> — reads one folded line on stdin and prints the RAW
# value token following EVERY occurrence of <flag> (quotes included), one per line.
#
# "Every", not "the last one", is the point (UC-02). extract_issuer and
# extract_identity anchor on a greedy `.*`, so on a line carrying two values the
# LAST one wins — and a hostile `--certificate-identity-regexp ''` placed FIRST is
# masked by a correct value placed second. The extraction direction favoured the
# attacker. Here every value is returned and every one has to match.
flag_value_tokens() {
  local flag="$1"
  grep -oE -- "${flag}[[:space:]]+('[^']*'|\"[^\"]*\"|[^[:space:]]+)" \
    | sed -E "s/^${flag}[[:space:]]+//" || true
}

# unquote <raw-token> — prints "quoted<TAB>value" or "bare<TAB>value".
unquote() {
  local raw="$1" n=${#1}
  case "$raw" in
    \'*\') printf 'quoted\t%s\n' "${raw:1:n-2}" ;;
    \"*\") printf 'quoted\t%s\n' "${raw:1:n-2}" ;;
    *) printf 'bare\t%s\n' "$raw" ;;
  esac
}

# one_value <file> <extractor> <label> — the extractor's single distinct value,
# or a hard failure if it found none (missing pin, or a broken pattern that would
# make every assertion vacuous) or several (the file disagrees with itself).
one_value() {
  local file="$1" extractor="$2" label="$3" out n
  out="$("$extractor" "$file")"
  n="$(printf '%s' "$out" | grep -c . || true)"
  if [[ "$n" -eq 0 ]]; then
    fail "$label: extracted NOTHING from ${file#"$ROOT"/} — the pin is missing, or the extraction pattern broke (which would make every assertion below vacuous)"
  fi
  if [[ "$n" -gt 1 ]]; then
    fail "$label: ${file#"$ROOT"/} carries $n DIFFERENT values, so the file disagrees with itself: $(printf '%s' "$out" | tr '\n' '|')"
  fi
  printf '%s' "$out"
}

# pin_violations <file> — UNIV-COSIGN (D-160). Prints one line per problem, and
# NOTHING AT ALL when every `cosign verify-blob` invocation in <file> carries the
# D-153 signer pin. Callers grade the OUTPUT rather than an exit status, so the
# identical function is asserted empty against the real tree and non-empty against
# a mutant, with no `set -e` games in between.
#
# WHY THIS REPLACED has_flag. Until D-160 the flag assertions were `has_flag`,
# which grepped the WHOLE folded extraction: "SOME invocation is pinned". Every
# individual assertion was sound and the conjunction still certified nothing —
# a reviewer added a second, UNPINNED `cosign verify-blob` to
# hack/release/verify-artifacts.sh and this gate exited 0. `one_value`'s `sort -u`
# collapsed the agreeing values, the drift gate saw one value per file, and §0
# printed the invocation count without ever asserting anything about it. The
# property needed is UNIVERSAL — *every* invocation is pinned — and the same hole
# was open on all three graded files, not just the one that was probed.
#
# WHY NOT "EXACTLY ONE INVOCATION PER FILE", the cheaper remedy: SECURITY.md
# legitimately publishes TWO — one over the archive, one over checksums.txt (which
# is what covers the SBOMs listed inside it) — so that assertion is false on the
# tree as it stands. Universal is also strictly stronger: it keeps holding when a
# file grows a third, correctly pinned call.
#
# CLASSIFICATION, AND WHERE IT IS *NOT* FAIL-CLOSED (R3-01 — read this before
# trusting the exemption). A candidate that is not at command position is waved
# through when the substring `cosign verify-blob` appears ANYWHERE ON THE LINE.
# The test is that substring, not "the line is prose", and it runs BEFORE the
# multi-token refusal below. So a line carrying a backticked mention AND one or
# more LIVE calls is skipped, not graded and not refused — measured, with the full
# gate green. On a line with NO backticked mention, anything not at command
# position — `foo && cosign verify-blob …`, a here-doc line, an un-backticked
# prose sentence — IS reported UNCLASSIFIABLE rather than skipped, because "this
# gate cannot tell whether that call is pinned" must not read as "that call is
# fine". That principle holds for those lines and is DEFEATED on a line that
# carries a backticked mention; narrowing the exemption needs quote/markdown-aware
# parsing, which is the machinery that produced UC-01/02/03, so it is recorded as
# a stated residual and surfaced in the PASS banner rather than patched blind.
#
# STATED LIMIT, deliberately not closed here (same posture as D-154): the
# denominator is the literal string `cosign verify-blob`. An invocation spelled
# through a variable (`"$COSIGN" verify-blob …`), built by `eval`, or assembled
# from fragments is invisible to this gate. Widening the trigger to bare
# `verify-blob` was considered and rejected — it would turn any future prose
# sentence in SECURITY.md that says "verify-blob" outside backticks into a red
# gate, and the next lane's remedy for that would be to loosen the classifier.
pin_violations() {
  local file="$1" rel cand line flag n_cand n_inv=0 n_occ tokens tok kind val published
  rel="${file#"$ROOT"/}"
  # The comparison is against SECURITY.md's published pair, so this must not run
  # before section 0 extracted it — otherwise every value check below compares
  # against the empty string and passes.
  [[ -n "${sec_issuer:-}" && -n "${sec_identity:-}" ]] \
    || fail "pin_violations was called before SECURITY.md's published issuer/identity pair was extracted — every value comparison in it would be vacuous"
  cand="$(cosign_candidates "$file")"
  n_cand="$(printf '%s' "$cand" | grep -c . || true)"
  if [[ "$n_cand" -eq 0 ]]; then
    printf 'NO-CANDIDATES: %s carries no `cosign verify-blob` occurrence at all — the call is gone, or the extraction pattern broke; either way every pin assertion over this file would be vacuous\n' "$rel"
    return 0
  fi
  while IFS= read -r line; do
    [[ -n "$line" ]] || continue
    if [[ "$line" != 'cosign verify-blob'* ]]; then
      # Not at command position. The ONE accepted non-invocation shape is a
      # markdown inline-code mention, e.g. SECURITY.md's capability table.
      if [[ "$line" == *'`cosign verify-blob`'* ]]; then
        continue
      fi
      printf 'UNCLASSIFIABLE: %s has a `cosign verify-blob` occurrence that is neither at command position nor a backticked prose mention, so this gate cannot decide whether it is pinned — failing closed: %s\n' "$rel" "$line"
      continue
    fi
    # UC-01: ONE FOLDED LINE MAY CARRY MORE THAN ONE CALL. Everything below grades
    # a line as a single invocation, so a line carrying two calls has to fail
    # closed rather than be graded as its first one. `a && b`, `a ; b` and a
    # backslash-folded chain all arrive here as one line.
    n_occ="$(occurrence_count "$line")"
    if [[ "$n_occ" -gt 1 ]]; then
      n_inv=$((n_inv + n_occ))
      # UC-06: say TOKENS, not "calls chained". cosign_candidates only drops a line
      # whose FIRST non-space character is `#`, so a TRAILING comment mentioning
      # verify-blob lands here too. Refusing it is right (this gate cannot grade a
      # line it cannot resolve to one call); describing it as a chained call is not,
      # and a message that misdiagnoses is what gets the counter loosened — the
      # UC-03 failure recurring inside the UC-01 fix.
      printf 'MULTI-OCCURRENCE: %s has %s `verify-blob` TOKENS on ONE line, and this gate grades a line as exactly one invocation, so it refuses rather than guess which one to grade — a pinned first call must not vouch for an unpinned second. Cause is either a second call chained onto this one, or `verify-blob` appearing in a TRAILING COMMENT on this line. Put each cosign call on its own line, and keep `verify-blob` out of trailing comments there: %s\n' "$rel" "$n_occ" "$line"
      continue
    fi
    n_inv=$((n_inv + 1))
    if [[ "$line" != *--bundle* ]]; then
      # --bundle is present in every real invocation across all three files, so
      # its absence means the backslash-continuation fold did NOT join this
      # command's flag lines. Reported separately from UNPINNED because the
      # remedy is different: fix the extractor, do not go looking for a missing
      # pin in a file that has one.
      printf 'FOLD-BROKEN: %s has a `cosign verify-blob` command that folded WITHOUT --bundle — the continuation fold is truncating this invocation, so its pin verdict below cannot be trusted: %s\n' "$rel" "$line"
      continue
    fi
    for flag in --certificate-oidc-issuer --certificate-identity-regexp; do
      [[ "$line" == *"$flag"* ]] \
        || printf 'UNPINNED[%s]: %s has a `cosign verify-blob` invocation without %s — keyless verification there accepts a signer this project never authorised: %s\n' "$flag" "$rel" "$flag" "$line"
    done
    # PRESENCE IS NOT A PIN, and NEITHER IS "one of the values is right" (UC-02).
    # `--certificate-identity-regexp ''` carries the flag and matches every Fulcio
    # identity ever issued. EVERY value the invocation supplies for each flag is
    # compared against SECURITY.md's published pair, because the greedy `.*` in
    # extract_identity/extract_issuer keeps only the LAST value on a line — so a
    # hostile empty regexp placed FIRST used to be masked by a correct one placed
    # second. Quote style is diagnosed separately from a wrong value (UC-03): a
    # bare regexp is a real defect (the shell globs and word-splits it) but it is
    # NOT "an empty regexp", and saying so is what stops a maintainer from
    # "fixing" it by widening the extractor.
    for flag in --certificate-oidc-issuer --certificate-identity-regexp; do
      [[ "$line" == *"$flag"* ]] || continue
      if [[ "$flag" == "--certificate-oidc-issuer" ]]; then published="$sec_issuer"; else published="$sec_identity"; fi
      tokens="$(printf '%s\n' "$line" | flag_value_tokens "$flag")"
      if [[ -z "$tokens" ]]; then
        printf 'UNPARSABLE-VALUE[%s]: %s carries %s but this gate could not read a value after it — the invocation is malformed, or the value uses a quoting style the extractor does not support (single and double quotes are supported; a BARE value is not, because the shell would glob and word-split it): %s\n' "$flag" "$rel" "$flag" "$line"
        continue
      fi
      while IFS= read -r tok; do
        [[ -n "$tok" ]] || continue
        kind="$(unquote "$tok" | cut -f1)"
        val="$(unquote "$tok" | cut -f2-)"
        if [[ "$flag" == "--certificate-identity-regexp" && "$kind" == "bare" ]]; then
          printf 'UNQUOTED-VALUE[%s]: %s pins the identity regexp WITHOUT quotes (%s). The value is a regexp: unquoted, the shell glob-expands `[Aa]` and word-splits it, so cosign is handed something other than what is written. This is a QUOTING defect, not a wrong or empty value — quote it, do not widen the extractor: %s\n' "$flag" "$rel" "$val" "$line"
          continue
        fi
        if [[ "$val" != "$published" ]]; then
          printf 'WRONG-VALUE[%s]: %s has a `cosign verify-blob` invocation supplying %s=%s, but SECURITY.md publishes %s — the flag is present and the guarantee is not (an EMPTY regexp matches every Fulcio identity ever issued): %s\n' "$flag" "$rel" "$flag" "${val:-<empty>}" "$published" "$line"
        fi
      done <<<"$tokens"
    done
  done <<<"$cand"
  [[ "$n_inv" -ge 1 ]] \
    || printf 'NO-INVOCATIONS: %s mentions `cosign verify-blob` but not once at command position — nothing was actually graded\n' "$rel"
}

# assert_all_pinned <file> <label> — pin_violations, as a hard failure.
assert_all_pinned() {
  local v
  v="$(pin_violations "$1")"
  if [[ -z "$v" ]]; then
    return 0
  fi
  fail "$2: not every \`cosign verify-blob\` invocation carries the D-153 signer pin (UNIV-COSIGN, D-160):
$v"
}

# log_unpinned_lines <stub-log> — the runtime twin of pin_violations: every line
# the stub cosign recorded that did NOT receive both pinned values. Empty output
# means every cosign invocation the script ACTUALLY made at runtime was pinned.
# §5d's original log assertions were positive-only greps ("some line carries the
# pin"), i.e. the same existential shape D-160 closes statically.
#
# The pinned values reach awk through ENVIRON, never through `-v`: awk processes
# escape sequences in a `-v` assignment, so `-v id='…github\.com…'` arrives as
# `github.com` and would never match the log line's literal backslash. That
# variant was written first and this section caught it, which is the only reason
# it is called out here.
log_unpinned_lines() {
  UC_PIN_ID="identity_re=${sec_identity}" UC_PIN_ISS="issuer=${sec_issuer}" awk '
    index($0, ENVIRON["UC_PIN_ID"]) == 0 || index($0, ENVIRON["UC_PIN_ISS"]) == 0 { print }
  ' "$1"
}

# ------------------------------------------- 0. extraction positive controls --

echo "== 0. extraction positive controls =="
[[ -f "$INSTALL" ]] || fail "hack/install.sh missing"
[[ -x "$INSTALL" ]] || fail "hack/install.sh is not executable"
[[ -f "$SECURITY" ]] || fail "SECURITY.md missing"

[[ -f "$VERIFY" ]] || fail "hack/release/verify-artifacts.sh missing"
[[ -x "$VERIFY" ]] || fail "hack/release/verify-artifacts.sh is not executable"

# UNIV-COSIGN (D-160): the graded set is the SAME three files everywhere below —
# named once here so a file cannot be graded on one property and skipped on
# another, which is how SECURITY.md came to have a drift check but no
# invocation-level check at all.
GRADED=("install.sh:$INSTALL" "SECURITY.md:$SECURITY" "verify-artifacts.sh:$VERIFY")
[[ "${#GRADED[@]}" -eq 3 ]] || fail "the graded-file table was emptied"

for target in "${GRADED[@]}"; do
  label="${target%%:*}"
  src="${target#*:}"
  cosign_candidates "$src" >"$WORK/cand.$label"
  [[ -s "$WORK/cand.$label" ]] || fail "no 'cosign verify-blob' occurrence found in $label — extraction broke, every flag assertion over it would be vacuous"
  cosign_invocations "$src" >"$WORK/inv.$label"
  [[ -s "$WORK/inv.$label" ]] || fail "$label mentions 'cosign verify-blob' but not once at command position — the invocation classifier is dropping every real call, so the universal check would grade nothing (D-160)"
  grep -qF -- '--bundle' "$WORK/inv.$label" || fail "the cosign invocation(s) extracted from $label do not contain the known-present --bundle flag — the awk fold is wrong"
  echo "OK: $label — $(grep -c . <"$WORK/cand.$label" | tr -d ' ') 'cosign verify-blob' occurrence(s), $(grep -c . <"$WORK/inv.$label" | tr -d ' ') of them invocation(s), anchored on --bundle"
done

sec_issuer="$(one_value "$SECURITY" extract_issuer "SECURITY.md issuer")"
sec_identity="$(one_value "$SECURITY" extract_identity "SECURITY.md identity regexp")"
echo "OK: SECURITY.md publishes issuer=${sec_issuer} identity=${sec_identity}"

# ------------------------------------------------- 1. REQ-AUD2-S03-01 (flags) --

echo "== 1. REQ-AUD2-S03-01 / AUD2-F01 / UNIV-COSIGN: EVERY cosign call in all THREE files is pinned =="
# Universal, not existential (D-160). Before this, hack/install.sh and
# hack/release/verify-artifacts.sh were graded "some invocation carries the flag"
# and SECURITY.md was not graded at invocation level at all — it only fed the
# drift comparison. A second, unpinned call in any of the three passed.
assert_all_pinned "$INSTALL" "hack/install.sh (SEC-03, REQ-AUD2-S03-01 — the adopter path)"
assert_all_pinned "$VERIFY" "hack/release/verify-artifacts.sh (AUD2-F01 — the maintainer/CI path run by 'task release-verify')"
assert_all_pinned "$SECURITY" "SECURITY.md (D-160 — the copy-paste recipe adopters actually run by hand)"
echo "OK: all $(( $(grep -c . <"$WORK/inv.install.sh") + $(grep -c . <"$WORK/inv.SECURITY.md") + $(grep -c . <"$WORK/inv.verify-artifacts.sh") )) cosign verify-blob invocation(s) across the three files carry --certificate-oidc-issuer, --certificate-identity-regexp and --bundle"

# ------------------------------------------- 2. REQ-AUD2-S03-03 (non-vacuity) --

echo "== 2. REQ-AUD2-S03-03 / AUD2-F01: deleting either flag from either script reddens section 1 =="
# Every mutation below is made on a TEMP COPY. Never undo a mutation with
# `git checkout --`: that reverts to HEAD rather than to the working tree, and it
# has silently eaten uncommitted work in a prior lane.
for target in "${GRADED[@]}"; do
  label="${target%%:*}"
  src="${target#*:}"
  for flag in --certificate-oidc-issuer --certificate-identity-regexp; do
    mutant="$WORK/${label}.no${flag}"
    grep -vF -- "$flag" "$src" >"$mutant"
    chmod +x "$mutant"
    if grep -qF -- "$flag" "$mutant"; then
      fail "mutation did not land: $flag is still present in $mutant"
    fi
    [[ "$(wc -l <"$mutant")" -lt "$(wc -l <"$src")" ]] \
      || fail "mutation did not land: $mutant has the same line count as ${src#"$ROOT"/}"
    mut_v="$(pin_violations "$mutant")"
    printf '%s\n' "$mut_v" | grep -qF -- "UNPINNED[$flag]" \
      || fail "pin_violations does not report UNPINNED[$flag] for a copy of $label with that flag line deleted — the assertion is vacuous. It said: ${mut_v:-<nothing>}"
    echo "OK: deleting $flag from $label makes the pin assertion red"
  done
done

echo "== 2b. UNIV-COSIGN (D-160): the check is UNIVERSAL, and every branch of it is exercised =="
# THE FINDING THIS SECTION CLOSES, reproduced in-gate. A reviewer added a second,
# UNPINNED `cosign verify-blob` to hack/release/verify-artifacts.sh and the gate
# exited 0; the same hole was open on hack/install.sh and SECURITY.md. Measured
# before the fix, on scratch copies of all three: rc=0, rc=0, rc=0.
#
# WHY THE MUTANTS ARE APPENDED AT EOF rather than spliced into the body next to
# the real call: pin_violations is a pure function of the file's TEXT — it folds
# continuations and classifies lines, and nothing in it depends on where in the
# file a command sits. Appending therefore drives the identical code path while
# being deterministic on all three files (one anchor, no per-file sed). The
# realistic in-body placement was exercised out-of-band during the lane, on scratch
# copies of the three files, with the same verdicts recorded below.
PROBE_BUNDLE='univ-cosign-probe.sigstore.json'
PROBE_ARTIFACT='univ-cosign-probe.tar.gz'
PINS_OK="--certificate-oidc-issuer ${sec_issuer} --certificate-identity-regexp '${sec_identity}'"

# append_raw <src> <dst> <text> — a copy of <src> with <text> appended verbatim.
# Used by the UC-01/UC-02/UC-03 mutants, which need exact control of the line
# shape (chaining operators, quote style, duplicated flags) rather than the
# canonical multi-line form append_invocation emits.
append_raw() {
  cp "$1" "$2"
  printf '%s\n' "$3" >>"$2"
  grep -qF -- "$PROBE_BUNDLE" "$2" || fail "mutation did not land: $2 has no appended text"
}

# append_invocation <src> <dst> <pinned|unpinned|offset|emptyid|wrongiss> — a copy of <src> with a
# SECOND `cosign verify-blob` appended in one of five shapes: pinned (the control),
# unpinned, offset (not at command position), emptyid and wrongiss (both flags
# present, guarantee absent).
append_invocation() {
  local src="$1" dst="$2" shape="$3"
  cp "$src" "$dst"
  case "$shape" in
    offset) printf 'true && cosign verify-blob \\\n' >>"$dst" ;;
    *) printf 'cosign verify-blob \\\n' >>"$dst" ;;
  esac
  case "$shape" in
    unpinned) ;;
    # both flags PRESENT, identity regexp empty: matches every Fulcio identity.
    emptyid)
      printf '  --certificate-oidc-issuer %s \\\n' "$sec_issuer" >>"$dst"
      printf "  --certificate-identity-regexp '' \\\\\\n" >>"$dst"
      ;;
    # both flags PRESENT, issuer pointing at a foreign IdP.
    wrongiss)
      printf '  --certificate-oidc-issuer %s \\\n' 'https://accounts.example.invalid' >>"$dst"
      printf "  --certificate-identity-regexp '%s' \\\\\\n" "$sec_identity" >>"$dst"
      ;;
    *)
      printf '  --certificate-oidc-issuer %s \\\n' "$sec_issuer" >>"$dst"
      printf "  --certificate-identity-regexp '%s' \\\\\\n" "$sec_identity" >>"$dst"
      ;;
  esac
  printf '  --bundle %s %s\n' "$PROBE_BUNDLE" "$PROBE_ARTIFACT" >>"$dst"
  grep -qF -- "$PROBE_BUNDLE" "$dst" || fail "mutation did not land: $dst has no appended invocation"
}

for target in "${GRADED[@]}"; do
  label="${target%%:*}"
  src="${target#*:}"
  base_inv="$(cosign_invocations "$src" | grep -c . || true)"

  # (a) a second UNPINNED invocation — must red, naming the file and quoting the
  #     offending command, and must flag ONLY it (the pinned siblings stay clean).
  m="$WORK/univ.${label}.second-unpinned"
  append_invocation "$src" "$m" unpinned
  v="$(pin_violations "$m")"
  [[ -n "$v" ]] \
    || fail "UNIV-COSIGN regression: a SECOND, UNPINNED 'cosign verify-blob' appended to $label left pin_violations EMPTY — the check is existential again, which is exactly the D-160 finding"
  printf '%s\n' "$v" | grep -qF -- 'UNPINNED[--certificate-identity-regexp]' \
    || fail "the second-unpinned mutant of $label reddened for the WRONG REASON (expected UNPINNED[--certificate-identity-regexp]): $v"
  printf '%s\n' "$v" | grep -qF -- 'UNPINNED[--certificate-oidc-issuer]' \
    || fail "the second-unpinned mutant of $label reddened for the WRONG REASON (expected UNPINNED[--certificate-oidc-issuer]): $v"
  printf '%s\n' "$v" | grep -qF -- "$m" \
    || fail "pin_violations reported a violation for $label without NAMING the file — a maintainer cannot act on it: $v"
  printf '%s\n' "$v" | grep -qF -- "$PROBE_BUNDLE" \
    || fail "pin_violations reported a violation for $label without quoting the OFFENDING invocation — it may be flagging the wrong call: $v"
  # Exactly the two UNPINNED lines and NOTHING else. "Two" is not a magic number:
  # the appended call carries neither flag, so both WRONG-VALUE branches are
  # guarded silent, it has --bundle so FOLD-BROKEN cannot fire, it is at command
  # position so UNCLASSIFIABLE cannot, and the pinned siblings must contribute
  # nothing. Asserting the SET rather than the count keeps this honest if a future
  # author adds a violation kind that fires unconditionally.
  n_v="$(printf '%s\n' "$v" | grep -c . || true)"
  n_unpinned="$(printf '%s\n' "$v" | grep -c '^UNPINNED\[' || true)"
  [[ "$n_v" -eq 2 && "$n_unpinned" -eq 2 ]] \
    || fail "pin_violations reported $n_v violation(s) ($n_unpinned of them UNPINNED) for a $label mutant with ONE unpinned call added — expected exactly the two UNPINNED lines, one per missing flag, and nothing else: $v"
  echo "OK: $label — a second UNPINNED invocation reddens, names the file, quotes the call, and spares the pinned siblings"

  # (b) a second CORRECTLY PINNED invocation — must stay GREEN. This is the
  #     property choice made explicit: "every invocation is pinned", NOT "exactly
  #     one invocation exists". SECURITY.md already ships two (archive +
  #     checksums.txt), so the cheaper count-based remedy is false on this tree.
  #     The count assertion is what keeps the green non-vacuous: if the extractor
  #     simply ignored the appended call, (b) would pass for the wrong reason.
  m="$WORK/univ.${label}.second-pinned"
  append_invocation "$src" "$m" pinned
  [[ "$(cosign_invocations "$m" | grep -c . || true)" -eq "$((base_inv + 1))" ]] \
    || fail "the second-PINNED mutant of $label did not increase the invocation count from $base_inv — the extractor never saw the appended call, so its green proves nothing"
  v="$(pin_violations "$m")"
  [[ -z "$v" ]] \
    || fail "a second, CORRECTLY PINNED 'cosign verify-blob' in $label was reported as a violation — the check is a count, not a pin check, and a legitimate second call would be blocked: $v"
  echo "OK: $label — a second CORRECTLY PINNED invocation is seen ($((base_inv + 1)) now) and stays green"

  # (c) an occurrence that is NOT at command position and NOT backticked prose —
  #     must fail CLOSED. It is appended fully PINNED on purpose: the only thing
  #     that can redden it is the classifier itself, so a classifier that silently
  #     swallowed the shape would turn this assertion green and be caught here.
  m="$WORK/univ.${label}.offset"
  append_invocation "$src" "$m" offset
  v="$(pin_violations "$m")"
  printf '%s\n' "$v" | grep -qF -- 'UNCLASSIFIABLE:' \
    || fail "a 'cosign verify-blob' occurrence that is neither at command position nor backticked prose did NOT fail closed in $label — an unpinned call hidden in that shape would pass: ${v:-<nothing>}"
  echo "OK: $label — an unclassifiable occurrence fails closed"

  # (d) EVERY invocation deleted — the extractor's own positive control. Without
  #     this, NO-CANDIDATES is a branch that was written and never run, and it is
  #     the branch deciding whether a broken extractor fails loudly or vacuously.
  m="$WORK/univ.${label}.none"
  grep -vF -- 'cosign verify-blob' "$src" >"$m"
  if grep -qF -- 'cosign verify-blob' "$m"; then
    fail "mutation did not land: $m still mentions cosign verify-blob"
  fi
  v="$(pin_violations "$m")"
  printf '%s\n' "$v" | grep -qF -- 'NO-CANDIDATES:' \
    || fail "a copy of $label with every 'cosign verify-blob' line deleted did NOT report NO-CANDIDATES — a file that lost its pinned call entirely would grade GREEN: ${v:-<nothing>}"
  echo "OK: $label — losing every invocation reports NO-CANDIDATES rather than passing vacuously"

  # (e) the continuation fold broken — must be reported as a FOLD problem, not as
  #     a missing pin, or the message sends the next maintainer to the wrong file.
  m="$WORK/univ.${label}.foldbroken"
  sed -E 's/^([[:space:]]*cosign verify-blob)[[:space:]]*\\$/\1/' "$src" >"$m"
  v="$(pin_violations "$m")"
  printf '%s\n' "$v" | grep -qF -- 'FOLD-BROKEN:' \
    || fail "stripping the line continuation after 'cosign verify-blob' in $label was not reported as FOLD-BROKEN — a truncated extraction would be misread as a missing pin: ${v:-<nothing>}"
  if printf '%s\n' "$v" | grep -qF -- 'UNPINNED['; then
    fail "the fold-broken mutant of $label ALSO reported UNPINNED — the two causes are conflated and the message is misleading: $v"
  fi
  echo "OK: $label — a broken continuation fold is named as such, not as a missing pin"

  # (f) EVERY invocation displaced off command position. This exercises
  #     NO-INVOCATIONS, the branch that decides whether "this gate graded nothing
  #     at all" reads as a pass. Without a mutant for it, it is code that was
  #     written and never run — which is the defect class this file exists to
  #     prevent, not to demonstrate.
  m="$WORK/univ.${label}.no-command-position"
  sed -E 's/^([[:space:]]*)cosign verify-blob/\1true \&\& cosign verify-blob/' "$src" >"$m"
  if cmp -s "$m" "$src"; then
    fail "mutation did not land: $m is byte-identical to ${src#"$ROOT"/}"
  fi
  v="$(pin_violations "$m")"
  printf '%s\n' "$v" | grep -qF -- 'NO-INVOCATIONS:' \
    || fail "a copy of $label whose every 'cosign verify-blob' sits off command position did NOT report NO-INVOCATIONS — a file this gate cannot grade at all would pass as pinned: ${v:-<nothing>}"
  echo "OK: $label — grading zero invocations reports NO-INVOCATIONS rather than passing"

  # (g) both flags PRESENT, identity regexp EMPTY. This is the shape a
  #     presence-only check waves through while cosign accepts every Fulcio
  #     identity ever issued — and `one_value`'s `sort -u` does not catch it
  #     either, because an empty capture is not a line and the file still reports
  #     exactly one distinct value.
  m="$WORK/univ.${label}.empty-identity"
  append_invocation "$src" "$m" emptyid
  v="$(pin_violations "$m")"
  printf '%s\n' "$v" | grep -qF -- 'WRONG-VALUE[--certificate-identity-regexp]' \
    || fail "a second invocation in $label carrying --certificate-identity-regexp '' was NOT reported — the check grades flag PRESENCE, not the pin, and an empty regexp matches every Fulcio identity: ${v:-<nothing>}"
  echo "OK: $label — an empty identity regexp is caught, not waved through as 'flag present'"

  # (h) both flags PRESENT, issuer pointing at a foreign IdP — the same defect on
  #     the issuer half, and a per-invocation drift the file-wide comparison in
  #     section 3 would report only as "two different values".
  m="$WORK/univ.${label}.wrong-issuer"
  append_invocation "$src" "$m" wrongiss
  v="$(pin_violations "$m")"
  printf '%s\n' "$v" | grep -qF -- 'WRONG-VALUE[--certificate-oidc-issuer]' \
    || fail "a second invocation in $label pinning a foreign OIDC issuer was NOT reported — each invocation's own value is not compared against SECURITY.md's published pair: ${v:-<nothing>}"
  echo "OK: $label — a foreign issuer on a second invocation is caught per-invocation"

  # ---- UC-01: a second call CHAINED ONTO THE SAME LINE as a pinned one. --------
  # This is the shape that defeated the first D-160 fix: it grades a folded LINE
  # as at most one invocation, so a pinned first call vouched for an unpinned
  # second. All three chaining operators are exercised, on all three files,
  # because the earlier matrix was broad in mutant shapes and narrow in this one.
  for chain in "&&" ";" "|"; do
    m="$WORK/univ.${label}.chained-$(printf '%s' "$chain" | tr -d ' ' | od -An -tx1 | tr -d ' \n')"
    append_raw "$src" "$m" "cosign verify-blob $PINS_OK --bundle $PROBE_BUNDLE $PROBE_ARTIFACT $chain cosign verify-blob --bundle evil-$PROBE_BUNDLE evil-$PROBE_ARTIFACT"
    v="$(pin_violations "$m")"
    printf '%s\n' "$v" | grep -qF -- 'MULTI-OCCURRENCE:' \
      || fail "UC-01: an UNPINNED 'cosign verify-blob' chained onto a pinned one with '$chain' on ONE line in $label was NOT caught — the check is per-LINE, not per-OCCURRENCE, which is the D-160 defect one level down: ${v:-<nothing>}"
    echo "OK: $label — a second call chained with '$chain' on one line fails closed (UC-01)"
  done

  # backslash-folded chain: the fold joins the two calls into one line, so this
  # must reach the same verdict by the same route.
  m="$WORK/univ.${label}.chained-folded"
  append_raw "$src" "$m" "cosign verify-blob $PINS_OK --bundle $PROBE_BUNDLE $PROBE_ARTIFACT && \\
  cosign verify-blob --bundle evil-$PROBE_BUNDLE evil-$PROBE_ARTIFACT"
  v="$(pin_violations "$m")"
  printf '%s\n' "$v" | grep -qF -- 'MULTI-OCCURRENCE:' \
    || fail "UC-01: a backslash-FOLDED chain hiding an unpinned second call in $label was NOT caught: ${v:-<nothing>}"
  echo "OK: $label — a backslash-folded chain fails closed (UC-01)"

  # the second call spelled so the literal trigger would miss it. occurrence_count
  # deliberately counts the shorter 'verify-blob' INSIDE an already-triggered
  # line, so these are seen even though the file-level trigger would not match.
  for spelling in '"$COSIGN" verify-blob' 'cosign  verify-blob' 'cosign "verify-blob"' "cosign 'verify-blob'" 'cosign verify-blob""'; do
    m="$WORK/univ.${label}.chained-spelling-$(printf '%s' "$spelling" | od -An -tx1 | tr -d ' \n')"
    append_raw "$src" "$m" "cosign verify-blob $PINS_OK --bundle $PROBE_BUNDLE $PROBE_ARTIFACT && $spelling --bundle evil-$PROBE_BUNDLE evil-$PROBE_ARTIFACT"
    v="$(pin_violations "$m")"
    printf '%s\n' "$v" | grep -qF -- 'MULTI-OCCURRENCE:' \
      || fail "UC-01: a second call spelled '$spelling' and chained onto a pinned one in $label was NOT counted as an occurrence: ${v:-<nothing>}"
    echo "OK: $label — a chained call spelled '$spelling' is still counted (UC-01)"
  done

  # FALSE-POSITIVE CONTROL for occurrence_count, the branch the UC-01 fix rests on.
  # A single, correctly pinned call whose BUNDLE happens to be named
  # `verify-blob-*.sigstore.json` must stay GREEN. Counting the bare substring
  # instead of whole words reds here — on a plausible filename — and the repair a
  # maintainer reaches for is loosening occurrence_count itself.
  m="$WORK/univ.${label}.filename-collision"
  append_raw "$src" "$m" "cosign verify-blob $PINS_OK --bundle verify-blob-$PROBE_BUNDLE verify-blob-$PROBE_ARTIFACT"
  v="$(pin_violations "$m")"
  [[ -z "$v" ]] \
    || fail "occurrence_count counts SUBSTRINGS, not whole words: a correctly pinned call whose bundle is named verify-blob-$PROBE_BUNDLE was reported as a violation in $label. That is a false positive on a plausible filename, on the branch the UC-01 fix rests on: $v"
  echo "OK: $label — a bundle filename containing 'verify-blob' does not fake a second occurrence (UC-01 false-positive control)"

  # ---- UC-02: ORDERING must not mask a defanged value. -------------------------
  # extract_issuer/extract_identity anchor on a greedy .*, so the LAST value on a
  # line wins. A hostile empty regexp placed FIRST used to be masked by a correct
  # value placed second: the extraction direction favoured the attacker.
  m="$WORK/univ.${label}.order-empty-first"
  append_raw "$src" "$m" "cosign verify-blob --certificate-oidc-issuer ${sec_issuer} --certificate-identity-regexp '' --bundle $PROBE_BUNDLE $PROBE_ARTIFACT && cosign verify-blob $PINS_OK --bundle b-$PROBE_BUNDLE b-$PROBE_ARTIFACT"
  v="$(pin_violations "$m")"
  [[ -n "$v" ]] \
    || fail "UC-02: an EMPTY identity regexp placed FIRST and masked by a correct value placed second in $label went unreported — ordering defeats the value check"
  echo "OK: $label — a defanged value cannot be masked by a correct one later on the line (UC-02)"

  # the same defect within ONE occurrence: the flag repeated, empty value first.
  m="$WORK/univ.${label}.dup-flag-empty-first"
  append_raw "$src" "$m" "cosign verify-blob --certificate-oidc-issuer ${sec_issuer} --certificate-identity-regexp '' --certificate-identity-regexp '${sec_identity}' --bundle $PROBE_BUNDLE $PROBE_ARTIFACT"
  v="$(pin_violations "$m")"
  printf '%s\n' "$v" | grep -qF -- 'WRONG-VALUE[--certificate-identity-regexp]' \
    || fail "UC-02: --certificate-identity-regexp supplied TWICE in $label, empty first and correct second, was not reported — only the last value is graded: ${v:-<nothing>}"
  echo "OK: $label — EVERY value of a repeated flag is graded, not just the last (UC-02)"

  m="$WORK/univ.${label}.dup-issuer-foreign-first"
  append_raw "$src" "$m" "cosign verify-blob --certificate-oidc-issuer https://accounts.example.invalid --certificate-oidc-issuer ${sec_issuer} --certificate-identity-regexp '${sec_identity}' --bundle $PROBE_BUNDLE $PROBE_ARTIFACT"
  v="$(pin_violations "$m")"
  printf '%s\n' "$v" | grep -qF -- 'WRONG-VALUE[--certificate-oidc-issuer]' \
    || fail "UC-02: --certificate-oidc-issuer supplied TWICE in $label, foreign first and correct second, was not reported: ${v:-<nothing>}"
  echo "OK: $label — a repeated issuer flag is graded on every value (UC-02)"

  # ---- UC-03: quote style is diagnosed, not misdiagnosed. ---------------------
  # A CORRECTLY pinned call written with double quotes must be GREEN. It used to
  # red as 'WRONG-VALUE ... identity=<empty/unparsable> ... (an empty regexp
  # matches every Fulcio identity)' — a true refusal with a false reason, and the
  # repair a maintainer would reach for is widening the extractor.
  m="$WORK/univ.${label}.double-quoted"
  append_raw "$src" "$m" "cosign verify-blob --certificate-oidc-issuer ${sec_issuer} --certificate-identity-regexp \"${sec_identity}\" --bundle $PROBE_BUNDLE $PROBE_ARTIFACT"
  [[ "$(cosign_invocations "$m" | grep -c . || true)" -eq "$((base_inv + 1))" ]] \
    || fail "the double-quoted mutant of $label did not raise the invocation count from $base_inv — the extractor never saw it, so its green proves nothing"
  v="$(pin_violations "$m")"
  [[ -z "$v" ]] \
    || fail "UC-03: a CORRECTLY pinned, double-quoted invocation in $label was reported as a violation — the gate refuses a correct call, and the message is what teaches a maintainer to widen the extractor: $v"
  echo "OK: $label — a correctly pinned double-quoted call is accepted (UC-03)"

  # a BARE regexp is a real defect (the shell globs and word-splits it) but it is
  # NOT an empty value, and the message must say so.
  m="$WORK/univ.${label}.bare-value"
  append_raw "$src" "$m" "cosign verify-blob --certificate-oidc-issuer ${sec_issuer} --certificate-identity-regexp ${sec_identity} --bundle $PROBE_BUNDLE $PROBE_ARTIFACT"
  v="$(pin_violations "$m")"
  printf '%s\n' "$v" | grep -qF -- 'UNQUOTED-VALUE[--certificate-identity-regexp]' \
    || fail "UC-03: an UNQUOTED identity regexp in $label was not diagnosed as a quoting defect — it must not be reported as an empty or wrong value: ${v:-<nothing>}"
  if printf '%s\n' "$v" | grep -qF -- 'WRONG-VALUE[--certificate-identity-regexp]'; then
    fail "UC-03: the unquoted-regexp mutant of $label ALSO reported WRONG-VALUE — quoting and value defects are conflated, which is the misdiagnosis that erodes the gate: $v"
  fi
  echo "OK: $label — an unquoted regexp is named as a QUOTING defect, not a wrong value (UC-03)"
done

# The exemption is narrow IN ONE DIMENSION, and 2c grades exactly that one: it is
# the BACKTICKS that exempt SECURITY.md's capability-table row, not the words.
# Strip them and the same row must fail closed, so the exemption cannot be widened
# to "any line mentioning cosign verify-blob" without this section reddening.
#
# IT IS NOT NARROW IN THE OTHER DIMENSION, and this section does NOT claim it is
# (R3-01): the substring is looked for ANYWHERE on the line, so a line carrying a
# backticked mention AND live calls is exempted. 2c grades the backticks-vs-words
# axis; the same-line axis is an open residual, disclosed in the PASS banner.
echo "== 2c. UNIV-COSIGN: the exemption keys on the BACKTICKS, not on the words =="
grep -qF -- '`cosign verify-blob`' "$SECURITY" \
  || fail "SECURITY.md no longer contains a backticked \`cosign verify-blob\` prose mention — 2c grades an exemption that is no longer exercised, so it is decorative (re-point it at whatever prose mention exists, or delete the exemption)"
prose_mutant="$WORK/SECURITY.prose-declassified.md"
sed 's/`cosign verify-blob`/cosign verify-blob/g' "$SECURITY" >"$prose_mutant"
if grep -qF -- '`cosign verify-blob`' "$prose_mutant"; then
  fail "mutation did not land: $prose_mutant still has a backticked mention"
fi
v="$(pin_violations "$prose_mutant")"
printf '%s\n' "$v" | grep -qF -- 'UNCLASSIFIABLE:' \
  || fail "un-backticking SECURITY.md's prose mention left it exempt — the exemption keys on the words, not the backticks, so any line containing 'cosign verify-blob' could be waved through as prose: ${v:-<nothing>}"
[[ -z "$(pin_violations "$SECURITY")" ]] \
  || fail "the unmutated SECURITY.md is not clean — 2c's mutant/control pair is not comparing what it claims"
echo "OK: the exemption fires on the backticks alone; removing them fails closed"

# --------------------------------------------- 3. REQ-AUD2-S03-04 (drift gate) --

echo "== 3. REQ-AUD2-S03-04 / AUD2-F01: THREE files publish ONE truth =="
inst_issuer="$(one_value "$INSTALL" extract_issuer "hack/install.sh issuer")"
inst_identity="$(one_value "$INSTALL" extract_identity "hack/install.sh identity regexp")"
ver_issuer="$(one_value "$VERIFY" extract_issuer "hack/release/verify-artifacts.sh issuer")"
ver_identity="$(one_value "$VERIFY" extract_identity "hack/release/verify-artifacts.sh identity regexp")"

drift_free() { # <issuer-a> <identity-a> <issuer-b> <identity-b>
  [[ "$1" == "$3" && "$2" == "$4" ]]
}

drift_free "$inst_issuer" "$inst_identity" "$sec_issuer" "$sec_identity" || fail \
  "DRIFT: hack/install.sh pins issuer=${inst_issuer} identity=${inst_identity} but SECURITY.md publishes issuer=${sec_issuer} identity=${sec_identity} — adopters following the published instructions and adopters running install.sh would verify against different signers (REQ-AUD2-S03-04)"
drift_free "$ver_issuer" "$ver_identity" "$sec_issuer" "$sec_identity" || fail \
  "DRIFT: hack/release/verify-artifacts.sh pins issuer=${ver_issuer} identity=${ver_identity} but SECURITY.md publishes issuer=${sec_issuer} identity=${sec_identity} — the maintainer/CI release check and the published adopter recipe would verify against different signers (AUD2-F01)"
drift_free "$inst_issuer" "$inst_identity" "$ver_issuer" "$ver_identity" || fail \
  "DRIFT: hack/install.sh pins issuer=${inst_issuer} identity=${inst_identity} but hack/release/verify-artifacts.sh pins issuer=${ver_issuer} identity=${ver_identity} — the adopter path and the maintainer path would verify against different signers (AUD2-F01)"
echo "OK: hack/install.sh, SECURITY.md and hack/release/verify-artifacts.sh agree byte-for-byte (issuer=${inst_issuer}, identity=${inst_identity})"

echo "== 3b. the drift comparison itself can fail (mutations, both directions) =="
sec_mutant="$WORK/SECURITY.drift.md"
# The rewrite is flag-relative, never a sed pattern built from the pinned value:
# the identity pin is itself a regexp ([Aa], escaped dots), so interpolating it
# into a pattern would silently match nothing and the mutation would not land.
sed -E "s|(--certificate-oidc-issuer )[^[:space:]]+|\\1https://accounts.example.invalid|g" "$SECURITY" >"$sec_mutant"
grep -qF 'https://accounts.example.invalid' "$sec_mutant" || fail "mutation did not land: $sec_mutant has no rewritten issuer"
mut_issuer="$(extract_issuer "$sec_mutant" | head -1)"
if drift_free "$inst_issuer" "$inst_identity" "$mut_issuer" "$sec_identity"; then
  fail "the drift comparison stayed green with a rewritten SECURITY.md issuer — the drift gate is vacuous"
fi
echo "OK: rewriting SECURITY.md's issuer alone turns the drift gate red"

inst_mutant="$WORK/install.drift.sh"
sed -E "s|(--certificate-identity-regexp )'[^']*'|\\1'^https://github\\.com/evil-mirror/assent/'|" "$INSTALL" >"$inst_mutant"
grep -qF 'evil-mirror' "$inst_mutant" || fail "mutation did not land: $inst_mutant has no rewritten identity regexp"
mut_identity="$(extract_identity "$inst_mutant" | head -1)"
if drift_free "$inst_issuer" "$mut_identity" "$sec_issuer" "$sec_identity"; then
  fail "the drift comparison stayed green with a rewritten install.sh identity regexp — the drift gate is vacuous"
fi
echo "OK: rewriting hack/install.sh's identity regexp alone turns the drift gate red"

echo "== 3c. AUD2-F01: the THIRD file is really in the comparison (mutations, both fields) =="
# Flag-relative rewrites again — the pin is itself a regexp ([Aa], escaped dots),
# so interpolating it into a sed pattern would match nothing and the mutation
# would not land. Temp copies only; the tracked file is never touched.
ver_mutant_id="$WORK/verify-artifacts.drift-identity.sh"
sed -E "s|(--certificate-identity-regexp )'[^']*'|\\1'^https://github\\.com/evil-mirror/assent/'|" "$VERIFY" >"$ver_mutant_id"
grep -qF 'evil-mirror' "$ver_mutant_id" || fail "mutation did not land: $ver_mutant_id has no rewritten identity regexp"
mut_ver_identity="$(extract_identity "$ver_mutant_id" | head -1)"
[[ "$mut_ver_identity" != "$ver_identity" ]] || fail "mutation did not land: $ver_mutant_id still extracts the real identity pin"
if drift_free "$inst_issuer" "$inst_identity" "$ver_issuer" "$mut_ver_identity"; then
  fail "the drift comparison stayed green with a rewritten verify-artifacts.sh identity regexp — the third file is not actually compared (AUD2-F01)"
fi

ver_mutant_iss="$WORK/verify-artifacts.drift-issuer.sh"
sed -E "s|(--certificate-oidc-issuer )[^[:space:]]+|\\1https://accounts.example.invalid|" "$VERIFY" >"$ver_mutant_iss"
grep -qF 'https://accounts.example.invalid' "$ver_mutant_iss" || fail "mutation did not land: $ver_mutant_iss has no rewritten issuer"
mut_ver_issuer="$(extract_issuer "$ver_mutant_iss" | head -1)"
[[ "$mut_ver_issuer" != "$ver_issuer" ]] || fail "mutation did not land: $ver_mutant_iss still extracts the real issuer pin"
if drift_free "$mut_ver_issuer" "$ver_identity" "$sec_issuer" "$sec_identity"; then
  fail "the drift comparison stayed green with a rewritten verify-artifacts.sh issuer — the third file is not actually compared (AUD2-F01)"
fi
echo "OK: rewriting either field in hack/release/verify-artifacts.sh alone turns the drift gate red"

# ------------------------------ 4/5. behaviour: stub cosign, both polarities --

mkdir -p "$WORK/bin"
cat >"$WORK/bin/cosign" <<'STUB'
#!/usr/bin/env bash
# Offline stub for `cosign verify-blob`, used by hack/release/install_cosign_pin_test.sh.
# Models cosign keyless verification:
#   * --certificate-identity-regexp given -> the bundle's certificate identity MUST match
#   * --certificate-oidc-issuer given     -> the bundle's issuer MUST be equal
#   * neither given                       -> ACCEPT ANY identity (the pre-fix hole)
# The permissive no-flag branch is deliberate: it is the dangerous half of SEC-03,
# and modelling it is what lets the mutation section prove the pin is load-bearing.
set -uo pipefail
sub="${1:-}"
shift || true
[[ "$sub" == "verify-blob" ]] || { echo "stub-cosign: unsupported subcommand: ${sub}" >&2; exit 2; }

issuer="" identity_re="" bundle=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --certificate-oidc-issuer) issuer="${2:-}"; shift 2 ;;
    --certificate-identity-regexp) identity_re="${2:-}"; shift 2 ;;
    --bundle) bundle="${2:-}"; shift 2 ;;
    *) shift ;;
  esac
done

if [[ -n "${COSIGN_STUB_LOG:-}" ]]; then
  printf 'issuer=%s identity_re=%s bundle=%s\n' "${issuer:-<none>}" "${identity_re:-<none>}" "${bundle:-<none>}" >>"$COSIGN_STUB_LOG"
fi
[[ -f "$bundle" ]] || { echo "stub-cosign: bundle not found: ${bundle}" >&2; exit 2; }

cert_identity="$(sed -nE 's/.*"certIdentity"[[:space:]]*:[[:space:]]*"([^"]*)".*/\1/p' "$bundle" | head -1)"
cert_issuer="$(sed -nE 's/.*"certIssuer"[[:space:]]*:[[:space:]]*"([^"]*)".*/\1/p' "$bundle" | head -1)"
echo "stub-cosign: bundle identity=${cert_identity} issuer=${cert_issuer}; pinned regexp=${identity_re:-<NONE>} issuer=${issuer:-<NONE>}" >&2

if [[ -n "$identity_re" ]] && ! [[ "$cert_identity" =~ $identity_re ]]; then
  echo "stub-cosign: none of the expected identities matched what was in the certificate: ${cert_identity} !~ ${identity_re}" >&2
  exit 1
fi
if [[ -n "$issuer" && "$cert_issuer" != "$issuer" ]]; then
  echo "stub-cosign: certificate issuer ${cert_issuer} != pinned ${issuer}" >&2
  exit 1
fi
echo "Verified OK"
STUB
chmod +x "$WORK/bin/cosign"

echo "== 4. stub cosign self-test (the harness must be able to say NO and YES) =="
# Deliberately SYNTHETIC values, not the project's pin: this section grades the
# STUB. Driving it with the real pin would make a wrong pin surface here as
# "the stub is broken", hiding the actual defect 4b/4c exist to name.
probe_re='^https://example\.test/owner/repo/'
probe_issuer='https://issuer.example.test'
printf '{"certIdentity":"https://example.test/other/repo/wf.yaml@refs/tags/v1","certIssuer":"%s"}\n' "$probe_issuer" >"$WORK/probe-evil.sigstore.json"
if PATH="$WORK/bin:$PATH" cosign verify-blob --certificate-oidc-issuer "$probe_issuer" \
    --certificate-identity-regexp "$probe_re" --bundle "$WORK/probe-evil.sigstore.json" /dev/null >/dev/null 2>&1; then
  fail "stub cosign ACCEPTED an identity outside the regexp it was handed — the stub is broken and every negative case below would be vacuous"
fi
printf '{"certIdentity":"https://example.test/owner/repo/wf.yaml@refs/tags/v1","certIssuer":"%s"}\n' "$probe_issuer" >"$WORK/probe-good.sigstore.json"
PATH="$WORK/bin:$PATH" cosign verify-blob --certificate-oidc-issuer "$probe_issuer" \
  --certificate-identity-regexp "$probe_re" --bundle "$WORK/probe-good.sigstore.json" /dev/null >/dev/null 2>&1 \
  || fail "stub cosign REJECTED an identity that matches the regexp it was handed — the stub is broken and every green below would be meaningless"
PATH="$WORK/bin:$PATH" cosign verify-blob --bundle "$WORK/probe-evil.sigstore.json" /dev/null >/dev/null 2>&1 \
  || fail "stub cosign rejected an UNPINNED verification — it must model the permissive pre-fix branch, or section 5c proves nothing"
echo "OK: stub cosign rejects a non-matching identity, accepts a matching one, and is permissive when unpinned"

# make_case <dir> <identity> <issuer> — archive + checksums + sigstore bundle.
make_case() {
  local dir="$1" identity="$2" issuer="$3"
  mkdir -p "$dir/payload"
  printf '#!/usr/bin/env bash\necho "assent version fixture-0.0.0"\n' >"$dir/payload/assent"
  chmod +x "$dir/payload/assent"
  tar -czf "$dir/assent_fixture_linux_amd64.tar.gz" -C "$dir/payload" assent
  (
    cd "$dir"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum assent_fixture_linux_amd64.tar.gz >checksums.txt
    else
      shasum -a 256 assent_fixture_linux_amd64.tar.gz >checksums.txt
    fi
  )
  printf '{"certIdentity":"%s","certIssuer":"%s"}\n' "$identity" "$issuer" \
    >"$dir/assent_fixture_linux_amd64.tar.gz.sigstore.json"
}

# run_install <script> <case-dir> <dest> — install.sh with --require-signature
# and the stub cosign first on PATH; returns the script's exit code.
run_install() {
  local script="$1" dir="$2" dest="$3" rc=0
  mkdir -p "$dest"
  set +e
  PATH="$WORK/bin:$PATH" COSIGN_STUB_LOG="$WORK/stub.log" "$script" \
    --archive "$dir/assent_fixture_linux_amd64.tar.gz" \
    --checksums "$dir/checksums.txt" \
    --dest "$dest" --require-signature >"$dir/out" 2>"$dir/err"
  rc=$?
  set -e
  return "$rc"
}

# The REAL v0.3.0 signer identity, decoded from the published bundle (see 4c).
GOOD_IDENTITY="https://github.com/PlatformRelay/Assent/.github/workflows/release.yaml@refs/tags/v0.3.0"
EVIL_IDENTITY="https://github.com/evil-mirror/assent/.github/workflows/release.yaml@refs/tags/v0.3.0"
EVIL_ISSUER="https://accounts.example.invalid"

echo "== 4b. positive control: this project's own signature installs =="
make_case "$WORK/case-good" "$GOOD_IDENTITY" "$sec_issuer"
: >"$WORK/stub.log"
run_install "$INSTALL" "$WORK/case-good" "$WORK/dest-good" \
  || fail "install.sh --require-signature REJECTED a bundle carrying this project's REAL v0.3.0 signer identity (${GOOD_IDENTITY}) — the pinned regexp does not match the project's own releases; check the repo-name casing, the Fulcio SAN says PlatformRelay/Assent: $(cat "$WORK/case-good/err")"
[[ -x "$WORK/dest-good/assent" ]] || fail "install.sh exited 0 but wrote no binary to the destination — the fixture is broken and the negative case below would prove nothing"
grep -qF -- "identity_re=${sec_identity}" "$WORK/stub.log" \
  || fail "install.sh did not actually HAND cosign the identity regexp at runtime (stub log: $(cat "$WORK/stub.log")) — a flag present in the text but not in argv is not a pin"
grep -qF -- "issuer=${sec_issuer}" "$WORK/stub.log" \
  || fail "install.sh did not actually hand cosign the OIDC issuer at runtime (stub log: $(cat "$WORK/stub.log"))"
# UNIV-COSIGN (D-160), runtime half. The two greps above are EXISTENTIAL — "some
# recorded invocation carried the pin" — the same shape the static check had. If
# install.sh made a second, unpinned cosign call at runtime they would both still
# pass. Assert instead that EVERY line the stub recorded carried both values.
unpinned_runtime="$(log_unpinned_lines "$WORK/stub.log")"
[[ -z "$unpinned_runtime" ]] \
  || fail "install.sh made a cosign call at runtime WITHOUT both pinned values — a second, unpinned verification would leave the pinned one's log line intact and pass the greps above (D-160): $unpinned_runtime"
# ...and the predicate itself must be able to say NO, or the line above is decorative.
cp "$WORK/stub.log" "$WORK/stub.log.univ-mutant"
printf 'issuer=<none> identity_re=<none> bundle=%s\n' "$PROBE_BUNDLE" >>"$WORK/stub.log.univ-mutant"
[[ -n "$(log_unpinned_lines "$WORK/stub.log.univ-mutant")" ]] \
  || fail "log_unpinned_lines cannot see an unpinned invocation appended to a stub log — the runtime universal check is vacuous (D-160)"
echo "OK: own-identity bundle installs, and EVERY cosign call install.sh made carried both pins"

echo "== 4c. REAL published-release identities (the assertion that would have caught the casing bug) =="
# Public certificate contents, decoded from the .sigstore.json bundles GitHub
# serves for each tag (`openssl x509 -inform DER -text` on the bundle's cert,
# X509v3 Subject Alternative Name). Committed as FIXTURES on purpose: this gate
# must stay offline, and a pin that is never matched against a real signer
# identity can be confidently, greenly wrong — the lowercase-only value
# SECURITY.md published until now verified v0.1.0 and REJECTED v0.2.0/v0.3.0,
# because the repository was renamed PlatformRelay/assent -> PlatformRelay/Assent
# and cosign matches --certificate-identity-regexp case-sensitively.
#
# Each fixture is checked twice: as a regexp match (bash ERE, which agrees with
# cosign's Go RE2 on the anchors, escaped dots and character class used here)
# AND end-to-end through the real install.sh with the stub cosign, which is what
# actually proves the flag as install.sh spells it accepts/rejects the identity.
REAL_SANS_ACCEPT=(
  "https://github.com/PlatformRelay/Assent/.github/workflows/release.yaml@refs/tags/v0.3.0"
  "https://github.com/PlatformRelay/Assent/.github/workflows/release.yaml@refs/tags/v0.2.0"
  "https://github.com/PlatformRelay/assent/.github/workflows/release.yaml@refs/heads/main"
)
# Must NOT match: other owners, typosquats, another forge, a lost anchor, and an
# unescaped-dot host (githubXcom matches only if the `.` was left un-escaped).
REAL_SANS_REJECT=(
  "https://github.com/evil-mirror/Assent/.github/workflows/release.yaml@refs/tags/v0.3.0"
  "https://github.com/PlatformRelay/assent-mirror/.github/workflows/release.yaml@refs/tags/v0.3.0"
  "https://github.com/PlatformRelayEvil/Assent/.github/workflows/release.yaml@refs/tags/v0.3.0"
  "https://gitlab.com/PlatformRelay/Assent/.gitlab-ci.yml@refs/tags/v0.3.0"
  "https://evil.example/https://github.com/PlatformRelay/Assent/.github/workflows/release.yaml@refs/tags/v0.3.0"
  "https://githubXcom/PlatformRelay/Assent/.github/workflows/release.yaml@refs/tags/v0.3.0"
)

[[ "${#REAL_SANS_ACCEPT[@]}" -ge 3 && "${#REAL_SANS_REJECT[@]}" -ge 6 ]] \
  || fail "the real-identity fixture tables were emptied — this is the assertion that catches signer-identity drift"

i=0
for san in "${REAL_SANS_ACCEPT[@]}"; do
  i=$((i + 1))
  [[ "$san" =~ $inst_identity ]] \
    || fail "the pinned regexp ${inst_identity} REJECTS a REAL published release identity: ${san} — 'install.sh --require-signature' would fail closed against this project's own artifacts (the SEC-03 casing defect)"
  make_case "$WORK/real-ok-$i" "$san" "$sec_issuer"
  run_install "$INSTALL" "$WORK/real-ok-$i" "$WORK/dest-real-ok-$i" \
    || fail "install.sh rejected the REAL release identity ${san} end-to-end: $(cat "$WORK/real-ok-$i/err")"
  [[ -x "$WORK/dest-real-ok-$i/assent" ]] || fail "install.sh exited 0 for ${san} but installed nothing"
  echo "OK: accepts real published identity ${san}"
done

i=0
for san in "${REAL_SANS_REJECT[@]}"; do
  i=$((i + 1))
  if [[ "$san" =~ $inst_identity ]]; then
    fail "the pinned regexp ${inst_identity} ACCEPTS ${san} — the pin is wider than the org/repo it is supposed to anchor"
  fi
  make_case "$WORK/real-no-$i" "$san" "$sec_issuer"
  if run_install "$INSTALL" "$WORK/real-no-$i" "$WORK/dest-real-no-$i"; then
    fail "install.sh --require-signature ACCEPTED ${san} end-to-end — a mirror installs"
  fi
  [[ ! -e "$WORK/dest-real-no-$i/assent" ]] || fail "install.sh failed for ${san} but still wrote a binary"
  echo "OK: rejects ${san}"
done

echo "== 4d. the real-identity fixture check itself can fail (mutation) =="
# Re-run the accept table against the OLD lowercase-only pin: it must reject the
# v0.2.0/v0.3.0 identities. If it does not, this section proves nothing.
old_pin='^https://github.com/PlatformRelay/assent/'
rejected=0
for san in "${REAL_SANS_ACCEPT[@]}"; do
  [[ "$san" =~ $old_pin ]] || rejected=$((rejected + 1))
done
[[ "$rejected" -ge 2 ]] \
  || fail "the pre-fix lowercase pin '${old_pin}' still matched every real identity — then this section could not have caught the casing bug and is decorative"
echo "OK: the pre-fix lowercase pin rejects ${rejected} of ${#REAL_SANS_ACCEPT[@]} real published identities (that is the bug this section exists to catch)"

echo "== 5. REQ-AUD2-S03-02: a foreign-identity bundle fails and installs NOTHING =="
make_case "$WORK/case-evil" "$EVIL_IDENTITY" "$sec_issuer"
if run_install "$INSTALL" "$WORK/case-evil" "$WORK/dest-evil"; then
  fail "install.sh --require-signature ACCEPTED a bundle signed by ${EVIL_IDENTITY} — a mirror-swapped archive installs (SEC-03, REQ-AUD2-S03-02)"
fi
[[ ! -e "$WORK/dest-evil/assent" ]] \
  || fail "install.sh failed but STILL wrote a binary to the destination — fail-closed means nothing is installed (REQ-AUD2-S03-02)"
grep -qi 'cosign verification failed' "$WORK/case-evil/err" \
  || fail "install.sh failed for the wrong reason (expected the cosign die message): $(cat "$WORK/case-evil/err")"
echo "OK: foreign identity -> exit non-zero, destination empty, cosign die message"

echo "== 5b. a foreign ISSUER with a matching identity also fails =="
make_case "$WORK/case-issuer" "$GOOD_IDENTITY" "$EVIL_ISSUER"
if run_install "$INSTALL" "$WORK/case-issuer" "$WORK/dest-issuer"; then
  fail "install.sh accepted a bundle whose OIDC issuer is ${EVIL_ISSUER} — the issuer pin is not reaching cosign (REQ-AUD2-S03-02)"
fi
[[ ! -e "$WORK/dest-issuer/assent" ]] || fail "issuer mismatch still installed a binary"
echo "OK: foreign issuer -> exit non-zero, destination empty"

echo "== 5c. REQ-AUD2-S03-03: with the pin deleted, the foreign bundle INSTALLS =="
# The behavioural half of non-vacuity: the same fixture section 5 rejects is
# accepted by a copy of install.sh with the identity pin removed. That is the
# finding itself, reproduced — and it proves section 5's red comes from the flag
# and not from some unrelated property of the fixture.
mutant_id="$WORK/install.mut-identity.sh"
grep -vF -- '--certificate-identity-regexp' "$INSTALL" >"$mutant_id"
chmod +x "$mutant_id"
make_case "$WORK/case-evil2" "$EVIL_IDENTITY" "$sec_issuer"
run_install "$mutant_id" "$WORK/case-evil2" "$WORK/dest-evil2" \
  || fail "the un-pinned mutant ALSO rejected the foreign bundle — then section 5's red is not caused by --certificate-identity-regexp and this gate proves nothing: $(cat "$WORK/case-evil2/err")"
[[ -x "$WORK/dest-evil2/assent" ]] \
  || fail "the un-pinned mutant exited 0 but installed nothing — the mutation demonstration is inconclusive"
echo "OK: deleting --certificate-identity-regexp re-opens SEC-03 (foreign archive installs) — the pin is load-bearing"

mutant_iss="$WORK/install.mut-issuer.sh"
grep -vF -- '--certificate-oidc-issuer' "$INSTALL" >"$mutant_iss"
chmod +x "$mutant_iss"
make_case "$WORK/case-issuer2" "$GOOD_IDENTITY" "$EVIL_ISSUER"
run_install "$mutant_iss" "$WORK/case-issuer2" "$WORK/dest-issuer2" \
  || fail "the issuer-less mutant ALSO rejected the wrong-issuer bundle — section 5b's red is not caused by --certificate-oidc-issuer: $(cat "$WORK/case-issuer2/err")"
[[ -x "$WORK/dest-issuer2/assent" ]] || fail "the issuer-less mutant exited 0 but installed nothing"
echo "OK: deleting --certificate-oidc-issuer re-opens the issuer half — that pin is load-bearing too"

# ------------------------- 5d-5g. AUD2-F01: the maintainer path, behaviourally --

# make_dist_case <dir> <identity> <issuer> [nobundle] — a dist/ directory in the
# shape verify-artifacts.sh expects: one archive, checksums.txt, and (unless
# `nobundle`) a .sigstore.json beside the archive. The archive name carries a
# semver so resolve_expected_version infers 9.9.9, and the fixture binary reports
# exactly `assent 9.9.9`, so the run reaches the END of the script rather than
# dying on the version check for an unrelated reason.
make_dist_case() {
  local dir="$1" identity="$2" issuer="$3" mode="${4:-bundle}"
  local archive="assent_9.9.9_linux_amd64.tar.gz"
  mkdir -p "$dir/payload"
  printf '#!/usr/bin/env bash\necho "assent 9.9.9"\n' >"$dir/payload/assent"
  chmod +x "$dir/payload/assent"
  tar -czf "$dir/$archive" -C "$dir/payload" assent
  rm -rf "$dir/payload"
  (
    cd "$dir"
    if command -v sha256sum >/dev/null 2>&1; then
      sha256sum "$archive" >checksums.txt
    else
      shasum -a 256 "$archive" >checksums.txt
    fi
  )
  if [[ "$mode" != "nobundle" ]]; then
    printf '{"certIdentity":"%s","certIssuer":"%s"}\n' "$identity" "$issuer" \
      >"$dir/${archive}.sigstore.json"
  fi
}

# run_verify <script> <dist-dir> — verify-artifacts.sh with --require-signature
# and the stub cosign first on PATH; returns the script's exit code.
run_verify() {
  local script="$1" dir="$2" rc=0
  set +e
  PATH="$WORK/bin:$PATH" COSIGN_STUB_LOG="$WORK/stub.log" "$script" \
    --dist "$dir" --require-signature >"$dir/out" 2>"$dir/err"
  rc=$?
  set -e
  return "$rc"
}

echo "== 5d. AUD2-F01: verify-artifacts.sh hands cosign BOTH pins at runtime =="
make_dist_case "$WORK/dist-good" "$GOOD_IDENTITY" "$sec_issuer"
: >"$WORK/stub.log"
run_verify "$VERIFY" "$WORK/dist-good" \
  || fail "verify-artifacts.sh --require-signature REJECTED a bundle carrying this project's REAL v0.3.0 signer identity (${GOOD_IDENTITY}): $(cat "$WORK/dist-good/err")"
# THE anti-vacuity assertion for this file (see the header): an empty stub log
# means the cosign branch was never entered, which is exactly what the snapshot
# path looks like — everything below it would then be proving nothing.
[[ -s "$WORK/stub.log" ]] \
  || fail "verify-artifacts.sh exited 0 WITHOUT ever invoking cosign — the bundle branch was skipped (the snapshot-path shape), so this whole section is vacuous (AUD2-F01)"
grep -qF -- 'assent_9.9.9_linux_amd64.tar.gz.sigstore.json' "$WORK/stub.log" \
  || fail "cosign ran but not on the fixture bundle (stub log: $(cat "$WORK/stub.log"))"
grep -qF -- "identity_re=${sec_identity}" "$WORK/stub.log" \
  || fail "verify-artifacts.sh did not actually HAND cosign the identity regexp at runtime (stub log: $(cat "$WORK/stub.log")) — a flag present in the text but not in argv is not a pin (AUD2-F01)"
grep -qF -- "issuer=${sec_issuer}" "$WORK/stub.log" \
  || fail "verify-artifacts.sh did not actually hand cosign the OIDC issuer at runtime (stub log: $(cat "$WORK/stub.log"))"
# UNIV-COSIGN (D-160), runtime half — see 4b. This is the file the reviewer probed:
# a second, unpinned `cosign verify-blob` here left the greps above green because
# the pinned call's log line still satisfied them.
unpinned_runtime="$(log_unpinned_lines "$WORK/stub.log")"
[[ -z "$unpinned_runtime" ]] \
  || fail "verify-artifacts.sh made a cosign call at runtime WITHOUT both pinned values — 'task release-verify' performed an unpinned verification alongside the pinned one (D-160): $unpinned_runtime"
echo "OK: the maintainer path verified the fixture and EVERY cosign call it made carried both pins"

echo "== 5e. AUD2-F01 vacuity control: a bundle-less dist never reaches cosign =="
# The paired control for 5d. Without it, `[[ -s stub.log ]]` could be green for
# reasons unrelated to the branch under test. This is the snapshot shape (D-110:
# snapshots ship no bundles), and it must leave the log EMPTY.
make_dist_case "$WORK/dist-nobundle" "" "" nobundle
: >"$WORK/stub.log"
if run_verify "$VERIFY" "$WORK/dist-nobundle"; then
  fail "verify-artifacts.sh --require-signature accepted a dist with no .sigstore.json bundle"
fi
[[ ! -s "$WORK/stub.log" ]] \
  || fail "cosign was invoked for a dist that carries no bundle — the stub log cannot discriminate 'reached the pinned call' from 'skipped it', so 5d's non-vacuity argument collapses"
echo "OK: no bundle -> cosign never runs and the log stays empty, so 5d's non-empty log really does mean the pinned call executed"

echo "== 5f. AUD2-F01: a foreign-identity bundle fails the maintainer check =="
make_dist_case "$WORK/dist-evil" "$EVIL_IDENTITY" "$sec_issuer"
: >"$WORK/stub.log"
if run_verify "$VERIFY" "$WORK/dist-evil"; then
  fail "verify-artifacts.sh --require-signature ACCEPTED an archive signed by ${EVIL_IDENTITY} — 'task release-verify' would green-light a mirror-swapped release (AUD2-F01)"
fi
# The checksum is verified BEFORE cosign, so a bare non-zero could be a SHA
# failure; the die message is what proves cosign is the reason.
grep -qi 'cosign verification failed' "$WORK/dist-evil/err" \
  || fail "verify-artifacts.sh failed for the wrong reason (expected the cosign die message): $(cat "$WORK/dist-evil/err")"
[[ -s "$WORK/stub.log" ]] || fail "the foreign-identity run never reached cosign — its red is not the pin's doing"
echo "OK: foreign identity -> exit non-zero with the cosign die message"

echo "== 5g. AUD2-F01: with the pin deleted, the same archive VERIFIES CLEAN =="
# The finding itself, reproduced on the maintainer path: the mutant is the
# pre-fix verify-artifacts.sh, and it green-lights the very archive 5f rejects.
ver_mut_id="$WORK/verify-artifacts.mut-identity.sh"
grep -vF -- '--certificate-identity-regexp' "$VERIFY" >"$ver_mut_id"
chmod +x "$ver_mut_id"
make_dist_case "$WORK/dist-evil2" "$EVIL_IDENTITY" "$sec_issuer"
run_verify "$ver_mut_id" "$WORK/dist-evil2" \
  || fail "the un-pinned mutant ALSO rejected the foreign bundle — then 5f's red is not caused by --certificate-identity-regexp and this section proves nothing: $(cat "$WORK/dist-evil2/err")"
grep -qF 'verify-artifacts: ok' "$WORK/dist-evil2/out" \
  || fail "the un-pinned mutant exited 0 but printed no success line — the demonstration is inconclusive: $(cat "$WORK/dist-evil2/out")"
echo "OK: deleting --certificate-identity-regexp re-opens SEC-03 on the maintainer path — the pin is load-bearing"

ver_mut_iss="$WORK/verify-artifacts.mut-issuer.sh"
grep -vF -- '--certificate-oidc-issuer' "$VERIFY" >"$ver_mut_iss"
chmod +x "$ver_mut_iss"
make_dist_case "$WORK/dist-issuer" "$GOOD_IDENTITY" "$EVIL_ISSUER"
if run_verify "$VERIFY" "$WORK/dist-issuer"; then
  fail "verify-artifacts.sh accepted a bundle whose OIDC issuer is ${EVIL_ISSUER} — the issuer pin is not reaching cosign (AUD2-F01)"
fi
make_dist_case "$WORK/dist-issuer2" "$GOOD_IDENTITY" "$EVIL_ISSUER"
run_verify "$ver_mut_iss" "$WORK/dist-issuer2" \
  || fail "the issuer-less mutant ALSO rejected the wrong-issuer bundle — the issuer red is not caused by --certificate-oidc-issuer: $(cat "$WORK/dist-issuer2/err")"
echo "OK: the issuer pin on the maintainer path is load-bearing too"

# ----------------------------------------------- 6. REQ-AUD2-S03-05 (wiring) --

echo "== 6. REQ-AUD2-S03-05: this gate is invoked by 'task check' (D-124) =="
extract_block() { # <file> <2-space-indented key>
  awk -v name="$2" '
    $0 == "  " name ":" { inblk = 1; next }
    inblk && /^  [A-Za-z0-9_.:-]+:[[:space:]]*$/ { inblk = 0 }
    inblk { print }
  ' "$1"
}
check_lists_task() { # <taskfile> <stage>
  extract_block "$1" check | grep -qE "^[[:space:]]+- task: $2\$"
}

extract_block "$TASKFILE" check >"$WORK/check.block"
[[ -s "$WORK/check.block" ]] || fail "Taskfile check: block extracted EMPTY — the wiring assertions would be vacuous"
grep -qE '^[[:space:]]+- task: build$' "$WORK/check.block" \
  || fail "Taskfile check: block does not contain the known-present '- task: build' — extraction is wrong"

check_lists_task "$TASKFILE" "$STAGE" \
  || fail "'task check' does not run '$STAGE' — a gate invoked by nothing is not a gate (D-124, REQ-AUD2-S03-05)"
extract_block "$TASKFILE" "$STAGE" >"$WORK/def.stage"
[[ -s "$WORK/def.stage" ]] || fail "'$STAGE' is listed in check: but not defined in Taskfile.yml"
grep -qF 'hack/release/install_cosign_pin_test.sh' "$WORK/def.stage" \
  || fail "the '$STAGE' task does not invoke hack/release/install_cosign_pin_test.sh"
echo "OK: task check runs $STAGE, which invokes this script"

mutant_tf="$WORK/Taskfile.no-stage.yml"
grep -vE "^[[:space:]]+- task: ${STAGE}\$" "$TASKFILE" >"$mutant_tf"
[[ "$(wc -l <"$mutant_tf")" -lt "$(wc -l <"$TASKFILE")" ]] || fail "mutation did not land: $mutant_tf has the same line count as Taskfile.yml"
if check_lists_task "$mutant_tf" "$STAGE"; then
  fail "check_lists_task reports $STAGE wired in a Taskfile with that line deleted — the wiring assertion is vacuous"
fi
echo "OK: deleting '- task: $STAGE' from check: turns the wiring assertion red"

echo "== 6b. the AUD-S18 CHECK_STAGES pin knows about this stage =="
# hack/audit/exitgate_test.sh grades `task check` stage-by-stage against that
# array; a stage missing from it is a stage the release exit gate does not grade.
grep -qE "^[[:space:]]+${STAGE}\$" "$AUDIT_GATE" \
  || fail "'$STAGE' is not in CHECK_STAGES in hack/audit/exitgate_test.sh — the release exit gate would not grade it (AUD-S18)"
echo "OK: $STAGE is pinned in CHECK_STAGES"

# ------------------------------------------------------ 7. the PASS banner --
#
# UC-07 — THE BANNER STATES ONLY WHAT WAS ASSERTED, AND REPORTS WHAT WAS SEEN.
#
# The previous banner was printed unconditionally and asserted "EVERY cosign
# verify-blob OCCURRENCE ... is pinned" plus "a second unpinned call reddens
# whether it sits on its own line or is chained onto a pinned one" — a UNIVERSAL
# claim bound to NO assertion. When UC-05's quoted spelling slipped past the
# occurrence count, that banner did not merely overstate: it printed something
# FALSE, on a green run, which is strictly worse than printing nothing. This
# repo's #1 defect class is an assertion that cannot fail; a claim that is not
# bound to an assertion is the same defect wearing a different hat.
#
# So the claim is now bound to the evidence and the residual travels in the same
# breath as the claim. Four rounds of review on this one gate have each found a
# spelling or a scope the previous round did not cover; the honest posture is that
# a fifth exists and has not been found yet. A gate that says "I classified and
# graded 4 occurrences, I refuse to guess about anything I could not classify, and
# here is what I cannot see at all" stays TRUE when that fifth spelling turns up.
# It is then INCOMPLETE rather than WRONG, and incomplete is recoverable.
#
# The counts below are exact at this point in the run: reaching here means
# pin_violations was EMPTY for all three files, so there were no MULTI-OCCURRENCE
# and no UNCLASSIFIABLE candidates, and therefore candidates = graded + prose.
echo "PASS: install_cosign_pin_test.sh — REQ-AUD2-S03-01..05 + AUD2-F01 + UNIV-COSIGN (D-160, UC-01..UC-08)"
echo "  OBSERVED (counts, not proofs of absence):"
for target in "${GRADED[@]}"; do
  label="${target%%:*}"
  n_c="$(grep -c . <"$WORK/cand.$label" | tr -d ' ')"
  n_i="$(grep -c . <"$WORK/inv.$label" | tr -d ' ')"
  # "exempted" is a fact about which branch the line took; calling it "prose" is an
  # INFERENCE, and R3-01 shows it is the wrong one — an exempted line may still
  # carry live calls. Report the branch, not the guess about what the line is.
  echo "    ${label}: ${n_c} \`cosign verify-blob\` occurrence(s) found — ${n_i} classified as invocations and GRADED, $((n_c - n_i)) NOT GRADED (exempted because the line contains a backticked \`cosign verify-blob\` mention; such a line may still carry live calls — see residual (1)), 0 refused as unclassifiable"
done
echo "  ASSERTED, and each shown to FAIL on a mutant carrying the defect it exists to catch:"
echo "    * every GRADED occurrence carries --certificate-oidc-issuer, --certificate-identity-regexp and --bundle,"
echo "      with EVERY value it supplies for those flags equal to SECURITY.md's published pair"
echo "      (issuer=${inst_issuer}, identity=${inst_identity}) — not merely the last value on the line;"
echo "    * hack/install.sh, SECURITY.md and hack/release/verify-artifacts.sh publish that pair byte-identically;"
echo "    * the pinned regexp accepts all ${#REAL_SANS_ACCEPT[@]} REAL published release identities and rejects all ${#REAL_SANS_REJECT[@]} negatives;"
echo "    * a foreign-signed bundle fails closed on the adopter path with nothing installed, and fails the maintainer path too;"
echo "    * on the mutants section 2b builds, a second UNPINNED call reddens both on its own line and chained onto a"
echo "      pinned one with && / ; / | (in the spellings 2b enumerates), while a second CORRECTLY pinned call does not."
echo "  NOT ASSERTED — the residual, stated here rather than somewhere a reader will not look:"
echo "    (1) THE BACKTICKED-MENTION EXEMPTION IS THE RESIDUAL THAT IS ACTUALLY REACHABLE. A candidate line that is"
echo "        NOT at command position is exempted whenever the BACKTICK-DELIMITED substring (a backtick, then"
echo "        cosign verify-blob, then a backtick) occurs ANYWHERE ON THE LINE. The test is that substring, NOT"
echo "        that the line is prose, and it runs BEFORE the multi-token refusal in (3) and before the"
echo "        UNCLASSIFIABLE fallback. So a line carrying BOTH such a mention AND one or more LIVE calls is"
echo "        exempted and NOT GRADED, and neither of those two checks ever runs on it. Measured (R3-01), on"
echo "        SECURITY.md and on a shell script; on the document the whole gate stays green, which is how it"
echo "        was missed."
echo "    (2) This gate finds calls by the literal string 'cosign verify-blob' and counts them as whitespace-delimited"
echo "        words with shell quotes stripped. A call spelled through a variable, built by eval, or assembled from"
echo "        fragments is NOT SEEN and therefore NOT GRADED. No such spelling is present in these three files today,"
echo "        which makes this the LESS reachable residual of the two."
echo "    (3) The counts above are what was classified — they are NOT a proof that no other invocation exists in"
echo "        these files. A line AT COMMAND POSITION carrying more than one verify-blob token is REFUSED"
echo "        (MULTI-OCCURRENCE), never graded on its first call; that refusal does NOT reach a line exempted"
echo "        under (1), because the exemption is tested first."
