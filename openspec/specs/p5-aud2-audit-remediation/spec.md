# P5-AUD2 — Audit remediation (2026-08-18): the risk-reduction wave

**Epic ID / REQ prefix:** `AUD2` / `REQ-AUD2-S0n-nn`.

**Origin:** `agent-context/PROJECT-AUDIT-2026-08-18.md` (verdict **READY WITH CONDITIONS** at
`af57be7`; both P1 conditions — RELSE-01 changelog regen and SEC-01 `toolchain go1.26.6` — were
closed the same day and **v0.3.0 shipped**). This epic is the audit's own **"Next (risk
reduction)" wave**, which the audit lists verbatim: the exec-transport trio (REL-01 bound
stdout, REL-02 `WaitDelay`, REL-07 stderr — *"one small lane, same function"*), REL-03
`ErrNotFound` discrimination, SEC-03 cosign identity pin, and the TEST-02 mutant-killing test.

**Vehicle:** four narrow, file-disjoint fix lanes plus one exit gate. Every story closes a
finding that the audit already stated **with its own verification recipe** — those recipes are
lifted verbatim into the `Verify:` lines below. No story invents scope beyond its finding.

---

## Problem

Four defects survive on `main` at `b4f5054`, each confirmed by reading the code. **Every line
number in this section is as of `b4f5054`** — the AUD2 fix lanes have since moved them (the
cosign call, for one, is at `hack/install.sh:127` on `main` today), so read them as evidence of
what the audit saw, not as a map of the current tree:

- **REL-01 (P2, prior F2, byte-identical across three audits)** — `CallExec` collects child
  stdout into an unlimited `bytes.Buffer` (`internal/provider/transport.go:158–162`) while
  `CallHTTP`, *twenty lines above in the same file*, is bounded fail-closed at
  `MaxResponseBytes` (8 MiB) through `readBounded`. A runaway digest-pinned provider OOMs the
  runner; `opts.Timeout` bounds wall-clock, not memory. The containment asymmetry is the
  finding — the mechanism to fix it already exists and is already tested for HTTP.
- **REL-02 (P2, NEW)** — `CallExec` sets no `WaitDelay`. On context timeout
  `exec.CommandContext` kills the child, but `cmd.Run()` blocks until every writer to the
  stdout pipe closes. A provider that forks a background grandchild inheriting stdout hangs
  `assent run` past its deadline until the CI runner's own job timeout: no decision, no
  diagnostic, no `unavailable` classification.
- **REL-07 (P3, same function)** — `CallExec` never wires `cmd.Stderr`. A failing provider
  yields `"exit status 1"` with zero diagnostic content, so an operator debugging a
  fail-closed REVIEW has nothing to read.
- **REL-03 (P2, NEW — decision-path adjacent)** — `cmd/assent/provider_host.go:82–86` treats
  **any** `FileAtRef` error on `providers/<name>.json` as "declaration absent" and `continue`s,
  including retry-exhausted 5xx and deterministic 401/403. The typed sentinel
  (`forge.ErrNotFound`) and a correct discriminator both already exist in this very repo —
  `fileAtRefOrAbsent` (`cmd/assent/run.go:465–474`) and, in the *same file* at
  `provider_host.go:281–285`, `loadResourceOwnerRegistry`, which D-130 fixed for exactly this
  conflation. This call site uses neither. The fail-closed *direction* holds (a missing CEL
  attribute → REVIEW), but a forge blip or a token-scope misconfiguration silently converts
  approvable MRs to REVIEW with a misleading `predicate.error`, and a `has()`-tolerant policy
  silently takes its fallback branch — a **wrong decision reached by an invisible path**.
- **TEST-02 (P2, NEW — mutation-verified by the auditor)** — deleting `EffectChallenge` from
  `isStricterInterventionEffect` (`internal/compare/classify_intervention.go:73–75`) leaves
  `./internal/compare/...` **and** the comparison-corpus dogfood green. A regression that stops
  classifying challenge-effect deltas as stricter interventions merges through every wired gate.
- **SEC-03 (P2, NEW)** — `hack/install.sh:114` runs `cosign verify-blob --bundle` with **no**
  `--certificate-identity-regexp` / `--certificate-oidc-issuer`, while `SECURITY.md:80–93` pins
  both in its manual instructions (same era: both citations are `b4f5054`). On cosign v2+ the flags are mandatory (the path errors out —
  so `--require-signature` is a *broken* promise) or, where it succeeds, **any** Fulcio identity
  is accepted and a mirror-swapped archive+bundle pair verifies clean.

---

## Non-goals

- **Do not start the "Later (hygiene)" wave.** TEST-01 (e2e), TEST-03 (per-package floor),
  TEST-04/05/06, SEC-04/05/06/07, RELSE-03/04, ARCH-02/03/05, REL-04/05/06, DOC-02..06 stay in
  the audit report and the backlog. They are named here only so their exclusion is deliberate.
- **Do not implement WG-S01 / D-145** (the `writes:false` runtime gate). It carries the
  **LGTM** governance marker (`openspec/specs/backlog.md`); per GOVERNANCE such stories are
  *surfaced to the maintainer*, never auto-merged, and `/agent-loop-local`'s decide-and-log does
  not override an explicit repo governance rule. It stays its own spec-first lane.
- **Do not start E10 / E11 / P5-DEM / P5-SEC-SC.**
- **Do not change the frozen schemas.** No story here touches `schemas/**`.
- **Do not raise `COVERAGE_MIN`** (91%, D-010/D-128); do not add a second copy of the number.
- **Do not change `MaxResponseBytes`** (8 MiB). S01 *reuses* the existing bound; inventing a
  second exec-specific limit would create the very duplication D-128 forbids for coverage.
- **Do not retry, re-order, or otherwise change forge call semantics** in S02. The story is a
  discrimination fix at one call site, not a transport change.
- **Do not weaken any existing assertion.** TEST-02's fix adds a case; it removes none.
- **Supersedes an unlanded draft:** a 2026-08-10-keyed `AUD2` decomposition exists only in the
  local stash `leave-aud2-not-this-epic` and was never committed. This spec is keyed to the
  **2026-08-18** audit and is the authoritative AUD2. The 08-10 draft's still-open items
  (F3 → RELSE-08 in-tree half, F5 → P5-DEM `Verify:` backfill, F7 → TEST-03 per-package floor)
  are **not** claimed here; they remain Later-wave/backlog items.

---

## ADRs and reuse

**ADRs:** 0004 (provider host boundary), 0011 (module boundaries / purity), 0015 §7 (provider
credentials — untouched, see OQ-32/D-147), 0020 (fail-closed enumeration).

**Decisions:** **D-130** (forge-error vs absent-file conflation, fixed once already for the
resource-owner registry — S02 is the *same* fix at the *sibling* call site), **D-128** (one
source per gate number), **D-110** (sigstore bundles beside archives), **D-124** (a gate
invoked by nothing is not a gate).

**Reuse, explicitly — none of these are to be re-derived:**
- `readBounded` + `MaxResponseBytes` (`internal/provider/transport.go:100–116`) — S01's bound.
- The three HTTP bound tests (`internal/provider/transport_test.go:52–182`: over-limit refused
  and the limit named in the error, exactly-at-limit accepted, bound stays MB-order) — S01
  mirrors their **shape** for exec.
- `forge.ErrNotFound` + `errors.Is` discrimination as written in `loadResourceOwnerRegistry`
  (`cmd/assent/provider_host.go:281–285`) and `fileAtRefOrAbsent` (`cmd/assent/run.go:465–474`)
  — S02 mirrors one of them rather than authoring a third idiom.
- `SECURITY.md:80–98`'s cosign flag pair (the `### Verify a tagged release` block) — S03
  mirrors it rather than starting a second published truth, and a drift gate holds the two
  byte-identical. **Amendment (implementation, AUD2-S03):** the published pair was itself wrong
  when this epic was written, so "mirror it" could not stay literal. The Fulcio SAN carries the
  repository's post-rename casing (`PlatformRelay/Assent` from v0.2.0 on; v0.1.0 signed as
  `assent`) and cosign matches `--certificate-identity-regexp` case-sensitively (Go RE2), so the
  published lowercase value rejected every release from v0.2.0 on — verified with real cosign
  against the real v0.3.0 artifact. Both files now publish
  `--certificate-identity-regexp '^https://github\.com/PlatformRelay/[Aa]ssent/'` (dots escaped:
  they were metacharacters). The "must not invent a pattern" intent stands, restated: the pin is
  **verified against committed real-SAN fixtures** decoded from the published bundles
  (`hack/release/install_cosign_pin_test.sh` §4c/4d), never chosen freely.
- `examples/comparison/promotion-gates/` suite + `cases/` + `records/` layout — S04's corpus entry.

---

## Judgment calls (decide-and-log)

**(a) Exec stdout bound — DECIDED: reuse `MaxResponseBytes`, no new knob.**
An exec provider and an HTTP provider answer the same `FactQuery` and feed the same resolver;
two different containment limits would be an unexplained asymmetry in the opposite direction
from today's. `readBounded` already discards the partial read so nothing can parse a truncated
document. Note the one real behaviour change: `CallExec` currently returns `out.Bytes()`
**alongside** a non-nil error on child failure; with a bounded reader an over-limit read must
return `nil` bytes plus the error, matching HTTP. Callers must be checked, not assumed.

**(b) `WaitDelay` value — DECIDED: `opts.Timeout`, not a new constant.**
The audit prescribes `cmd.WaitDelay = opts.Timeout`. This keeps the single operator-declared
timeout as the one source of the bound (D-128 in spirit) and makes the worst case ~2× timeout,
which is what the story's `Verify:` line measures. A fixed grace constant would be a second,
undeclared number.

**(c) Stderr disposition — DECIDED: bounded capture, folded into the error, never into facts.**
Stderr is captured into its own bounded buffer and appended to the returned error text
(truncated for legibility). It must **not** be concatenated into the fact payload: the resolver
parses stdout as the provider's answer, and mixing streams would let a chatty provider corrupt
a decision input. Both buffers are bounded so REL-01's fix is not reopened through the back door.

**(d) REL-03 failure mode — DECIDED: hard-fail the run, do not synthesize `unavailable`.**
The audit offers both. Hard-fail is chosen because it is what D-130 already chose at the
sibling call site eight lines of context away, and a second, laxer policy for the same class of
error is precisely the "no second, laxer load path" defect this project keeps re-finding.
`resolveRunFacts` already hard-fails on a malformed declaration (`provider_host.go:88–90`), so
failing on an unreadable forge is the *consistent* behaviour, not a new one. A `continue` stays
for, and only for, `errors.Is(err, forge.ErrNotFound)`.

**(e) AUD2's exit gate is PR-visible — DECIDED: a `task check` stage, and update the pin.**
RELSE-08 is on record: the `release-exitgate` **job** carries `if: github.event_name !=
'pull_request'`, so a gate wired only there is invisible to every PR and can redden `main`
after a green merge — which is exactly how AUD-S18's stale `CHECK_STAGES` pin went unseen
across four merges (INBOX 2026-08-16). AUD2's gate therefore becomes a `task check` stage, and
**the same lane** adds it to `CHECK_STAGES` in `hack/audit/exitgate_test.sh` — deliberately, in
one commit, which is what that pin exists to force.

---

## Executability

**Every story `[autonomous]`.** No network, no live forge, no PAT, no live provider, no cosign
keypair from a CA (S03's negative case uses a locally generated keyless-style bundle or a
stubbed `cosign` on `PATH`, whichever the implementer proves reddens). All four fixes are
covered by Go unit tests or shell gate scripts that run offline.

**TDD is not optional here.** Three of the four findings exist *because* a passing suite did not
notice them; a fix whose test cannot fail reproduces the defect at one remove. Each story's
`Definition of done` names the mutation that must redden.

**Dependency order:** **{S01 ∥ S02 ∥ S03 ∥ S04}** — fully parallel, file-disjoint — **→ S05**
(exit gate, requires all four landed). **Do first: any of them.** S01 is the largest.

---

## Story index

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| AUD2-S01 | REL-01/02/07: exec transport trio — bound stdout, `WaitDelay`, capture stderr | **[autonomous]** | none | closes the exec/HTTP containment asymmetry; a wedged provider cannot outlive its deadline |
| AUD2-S02 | ⚠️ REL-03: `ErrNotFound` discrimination on the provider-declaration fetch | **[autonomous · engine-grade]** | none | a forge blip can no longer masquerade as an absent declaration |
| AUD2-S03 | SEC-03: pin cosign signer identity + OIDC issuer in `hack/install.sh` | **[autonomous]** | none | `--require-signature` becomes a real guarantee, not a passing no-op |
| AUD2-S04 | TEST-02: kill the demonstrated `EffectChallenge` mutant (unit + corpus) | **[autonomous]** | none | a wired gate reddens on the mutation the auditor proved survives |
| AUD2-S05 | Exit gate: S01–S04 dispositioned, wired **PR-visibly** into `task check` | **[autonomous]** | S01–S04 | **the AUD2 exit gate**; RELSE-08 blind spot not repeated |

---

## AUD2-S01 — Exec transport trio: bound stdout, `WaitDelay`, capture stderr [autonomous]

**As an** operator running `assent` in CI **I want** an exec provider to be contained in memory,
unable to outlive its declared timeout, and to report why it failed **so that** a misbehaving
provider degrades to a fail-closed `unavailable` decision instead of OOMing or wedging the job
with no diagnostic.

**Goal:** in `internal/provider/transport.go`'s `CallExec` — (1) route child stdout through the
existing `readBounded`/`MaxResponseBytes` semantics so an over-limit response is an error with
`nil` bytes, never a truncated parse; (2) set `cmd.WaitDelay = opts.Timeout` so a grandchild
holding the stdout pipe cannot block `Run()` past the deadline, and classify the resulting
`exec.ErrWaitDelay` as `unavailable`; (3) capture stderr into a second bounded buffer and fold a
truncated excerpt into the returned error.

**Operator input:** none.

**Dependencies:** none.

**Definition of done:** the three exec bound tests mirror the three HTTP ones and each reddens
when the bound is removed; a stub provider that forks a background `sleep 60 &` inheriting
stdout returns within ~2× `opts.Timeout` (and reverting the `WaitDelay` line makes that test
hang/fail); a stub that exits non-zero after writing to stderr produces an error containing that
stderr text (and reverting the stderr wiring reddens it). `MaxResponseBytes` is unchanged and
still appears exactly once as a declared bound.

**Not in scope:** `CallHTTP` (REL-04's status-code check is Later-wave); changing
`MaxResponseBytes`; the resolver's classification table beyond mapping the new wait-delay error
to `unavailable`; provider credentials (OQ-32).

Requirements:

- **REQ-AUD2-S01-01** — Given an exec provider whose stdout exceeds `MaxResponseBytes`, when
  `CallExec` runs, then it returns a non-nil error naming the limit and **nil** bytes (no
  truncated payload reaches the caller).
  - Test: `internal/provider/transport_test.go` (exec over-limit case, mirroring the HTTP one at :52)
  - Verify: `go test ./internal/provider/...`
  - Level: L1
- **REQ-AUD2-S01-02** — Given an exec provider whose stdout is **exactly** `MaxResponseBytes`,
  when `CallExec` runs, then it succeeds and returns exactly that many bytes (off-by-one fence,
  mirroring the HTTP at-limit test at :112).
  - Test: `internal/provider/transport_test.go` (exec at-limit case)
  - Verify: `go test ./internal/provider/...`
  - Level: L1
- **REQ-AUD2-S01-03** — Given the exec bound is removed from `CallExec`, when the suite runs,
  then REQ-AUD2-S01-01's test fails (non-vacuity: the test measures the bound, not the stub).
  - Test: mutation performed and recorded in the story's review evidence
  - Verify: delete the `readBounded` call in `CallExec`; `go test ./internal/provider/...` reddens
  - Level: L1
- **REQ-AUD2-S01-04** — Given an exec provider that forks a background child inheriting stdout
  and exits, when the context deadline elapses, then `CallExec` returns within approximately
  twice `opts.Timeout` rather than blocking until the process tree drains.
  - Test: `internal/provider/transport_test.go` (stub script forking `sleep 60 &`)
  - Verify: `go test ./internal/provider/... -run WaitDelay -timeout 60s`
  - Level: L1
- **REQ-AUD2-S01-05** — Given the `WaitDelay` assignment is reverted, when REQ-AUD2-S01-04's
  test runs, then it fails (by timeout or by an explicit elapsed-time assertion) — the test
  must not pass on a build without the fix.
  - Test: mutation performed and recorded in the story's review evidence
  - Verify: remove `cmd.WaitDelay`; `go test ./internal/provider/... -run WaitDelay -timeout 60s` reddens
  - Level: L1
- **REQ-AUD2-S01-06** — Given an exec provider that writes a distinctive line to stderr and
  exits non-zero, when `CallExec` runs, then the returned error contains that line (bounded and
  truncated), and the returned bytes do **not** contain it (streams are not merged).
  - Test: `internal/provider/transport_test.go` (stderr diagnostic case)
  - Verify: `go test ./internal/provider/...`
  - Level: L1
- **REQ-AUD2-S01-07** — Given a wait-delay-killed or over-limit exec provider, when facts are
  resolved, then the outcome is classified `unavailable` (fail-closed), never a silent success
  or a partially parsed fact.
  - Test: `internal/provider` resolver-facing test asserting the classification
  - Verify: `go test ./internal/provider/...`
  - Level: L1

---

## AUD2-S02 — REL-03: `ErrNotFound` discrimination on the provider-declaration fetch [autonomous · engine-grade]

**As a** reviewer whose MR outcome depends on provider facts **I want** an unreachable or
unauthorized forge to be reported as a failure **so that** a transient 5xx or a mis-scoped token
cannot silently masquerade as "this provider declares nothing" and quietly change my decision.

**Goal:** at `cmd/assent/provider_host.go:82–86`, `continue` **only** when
`errors.Is(err, forge.ErrNotFound)`; every other `FileAtRef` error returns a wrapped error
naming the provider, the declaration path, and the ref — the identical shape
`loadResourceOwnerRegistry` already uses at `provider_host.go:281–285` (D-130). No new
discriminator idiom is authored.

**Operator input:** none. D-130 already ruled this class; this is its sibling call site.

> **Symbol-name correction (2026-08-18).** This section originally named the function
> `loadProviderHosts`, lifted from the audit's REL-03 prose. **No such symbol exists in the
> tree** — `grep -rn loadProviderHosts` returns zero hits. The site described (same file, same
> line range, the bare `continue` on `providers/<name>.json`) is the `declPath` fetch inside
> **`resolveRunFacts`** (`cmd/assent/provider_host.go:82`), and that is what the REQs below
> mean. An audit's prose is evidence, not authority: `grep`-confirm every symbol a spec asserts
> before it lands.

**Dependencies:** none. **This is a decision-path change** — the reviewer must treat it as
engine-grade: it converts a class of silent REVIEWs into loud failures.

**Definition of done:** a fake forge port returning `forge.ErrNotFound` for
`providers/<name>.json` still skips that provider and the run proceeds exactly as today
(byte-identical decision for the existing fixtures); the same fake returning a generic
`errors.New("503 service unavailable")` — and separately a 401-shaped error — makes
`resolveRunFacts` return an error rather than continue; reverting the `errors.Is` guard back
to a bare `continue` reddens both negative tests.

**Not in scope:** retries or backoff (AUD-S11 already owns idempotent-GET retry); changing
`forge.ErrNotFound`'s definition or any adapter's wrapping; the `unavailable` fact-value path
(that is S01's classification, a different layer); `Config.Classes`/`match.paths` inertness
(the separate 2026-08-16 INBOX finding — do **not** fold it in).

Requirements:

- **REQ-AUD2-S02-01** — Given a forge port that returns `forge.ErrNotFound` for a provider
  declaration, when `resolveRunFacts` runs, then that provider is skipped and no error is
  returned (absence stays absence — today's behaviour preserved).
  - Test: `cmd/assent/provider_host_test.go` (fake port, not-found case)
  - Verify: `go test ./cmd/assent/...`
  - Level: L1
- **REQ-AUD2-S02-02** — Given a forge port that returns a non-`ErrNotFound` error (5xx-shaped)
  for a provider declaration, when `resolveRunFacts` runs, then it returns a non-nil error
  naming the provider, the declaration path, and the ref.
  - Test: `cmd/assent/provider_host_test.go` (fake port, transport-error case)
  - Verify: `go test ./cmd/assent/...`
  - Level: L1
- **REQ-AUD2-S02-03** — Given a forge port that returns an authorization-shaped error (401/403)
  for a provider declaration, when `resolveRunFacts` runs, then it returns a non-nil error —
  a token-scope misconfiguration is never read as an absent file.
  - Test: `cmd/assent/provider_host_test.go` (fake port, unauthorized case)
  - Verify: `go test ./cmd/assent/...`
  - Level: L1
- **REQ-AUD2-S02-04** — Given the `errors.Is(err, forge.ErrNotFound)` guard is reverted to an
  unconditional `continue`, when the suite runs, then REQ-AUD2-S02-02 and -03 both fail
  (non-vacuity: the tests measure the discrimination, not the fake).
  - Test: mutation performed and recorded in the story's review evidence
  - Verify: revert the guard; `go test ./cmd/assent/...` reddens on both cases
  - Level: L1
- **REQ-AUD2-S02-05** — Given the shipped example fixtures, when the full gate runs, then every
  existing decision outcome is unchanged (this story alters behaviour only on the error paths
  that previously could not be observed).
  - Test: existing `cmd/assent` corpus + `task dogfood-examples`
  - Verify: `task check`
  - Level: L2

---

## AUD2-S03 — SEC-03: pin cosign signer identity and OIDC issuer in `hack/install.sh` [autonomous]

**As an** adopter running `install.sh --require-signature` **I want** the signature check to
accept only signatures produced by this project's own release workflow **so that** a
mirror-swapped archive shipped with its own validly signed bundle is rejected rather than
installed.

**Goal:** `hack/install.sh:114`'s `cosign verify-blob --bundle` gains
`--certificate-oidc-issuer https://token.actions.githubusercontent.com` and
`--certificate-identity-regexp '^https://github\.com/PlatformRelay/[Aa]ssent/'` — published
identically in `SECURITY.md:80–93` — plus a gate that fails if the script and `SECURITY.md`
ever drift apart. (Both citations are `b4f5054` line numbers, as in Problem above; on `main`
today the call is `hack/install.sh:127` and the published block is `SECURITY.md:80–97`, grown by
the casing explanation D-153 added.)

**Amendment (implementation):** this Goal originally quoted
`'^https://github.com/PlatformRelay/assent/'`, the value `SECURITY.md` published at the time, on
the assumption that copying it was sufficient. It was not: the repository was renamed
`PlatformRelay/assent` → `PlatformRelay/Assent` between v0.1.0 and v0.2.0, the keyless signing
certificate's SAN carries GitHub's canonical casing, and cosign matches the identity regexp
case-sensitively (Go RE2). The lowercase value therefore **rejected the project's own v0.2.0 and
v0.3.0 artifacts** — `--require-signature` would have failed closed on genuine releases, and
`SECURITY.md`'s copy-paste instructions were broken for adopters the same way (see D-153). The
shipped value widens the casing and escapes the dots, and nothing else: the `^` anchor and the
owner/repo scope are unchanged, so another owner, an `assent-mirror` typosquat, another forge and
an unescaped-dot host (`githubXcom`) all still fail.

**Operator input:** none. The issuer and identity pattern are published, not chosen — but
"published" is not "correct": the pin is checked against the real Subject Alternative Names
decoded from the v0.1.0/v0.2.0/v0.3.0 bundles, committed as offline fixtures.

**Dependencies:** none.

**Definition of done:** the script's cosign invocation carries both flags; a bundle signed by
any other identity fails verification (proved with a stubbed `cosign` on `PATH` that asserts on
the flags it receives, or a locally generated mismatching bundle — whichever the implementer
demonstrates actually reddens); deleting either flag from the script reddens the drift gate; and
(amendment) the pinned regexp accepts every **real** published release identity, proved against
committed SAN fixtures rather than self-made ones.

**Not in scope:** changing the release workflow's signing identity; the checksum-manifest
verification path in `SECURITY.md`; SEC-05 (PAT rotation, operator-only); adding cosign to any
CI job that does not already have it.

Requirements:

- **REQ-AUD2-S03-01** — Given `hack/install.sh` verifies a sigstore bundle, when the cosign
  command is constructed, then it includes both `--certificate-oidc-issuer` and
  `--certificate-identity-regexp` with the values published in `SECURITY.md`.
  - Test: `hack/release/install_cosign_pin_test.sh` (new)
  - Verify: `bash hack/release/install_cosign_pin_test.sh`
  - Level: L1
- **REQ-AUD2-S03-02** — Given a bundle whose certificate identity does not match the pinned
  regexp, when `install.sh` runs with `--require-signature`, then installation fails and no
  binary is written to the destination.
  - Test: `hack/release/install_cosign_pin_test.sh` (stub `cosign` rejecting on identity mismatch)
  - Verify: `bash hack/release/install_cosign_pin_test.sh`
  - Level: L1
- **REQ-AUD2-S03-03** — Given either pinning flag is deleted from `hack/install.sh`, when the
  gate runs, then it exits non-zero (non-vacuity).
  - Test: `hack/release/install_cosign_pin_test.sh` (mutation on a temp copy of the script)
  - Verify: `bash hack/release/install_cosign_pin_test.sh`
  - Level: L1
- **REQ-AUD2-S03-04** — Given `SECURITY.md`'s published issuer or identity regexp is changed
  without the script following, when the gate runs, then it exits non-zero (drift gate — one
  published truth, D-128 in spirit).
  - Test: `hack/release/install_cosign_pin_test.sh` (extract both, compare)
  - Verify: `bash hack/release/install_cosign_pin_test.sh`
  - Level: L1
- **REQ-AUD2-S03-05** — Given the new gate script exists, when it is removed from its invoking
  task/CI stage, then a pin fails (D-124: a gate invoked by nothing is not a gate).
  - Test: the AUD2 exit gate (S05) or an existing wiring pin
  - Verify: `task check`
  - Level: L1
- **REQ-AUD2-S03-06** *(added by AUD2-S05's spec-hygiene pass)* — Given the certificate Subject
  Alternative Names decoded from the **real** published v0.1.0/v0.2.0/v0.3.0 bundles, committed
  as offline fixtures, when the gate runs, then every one of them is accepted by the pinned
  regexp — as a bash-ERE match **and** end-to-end through `install.sh` with the stub `cosign` —
  and a table of near-miss identities (other owner, `assent-mirror` typosquat, another forge,
  lost anchor, unescaped-dot host) is rejected by both; and given the pre-fix lowercase-only pin,
  the same real-SAN table must show at least two REJECTIONS, so the section is proved capable of
  catching the D-153 casing defect rather than being decorative. Emptying either fixture table
  fails the gate.
  - Test: `hack/release/install_cosign_pin_test.sh` §4c/§4d (real-SAN fixtures + their own
    mutation control)
  - Verify: `bash hack/release/install_cosign_pin_test.sh`
  - Level: L1
  - Why this is a REQ and not a Definition-of-done bullet: it was only a DoD line, so nothing
    with a `Test:`/`Verify:` annotation pinned §4c/§4d against deletion — while D-153 itself
    calls that fixture "the assertion that would have caught the defect". A pin the spec does
    not name is a pin the next lane can delete without contradicting the spec.

---

## AUD2-S04 — TEST-02: kill the demonstrated `EffectChallenge` mutant [autonomous]

**As a** maintainer relying on `assent compare` to grade a policy change **I want** the
challenge effect's stricter-intervention classification to be defended by a named test **so
that** dropping it from `isStricterInterventionEffect` cannot merge through every wired gate —
which the 2026-08-18 auditor demonstrated it currently can.

**Goal:** one unit case in `internal/compare` — baseline APPROVE, candidate adds an
`EffectChallenge` finding → the delta is classified a **stricter intervention** — plus one
comparison-corpus entry under `examples/comparison/promotion-gates/` (suite + `cases/` bundle +
`records/`, following the existing shape) so the dogfood tier covers it too.

**Operator input:** none.

**Dependencies:** none.

**Definition of done:** removing `|| e == aggregate.EffectChallenge` from
`isStricterInterventionEffect` (`internal/compare/classify_intervention.go:73–75`) reddens
`go test ./internal/compare/...` **and** `task dogfood-comparison`. The mutation is actually
performed and the red output recorded in the story's review evidence — the audit's claim is that
this mutation *survives*, so an unexecuted assertion that it now dies proves nothing.

**Not in scope:** `isMissedInterventionEffect` (a separate predicate — note it deliberately
excludes `EffectChallenge`; do not "fix" it); adding effects to `aggregate`; the compare exit
gate's own structure; the format-token truncation residual (2026-08-16 INBOX).

Requirements:

- **REQ-AUD2-S04-01** — Given a baseline decision with no intervention and a candidate that
  adds a finding with `EffectChallenge`, when the comparison classifies the delta, then it is
  reported as a stricter intervention.
  - Test: `internal/compare/classify_intervention_test.go` (new case)
  - Verify: `go test ./internal/compare/...`
  - Level: L1
- **REQ-AUD2-S04-02** — Given `EffectChallenge` is removed from `isStricterInterventionEffect`,
  when the unit suite runs, then REQ-AUD2-S04-01's test fails (the mutant is killed).
  - Test: mutation performed and the red output recorded in the story's review evidence
  - Verify: delete the `EffectChallenge` term; `go test ./internal/compare/...` reddens
  - Level: L1
- **REQ-AUD2-S04-03** — Given a corpus case exercising the same challenge-effect delta, when
  the comparison dogfood runs, then it passes against its recorded expectation.
  - Test: `examples/comparison/promotion-gates/` (new case + record), driven by `examples/comparison/validate_test.go`
  - Verify: `task dogfood-comparison`
  - Level: L2
- **REQ-AUD2-S04-04** — Given the same mutation, when the comparison dogfood runs, then it
  fails (the corpus tier is not vacuous either).
  - Test: mutation performed and recorded in the story's review evidence
  - Verify: delete the `EffectChallenge` term; `task dogfood-comparison` reddens
  - Level: L2

---

## AUD2-S05 — Exit gate: S01–S04 dispositioned, wired PR-visibly [autonomous]

**As a** maintainer **I want** a single gate that fails if any AUD2 remediation is silently
reverted, running on **pull requests** as well as on `main` **so that** the RELSE-08 blind spot
that hid AUD-S18's stale pin for four merges is not reproduced by this epic's own gate.

**Goal:** `hack/audit/aud2_exitgate_test.sh` asserting, per finding: the exec bound, `WaitDelay`
and stderr wiring are present in `CallExec`; the `errors.Is(..., forge.ErrNotFound)` guard is
present at the provider-declaration call site; both cosign pin flags are present in
`hack/install.sh` and agree with `SECURITY.md`; `EffectChallenge` is present in
`isStricterInterventionEffect` **and** the named unit case exists. Wire it as a `task check`
stage **and** add that stage to `CHECK_STAGES` in `hack/audit/exitgate_test.sh` in the same
commit (judgment call (e)) — the AUD-S18 pin exists precisely to force this to be deliberate.

**Operator input:** none.

**Dependencies:** S01, S02, S03, S04 all landed on `main`.

**Definition of done:** `task check` runs the new stage; AUD-S18's stage-count assertion is
green with the new stage pinned; reverting **any one** of the four fixes reddens the gate with a
message naming the finding ID; and the gate itself is listed in `CHECK_STAGES`, so deleting the
stage from the Taskfile reddens `task audit-exitgate-test`.

**Not in scope:** making `release-exitgate` a required PR check (AUD-RELSE-08's operator half —
live branch-protection settings, unreachable from the tree); re-grading AUD-S18's own
assertions; the Later wave.

Requirements:

- **REQ-AUD2-S05-01** — Given all four remediations are present on `main`, when the AUD2 exit
  gate runs, then it exits 0 and names each of REL-01/02/07, REL-03, SEC-03, TEST-02 as
  dispositioned.
  - Test: `hack/audit/aud2_exitgate_test.sh`
  - Verify: `bash hack/audit/aud2_exitgate_test.sh`
  - Level: L1
- **REQ-AUD2-S05-02** — Given any single remediation is reverted, when the gate runs, then it
  exits non-zero with a message naming that finding ID (four separate mutations, each proved).
  - Test: `hack/audit/aud2_exitgate_test.sh` (mutations on temp copies)
  - Verify: `bash hack/audit/aud2_exitgate_test.sh`
  - Level: L1
- **REQ-AUD2-S05-03** — Given the gate is wired as a `task check` stage, when `task
  audit-exitgate-test` runs, then `CHECK_STAGES` includes the new stage and the Taskfile/pin
  stage counts agree.
  - Test: `hack/audit/exitgate_test.sh` (existing AUD-S18 stage pin, updated in this lane)
  - Verify: `task audit-exitgate-test`
  - Level: L1
- **REQ-AUD2-S05-04** — Given the new stage is deleted from `Taskfile.yml`'s `check:`, when the
  AUD-S18 gate runs, then it exits non-zero (the gate cannot be silently unwired).
  - Test: `hack/audit/exitgate_test.sh`
  - Verify: `task audit-exitgate-test`
  - Level: L1
- **REQ-AUD2-S05-05** — Given the gate runs on a pull request, when CI executes, then it is
  part of the `verify` composite that PRs require — not only the push-only `release-exitgate`
  job (RELSE-08 not reproduced).
  - Test: `hack/audit/aud2_exitgate_test.sh` (`check_pr_wiring`, both polarities)
  - Verify: `task check`
  - Level: L1
  - **What this REQ does and does not reach (AUD2-S05, stated so the gate does not overstate
    itself).** *Reached in-tree:* the gate is a step of the `verify` job, which has no job-level
    `if:` and therefore executes on every `pull_request`; the gate asserts that placement and
    reddens both when the step is deleted and when the `verify` job grows
    `release-exitgate`'s push-only condition. *Not reached in-tree, and deliberately not
    asserted:* (a) whether `verify` is a **required** status check is a live branch-protection
    setting no file can change — the operator half of RELSE-08, already listed under Not in
    scope; and (b) the `task check` **stage** still does not run on pull requests, because the
    only job that runs `task check` is `release-exitgate` and it keeps
    `if: github.event_name != 'pull_request'`. The gate's PR coverage is the standalone script
    invocation, not the stage.

---

## Paths owned (file-disjoint guidance)

| Story | Owns (write) | Reads only |
| --- | --- | --- |
| S01 | `internal/provider/transport.go`, `internal/provider/transport_test.go` | `internal/provider/*` |
| S02 | `cmd/assent/provider_host.go`, `cmd/assent/provider_host_test.go` | `cmd/assent/run.go`, `internal/forge/*` |
| S03 | `hack/install.sh`, `SECURITY.md` (the published issuer/identity pair only), `hack/release/install_cosign_pin_test.sh` (new), `Taskfile.yml` (its own stage only), `hack/audit/exitgate_test.sh` (its own `CHECK_STAGES` entry only) | — |
| S04 | `internal/compare/classify_intervention_test.go`, `examples/comparison/promotion-gates/**` | `internal/compare/classify_intervention.go` |
| S05 | `hack/audit/aud2_exitgate_test.sh` (new), `hack/audit/exitgate_test.sh` (its own `CHECK_STAGES` entry only), `Taskfile.yml` (its own stage only), `.github/workflows/verify.yaml` (its own `verify`-job step + the stale stage-count comment), this spec (the hygiene amendments noted below the table) | all of the above |

**Integrator-owned, NOT implementer-owned — do not edit these in a lane:**
`CHANGELOG.md` (regenerated with `task changelog-write` **after** the final rebase, because
rebasing rewrites the SHAs the generator reads — a changelog written pre-rebase is stale by
construction) and `openspec/specs/backlog.md`'s AUD2 status column. Both are shared across all
five lanes and have reddened `main` twice in three days when edited per-lane (INBOX
2026-08-16, 2026-08-18/RELSE-01).

S03 and S05 both touch `Taskfile.yml` **and** `hack/audit/exitgate_test.sh`: every lane that adds
a `check:` stage must add its own `CHECK_STAGES` entry *in the same commit*, because the AUD-S18
pin asserts the two lists are equal — that is the whole point of the pin, and a lane that
"stays out of a shared file" instead reddens `main` (the AUD-S18/RELSE-08 incident). Both writes
are therefore correct and unavoidable, and both are scoped to the lane's own one-line entry.
S03 likewise writes `SECURITY.md`, not just reads it: SEC-03's one published truth lives there,
and D-153's re-pin had to correct it. S05 depends on S01–S04 and therefore never runs
concurrently with S03; neither may reorder existing stages.

**S05 also carries the spec-hygiene amendments** surfaced by AUD2-S03's re-review, all inside
this file: `REQ-AUD2-S03-06` (above) promotes the real-SAN fixture assertion from a
Definition-of-done bullet to an annotated requirement; this table now records S03's `SECURITY.md` and
`CHECK_STAGES` writes; and the Problem section's line numbers are now explicitly dated to
`b4f5054` with the `SECURITY.md` range corrected to the same era.

---

## Exit gate (epic)

AUD2 is complete when: (1) all five stories are merged to `main`; (2) `task check` is green
locally **and** the push-triggered `verify` run on the merge-commit SHA is green **including the
`release-exitgate` job** (the job PRs skip — RELSE-08); (3) `bash
hack/audit/aud2_exitgate_test.sh` exits 0 and each of its four revert-mutations is demonstrated
to redden it; (4) the audit's Next wave is recorded closed in
`agent-context/PROJECT-AUDIT-2026-08-18.md`'s follow-up section and in the INBOX, with the
Later wave and WG-S01 explicitly still open.
