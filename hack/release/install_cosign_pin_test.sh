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
# ONE PUBLISHED TRUTH. The issuer/identity pair is published in SECURITY.md;
# hack/install.sh copies it byte-identically. Section 3 below is a drift gate:
# if either file is edited alone, this script reddens (REQ-AUD2-S03-04).
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

# cosign_invocation <file> — the `cosign verify-blob …` command with its
# backslash continuations folded onto one line. Comment lines are skipped so a
# flag mentioned only in prose can never satisfy an assertion.
cosign_invocation() {
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

# extract_issuer <file> — every distinct --certificate-oidc-issuer value.
extract_issuer() {
  sed -nE "s/.*--certificate-oidc-issuer[[:space:]]+['\"]?([^'\"[:space:]]+)['\"]?.*/\1/p" "$1" | sort -u
}

# extract_identity <file> — every distinct --certificate-identity-regexp value.
# The value is single-quoted in both files (a shell regexp must be); an unquoted
# value extracts as nothing and trips the positive controls rather than passing.
extract_identity() {
  sed -nE "s/.*--certificate-identity-regexp[[:space:]]+'([^']*)'.*/\1/p" "$1" | sort -u
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

# has_flag <file> <flag> — the flag appears inside the cosign invocation.
has_flag() {
  cosign_invocation "$1" | grep -qF -- "$2"
}

# ------------------------------------------- 0. extraction positive controls --

echo "== 0. extraction positive controls =="
[[ -f "$INSTALL" ]] || fail "hack/install.sh missing"
[[ -x "$INSTALL" ]] || fail "hack/install.sh is not executable"
[[ -f "$SECURITY" ]] || fail "SECURITY.md missing"

cosign_invocation "$INSTALL" >"$WORK/invocation"
[[ -s "$WORK/invocation" ]] || fail "no 'cosign verify-blob' invocation found in hack/install.sh — extraction broke, every flag assertion would be vacuous"
grep -qF -- '--bundle' "$WORK/invocation" || fail "the extracted cosign invocation does not contain the known-present --bundle flag — the awk fold is wrong"
echo "OK: cosign invocation extracted from hack/install.sh ($(wc -l <"$WORK/invocation" | tr -d ' ') command(s)), anchored on --bundle"

sec_issuer="$(one_value "$SECURITY" extract_issuer "SECURITY.md issuer")"
sec_identity="$(one_value "$SECURITY" extract_identity "SECURITY.md identity regexp")"
echo "OK: SECURITY.md publishes issuer=${sec_issuer} identity=${sec_identity}"

# ------------------------------------------------- 1. REQ-AUD2-S03-01 (flags) --

echo "== 1. REQ-AUD2-S03-01: hack/install.sh's cosign call carries both pins =="
has_flag "$INSTALL" '--certificate-oidc-issuer' \
  || fail "hack/install.sh's cosign verify-blob has no --certificate-oidc-issuer — keyless verification accepts any issuer (SEC-03, REQ-AUD2-S03-01)"
has_flag "$INSTALL" '--certificate-identity-regexp' \
  || fail "hack/install.sh's cosign verify-blob has no --certificate-identity-regexp — a mirror-swapped archive with its own valid bundle verifies clean (SEC-03, REQ-AUD2-S03-01)"
echo "OK: both --certificate-oidc-issuer and --certificate-identity-regexp are in the invocation"

# ------------------------------------------- 2. REQ-AUD2-S03-03 (non-vacuity) --

echo "== 2. REQ-AUD2-S03-03: deleting either flag from install.sh reddens section 1 =="
for flag in --certificate-oidc-issuer --certificate-identity-regexp; do
  mutant="$WORK/install.no${flag}.sh"
  grep -vF -- "$flag" "$INSTALL" >"$mutant"
  chmod +x "$mutant"
  if grep -qF -- "$flag" "$mutant"; then
    fail "mutation did not land: $flag is still present in $mutant"
  fi
  [[ "$(wc -l <"$mutant")" -lt "$(wc -l <"$INSTALL")" ]] \
    || fail "mutation did not land: $mutant has the same line count as hack/install.sh"
  if has_flag "$mutant" "$flag"; then
    fail "has_flag still reports $flag present in a script with that line deleted — the assertion is vacuous"
  fi
  echo "OK: deleting $flag makes the flag assertion red"
done

# --------------------------------------------- 3. REQ-AUD2-S03-04 (drift gate) --

echo "== 3. REQ-AUD2-S03-04: install.sh and SECURITY.md publish ONE truth =="
inst_issuer="$(one_value "$INSTALL" extract_issuer "hack/install.sh issuer")"
inst_identity="$(one_value "$INSTALL" extract_identity "hack/install.sh identity regexp")"

drift_free() { # <issuer-a> <identity-a> <issuer-b> <identity-b>
  [[ "$1" == "$3" && "$2" == "$4" ]]
}

drift_free "$inst_issuer" "$inst_identity" "$sec_issuer" "$sec_identity" || fail \
  "DRIFT: hack/install.sh pins issuer=${inst_issuer} identity=${inst_identity} but SECURITY.md publishes issuer=${sec_issuer} identity=${sec_identity} — adopters following the published instructions and adopters running install.sh would verify against different signers (REQ-AUD2-S03-04)"
echo "OK: hack/install.sh and SECURITY.md agree byte-for-byte (issuer=${inst_issuer}, identity=${inst_identity})"

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
echo "OK: own-identity bundle installs, and both pins reached cosign's argv"

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

echo "PASS: install_cosign_pin_test.sh — SEC-03 closed (REQ-AUD2-S03-01..05): hack/install.sh pins issuer=${inst_issuer} identity=${inst_identity}, byte-identical to SECURITY.md; a foreign-signed bundle fails closed with nothing installed; both pins proved load-bearing by mutation; the gate is wired into task check"
