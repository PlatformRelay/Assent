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
2. **The conformance suite cannot be reused.** All ~1,166 lines of
   `internal/forge/conformance` live in `_test.go` files, which Go cannot import. The suite
   that defines "behaves like a forge" is therefore unrunnable by a second adapter — the
   GitHub adapter would be developed against no executable contract, and
   `catalog.yaml`'s `github-deferred` rows could never be flipped by construction.
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
separately. All four must be decided before adapter code, because each one changes what the
adapter is written *against*.

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
   `FileAtRef(project, path, ref string) ([]byte, error)`.
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

Ordering is normative: (1) and (2) before (3) and (4), and all four before the first GitHub
API call.

## Consequences

**Easier.** A second adapter is TDD-able against an executable contract on day one. The
`github-deferred` catalog rows become flippable by running the same suite. `capabilityGap`,
and therefore every arming refusal, means one thing across forges. The audit's
"unprobed mitigations" pattern (SEC-01/SEC-04/RELI-03) gets a structural answer for new
capabilities: unprobed is `unknown`, and `unknown` does not arm.

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
guess."** This is the strongest argument, and it is why the *dossier* exists: P1-E3-S03
already studied GitHub's real behaviour (review lifecycle, GraphQL-only thread resolution,
merge queue as merge-result pin, dismissal restrictions) without writing adapter code, and
§4 explicitly records the port-design consequences. The seam is therefore informed by real
GitHub semantics, not by GitLab plus optimism. The residual risk is real but bounded, and
the mitigation is ordering, not faith: `forge.RunPort` is `internal/`, so if S07–S12 prove a
port assumption wrong, the port changes in the same epic that found the problem — at the
cost of a refactor, never a compatibility break.

**"The capability model is speculative generality."** It would be, at one adapter. At two it
is the difference between one fail-closed guarantee and two coincidentally similar ones, and
the audit already found three live cases (SEC-01, SEC-04, RELI-03) where an unprobed setting
was cited as a safety argument. `unknown == absent` converts that class of defect from a
per-adapter bug into a port-level impossibility.
