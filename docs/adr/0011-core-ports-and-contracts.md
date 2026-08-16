# ADR-0011: Core Go ports and public contracts (draft shapes)

| | |
| --- | --- |
| **Status** | Accepted (partial: Reconcile + schemas-are-API per ADR-0017 §1/§7; P2-E5) |
| **Date** | 2026-07-21 |
| **Deciders** | Konrad Heimel |
| **Context links** | [ADR-0003](0003-canonical-change-model.md) · [ADR-0004](0004-plugin-architecture.md) · [ADR-0005](0005-forge-abstraction-gitlab-first.md) · [ADR-0007](0007-rule-effects-decision-aggregation.md) · [ADR-0009](0009-execution-modes.md) |

## Context

Contracts before code (meta-plan Phase 3). These sketches exist to *get a feeling* for the
seams and to be attacked in review; exact shapes freeze in Phase 3 with fixtures. Everything
here is draft.

## Data contracts (serialized, versioned — the real public API)

```go
// --- change model (ADR-0003, ADR-0008) ---
type ChangeSet struct {
	Files   []FileEvent // added | deleted | renamed | modified | opaque
	Changes []Change
}
type Change struct {
	File        string   // repo-relative path
	Path        string   // RFC-6901-style pointer within the file
	Kind        Kind     // add | modify | delete
	Old, New    Value    // typed scalars/trees; nil per Kind
	Classes     []string // set by classifier
	Environment string   // set by classifier
}

// --- policy input: everything a predicate may see (pure) ---
type PolicyInput struct {
	Change  *Change           // the matched change (change scope)
	Changes []Change          // all changes in this class slice
	Branch  BranchState       // branch scope only: parsed trees at head SHA (lazy)
	Facts   map[string]any    // provider results, keyed by provider name
	MR      MergeRequestMeta  // author, source/target branch, labels, forge
}

// --- decision (ADR-0007) ---
type Finding struct {
	Rule    string
	Effect  Effect // comment | challenge | block | vouch | score
	Points  int
	Paths   []string
	Message string
}
type Decision struct {
	Outcome   Outcome // APPROVE | REVIEW | BLOCK
	Findings  []Finding
	Score     int
	Threshold int
	Trace     Trace // classes, bindings, per-rule eval — powers `explain`
	Pins      Pins  // head SHA, policy SHA, tool version — replayability (OQ-9)
}
```

## Ports (Go interfaces at the hexagon boundary)

```go
// Format adapters (ADR-0003): one per file type, registry-selected by extension.
type FormatAdapter interface {
	Match(path string) bool
	Parse(data []byte) (Value, error)
}

// Predicate backends (ADR-0002): assert-tree/CEL and rego implement this.
type PredicateBackend interface {
	Compile(rule RuleSpec) (Predicate, error) // compile-once, at policy load
}
type Predicate interface {
	Eval(in PolicyInput) (Result, error) // pure; no I/O possible by construction
}

// Providers (ADR-0004): permissions are just facts; one port, four transports
// (builtin | http | exec | grpc). Resolved BEFORE evaluation; results become
// PolicyInput.Facts — predicates never call out.
type Provider interface {
	Name() string
	Resolve(ctx context.Context, q FactQuery) (map[string]any, error)
}
// FactQuery carries MR author, touched classes/paths — enough for a permission
// service to answer "which groups / which owned entries" in one round-trip.

// Forge (ADR-0005): read side.
type Forge interface {
	MergeRequest(ctx context.Context, ref MRRef) (MergeRequestMeta, error)
	EnsureCheckout(ctx context.Context, ref MRRef, dir string) (BranchInfo, error)
	Threads(ctx context.Context, ref MRRef) ([]Thread, error) // resolution state
}

// Publisher (ADR-0007/0009): write side; one method per effect, plus verdict.
// Dry-run/explain swap in a recorder implementation — core can't tell.
type Publisher interface {
	Comment(ctx context.Context, f Finding) error
	OpenThread(ctx context.Context, f Finding) error // resolvable
	Approve(ctx context.Context, d Decision) error
	Deny(ctx context.Context, d Decision) error
	Merge(ctx context.Context, d Decision) error
}
```

## Invariants

- `internal/core` + `internal/change` import no port implementations, and neither does the
  rest of the pure tree this boundary is designed to cover as it grows (`internal/glob`,
  `internal/lint`, `internal/catalogue`, and later `internal/evaldecode`/`internal/compare`
  as those packages join the decision path — Amendment 3). The enforcement mechanism is two
  machine-checked gates, not review: golangci-lint `depguard` deny-rules (package-level
  imports) and an AST purity walk (`TestCorePurity`, call-level: no `time.Now`,
  `os.Getenv`/`os.Environ`, `math/rand`). Amendment 3 records the package list and the
  extension of both gates to their final scope.
- `Predicate.Eval` is pure: facts pre-resolved, branch state pre-parsed (lazy but memoized),
  no clock, no randomness. This is what makes golden tests and replay trivial.
- Every contract change goes through an openspec change proposal; serialized forms carry
  `apiVersion`.

## Counterpoints considered

- *"Providers as two interfaces (PermissionProvider / FactProvider)."* — Collapsed into one:
  a permission check *is* a fact ("author's groups", "owned entries"). Fewer seams, and the
  per-company reimplementation story is one interface with four transport options.
- *"Let rules call providers on demand."* — Rejected: kills purity, caching, and dry-run
  fidelity; pre-resolution with declared provider deps keeps evaluation a pure function.

## Amendment (2026-07-21, adversarial review F3/F9/F11)

- **SHA-guarded writes (F3):** `Approve(ctx, d Decision, sha Pin)` and
  `Merge(ctx, d Decision, sha Pin)` — adapters must use the forge's compare-and-swap
  (GitLab `merge?sha=`, GitHub merge `sha`); on mismatch they fail closed. Conformance-suite
  case, per ADR-0015 §2.
- **Hermetic pins (F9):** `Pins` additionally records the **full resolved fact set** and
  per-provider resolution timestamps. Replay of a historical decision re-uses pinned facts —
  never re-resolves. `scan` resolves facts at scan time unless a fact snapshot is supplied;
  its report must carry a `facts: live` caveat flag that `stats` surfaces next to any
  backtest percentage.
- **Per-change predicate binding (F11):** a rule's predicate is evaluated **once per matched
  change**, with `old/new/path/kind/file/entry/oldEntry` bound to that change (scope table in
  the ADR-0013 appendix). A `vouch` covers exactly the changes whose predicate returned true;
  false or error leaves that change uncovered (tri-state per ADR-0007 amendment 1). `entry` /
  `oldEntry` (containing entry at head/base) are added to the PolicyInput contract.

## Amendment 2 (2026-07-21, second review P1-4/P1-5/P2-11)

- **Content-keyed facts (P1-4):** `FactQuery` carries the class-sliced ChangeSet (paths,
  old/new values, classes, environment) — not just author+paths — and `config.yaml` provider
  entries may declare **key extractors** (JSON-pointer expressions over changed entries,
  e.g. `extract: { costCenter: "/metadata/costCenter" }`) whose extracted values arrive in
  the query. Without this, any "referenced X must exist" provider is unimplementable.
- **Publisher lifecycle ops (P1-5):** the port gains `UpsertComment` (idempotent, marker-
  keyed), and `SyncThreads` (open new / resolve stale / **reopen or re-post when a resolved
  thread's underlying value changed**) driven by the finding-lifecycle state machine in
  ADR-0012 amendment 2. `Comment`/`OpenThread` alone cannot express idempotent re-runs.
- **Positions (P2-11):** `Change` carries file+line/column spans (ADR-0003 amendment 2) so
  findings can anchor inline threads; `Finding` gains an optional `Anchor`.

## Amendment 3 (2026-08-06, D-123 — boundary enforcement mechanism made true)

The first invariant above claimed "arch-lint enforced" while enforcement was in fact
manual review plus a purity walk covering only part of the pure tree (audit finding
ARCH-01, open across three audits). As of D-123 the invariant reads, and is enforced as:

- `internal/core/**`, `internal/change/**`, `internal/glob`, `internal/lint`,
  `internal/catalogue`, `internal/evaldecode`, `internal/compare`, and `schemas/**`
  import no port implementations (`internal/forge/**`, `internal/render/**`, `cmd/**`)
  and no `net/**` — enforced by golangci-lint `depguard` deny-rules in `.golangci.yml`
  (package-level, fails `task check`/CI verify);
- the same tree contains no `time.Now`, `os.Getenv`/`os.Environ`, or `math/rand`
  call-sites — enforced by the `TestCorePurity` AST walk in
  `internal/core/purity_test.go`, extended beyond its original directories to
  `../evaldecode`, `../compare`, and `../../schemas` (call-level; keeps its adversarial
  self-test proving the guard would fire).

`internal/evaldecode` and `internal/compare` are added to the guarded tree
deliberately: both sit on decision paths (engine input decode; D-116/D-117 compare
gates) and inherit the hard rule that nothing probabilistic, wall-clock- or
randomness-dependent may live there. "arch-lint enforced" elsewhere in this ADR should
be read as "depguard + purity-walk enforced" per this amendment.

## Amendment 4 (2026-08-16, D-144 — Rego/OPA capability boundary for `internal/core/policy`)

E11 (ADR-0002 v2's Rego/OPA complex-rule backend) adopts `github.com/open-policy-agent/opa/rego`
inside `internal/core/policy`. OPA ships an `http.send` Rego builtin (plus its own clock and
randomness use via `time.now_ns`/`rand.intn`), so the package's *dependency closure* reaches
`net/http` even though the file itself imports nothing but `opa/rego`. Neither of Amendment 3's
enforcement mechanisms catches this: `.golangci.yml`'s `pure-tree` depguard denies `net/**` only
over each file's own direct imports, and `internal/core/purity_test.go`'s AST walk flags only
call-sites the file itself writes. A file importing `opa/rego` passes both gates green while
quietly linking the network stack transitively (verified during E11 design, D-141). This
amendment records the operator's resolution of that gap (D-144).

- **What narrows, precisely.** AGENTS.md hard rule 7's own text — no LLM calls, no wall-clock or
  randomness dependence — is unchanged and still call-level-enforced everywhere in the guarded
  tree, `internal/core/policy` included: first-party code in that package still may not call
  `time.Now`, `os.Getenv`, or `math/rand`, and `TestCorePurity` keeps checking it. What narrows is
  **Amendment 3's separate `net/**` link-deny**, and only for `internal/core/policy`'s dependency
  closure: OPA's own use of the clock, randomness, and `http.send` is no longer *absent from the
  package's link graph* (structural, greppable) — it is present but made *uncallable from policy*
  (behavioural, resting on a capability configuration). Read Amendment 3's `net/**` invariant, for
  this one package only, as capability-enforced rather than link-enforced; it stays link-enforced,
  unamended, everywhere else in the guarded tree listed there.
- **Why this package, not an injected boundary.** The evaluator lives in `internal/core/policy`,
  not behind a port interface implemented in `cmd/assent` (rejected option (d2)). Moving it out
  would keep the tree formally OPA-free, but it would place live decision-path evaluation entirely
  outside every rule-7 guard — a weaker guarantee dressed as a stronger one. (d1) keeps the
  evaluator inside the guarded, tested, reviewed tree and states the narrower guarantee honestly
  instead of hiding it behind a boundary that looks stricter and enforces nothing. ((d3), dropping
  OPA outright, was rejected as strictly worse than either.)
- **What compensates, together:**
  1. **Capability sandbox** (E11-S04, REQ-E11-S04-01): the OPA runtime is configured with a
     deny-by-default capability set — `http.send`, `net.*`, `opa.runtime`, `time.*`, `rand.*`, and
     any other I/O builtin are absent from what a compiled module may call; a module referencing
     one fails to **compile**, not at runtime.
  2. **Golden allowlist** (E11-S04, REQ-E11-S04-02): the effective allowed-builtin set is pinned
     against a committed golden file, so an OPA upgrade that introduces new builtins cannot widen
     what policy can call without a deliberate, reviewed diff.
  3. **Transitive purity check** (E11-S04, REQ-E11-S04-03): `internal/core/purity_test.go` and
     `.golangci.yml` gain a `go list -deps`-based assertion over the guarded tree's transitive
     closure — not just direct imports — allowlisting exactly the OPA import path and failing on
     any other dependency that reaches `net`/`net/http`. This closes the non-transitivity gap
     itself, independent of Rego, and is a strict improvement over today's gates regardless of how
     (d) had resolved. **Sequencing note:** because E11-S03 is the story that adds OPA to
     `go.mod`, and both purity gates are non-transitive today, S03 would land green under the old
     gates even though it is the story that effects this narrowing — the transitive check is part
     of S04's guard work but must be in place *before* S03's dependency lands, not after, or S03
     merges the very gap this amendment closes.
  - This amendment is itself the deliverable REQ-E11-S04-04 requires (an ADR amendment plus a
    `D-nnn` row landing before E11-S05); it fulfils that requirement rather than merely describing
    it. Item 3 above is REQ-E11-S04-03, not REQ-E11-S04-04 — the D-144 decision-log row cites
    REQ-E11-S04-04 for the transitive check, which is loose; the spec text names REQ-E11-S04-03
    for that work.
  - Both the sandbox and the transitive purity check are engine-grade, security-relevant changes
    to the decision-path boundary and land under maintainer LGTM, per E11-S04's own tag and
    GOVERNANCE.
- **What this does not authorize.** This amendment is scoped to the OPA/Rego evaluator inside
  `internal/core/policy`. It does not relax rule 7's own text anywhere. It does not relax
  Amendment 3's structural, link-enforced `net/**` guarantee for any other package in the guarded
  tree (`internal/core/**` elsewhere, `internal/change/**`, `internal/glob`, `internal/lint`,
  `internal/catalogue`, `internal/evaldecode`, `internal/compare`, `schemas/**`) — a second
  transitively-networked dependency anywhere in that tree is still a purity-gate failure, not a
  precedent this amendment sets. It does not authorize a wall-clock evaluation timeout as a
  substitute safeguard; the machine-independent evaluation budget that bounds Rego execution is a
  separate requirement (E11-S06) and is not re-litigated here.

See D-144 (`docs/decisions/decisions.md`) for the full evidentiary trail — including the (d1)
vs (d2) vs (d3) tradeoff in full and the supply-chain question it does not settle — and D-141 for
the judgment call this amendment resolves.
