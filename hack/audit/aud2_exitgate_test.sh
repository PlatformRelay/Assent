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
# WHY THIS GATE IS TEXTUAL AND NOT A TEST RUN. Each remediation already has its
# own behavioural tests, which `task test` runs. What those tests do NOT do is
# survive their own deletion: TEST-02 is precisely the finding that a fix can be
# reverted with every wired gate still green. This gate is the anti-revert pin —
# a cheap, toolchain-free assertion over the source, which is why it can run as
# an early step of the PR-visible `verify` job (REQ-AUD2-S05-05) instead of only
# inside the push-only release-exitgate job (RELSE-08, the blind spot that hid
# AUD-S18's stale pin for four merges).
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

# body_of <file> <func> <out> <finding> <anchor> <decoy-header> — extract with
# both controls. Returns non-zero (naming <finding>) if the extraction produced
# nothing, lost its anchor, or swallowed the decoy function.
body_of() {
  local file="$1" fn="$2" out="$3" finding="$4" anchor="$5" decoy="$6"
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
  if [[ -n "$decoy" ]] && grep -Fq -- "$decoy" "$out"; then
    echo "  ${finding}: the extracted ${fn} body ALSO contains '${decoy}' — the extraction ran past the end of the function and into a neighbour that carries a decoy occurrence, so a same-file grep is what is really being graded" >&2
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
  # Decoy: CallHTTP is where MaxResponseBytes legitimately appears already.
  body_of "$file" CallExec "$body" "REL-01/02/07" 'cmd.Run()' 'func CallHTTP' || return 1

  # -- REL-01: stdout is bounded, and bounded by the SINGLE declared bound.
  local stdout_var
  stdout_var="$(rhs_of "$body" 'cmd\.Stdout')"
  if [[ -z "$stdout_var" ]]; then
    echo "  REL-01: CallExec does not assign cmd.Stdout at all — with no sink the bound cannot exist (AUD2-S01)" >&2
    rc=1
  elif ! grep -Eq "^[[:space:]]*${stdout_var}[[:space:]]*:?=[[:space:]]*newBoundedCapture\(MaxResponseBytes\)" "$body"; then
    echo "  REL-01: CallExec's stdout sink '${stdout_var}' is not created by newBoundedCapture(MaxResponseBytes) — a runaway digest-pinned provider fills memory unbounded while CallHTTP, in the same file, is bounded fail-closed; opts.Timeout bounds wall-clock, not bytes (AUD2-S01)" >&2
    rc=1
  fi

  # -- REL-02: killing the child does not close a pipe its grandchildren hold.
  local wait_rhs
  wait_rhs="$(rhs_of "$body" 'cmd\.WaitDelay')"
  if [[ -z "$wait_rhs" ]]; then
    echo "  REL-02: CallExec sets no cmd.WaitDelay — on context timeout cmd.Run() blocks until every writer to the stdout pipe closes, so a provider that forks a background grandchild hangs 'assent run' past its deadline with no decision, no diagnostic and no 'unavailable' classification (AUD2-S01)" >&2
    rc=1
  elif [[ "$wait_rhs" == "0" ]]; then
    echo "  REL-02: CallExec sets cmd.WaitDelay = 0, which is Go's 'wait forever' zero value — the REL-02 hang is reopened while the assignment still looks present (AUD2-S01)" >&2
    rc=1
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
    if ! grep -Eq "return .*[ ,(]${stderr_var}[ ,)]" "$body"; then
      echo "  REL-07: CallExec captures stderr into '${stderr_var}' but never folds it into the error it returns — collected-and-discarded diagnostics leave REL-07 open with the capture still visible in the source (AUD2-S01)" >&2
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
  # Anchor: the FileAtRef call itself. Decoy: loadResourceOwnerRegistry further
  # down the same file carries the identical errors.Is guard (D-130).
  body_of "$file" resolveRunFacts "$body" "REL-03" 'FileAtRef' 'func loadResourceOwnerRegistry' || return 1

  if ! grep -Fq 'errors.Is(err, forge.ErrNotFound)' "$body"; then
    echo "  REL-03: resolveRunFacts does not discriminate forge.ErrNotFound on the provider-declaration fetch — every FileAtRef error (retry-exhausted 5xx, deterministic 401/403) is treated as 'declaration absent' and skipped, so a forge blip silently converts approvable MRs to REVIEW by an invisible path. NOTE: the identical guard in loadResourceOwnerRegistry does NOT close this; the finding is at this call site (AUD2-S02)" >&2
    rc=1
  fi
  # Discrimination is only half of it: the non-absent branch must actually stop
  # the run rather than fall through to the same `continue`.
  if ! grep -Eq '^[[:space:]]*return nil, nil, fmt\.Errorf\(' "$body"; then
    echo "  REL-03: resolveRunFacts has no error return on the provider-declaration path — a discriminated non-ErrNotFound error that is not returned is the same silent skip with extra steps (AUD2-S02)" >&2
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
  # Anchor: EffectBlock, present in every version. Decoy: the sibling predicate
  # isMissedInterventionEffect sits immediately above it.
  body_of "$src" isStricterInterventionEffect "$body" "TEST-02" 'aggregate.EffectBlock' 'func isMissedInterventionEffect' || return 1

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
mutate "$m" 's|return nil, execFailure(err, stderr)|return nil, err|' 'return nil, err'
expect_red check_exec_containment "stderr is captured but discarded instead of folded into the returned error (REL-07)" \
  "never folds it into the error it returns" "$m"

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

# ---- wiring mutations ------------------------------------------------------
m="$WORK/Taskfile.nostage.yml"
grep -vE "^[[:space:]]+- task: ${STAGE}\$" "$TASKFILE" >"$m"
assert_changed "$TASKFILE" "$m"
expect_red check_task_wiring "'- task: ${STAGE}' was deleted from check: (REQ-AUD2-S05-04)" \
  "'task check' does not run" "$m"

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

# ============================================================================
# Wiring, real tree
# ============================================================================

echo
echo "== (6) REQ-AUD2-S05-03/04/05 — this gate is wired where it can be seen =="
expect_green check_task_wiring "'task check' runs ${STAGE}, which invokes ${SELF}" "$TASKFILE"
expect_green check_stage_pinned "${STAGE} is pinned in CHECK_STAGES (AUD-S18 grades it)" "$AUD_GATE"
expect_green check_pr_wiring "the pull-request-visible 'verify' job runs ${SELF}, undisarmed" "$WORKFLOW"

((MUTATIONS_PROVED >= 15)) ||
  fail "only ${MUTATIONS_PROVED} mutation controls ran — REQ-AUD2-S05-02 requires every assertion to be proved capable of failing; a section was skipped"

# ============================================================================
# REQ-AUD2-S05-01 — the disposition statement
# ============================================================================

echo
echo "== AUD2 findings dispositioned (REQ-AUD2-S05-01) =="
echo "  REL-01  (AUD2-S01) CLOSED — CallExec's stdout is bounded at MaxResponseBytes"
echo "  REL-02  (AUD2-S01) CLOSED — CallExec sets cmd.WaitDelay = opts.Timeout"
echo "  REL-07  (AUD2-S01) CLOSED — CallExec captures stderr and folds it into the error"
echo "  REL-03  (AUD2-S02) CLOSED — resolveRunFacts skips only on forge.ErrNotFound"
echo "  SEC-03  (AUD2-S03) CLOSED — hack/install.sh pins the signer identity + OIDC issuer, no drift from SECURITY.md"
echo "  TEST-02 (AUD2-S04) CLOSED — EffectChallenge is classified and ${CHALLENGE_TEST} defends it"
echo
echo "PASS: aud2_exitgate_test.sh — all four AUD2 remediations present (REL-01/02/07, REL-03, SEC-03, TEST-02), ${MUTATIONS_PROVED} mutation controls proved every assertion can fail, and the gate is wired into 'task check', pinned in CHECK_STAGES, and run by the pull-request-visible verify job"
