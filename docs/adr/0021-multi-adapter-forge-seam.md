# ADR-0021: Multi-adapter forge seam — `forge.RunPort`, neutral capabilities, transport policy

| | |
| --- | --- |
| **Status** | Proposed |
| **Date** | 2026-08-10 |
| **Deciders** | Operator (maintainer LGTM required — core-contract work per GOVERNANCE) |
| **Context links** | ADR-0005 (forge abstraction, GitLab first / GitHub second), ADR-0011 (core ports), ADR-0015 §2/§4, ADR-0017 §1/§7, ADR-0019, ADR-0020; D-012/D-017/D-019 (E10 lock), D-140 (E10 unlock); `docs/planning/forge-dossier-github.md`; `docs/planning/design-notes/e10-forge-port-lift.md`; audit 2026-08-09 ARCH-18/ARCH-19; spec `openspec/specs/p5-e10-github-forge/spec.md` |

## Context

E10 (GitHub adapter) is unlocked by D-140. assent has exactly one forge adapter, and the
seam a second adapter must plug into is only half-built:

1. **The composite port does not exist.** AUD-S15 lifted `MRInfo` and `ErrNotFound` into
   `internal/forge` (`port.go`), but `cmd/assent`'s `forgePort` is still an anonymous
   interface literal declared at the call site, and `run.go` still calls
   `gitlab.SyntheticDigest` directly. `port.go`'s own scope note records both as E10 work.
2. **The conformance suite cannot be reused.** All 1,155 lines of
   `internal/forge/conformance` live in `_test.go` files, which Go cannot import. The suite
   that defines "behaves like a forge" is therefore unrunnable by a second adapter — the
   GitHub adapter would be developed against no executable contract.
   *(Correction, 2026-08-10 adversarial review: an earlier draft of this ADR also claimed the
   extraction would unblock `catalog.yaml`'s `github-deferred` rows. **That was false** —
   both such rows are `level: L3, package: test/e2e`, so they are gated on live GitHub
   infrastructure (E10-S18), not on importability. The extraction's real and sufficient
   justification is the executable contract.)*
   The deeper problem the same review exposed: the cases assert on `*fake.Forge` internals
   (`sha_guard_test.go:49` takes `*fake.Forge`; `reconciliation_test.go:220` type-asserts to
   it), so a `Factory` returning a `forge.RunPort` is only **half** the contract — the other
   half is a port-level **observation surface** defining what a case is allowed to assert.
   Without it, the cheap way to make cases run on both forges is to weaken assertions to
   what both can observe, which is how a suite silently stops proving the SHA-guard.
3. **Capability vocabulary is adapter-private** (audit bucket A). `docs/planning/forge-dossier-github.md`
   §4 enumerates eleven capability flags the port needs; `probeCapabilities` reads three
   project fields, and `capabilityGap` is computed in GitLab terms. Arming decisions
   (ADR-0015 §4) hang off that vocabulary, so a second adapter would either restate it or
   silently arm under a different meaning of "capable".
4. **Transport policy is adapter-private** (audit bucket B). Bounded reads and pagination
   caps (AUD-S10), idempotent-GET retry/backoff and context deadlines (AUD-S11) were built
   into `internal/forge/gitlab`. GitHub additionally needs a **GraphQL** client (thread
   resolution is GraphQL-only per dossier §4) and **two auth shapes** (PAT and GitHub App
   installation token). Left at the adapter, the two forges' availability and fail-closed
   behaviour diverge with nothing detecting it.

The design note `e10-forge-port-lift.md` covers (1) only. Items (3) and (4) are the "two
design buckets" the 2026-08-09 audit flagged as under-scoped; item (2) it flagged
separately.

**An adversarial review of the first draft of this ADR (2026-08-10) found three further
buckets, two of which are P0.** They are recorded here because each is an *addressing or
representation* failure — a class the P1-E3-S03 dossier structurally could not surface, since
it studied GitHub's API **endpoints**, not how the port **names** things:

5. **Head-content addressing — the port cannot read a GitHub fork PR's head (P0).**
   `cmd/assent/run.go:270,274` reads the governed subject's base and head via
   `FileAtRef(project, path, ref)` with `info.TargetBranch` / `info.SourceBranch` — a **branch
   name inside one project** — and `forge.MRInfo` carries no source-repository identifier. On
   GitHub, a fork PR's head branch does not exist in the base repo, so the read 404s;
   `fileAtRefOrAbsent` (run.go:465-468) maps `forge.ErrNotFound` to `nil`, and
   `change.OneSidedLifecycle(base, nil)` (`internal/change/onesided.go:20-21`) returns
   `KindDelete, true`. **Every fork PR would be evaluated as a whole-file deletion the
   contributor never made** — a spurious BLOCK, or an APPROVE on fabricated change semantics.
   Freezing `FileAtRef`'s signature at story 2 without deciding this is the single largest
   latent refactor in the epic.
6. **Error taxonomy at the port.** `forge.ErrNotFound` is not a transport code — it is a
   **semantic presence signal** consumed by `fileAtRefOrAbsent` and turned into a `FileEvent`.
   GitHub returns 404 for permission-denied resources too, whereas the GitLab adapter
   separates 401/403 (`ErrUnauthorized`) from 404 at the status-code level. Absent an explicit
   status→sentinel mapping per adapter, "absent" and "forbidden" collapse — the
   absent-means-trusted pattern the 2026-08-09 audit found three times, arriving by a new route.
7. **Identity at the port.** `RunPort` carries no identity; the GitLab adapter smuggles
   `botAuthor` through its constructor. "Which artifacts are mine?" — the basis of marker
   filtering and spoof resistance — becomes a **port** concept once there are two adapters and
   two auth shapes (a PAT's identity is a `User`, an App's is a bot).
8. **Record surface for multi-capability gaps (P0).** `schemas/decision/v1alpha1/decision-record.schema.json`
   defines `$defs.pins` with `additionalProperties: false` and a **single string**
   `capabilityGap`, required *iff* `mergeResultDigest` is `null` (and forbidden otherwise, via
   an `if/then/else`). It models exactly one capability — merge-result pinning — which is why
   it is singular and coupled to that field. An eleven-capability report **has nowhere valid
   to be recorded**, and the epic's `git diff schemas/ == 0` goal forbids widening it. Note
   the ordering irony: this ADR's normative order puts the port before the capability model,
   but the capability model has no representation in the frozen contract — so the bucket that
   had to be decided first is the one nobody enumerated.

All eight must be decided before adapter code, because each one changes what the adapter is
written *against*.

## Options

| Option | Pros | Cons |
| --- | --- | --- |
| **A. Adapter-first** — write the GitHub adapter against the existing implicit seam, refactor after | Fastest first commit; concrete code reveals the real seam | The seam gets defined by two accidents instead of one contract; no executable conformance to TDD against; `cmd/assent` would import a second adapter package, entrenching the ARCH-02 leak the port lift just removed |
| **B. Port-first, capabilities and transport left adapter-private** | Smaller ADR; matches the design note exactly | Reproduces the audit's under-scope verbatim: arming semantics and availability behaviour stay per-adapter, and the fail-closed guarantee becomes per-adapter rather than per-port |
| **C. Port-first with a neutral capability model and port-level transport requirements, conformance extracted to an importable package (chosen)** | One executable contract both adapters are proven against; `capabilityGap` and fail-closed arming mean the same thing on both forges; GitHub's GraphQL/App-auth needs are expressed as adapter-internal freedom under port-level requirements | Largest up-front cost; five stories land before a single GitHub API call; touches core contract, so maintainer LGTM gates it |
| **D. Generalize to a plugin/gRPC forge protocol** | Third-party forges without recompiling | No named consumer (D-012 reasoning applies unchanged); freezes a wire contract for a seam with two known implementations; out of scope for v1 |

## Decision

**Adopt Option C.** Before any GitHub API call, E10 establishes a single forge seam
consisting of four committed pieces:

1. **`forge.RunPort`** — a *named* composite interface in `internal/forge`, replacing
   `cmd/assent`'s anonymous port literal:
   `forge.Forge` + `forge.Snapshotter` + `forge.Resolver` +
   `Describe(project, mr string) (forge.MRInfo, error)` +
   `FileAtRef(project, path, ref string) ([]byte, error)` +
   `FileAtBase(mr, path string) ([]byte, error)` / `FileAtHead(mr, path string) ([]byte, error)`.
   **The two accessors are not redundant and neither replaces the other** — item 5 decides
   which is legal where. `FileAtRef` survives because *policy* is ref-addressed by contract:
   ADR-0015 §1 requires the MergePolicy, RulesetBinding and pack to load from the **target
   ref by name**, which `cmd/assent/run.go:203,211,253` does today and must keep doing.
   `FileAtBase`/`FileAtHead` are the **governed subject's** only legal accessors. An adapter
   that implements `FileAtBase` by delegating to `FileAtRef(project, path, sourceBranch)`
   reintroduces the defect item 5 exists to kill.
   `cmd/assent` depends on `forge.RunPort` **only** — a depguard rule denies
   `cmd/assent` importing `internal/forge/gitlab` **and** `internal/forge/github`, replacing
   the current three-symbol allowlist. The merge-digest *scheme* is adapter-owned:
   `gitlab.SyntheticDigest` call-sites collapse onto `Snapshot.Heads.MergeResultDigest`.

2. **An importable conformance package.** The case bodies move from
   `internal/forge/conformance/*_test.go` into importable Go (`RunSuite(t, Factory)` over a
   `forge.RunPort` factory), leaving thin `_test.go` entry points per adapter. `catalog.yaml`
   remains the index and gains the adapter dimension. A case is the *same* case on both
   forges or it is not a conformance case.

3. **A neutral capability model.** `forge.Capability` is a closed, port-owned enum seeded
   from dossier §4's eleven flags; adapters return a `forge.CapabilityReport` of
   `supported | absent | unknown` per capability with an adapter-supplied reason.
   `capabilityGap` is computed **at the port** from that report, never by an adapter, and
   `unknown` is treated exactly as `absent` for arming (**unprobed is not proof**). This is
   the port-level statement of ADR-0015 §4's "refuses to arm when it cannot verify".

4. **Port-level transport requirements.** Bounded response reads, pagination caps,
   idempotent-GET-only retry with backoff, and context deadlines become *requirements of the
   port* with conformance cases, not properties of one client. Auth shape (PAT vs. GitHub App
   installation token) and protocol (REST vs. GraphQL) stay **adapter-internal freedom** —
   the port never names a transport.

5. **An explicit addressing model, decided before the port is frozen.** The port stops
   addressing **the governed subject** by `(project, branch-name)` and instead exposes the two
   sides of the change relative to the merge request itself — `FileAtBase(mr, path)` /
   `FileAtHead(mr, path)` — leaving each adapter to own how it reaches a fork's head
   (`refs/pull/N/head` on GitHub, source-project ID on GitLab). A conformance case **must**
   prove that a fork MR with an unchanged governed file yields *no* lifecycle event, on both
   adapters. Smuggling `refs/pull/N/head` into `MRInfo.SourceBranch` is explicitly rejected:
   it corrupts a documented field and leaks into rendering.
   **Scope of the narrowing, stated precisely because item 1 keeps both accessors:** it binds
   the governed subject only — `run.go:270,274`, the reads whose 404-maps-to-`nil` feeds
   `change.OneSidedLifecycle` and mints the fabricated whole-file DELETE. The **policy** loads
   at `run.go:203,211,253` are *deliberately* still `FileAtRef(project, path, targetBranch)`:
   they read the protected target ref of the target project, which is exactly the trust
   boundary ADR-0015 §1 draws, and a fork's head must never be able to reach them. Rewriting
   those onto an MR-relative accessor would be a trust-boundary regression, not a cleanup.
   *Consequence accepted:* this is a larger refactor than the design note anticipated and it
   collides with the byte-identical-golden requirement; the goldens are re-proved equal on
   GitLab rather than assumed.

6. **A per-adapter HTTP-status → port-sentinel mapping, with a conformance case per sentinel.**
   `ErrNotFound` means *absent*, never *forbidden*: an adapter that cannot distinguish them
   for a given endpoint must return an error, not absence. A permission failure must never
   render as a deleted file.

7. **Identity is a port concept.** `RunPort` exposes the authenticated identity, and marker
   filtering matches **that identity** — not "any bot". Both auth shapes are covered, with a
   case proving PAT-mode markers are recognised as our own (otherwise assent is blind to its
   own comments and duplicates them forever).

8. **The capability report's record surface is decided here, not in a story.** Given `pins` is
   closed and single-valued, the options are (i) accept a `schemas/decision/**` change and
   drop the `git diff schemas/ == 0` goal, or (ii) scope the multi-capability report to
   `doctor` output and arming-refusal reasons only, never the DecisionRecord. **Option (ii) is
   chosen for v1**, with its cost stated plainly rather than hidden: *a capability gap that
   blocks a merge leaves no trace in the DecisionRecord beyond the existing single
   `capabilityGap` string.* Recording it in the record's open top-level object is rejected —
   a safety-bearing field that no schema validates and no consumer must read is a fail-closed
   guarantee in name only. Revisiting (i) is a `v1alpha2` conversation.

Ordering is normative: (5) and (8) are decided **in this ADR**; (1) and (2) before (3) and
(4); (6) and (7) land with the port; and all of it before the first GitHub API call.

## Consequences

**Easier.** A second adapter is TDD-able against an executable contract on day one. Capability
gaps, and therefore every arming refusal, mean one thing across forges. The audit's
"unprobed mitigations" pattern (SEC-01/SEC-04/RELI-03) gets a structural answer for new
capabilities: unprobed is `unknown`, and `unknown` does not arm. (The two `github-deferred`
catalog rows are **not** unblocked by any of this — they are L3 live-infrastructure proofs,
gated on E10-S18.)

**The `unknown == absent` adoption cliff, stated rather than discovered.** The adversarial
review established, and this ADR accepts, that the rule has teeth in both directions. Two
capabilities plausibly report `unknown` on GitHub forever: **`protected-pipeline-source`** —
ADR-0015 §4 makes protected-config the load-bearing arming prerequisite, and GitHub has no
single readable analogue of `ci_config_path` — and **`eligible-approval-evidence`**, since
the dossier §2 records that no API returns the computed per-PR eligible code owners. Under
`unknown ⇒ never arm`, a GitHub adapter that comments but never gates is a *plausible
shipped outcome*, and every fail-closed test would be green while it happened.

This ADR does **not** resolve that by loosening the rule — loosening it reproduces the SEC-04
pattern exactly, where a heuristic (`strings.Contains(path, "@")`) stood in for verification
and a `pull_request_target` workflow could arm on attacker-controlled config. It resolves it
by requiring, **before the capability enum is frozen**, a written *operationally decidable
predicate* for every flag: what concrete, probeable condition makes it `supported` on each
forge. A tri-state with no decidable membership test is a vocabulary, not a model. Where no
such predicate exists, the honest outcome is that the capability is unavailable on that forge
and the gate cannot be armed there — a product limitation to state in the docs, never to
paper over.

**Harder.** Five stories land before any GitHub behaviour. Every capability the GitLab
adapter currently probes informally must be restated as an explicit report entry, which will
surface capabilities it does not actually probe — that surfacing is the point, but it may
turn GitLab arming paths that pass today into honest capability gaps. Any such change is a
user-visible behaviour change and must be recorded as its own decision row, not absorbed
silently into E10.

**We commit to:** `cmd/assent` never importing a concrete adapter; the conformance suite
being the only definition of forge-correct behaviour; `unknown == absent` for arming.

**Reversible how:** the port is internal (`internal/forge`), not public API — no
`apiVersion` implications and no compatibility window. Reverting means re-inlining the
composite interface at the call site and deleting the capability model; the conformance
extraction would be kept regardless, as it is a pure test-architecture improvement.

## Counterpoints considered

**"Option A is how you actually learn the seam — a port designed against one adapter is a
guess."** The first draft answered this by pointing at the dossier: P1-E3-S03 studied
GitHub's real behaviour without writing adapter code, so the seam is informed by evidence
rather than GitLab-plus-optimism.

**The adversarial review broke that answer, and the correction is kept here rather than
quietly edited away.** The dossier is an *endpoint* study, not an *addressing* study. Every
P0 above — fork-head addressing (5), status→sentinel collapse (6), eleven gaps in a
single-valued field (8) — is a representation failure the dossier structurally could not
surface, because naming and representation are not properties of an API surface. "Informed by
real GitHub semantics" was true of the endpoints and false of the model.

The "cheap to be wrong" claim needed the same correction. It holds for a signature tweak. It
does not hold for (5), which propagates through `run.go`'s `decide`/`mrFrom`/`buildDesired`/
`run_render.go` and collides with the byte-identical-golden requirement — that is not a
refactor inside the epic, it is a substantial part of the epic.

**Why Option C still wins anyway**: the review's findings are an argument for deciding
*more* up front, not less. Each P0 was found by reading the port against the code — exactly
what a seam-first epic forces someone to do — and every one of them would otherwise have been
found by a GitHub adopter, in production, on a fork PR. What changes is not the option but
its price: the addressing and representation model is now decided in this ADR (items 5–8) and
gated by a written design note before story zero, rather than being discovered at S07.

**"The capability model is speculative generality."** It would be, at one adapter. At two it
is the difference between one fail-closed guarantee and two coincidentally similar ones, and
the audit already found three live cases (SEC-01, SEC-04, RELI-03) where an unprobed setting
was cited as a safety argument. `unknown == absent` converts that class of defect from a
per-adapter bug into a port-level impossibility.
