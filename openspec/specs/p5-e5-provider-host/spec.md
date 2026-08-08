# P5-E5 — Provider host + builtins

**Epic ID / REQ prefix:** `E5` / `REQ-E5-S0n-nn`.

**Problem**: Controlling facts never reach CEL on the live path. `EvaluationInput.Facts` is
bound fail-safe (`factsToCEL` only injects `value` when `state=="resolved"`), E3 refuses
`failure:open` on controlling providers, and P3 froze the provider wire schemas — but the
**product host is missing**. `internal/provider/` today only negotiates majors
(`HostMajor=1`, D-032). Spike C (`hack/spikes/provider/**`, `docs/planning/spikes/spike-c-provider.md`)
proved `ResolveFacts`, projection minimization, HTTP/exec transports, `ScrubEnv`, and write-token
isolation — none of that is promoted. `assent run` always evaluates with `Facts:{}` and empty
`pins.factsResolvedAt`; `assent test` stubs via `facts.yaml` (ADR-0014 / E6 fence — correct and
must stay). Two highest-MR-weight backlog gaps are blocked on facts: **REF-GAP-1** (`resource→owner`)
and **REF-GAP-2** (in-repo state as a fact — quota/placement/limits/reviewers). Operator INBOX
follow-ups from E3-S05 name the **`sensitive` tier** (F1/F2) as an E5 must-hold.

**Key ground truth (de-risks the epic):**
- Frozen wire schemas already exist: `schemas/provider/v1alpha1/{request,response}.schema.json`
  (promoted byte-for-byte from Spike C in P3-E1). Epic DoD prefers **`git diff schemas/provider/` == 0**
  for the protocol; any Config-schema widening for digest-pin / declarations is a **named judgment
  call** (see (a)), never a silent widen.
- Decision-side fail-safe is already live (E2-S05): non-resolved facts never bind `.value`. The host
  must emit explicit `unavailable|invalid|expired` — never invent `resolved` on failure, never omit
  a requested key.
- E6 stays hermetic: adopter tests continue to stub `facts.yaml`; live resolution must not leak into
  `assent test` (ADR-0014).
- `internal/core` stays I/O-free (`TestCorePurity`); resolution lives at the provider host /
  `cmd/assent` edge only.

**Scope**: (S01) promote Spike host core + negotiation + state-machine goldens; (S02) projection-
minimized query builder + `maxAge` defaults from `docs/planning/provider-contract.md` +
`trusted-full-content` gate; (S03) HTTP + exec transports + `ScrubEnv`/argv hygiene + isolation
harness in CI + digest-pin verify; (S04) **sensitive tier** (15m default, propagate
`Fact.Sensitive`, refuse lengthening maxAge); (S05) wire host into `assent run` (fill Facts +
`factsResolvedAt`; preserve provider-less dry path); (S06) builtin forge-groups (hermetic fake +
optional live); (S07) builtin `repo-file` most-specific-first (**closes REF-GAP-2**, unlocks C5/C6);
(S08) referenced-resource ownership provider (**closes REF-GAP-1**, unlocks C7); (S09) optional v1
builtins polish / ownership-file; (S10) exit gate (isolation CI + state goldens + hermetic resolved
facts on example path + seed REF-EX C5–C7 when ready).

**Non-goals** (fenced): **gRPC / WASM sandbox** (ADR-0004 tier 4 / D-012 post-v1); **renderer
redaction** (E8 consumes the `sensitive` flag — host only propagates it); **live provider calls from
`assent test`** (E6 fence); **REF-GAP-3 companion-file correlation / C8** (known-limitation, not an
E5 close); **OIDC/LDAP** as first-wave builtins (defer until a named consumer — S09 optional);
**widening frozen EvaluationInput / MergePolicy schemas**.

**ADRs**: 0004 (tiered providers), 0011 (`Provider.Resolve` port; facts before eval), 0015 §7
(write-token isolation, digest-pin, timeouts → fail-closed), 0017 §6 (four states; controlling
facts never fail open), 0012/0016 (`sensitive` → redaction handoff), 0014 (tests stub providers).
**Reuse**: Spike C envelopes/classifier/transports/isolation tests; `internal/provider.Negotiate`;
frozen provider schemas; `policy.Provider` + E3 `ValidateProviderPosture`; `aggregate.Fact` /
`factsToCEL`. **New**: product `ResolveFacts` host, transports, builtins, `assent run` wiring.

**Executability**: S01–S05, S07, S08 (in-repo map), S09–S10 **`[autonomous]`** with fakes/fixtures.
S06 live GitLab **`[infra-gated]`** (hermetic fake path is autonomous). Isolation harness is
machine-local (no live cluster). TDD; determinism double-run on host outputs; `TestCorePurity`
untouched for core.

**Judgment calls (decide-and-log / operator)**:
(a) **🟡 DECIDED (default) — digest-pin + declaration fields without silently widening frozen
`config.schema.json` in S01–S03.** Spike's richer `Config` (digest, projections, capabilities) is
ahead of the thin frozen `policy.Provider{Type,URL,Failure}` (`config.schema.json:74-90`). **Default:**
S01–S03 land the host against an **internal declaration** type (loaded beside Config or from a
provider-declaration doc the host owns), keep `git diff schemas/policy/` == 0, and open a follow-up
schema story only if a public Config field is required for adopters. Revisit if an adopter-facing
knob is unavoidable — that becomes a deliberate schema lane, never a silent widen.
(b) **DECIDED — E6 stays stubbed.** Live resolution is `assent run` / assembly only.
(c) **DECIDED — REF-EX C5–C7 authored in S10 (or immediately after S07/S08), never before** — stubbing
them earlier would lie about APPROVE paths.
(d) **DECIDED — S07 before S06 for product value** once S05 exists: REF-GAP-2 unlocks C5/C6 without
live forge. S06 ∥ S07 after S05 when file-disjoint.

**Dependency order**: **S01 → S02 → S03 → S04 → S05 → {S06 ∥ S07} → S08 → S09? → S10**.
**Engine-grade / fail-safety review:** S01, S02, S03 (isolation), S04 (sensitive), S05 (run wiring),
S07/S08 (controlling facts). **Closes REF-GAP-2: S07. Closes REF-GAP-1: S08. Sensitive tier (INBOX
F1/F2): S04.**

---

## E5-S01 — ⚠️ Promote Spike host core: `ResolveFacts` + negotiation + state-machine goldens [autonomous · engine-grade]

> **⚠️ Decision-adjacent fail-safety:** a buggy host that emits `resolved` on timeout/schema-miss/omit,
> or drops a requested key, fail-OPENs controlling facts. Fresh reviewer pointed at the four-state
> classifier and "never absent key / never resolved-on-failure".

**As a** policy runtime **I want** Spike C's `ResolveFacts` classifier + major negotiation promoted into
`internal/provider` **so that** every requested fact is present with an explicit state before CEL runs.

**Goal**: Move (not re-invent) the Spike classifier into product: timeout→`unavailable`;
schema/queryId/omit→`invalid`; `expiresAt<=asOf`→`expired` (value dropped); wire
`Negotiate(HostMajor, providerMajor)` so mismatch ⇒ capability gap ⇒ all requested facts
`unavailable` + `AutoMergeEligible=false`. State-machine goldens per state. No HTTP/exec yet
(stub transport). `git diff schemas/provider/` == 0.

**Dependencies**: P3-E1 provider schemas, P3-E2 negotiation (already in tree), Spike C.

**Definition of done**: goldens for resolved/unavailable/invalid/expired; major mismatch → all
unavailable; no omitted keys; `TestCorePurity` green; schemas untouched.

Requirements:
- **REQ-E5-S01-01** *(ENGINE · fail-safe)* — `ResolveFacts` returns every requested key with an explicit state; never omits; never marks `resolved` on transport/classifier failure. Test: `internal/provider/resolve_test.go`; Verify: `go test ./internal/provider/... -run TestResolveFactsStateMachine`; Level: L0
- **REQ-E5-S01-02** — major mismatch via `Negotiate` ⇒ capability gap + all facts `unavailable`. Test: `internal/provider/resolve_test.go`; Verify: `go test ./internal/provider/... -run TestResolveMajorMismatch`; Level: L0
- **REQ-E5-S01-03** — Spike classifier behaviors (timeout / schema / expired) preserved byte-stable in goldens. Test: `internal/provider/testdata/**`; Verify: `go test ./internal/provider/... -run TestResolveGoldens`; Level: L0

---

## E5-S02 — Projection-minimized `FactQuery` + `maxAge` defaults + `trusted-full-content` gate [autonomous · engine-grade]

**As a** host **I want** queries minimized to declared pointers and freshness defaults applied from
`provider-contract.md` **so that** undeclared change content (incl. secret refs) never enters a
provider and stale facts cannot silently arm.

**Goal**: `BuildQuery` ∩ declared projections; refuse `fullContent` without `trusted-full-content`;
host `maxAge` defaults (principal/authz **1h**, registry **24h**, `sensitive:true` **15m**,
global cap **24h**) are the **validation ceiling** from `provider-contract.md`: a declaration that
**omits** `maxAge` is a **load-time error** (not a silent "no limit" and not a silent fill-in);
a declaration that **exceeds** the type default / sensitive 15m / 24h cap is **rejected at load**
(never clamped). Cross-check the provider's echoed declaration against the host config (Spike C
load-bearing check).

**Dependencies**: E5-S01.

**Definition of done**: undeclared pointers stripped; fullContent without capability refused at
build/load; omit→reject + exceed→reject pinned against `provider-contract.md`; declaration
cross-check golden green.

Requirements:
- **REQ-E5-S02-01** — projection minimization: only declared pointers appear in `FactQuery`. Test: `internal/provider/query_test.go`; Verify: `go test ./internal/provider/... -run TestBuildQueryMinimized`; Level: L0
- **REQ-E5-S02-02** *(fail-safe)* — `fullContent` without `trusted-full-content` refused. Test: `internal/provider/query_test.go`; Verify: `go test ./internal/provider/... -run TestFullContentCapabilityGate`; Level: L0
- **REQ-E5-S02-03** — host maxAge table matches `provider-contract.md`; **omit** `maxAge` → load-time error; **exceed** type default / sensitive 15m / 24h cap → rejected at load (never clamped). Sensitive 15m shared with S04. Test: `internal/provider/maxage_test.go`; Verify: `go test ./internal/provider/... -run 'TestMaxAgeOmitRejected|TestMaxAgeExceedRejected'`; Level: L0
- **REQ-E5-S02-04** — host cross-checks the provider's echoed declaration against config (type/cardinality/subject/sensitive/maxAge); mismatch → `invalid` (never silently accept). Test: `internal/provider/declaration_test.go`; Verify: `go test ./internal/provider/... -run TestDeclarationCrossCheck`; Level: L0

---

## E5-S03 — HTTP + exec transports + isolation harness in CI + digest-pin verify [autonomous · engine-grade]

> **⚠️ Trust-boundary lane (ADR-0015 §7).** Write-token isolation and digest-pin are security surface.
> Reviewer pointed at env scrubbing, argv credential hygiene, and the malicious-exec harness.

**As an** operator **I want** HTTP/exec transports that never inherit the host env and that verify
digest-pinned exec binaries **so that** forge write tokens cannot leak into providers.

**Goal**: Promote `CallHTTP` / `CallExec` + `ScrubEnv` (drop `TOKEN|SECRET` names; never inherit host
env); **argv must not carry credentials** (Spike C residual — real host keeps secrets out of
provider argv, not only env); digest-pin verify before exec (Judgment call (a) — pin via
internal declaration if Config schema stays frozen); promote Spike `maliciousexec` isolation harness
into CI (`task` / verify step).

**Dependencies**: E5-S01, E5-S02.

**Definition of done**: isolation harness green in CI; canaries absent from provider env; unpinned
exec refused; HTTP timeout → `unavailable` (not resolved).

Requirements:
- **REQ-E5-S03-01** *(SECURITY)* — exec/HTTP child env is scrubbed; forge write-token canaries never appear. Test: `internal/provider/isolation_test.go` (+ CI job); Verify: `go test ./internal/provider/... -run TestIsolationNoWriteToken`; Level: L1
- **REQ-E5-S03-02** *(SECURITY)* — exec binary digest mismatch or missing pin → refuse (facts unavailable). Test: `internal/provider/exec_test.go`; Verify: `go test ./internal/provider/... -run TestExecDigestPin`; Level: L0
- **REQ-E5-S03-03** *(SECURITY · argv hygiene)* — provider argv never carries credentials / forge write-token canaries (adversarial canary in argv must not appear in the spawned process args). Test: `internal/provider/isolation_test.go`; Verify: `go test ./internal/provider/... -run TestIsolationNoCredentialInArgv`; Level: L1
- **REQ-E5-S03-04** — transport timeout → `unavailable`, never `resolved`. Test: `internal/provider/transport_test.go`; Verify: `go test ./internal/provider/... -run TestTransportTimeoutUnavailable`; Level: L0

---

## E5-S04 — ⚠️ Sensitive tier: 15m maxAge + propagate `Fact.Sensitive` + refuse lengthening [autonomous · engine-grade]

> **⚠️ Closes operator INBOX F1/F2 from E3-S05.** Sensitive facts must not outlive 15m or lose the
> redaction marker before E8.

**As a** host **I want** `declaration.sensitive:true` to force the 15m maxAge default (never longer)
and to propagate `aggregate.Fact.Sensitive` **so that** E8 can redact and deferred merge cannot arm
on stale sensitive facts.

**Goal**: Honor sensitive declarations; refuse provider/declaration attempts to lengthen maxAge
beyond 15m for sensitive facts; set `Fact.Sensitive` on the envelope bound into CEL; document the
redaction handoff to E8 (no renderer work here).

**Dependencies**: E5-S02 (maxAge table); preferably after S03.

**Definition of done**: sensitive → ≤15m pinned; lengthening refused; `Sensitive` true on bound fact;
non-sensitive unchanged.

Requirements:
- **REQ-E5-S04-01** *(fail-safe · INBOX F1/F2)* — sensitive declaration applies ≤15m maxAge; a longer declared maxAge is **rejected at load** (not clamped) per `provider-contract.md`. Test: `internal/provider/sensitive_test.go`; Verify: `go test ./internal/provider/... -run TestSensitiveMaxAge`; Level: L0
- **REQ-E5-S04-02** — resolved sensitive facts set `Fact.Sensitive=true` for the CEL/E8 handoff. Test: `internal/provider/sensitive_test.go`; Verify: `go test ./internal/provider/... -run TestSensitivePropagates`; Level: L0

---

## E5-S05 — Wire host into `assent run`: Facts + `factsResolvedAt`; keep core pure [autonomous · engine-grade]

**As a** repo operator **I want** `assent run` to resolve configured providers into
`EvaluationInput.Facts` and pin `factsResolvedAt` **so that** live MRs evaluate against real facts,
not an empty map — while provider-less runs stay byte-identical to today.

**Goal**: At the `cmd/assent` edge, resolve declared providers → fill Facts before `Cover*`; populate
`pins.factsResolvedAt`; **nil/empty providers ⇒ today's empty-Facts path** (no behavior change).
Never import provider I/O into `internal/core`. E6 / `assent test` untouched (still `facts.yaml`).

**Dependencies**: E5-S01..S04.

**Definition of done**: hermetic fake provider → Facts reach CEL and affect a fixture decision;
provider-less double-run byte-identical to pre-S05; `TestCorePurity` green; E6 suite unchanged.

Requirements:
- **REQ-E5-S05-01** — `assent run` with a fake provider fills Facts + `factsResolvedAt` and the decision reflects a resolved fact. Test: `cmd/assent/run_provider_test.go`; Verify: `go test ./cmd/assent/... -run TestRunResolvesProviderFacts`; Level: L1
- **REQ-E5-S05-02** *(compat)* — provider-less / no `--config` path remains byte-identical to pre-S05. Test: `cmd/assent/run_provider_test.go`; Verify: `go test ./cmd/assent/... -run TestRunProviderlessUnchanged`; Level: L1
- **REQ-E5-S05-03** — `assent test` still stubs via `facts.yaml` (no live resolve). Test: `cmd/assent/test_provider_fence_test.go`; Verify: `go test ./cmd/assent/... -run TestAssentTestNeverCallsProviderHost`; Level: L1

---

## E5-S06 — Builtin forge-groups (`builtin/gitlab-groups` / forge-groups) [autonomous hermetic · infra-gated live]

**As a** policy author **I want** `facts.author.groups` from the forge **so that** ownership /
group-membership rules evaluate on live MRs.

**Goal**: Implement the builtin named by example configs (`builtin/gitlab-groups`); hermetic fake-
client path for L0/L1; optional live GitLab behind build tag / infra gate. Isolation pass once.

**Dependencies**: E5-S05. ∥ S07 when file-disjoint.

**Definition of done**: hermetic groups fact resolves; live path documented as infra-gated; example
config type string wires.

Requirements:
- **REQ-E5-S06-01** — hermetic fake forge → `author.groups` resolved. Test: `internal/provider/builtin/gitlab_groups_test.go`; Verify: `go test ./internal/provider/... -run TestBuiltinGitlabGroups`; Level: L0
- **REQ-E5-S06-02** — unknown/missing membership → non-resolved state (never empty-resolved allow). Test: same; Verify: `go test ./internal/provider/... -run TestBuiltinGitlabGroupsFailClosed`; Level: L0

---

## E5-S07 — Builtin `repo-file` (most-specific-first) — closes REF-GAP-2 [autonomous · engine-grade]

**As a** policy author **I want** in-repo quota/placement/limits/reviewers files exposed as facts
**so that** C5 (quota-ceiling) and C6 (placement allow-list) can evaluate without a network provider.

**Goal**: `builtin/repo-file` resolves paths most-specific-first from the checkout / declared roots;
missing file → `unavailable` (not resolved-empty); controlling use stays fail-closed.

**Dependencies**: E5-S05. Closes **REF-GAP-2**. Unlocks REF-EX **C5/C6** (authored in S10).

**Definition of done**: fixture tree proves most-specific-first; missing → unavailable; hermetic
decision changes when the fact resolves.

Requirements:
- **REQ-E5-S07-01** *(closes REF-GAP-2)* — most-specific-first path resolution over a fixture tree. Test: `internal/provider/builtin/repo_file_test.go`; Verify: `go test ./internal/provider/... -run TestBuiltinRepoFileMostSpecific`; Level: L0
- **REQ-E5-S07-02** *(fail-safe)* — absent file → `unavailable`, never `resolved` with nil/empty pretending presence. Test: same; Verify: `go test ./internal/provider/... -run TestBuiltinRepoFileAbsentUnavailable`; Level: L0
- **REQ-E5-S07-03** *(SECURITY · D-129, closes OQ-28)* — **filesystem** containment, not only path containment. `RepoFileOpts.FS` must be a symlink-safe root (`(*os.Root).FS()` via `builtin.OpenRepoRoot`); `cmd/assent` injects one for the checkout tree, which is the MR head. Independently, a candidate whose path traverses a symlink at ANY component is refused (`unavailable`, contributor-readable reason naming the candidate) and STOPS the walk-up — it is never skipped to a less-specific file, and in-root symlinks are refused too because a link that stays inside the FS root can still leave the declared `roots` clip. Legitimate in-root resolution is unchanged. Test: `internal/provider/builtin/repo_file_symlink_test.go`, `cmd/assent/run_provider_symlink_test.go`; Verify: `go test ./internal/provider/... -run TestRepoFileSymlink && go test ./cmd/assent/... -run TestRunCheckout`; Level: L0+L1

---

## E5-S08 — Referenced-resource ownership provider — closes REF-GAP-1 [autonomous · engine-grade]

**As a** policy author **I want** a `resource→owner` fact **so that** a list/ACL naming another team's
resource can drive `ownership` / `require-review` (C7).

**Goal**: v1 shape **in-repo ownership map via repo-file or a thin dedicated builtin** (decide-and-log
in-lane if split); HTTP registry deferred unless already needed. Fail-closed on unknown resource.

**Dependencies**: E5-S07 (likely reuses repo-file) or S05+standalone map. Closes **REF-GAP-1**.
Unlocks REF-EX **C7**.

**Definition of done**: known resource → owner principal/group resolved; unknown → unavailable/
invalid (never APPROVE via missing owner); C7 seedable in S10.

Requirements:
- **REQ-E5-S08-01** *(closes REF-GAP-1)* — `resource→owner` resolves for a fixture map. Test: `internal/provider/builtin/resource_owner_test.go`; Verify: `go test ./internal/provider/... -run TestResourceOwnerResolves`; Level: L0
- **REQ-E5-S08-02** *(fail-safe)* — unknown resource does not resolve to an empty owner that would satisfy ownership. Test: same; Verify: `go test ./internal/provider/... -run TestResourceOwnerUnknownFailClosed`; Level: L0
- **REQ-E5-S08-03** *(SECURITY · D-129 / D-130)* — the ownership registry decides who may approve, so it is a decision input: it loads from the **target ref** first, and the checkout is a fallback **only when the registry is genuinely ABSENT at that ref** — `errors.Is(err, forge.ErrNotFound)`, the neutral port sentinel, never `gitlab.ErrNotFound` (ARCH-02 / depguard). Any other forge error (503, 429, 401, a broken proxy) is **not** absence: it yields no client and no registry, so the owner fact never resolves (fail-safe REVIEW). Falling back on any error was a fail-OPEN — a merely-unreachable forge promoted the merge request's own head tree to the who-may-approve authority with nothing surfaced — so "never a shadow" holds only with the absence gate in place (ADR-0015 §1 / GUIDELINES §Safety 2+3). A registry reached through a symlink is refused with an error (no client ⇒ the owner fact never resolves), never a partial load. Test: `internal/provider/builtin/resource_owner_symlink_test.go`, `cmd/assent/provider_host_registry_test.go` (incl. `TestResourceOwnerRegistryTransientForgeErrorNeverFallsBackToCheckout`); Verify: `go test ./internal/provider/... -run TestResourceOwnerRegistry && go test ./cmd/assent/... -run TestResourceOwnerRegistry`; Level: L0+L1

---

## E5-S09 — Optional v1 builtins polish (ownership-file / HTTP generic) [autonomous]

**As a** maintainer **I want** CODEOWNERS-style ownership-file and HTTP-generic polish **so that**
ADR-0004's first-wave list is covered without blocking the exit gate on OIDC/LDAP.

**Goal**: ownership-file builtin and/or HTTP client polish; **defer OIDC/Keycloak/LDAP** until a
named consumer. Skip entirely if S07/S08 already cover the ownership-file need — log the skip.

**Dependencies**: E5-S05. Optional before S10.

**Definition of done**: either shipped with tests, or explicitly deferred in decisions.md with reason.

Requirements:
- **REQ-E5-S09-01** — ownership-file OR documented deferral with decision log. Test: `internal/provider/builtin/ownership_file_test.go` (if shipped); Verify: `go test ./internal/provider/... -run TestOwnershipFile` OR decision row; Level: L0

---

## E5-S10 — Exit gate: isolation CI + state goldens + hermetic resolved facts + seed C5–C7 [autonomous · engine-grade]

**As a** maintainer **I want** the provider host proven in CI — isolation, four states, and at least
one hermetic example path evaluating with **resolved** (not only stubbed) facts — **so that** E5 is
trustworthy and REF-EX C5–C7 can be authored honestly.

**Goal**: (1) isolation harness required in verify; (2) state-machine goldens green; (3) controlling
`failure:open` still refused (E3 lint regression); (4) hermetic run/assembly path with resolved
facts on a fixture pack; (5) seed REF-EX C5–C7 archetype fixtures once S07/S08 facts exist
(Judgment call (c)); (6) `git diff schemas/provider/` == 0 unless a logged schema story landed.

**Dependencies**: E5-S01..S08 (S09 optional).

**Definition of done**: exit-gate tests green; C5–C7 seeded or explicitly parked with reason; Spike C
isolation evidence promoted; backlog REF-GAP-1/2 marked closed.

Requirements:
- **REQ-E5-S10-01** — isolation + state-machine goldens enforced in `task check` / verify. Test: CI + `internal/provider/...`; Verify: `task check`; Level: L1
- **REQ-E5-S10-02** — hermetic path evaluates with resolved provider facts (not empty map). Test: `cmd/assent/run_provider_test.go` / examples fixture; Verify: `go test ./cmd/assent/... -run TestE5ExitGateResolvedFacts`; Level: L1
- **REQ-E5-S10-03** — REF-EX C5–C7 seeded OR decide-and-log deferral with concrete blocker. Test: `examples/archetypes/**` (if seeded); Verify: pack/fixture tests OR decision row; Level: L1
- **REQ-E5-S10-04** — controlling `failure:open` still refused (E3 lint regression; later-phases exit gate). Test: `internal/lint/...` / posture; Verify: `go test ./internal/lint/... -run 'TestProviderPosture|TestFailOpen'`; Level: L1
