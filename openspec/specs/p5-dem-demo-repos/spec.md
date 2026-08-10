# P5-DEM — Public demo repositories + provider extensibility proof

**Epic ID / REQ prefix:** `DEM` / `REQ-DEM-S0n-nn`.

**Origin:** operator instruction (2026-08-10) — *"does there already exist a sample gitlab
repository we can test with as well as a sample github repository? … design two repositories
showing different self service patterns … Everybody should be able to see assent in action
themselves … I want to check how extensible our user resolution is."* Decisions recorded as
**D-142**; the provider-credential gap found while designing is **OQ-32**.

**Answer to the question that opened the epic: no.** Neither a public sample GitLab repo nor
any GitHub presence exists. What exists today, and must not be confused with it:

| Thing | What it actually is | Public demo? |
| --- | --- | --- |
| `examples/repos/{topic-registry,service-catalog,infra-vars}` | in-tree **layouts** — governed content only, no `.assent/` tree | no |
| `examples/packs/{topic-registry,service-catalog,infra-vars}` | in-tree **policy trees** — `assent lint`/`assent test` green under `task check` | no |
| `examples/repos/corpus/{julieops,octodns,kubernetes-org}` | vendored open-source excerpts (OQ-16) | no |
| `gitlab.com/konrad.heimel/assent-lab` | the operator's personal project used for the **D-042** adoption proof | no — private-shaped, not a showcase |
| GitHub | nothing. The adapter itself is spec-only (E10, D-140) | no |

So "everybody can see assent in action themselves" is currently false, and the two epics
unlocked on 2026-08-10 do not make it true: E10 ships a GitHub *adapter*, not a repo to point
it at.

---

## The constraint that shapes the whole epic — and a second one found while designing it

### 🔴 `(class, environment)` routing is not wired, so a multi-class demo repo fails closed in **both** tiers, on **both** forges

Found by reading the code against this design, not assumed. It is the single most consequential
finding of the design session, and it invalidates the assumption every demo repo rests on —
that a repo can showcase several self-service patterns at once.

- **`assent run` (tier 2):** `cmd/assent/run.go:493` `selectBinding` **fails closed on any
  RulesetBinding with more than one binding.** Its own comment: *"Full (class, environment)
  routing needs the Config class-matcher … which is not wired in this lane."*
- **`assent test` (tier 1):** `cmd/assent/test.go:366` `selectBindingForTest` collapses a
  multi-binding document to its **strictest** binding (D-060, fail-safe direction) — but
  **fails closed when two bindings differ in `class`, `packs`, or `require[]`**, because
  collapsing those could flip a decision.

Both demo repos as designed have **three classes with different `require[]` lists**. So as
specified they fail closed on `assent run` *and* on `assent test`. **This is not a GitHub gap
and not a forge gap** — it is a routing gap at the `cmd/assent` seam that no existing example
surfaces.

**Why nobody noticed:** D-060 recorded the shipped packs' dev/prod split as *"empirically
decision-neutral for the corpus (every case identical under dev and prod)"*. That is the tell.
Every shipped example is **single-class** with a decision-neutral environment split, so the
collapse is invisible and the matcher is never missed. The demo is the first artifact that
needs real routing — which is exactly what a demo is for.

**Consequence for scope:** **DEM-S00 wires the matcher**, ahead of everything, blocking every
assembly story. It is not a workaround; it is the feature the demo forces. Note also that the
single clearest demonstration in the epic — **the identical diff that APPROVEs in `dev` and
REVIEWs in `prod`** (DEM-S09-03) — is **currently impossible in either tier**, for the same
reason.

### E10 is spec-only, so GitHub tier 2 does not exist yet

**E10 is spec-only.** `openspec/specs/p5-e10-github-forge/spec.md` S06–S12 are the adapter and
S18 is `[infra-gated · operator]` live proof. **assent cannot run on a GitHub PR today.** A
design that ships a GitHub demo repo implying otherwise would be a lie in the exact class
GUIDELINES calls out ("examples that don't run are lies").

The epic therefore commits to a **two-tier demo contract**, stated on the front page of both
repos rather than buried:

- **Tier 1 — clone and run. Forge-independent. Deliverable once DEM-S00 lands** (today only for
  a single-class repo — see the routing finding above).
  `git clone && assent test . && assent lint .` — real rules, real ChangeSets, real decisions,
  with **no forge account, no token, no network, no IdP**. Facts come from committed
  `facts.yaml` fixtures; the E6 adopter-test harness already delivers everything this needs.
  This tier is the "everybody" claim and it is the epic's primary deliverable.
- **Tier 2 — live MR/PR on a real forge.** GitLab: **available now** (E4 adapter, D-042
  precedent). GitHub: **blocked on E10-S18** — the repo ships tier-1-only and says so.

Tier 1 is not a consolation prize. It is the only tier a reader can exercise in sixty seconds
without credentials, and it is the tier that survives a forge outage, an expired token, or a
reader who does not want to hand a demo an OAuth scope.

---

## The two repositories

Four self-service patterns were named (raw `.tf`, tfvars, Kafka topic/ACL, ArgoCD
Application). They are split by **governance shape, not by tool** — because the shape is what
assent has opinions about, and grouping by tool would produce two repos that demonstrate the
same three archetypes twice.

### Repo 1 — `assent-demo-platform` (GitLab)

**Self-service pattern: *request a platform resource.*** A team opens an MR asking for a Kafka
topic, a Kafka ACL, or an ArgoCD Application.

The distinguishing shape is **referential**: a request names things that live in *other files*.
An ACL names a topic and a principal; an ArgoCD Application names a project, a destination
namespace, and a source repo. **The decision cannot be made by reading the changed file
alone** — which is precisely the class `builtin/resource-owner` (E5-S08, "closes REF-GAP-1")
and `builtin/repo-file` (E5-S07) exist for, and which no shipped example currently exercises
end to end.

| Class | Path glob | Format | Status |
| --- | --- | --- | --- |
| `kafka-topic` | `platform/kafka/<env>/topics/*.yaml` | YAML, one file per topic | extends the existing `topic-registry` shape |
| `kafka-acl` | `platform/kafka/<env>/acls/*.yaml` | YAML, one file per ACL grant | **new shape** |
| `argocd-application` | `platform/argocd/<env>/applications/*.yaml` | YAML, Argo CD `Application` kind | **new shape** |

Archetypes exercised: **ownership**, **allow-listed-fields**, **no-destruction**,
**environment-split**, **freshness** — plus **cross-manifest reference integrity**, which no
existing archetype covers.

Forge: **GitLab**, because the adapter is mature and both tiers work on day one.

### Repo 2 — `assent-demo-terraform` (GitHub)

**Self-service pattern: *change infrastructure.*** A team opens a PR that either instantiates a
blessed module (`.tf`) or tunes per-environment inputs (`.tfvars`).

The distinguishing shape is **magnitude and blast radius**: how much changed, in which
environment, and whether it destroys state-bearing resources. It is also the epic's honest
demonstration of the **opaque-change fallback** — HCL that the fact model cannot structure must
route to REVIEW rather than be silently judged.

| Class | Path glob | Format | Status |
| --- | --- | --- | --- |
| `tf-module-instance` | `stacks/<env>/<stack>/main.tf` | raw HCL | **new shape** |
| `tf-vars` | `stacks/<env>/<stack>/*.tfvars` | tfvars | extends the existing `infra-vars` shape |
| `tf-backend` | `stacks/<env>/backend.tf` | raw HCL | **deliberately ungoverned → always REVIEW** (D-063) |

Archetypes: **bounded-change**, **environment-split**, **no-destruction**,
**allow-listed-fields**, **schema-validity**, plus the **opaque-change fallback** and the
**ungoverned-whole-file → REVIEW** route.

Forge: **GitHub**, for two reasons. Terraform-on-GitHub is the idiom readers expect; and this
repo becomes **E10-S18's live adoption target**, so the epic that needs a real GitHub
repository gets one instead of minting a throwaway. Until S18 lands, repo 2 is tier-1 only.

### Why not one repo

A single repo would collapse the two shapes into one `.assent/config.yaml` and hide the thing
worth showing: that **class + environment + binding** is how an adopter separates governance
regimes, and that two genuinely different regimes look different. Two repos also let the
GitHub one be honestly tier-1-only without degrading the GitLab one.

---

## Provider extensibility — the user-resolution answer

The second half of the operator's question: *how easily can an adopter swap in Entra ID,
Keycloak, or something else?*

### What was verified (not assumed)

The seam is **genuinely well built, and extending it requires no core change and no fork**:

- The wire contract is **frozen and published** —
  `schemas/provider/v1alpha1/request.schema.json` (`FactQuery`) and `response.schema.json`
  (`FactResponse`), `apiVersion: provider.assent.dev/v1alpha1`, `additionalProperties:false`
  throughout, with the fail-closed state machine (`resolved` / `unavailable` / `invalid` /
  `expired`) and the `resolved ⇒ value+expiresAt` implication encoded in the schema itself.
  `API_STABILITY.md:20` binds provider protocol majors to exact negotiation.
- The host classifier produces **exactly one fact per requested output on every path**
  (`ResolveFacts`), so a provider that times out, returns garbage, or omits an output cannot
  produce a silently-absent key.
- Projection minimization means a provider sees **only the JSON Pointers it declared,
  intersected with what the change touched** — never full content without the explicit
  `trusted-full-content` capability.
- Extension is a **two-file, repo-side change on the protected target ref**, verified in
  `cmd/assent/provider_host.go:61,81-88`:
  1. `.assent/config.yaml` → `providers.<name>: {type: http, url: …, failure: closed}`
  2. `.assent/providers/<name>.json` → the host declaration carrying typed `outputs`
  Both are read **from the target ref**, so an MR author cannot alter their own fact semantics.
  **No rebuild, no plugin compile, no fork.**

That is a better answer than "extensible in principle". But four gaps stand between it and
"easily extendable", and this epic states all four rather than demoing around them.

### G1 — no worked example exists, and no document names the path *(P1 — blocks the demo)*

`find examples -type d -name providers` returns **nothing**. All three shipped example packs
declare `providers:` blocks (`builtin/gitlab-groups`, and `type: http` pointing at
`quota.example.com` / `oncall.example.com` / `sizing.example.com`) with **no matching
`.assent/providers/<name>.json`**. On a live run, `provider_host.go:83-87` hits
`FileAtRef` → not found → `continue`: **every one of those providers is silently skipped.**
Grep for `providers/` across `docs/usage/**` and `docs/*.md` returns nothing — the path an
adopter must know is documented nowhere.

So today an adopter copying a shipped example gets a provider block that does nothing, with no
error and no documentation to recover from. **This is the single largest barrier to the
extensibility claim**, and it is cheap to fix. → **DEM-S01**.

### G2 — the transports cannot carry a credential, so no provider can call an IdP directly *(P1 — architectural; OQ-32)*

Verified across three surfaces that agree:

- `CallHTTP` (`internal/provider/transport.go`) sets **only** `Content-Type: application/json`.
  There is no header map, no bearer token, no client certificate.
- The repo-side provider schema (`config.schema.json` `$defs/provider`) is
  `additionalProperties:false` over exactly `{type, url, failure}` — nowhere to put one.
- `ScrubEnv`/`ScrubArgv` build the exec child's environment **from scratch** and refuse any
  name matching `(?i)(TOKEN|SECRET)` **even when explicitly configured** — so the exec tier
  cannot carry one either.

This is not an oversight; it is ADR-0015 §7 working exactly as designed, and Spike C's
`TestIsolation` proves it against a deliberately hostile provider. But it has a consequence
nobody has written down: **Entra ID and Keycloak both require a bearer token on every call, so
neither can be reached by a provider assent invokes.**

The only shape that works today is a **broker**: a small service that holds the IdP credential
*itself* and is reachable by assent without one — loopback/sidecar, or mTLS terminated outside
assent's transport. That is a defensible and arguably correct architecture (the credential
never enters the decision path, and the blast radius of a compromised provider stays bounded),
but it is **undocumented and undecided**, and it narrows what `docs/vision.md:67` promises
("pluggable providers: Keycloak, LDAP, GitLab/GitHub groups…"). `docs/architecture/c4-context.md:19`
is already the only place telling the truth: *"Keycloak / LDAP: no builtin — reachable only via
the generic HTTP/exec provider transport."*

**The demo must model the broker explicitly** rather than implying a direct IdP call, and the
operator ruling on whether to bless the broker pattern, add a narrow credential channel, or
state the limitation is **OQ-32**. DEM-S03 is written to be correct under the broker reading
(the recommended default) and is not blocked by the ruling.

### G3 — exec providers must be re-pinned on every version bump *(P2 — ergonomics, state it)*

`ExecDeclaration.Digest` is required before spawn (D-065 / REQ-E5-S03-02), so a third party
shipping an exec provider forces every adopter to recompute a `sha256:` pin on each release.
Real cost, correct trade-off, **not fixed here** — the docs say so, and the reference provider
is HTTP precisely to avoid it.

### G4 — no builtin exists for OIDC/Keycloak/LDAP, and the epic recommends keeping it that way

ADR-0004 §1 lists "OIDC/Keycloak group lookup, LDAP" among planned builtins; only
`forge-groups`, `repo-file`, and `resource-owner` shipped. **Recommendation: do not add them.**
Each is an unbounded authentication surface (token acquisition, refresh, tenant/realm
discovery, cert pinning) that would land inside the decision path's dependency tree for no
capability the HTTP transport lacks. The extensibility answer is a *published contract plus a
copyable reference implementation*, not a growing builtin catalogue — and it is a stronger
answer, because it works for the adopter whose IdP nobody has heard of.

### The four-layer demo of user resolution

Both repos demonstrate the ladder, each rung runnable:

| Layer | Mechanism | Network? | Tier |
| --- | --- | --- | --- |
| **L0** | `.assent/tests/**/facts.yaml` fixtures | none | 1 — works for every reader |
| **L1** | `builtin/repo-file` + `builtin/resource-owner` — ownership from files in the repo | none | **2 only** — see the fence note below |
| **L2** | `builtin/forge-groups` — the author's forge group membership | forge only | 2 (GitLab now, GitHub with E10) |
| **L3** | `type: http` → **your** IdP broker | broker only | 2, adopter-supplied |

🔴 **L1 does NOT run at tier 1, and the README must not imply it does.** `assent test` is
**fenced from the live provider host by a guard test**: `cmd/assent/test_provider_fence_test.go:69-80`
fails the build if `test.go` imports `internal/provider` or contains `ResolveFacts(`,
`ResolveFactsChecked(`, `CallHTTP(`, `CallExec(` or `resolveRunFacts(` — *"assent test must stay
on facts.yaml stubs (ADR-0014)"*. Tier-1 facts come **solely** from `adoptertest.MapFacts`
(`internal/adoptertest/adoptertest.go:109-119`). So at tier 1 a reader sees ownership resolve
from a **fixture literal**, not from repo files. This is deliberate existing design, not a
defect — but claiming L1 at tier 1 would make the demo's load-bearing claim false, which for a
demo whose whole premise is *"everybody can see assent in action themselves"* is the worst
possible place to over-claim. Any README sentence implying live provider resolution at tier 1 is
a story failure.

L3 ships a runnable reference: `contrib/providers/idp-groups/` — one small Go binary, two
adapter shapes (**Entra ID** `transitiveMemberOf`, **Keycloak** `users/<id>/groups`), both
emitting the identical `FactResponse`. Promoted from `hack/spikes/provider/toy.go`, which
already proves the envelope and the fail-closed states. The demo README's L2→L3 step is
**two files changed, zero lines of assent rebuilt** — which is the operator's question,
answered by something the reader can run rather than by a claim.

---

## Judgment calls (decide-and-log / operator)

**(a) DECIDED — the demo trees live in-tree under `examples/demo/<repo>/` and are mirrored to
the public repositories.** Authoring them only in the public repos would put them outside
`task check`, and an ungated example rots on the first schema or engine change — the exact
failure GUIDELINES names. In-tree means a broken demo reds assent's own build (DEM-S12). Cost:
a mirror step (DEM-S13) and the risk of mirror drift, which DEM-S12 gates by diffing.

**(b) DECIDED — platform→GitLab, terraform→GitHub, and repo 2 is tier-1-only until E10-S18.**
Assignment is a judgment call, not a technical constraint: both trees are forge-independent at
tier 1. The reasoning is idiom (Terraform+GitHub) plus giving E10-S18 a real target. **Repo 2's
README states the pending live tier in its first screenful** — a GitHub repo that looks like it
runs assent on PRs and does not would be worse than no repo.

**(c) DECIDED — no Keycloak/Entra/LDAP builtins; broker-shaped HTTP provider plus a reference
implementation.** Rationale in G4. Reversible: adding a builtin later is additive.

**(d) 🔴 OPERATOR — creating public repositories under the `PlatformRelay` org is outward-facing
and is NOT covered by existing push authorization.** AGENTS.md rule 2 grants push to
`PlatformRelay/assent`; minting new public repos, choosing their names, and seeding demo
branches is a separate act. **DEM-S13 is operator-gated and nothing before it is blocked** —
S01–S12 land in this repo and the trees are complete and green before a public repo exists.

**(e) 🔴 OPERATOR (OQ-32) — is the broker pattern the blessed answer for IdP-backed providers,
or should a narrow credential channel exist?** Options: **(e1)** bless the broker, document it,
amend `docs/vision.md:67` to stop implying direct Keycloak/LDAP calls (**RECOMMENDED** — costs
nothing, keeps ADR-0015 §7 intact); **(e2)** add a repo-side header/secret-ref channel to the
HTTP transport — reopens a frozen schema *and* the trust boundary that Spike C's isolation
proof rests on; **(e3)** say nothing and let adopters discover it. Only (e2) blocks anything,
and it blocks nothing in this epic.

**(f) DECIDED — repo 1's cross-manifest reference rules double as an E11 tier-1 ceiling probe.**
D-141 requires E11-S01 to document, with concrete rules per shape, where CEL runs out —
"cross-manifest" is one of the four named shapes. DEM-S05 writes real cross-manifest rules and
**records whether each is CEL-expressible**, feeding E11-S01 evidence instead of hypotheticals.
DEM-S05 does **not** depend on E11 and must not wait for it: any rule found to exceed tier 1 is
recorded and the demo uses the expressible formulation.

---

## Scope

**Wave A — unblock (the demo is impossible until these land).**
**S00 wire `(class, environment)` binding routing** (without it a multi-class repo fails closed
in both tiers) · S01 provider host-declaration worked examples + docs + gate (closes G1) · S02
provider-author guide publishing the frozen wire contract (incl. G2/G3 constraints) · S03
reference IdP-groups broker provider (Entra + Keycloak adapters).

**Wave B — repo 1, platform resources (GitLab).**
S04 new sample shapes (Kafka ACL, ArgoCD Application) · S05 `kafka-acl` class + cross-manifest
reference rules + tier-1 ceiling record · S06 `argocd-application` class + rules · S07 repo-1
assembly (`.assent/` tree, README, demo branches).

**Wave C — repo 2, terraform (GitHub).**
S08 raw `.tf` sample shape + HCL structuring truth + opaque-change fallback pinned in both
polarities · S09 `tf-module-instance` class + rules · S10 `tf-backend` ungoverned→REVIEW route
(D-063) · S11 repo-2 assembly, tier-1-only README.

**Wave D — make it real.**
S12 demo CI: both trees run under `task check`, mirror drift gated · S13 `[operator]` publish
the repositories · S14 `[infra-gated · operator]` live tier-2 proof on GitLab.

**Non-goals** (fenced): **implementing the GitHub adapter** (E10 — this epic consumes it and
supplies its S18 target, it does not build it); **the Rego backend** (E11 — S05 supplies
evidence, nothing more); **new provider builtins** (judgment call (c)); **widening any frozen
schema** — including the HTTP-transport credential channel, which is OQ-32's (e2) and out of
scope until ruled; **generalizing further private shapes** (D-008/D-019/D-029 shapes 4–5 —
these three new shapes are authored generic, not sanitized from `references/`); **a hosted
playground or web sandbox** (a bigger, different product decision); **`assent init`** (does not
exist; the demo copies a `.assent/` tree, as `docs/usage/walkthrough.md` already says).

**Hard constraint on every authored file, non-negotiable:** AGENTS.md rule 1 / D-002. The
Kafka-ACL, ArgoCD, and Terraform shapes are **generated generic equivalents**, never derived
from `references/**` (gitignored private material). `hack/check-sanitization.sh` must be green,
and no employer, internal system, tenant, realm, cluster, or hostname from those trees may
appear in any form — including as a "renamed" analogue that preserves a recognizable
structure.

**ADRs**: 0004 (plugin architecture — G4 amends its builtin list in practice), 0015 §7
(credential isolation — the constraint behind G2), 0017 §6 (fact states, `trusted-full-content`),
0008 (classification/routing — the three classes per repo), 0003 (canonical change model —
opaque-change fallback), 0014 (adopter-test format — tier 1's whole substrate), 0021 + E10
(repo 2's tier-2 path). **No new ADR is proposed**: this is examples, docs, and a contrib
provider. If OQ-32 resolves to (e2), *that* needs an ADR and it is not this one.

**Executability**: S00–S12 **`[autonomous]`** — hermetic, no forge, no network (the reference
provider is tested against an `httptest` server and the frozen response schema). **S00
additionally `[engine-grade · maintainer LGTM]`**: it changes decision routing at the
`cmd/assent` seam, which is exactly the class `/agent-loop-auto` must surface rather than
auto-merge. S13/S14 **`[operator]`** / **`[infra-gated · operator]`**.

**Dependency order**: **S00** → S01 → S02 → S03 → {S04 → S05 → S06 → S07} ∥ {S08 → S09 → S10 →
S11} → S12 → S13 → S14. **Do first: S00** — until `(class, environment)` routing exists, both
demo repos fail closed in both tiers and every assembly story is unbuildable; S10 also probes
the same seam. **Then S01** — until a working `.assent/providers/<name>.json` exists anywhere in
the repository, every provider claim in both demos is unverifiable, and S02's guide would
document a path with no worked instance behind it.

---

## Wave A — unblock

### DEM-S00 — Wire `(class, environment)` binding routing at the `cmd/assent` seam `[autonomous · engine-grade · maintainer LGTM]`

**As** an adopter whose repo governs more than one kind of resource, **I want** the covering
binding selected by the changed file's class and environment, **so that** a multi-class repo
evaluates at all instead of failing closed.

**Do this first.** Every assembly story (S07, S11) and the environment-split demonstration
(S09-03) are blocked on it. The *matcher* is cheap — `policy.Config` already carries
`Environments []NamedMatch` and `Classes []NamedMatch` with `Match PathMatch`
(`internal/core/policy/policy.go:196-228`), populated in every shipped example pack, and
`internal/glob.Match` (used by `internal/core/classify/matcher.go:31`) already implements
`*`/`**`. So `internal/core` can stay byte-unchanged.

**🔴 But the earlier claim that "the data already reaches these call sites" was FALSE, and the
correction changes this story's size.** Found by the independent review of PR #47 and verified
against the tree. It is **not** pure wiring, and an implementer told otherwise will get stuck:

- **`assent run`: Config is loaded *after* the binding is selected, and only sometimes.**
  `selectBinding(rb)` is `run.go:219`; the Config load is step 2b at `run.go:224-237` and is
  guarded by `if cfg.config != ""` — `--config` is **optional**. So at the moment routing must
  happen, `conf` does not exist. S00 must move the load ahead of selection **and decide
  explicitly what happens when `--config` is absent** — a repo with a multi-binding document
  and no Config has no routing input, and that case must **fail closed**, never silently
  fall back to a collapse or to binding zero. That decision is part of this story.
- **`assent test` never loads a Config at all.** `test.go:67` calls `catalogue.LoadFromDir`,
  and `internal/catalogue/catalogue.go:124-127` says so in its own words: *"Config is not among
  the D-017 B10 field derivations … so Config is deliberately absent; a later story that needs
  config-derived fields adds it then."* `Input` is `{Packs, Bindings}`. **S00 is that later
  story.** Extending `catalogue.Input` to carry Config is a deliberate **E6 contract change**,
  not wiring — it must be called that, reviewed as that, and it is why this story is
  `engine-grade · maintainer LGTM`.
- **`Config.Classes` has zero production readers today** (`grep '\.Classes'` finds only
  `Spec.Classes` on profiles, plus tests). It is *precisely* parsed-and-discarded — the opposite
  of the original claim.

**Why this matters more than a wording fix:** tier 1 (`assent test`) is the epic's *primary*
deliverable — the "everybody can see it" claim. An implementer who wires only `run.go` leaves
tier 1 failing closed for both demo repos while the story reads done.

**REQ-DEM-S00-01** — A changed file's `(class, environment)` is resolved from
`Config.classes[].match.paths` and `Config.environments[].match.paths`, and the covering
`RulesetBinding` entry is selected by that pair. Last-match-wins for environments, matching the
semantics the example packs already document.

**REQ-DEM-S00-02** — **Fail-closed is preserved wherever routing is genuinely ambiguous — this
is the story's central constraint, not a caveat.** The current code's virtue is that it never
guesses; that must survive. A changed file matching **two classes**, and a resolved
`(class, environment)` pair with **no covering binding**, must both refuse with a named error
rather than pick one. Both pinned as tests.

**REQ-DEM-S00-03** — Call sites: `run.go:219` and `test.go:87`. **`compare.go:407` stays
fenced** — D-060 fenced it deliberately and this story does not reopen it.

**REQ-DEM-S00-04** — **D-060's `selectBindingForTest` strictest-collapse is DELETED, not left
as a fallback.** Leaving both paths live is how a fail-closed guarantee is lost quietly: a
future routing bug would silently degrade to the collapse instead of refusing. The D-060 row is
amended to record the supersession.

**REQ-DEM-S00-05** — `classify`'s reserved classes (`unclassified`, `assent-policy`) are handled
explicitly by the matcher: `assent-policy` keeps GUARD-1 dominance, and `unclassified` must not
resolve to a vouch-carrying binding (ADR-0008 §1's classification stage plus the
2026-07-21 fail-safe-by-construction amendment, enforced by `classify.ValidateRouting` /
`ErrReservedClassRouting` at `internal/core/classify/classify.go:127-145`). **This is the same seam DEM-S10 probes** —
the two stories must agree, and S10 is written against whatever S00 establishes.

**REQ-DEM-S00-06** — `internal/core` byte-unchanged and `git diff schemas/` == 0, the DoD the
neighbouring epics use. This is a `cmd/assent`-edge change.

**Given** a repo with `kafka-topic` and `kafka-acl` classes carrying different `require[]`,
**when** an MR changes only a topic file, **then** the `kafka-topic` binding is selected and the
run completes (today: fails closed on both `run` and `test`).
**Given** a changed file matching two class globs, **when** routing runs, **then** it refuses,
naming both classes — never a silent pick.
**Given** a file in an environment with no covering binding, **when** routing runs, **then** it
refuses.

> **Scope discipline.** This story wires routing and nothing else. It does **not** add class
> inheritance, per-class provider overrides, or glob precedence rules beyond what the schema
> already defines. If routing turns out to need a semantic the schema cannot express, that is a
> finding to record — not a schema widen to slip in here.

### DEM-S01 — Provider host declarations: worked examples, docs, and a gate `[autonomous]`

**As** an adopter copying a shipped example pack, **I want** the provider blocks to actually
resolve, **so that** I am not silently running with zero facts.

**REQ-DEM-S01-01** — Every `providers:` key in every example pack under `examples/packs/**`
has a matching host declaration at `<dir(config)>/providers/<name>.json`, valid against
`provider.LoadProviderConfig`, declaring typed `outputs`.

**REQ-DEM-S01-02** — A test in `examples`' gate walks every `.assent/config.yaml` in the tree,
and **fails** when a declared provider name has no matching `providers/<name>.json`. Both
polarities pinned: a fixture missing its declaration must red the test.

**REQ-DEM-S01-03** — `docs/usage/` documents the declaration path, that it is read **from the
target ref** (and why: an MR author must not be able to redefine their own fact semantics),
the split between repo-side `{type,url,failure}` and host-side `outputs`/`exec`/`repoFile`/
`resourceOwner`, and the **silent-skip behaviour** of a missing declaration.

**REQ-DEM-S01-04** — The `http`-typed example providers either gain a declaration *and* are
re-pointed at the reference broker (S03), or are converted to a builtin. **No example may ship
a provider block that cannot resolve.**

**Given** an example pack with `providers.author`, **when** its `providers/author.json` is
deleted, **then** the gate fails naming the pack and the provider.
**Given** the shipped packs as committed, **when** the gate runs under `task check`, **then**
it passes.

> **Note on the silent skip itself.** `provider_host.go:83-87` returns `continue` on a missing
> declaration, with a stated rationale (inventing `unavailable` keys would change CEL from
> "absent" to "false" — a real and correct concern). This story does **not** change that
> behaviour; it makes the condition impossible in shipped examples and discoverable in docs.
> Whether a *configured* provider with no declaration should be an operator error rather than a
> skip is a decision-path question and belongs in its own reviewed lane, not here.

### DEM-S02 — Provider-author guide `[autonomous]`

**As** a platform engineer at an organization assent has never heard of, **I want** a document
I can implement a provider from, **so that** I do not have to read Go source to learn the wire
format.

**REQ-DEM-S02-01** — `docs/usage/providers.md` (nav-linked) documents the request/response
envelopes **by reference to the frozen schemas**, never by restating them (restated schemas
drift; `docs/planning/provider-contract.md`'s 45 lines about `maxAge` are not a wire contract
and are cross-linked, not replaced).

**REQ-DEM-S02-02** — The fail-closed state table is documented with the adopter-facing
consequence of each state, plus the two rules a provider author will otherwise get wrong:
**echo `queryId`**, and **derive every timestamp from the host-pinned `asOf`** — never from a
provider wall clock (hard rule 7).

**REQ-DEM-S02-03** — The **credential constraint (G2)** is stated plainly: no header, token, or
client certificate can reach an HTTP provider, and `(?i)TOKEN|SECRET` env/argv names are
refused on exec. The broker pattern is documented as the shape that works, with the reasoning
(ADR-0015 §7), and cross-links **OQ-32**.

**REQ-DEM-S02-04** — The exec digest-pin re-pinning burden (G3) is stated, with the HTTP/broker
transport recommended for third-party providers.

**Given** a reader with no access to this repository's Go source, **when** they follow the
guide, **then** they can produce a response the host classifies `resolved` — pinned by a test
that runs the guide's own copy-pasteable example payload through `ResolveFactsChecked`.

### DEM-S03 — Reference IdP-groups broker provider `[autonomous]`

**As** an adopter on Entra ID or Keycloak, **I want** a working provider I can copy, **so that**
swapping user resolution is an afternoon, not a project.

**REQ-DEM-S03-01** — `contrib/providers/idp-groups/` ships a single small Go binary serving
`POST /` with a `FactResponse`, structured as an **IdP-agnostic core** plus two thin adapters:
**Entra ID** (`transitiveMemberOf`-shaped) and **Keycloak** (`users/<id>/groups`-shaped).

**REQ-DEM-S03-02** — The broker holds the IdP credential itself; **assent passes none**. The
README states the deployment contract (loopback/sidecar or mesh-terminated mTLS) and why.

**REQ-DEM-S03-03** — Every response is validated against
`schemas/provider/v1alpha1/response.schema.json` in test, and every timestamp derives from the
request's `asOf`. Tests are hermetic (`httptest` upstreams); **no live IdP is contacted in CI**.

**REQ-DEM-S03-04** — Failure paths are pinned: upstream 5xx/timeout → the host classifies
`unavailable`; unknown subject → `unavailable` with a reason, **never `resolved` with `[]`**
(the REQ-E5-S06-02 rule — an empty group set is a *false* authorization answer, not a missing
one).

**REQ-DEM-S03-05** — `contrib/` is explicitly **not** part of the decision path and not covered
by `API_STABILITY.md`; a `README` says so, and no `internal/core` package may import it.

**Given** an Entra-shaped upstream returning two groups, **when** the host queries the broker,
**then** exactly one `resolved` fact with the sorted group set and `expiresAt = asOf + maxAge`.
**Given** the upstream times out, **when** the host queries, **then** exactly one `unavailable`
fact — and a rule controlling authorization on it fails closed.

---

## Wave B — repo 1: platform resources (GitLab)

### DEM-S04 — New sample shapes: Kafka ACL + ArgoCD Application `[autonomous]`

**REQ-DEM-S04-01** — `examples/repos/` gains generated-generic Kafka ACL manifests (principal,
resource type/name/pattern, operation, permission, environment) and Argo CD `Application`
manifests (project, source repo/path/targetRevision, destination server/namespace, sync
policy), each one entry per file, with an owner field.
**REQ-DEM-S04-02** — `hack/check-sanitization.sh` green; **no content derived from
`references/**`** (D-002). Reviewed against that tree for structural resemblance, not just
string matches.
**REQ-DEM-S04-03** — `examples/repos/README.md`'s shape table gains the two rows; the D-029
deferred-shapes rows stay untouched (these are new generic shapes, not the deferred private
generalizations).

### DEM-S05 — `kafka-acl` class + cross-manifest reference rules `[autonomous]`

**REQ-DEM-S05-01** — Rules: the ACL's referenced **topic must exist** on the merge result; the
requesting author's team must **own the referenced topic** (via `builtin/resource-owner`); the
principal must match the requesting team's allowed principal pattern; `permission: allow` on a
prod `*`-pattern resource is blocked.
**REQ-DEM-S05-02** — Both polarities per rule, as directory cases with `facts.yaml`.
**REQ-DEM-S05-03** — **Tier-1 ceiling record.** For each rule, record whether it is
CEL-expressible under ADR-0013 and, where it is not, the concrete shape that defeats it. The
record is written into this spec directory and is a **named input to E11-S01** (D-141).
**REQ-DEM-S05-04** — 🔴 **RESCOPED TO TIER 2 — as originally written this was unbuildable.**
The intent stands: a referenced topic *deleted in the same changeset* the ACL references must
not evaluate as present. But **the evaluation unit is one file**: `assent run` takes exactly one
`--subject file:<path>` (`cmd/assent/run.go:266`) and diffs that file alone (`:289`); the
changed-file fold (`:316-336`) propagates only `classify.ClassAssentPolicy` and opacity, and
`adoptertest.Case` is singular (`File string; Base, Head []byte`,
`internal/adoptertest/adoptertest.go:150-160`). No single evaluation can contain both files, and
there is no cross-subject aggregation at the run seam. **The escape hatch is real but tier-2
only:** `builtin/repo-file` reads the *merged-result checkout* (ADR-0008 §4,
`OpenRepoRoot` in `internal/provider/builtin/repo_file.go`), so same-MR presence **is**
resolvable — under `--checkout`, i.e. DEM-S14 `[infra-gated · operator]`. **Anti-tautology
clause:** satisfying this with a hand-authored `facts.yaml` value asserting the topic is absent
proves nothing and does **not** close the REQ.

**Given** an MR adding an ACL for a topic owned by another team, **when** assent evaluates,
**then** BLOCK with a finding naming the owning team.
**Given** an MR adding a dev ACL for a topic the author's team owns, **when** assent evaluates,
**then** APPROVE.

### DEM-S06 — `argocd-application` class + rules `[autonomous]`

**REQ-DEM-S06-01** — Rules: destination `namespace` within the team's allow-list
(`builtin/repo-file` walk-up, most-specific-first); `project` within the allow-list; source
repo within the allow-listed org; `syncPolicy.automated.prune: true` on a prod Application →
REVIEW; removing an Application in prod → no-destruction BLOCK.
**REQ-DEM-S06-02** — Both polarities per rule.
**REQ-DEM-S06-03** — The per-environment allow-list demonstrates `repo-file` most-specific-first
resolution across at least two levels, with the walk-up visible in the fixture layout.
🔴 **This is a tier-2 requirement and must be labelled as one.** `assent test` cannot exercise
it: the E6 fence (`cmd/assent/test_provider_fence_test.go:69-80`) keeps tier 1 on
`facts.yaml` stubs, so **nothing walks up at tier 1**. Satisfying this REQ with an authored
`facts.yaml` value that merely *looks* like a resolved owner is a **test that cannot fail** and
does not satisfy it. Either demonstrate the walk-up at tier 2 (DEM-S14), or state plainly in the
fixture and the README that tier 1 shows a stubbed fact and the resolution itself is tier 2.

### DEM-S07 — Repo 1 assembly `[autonomous]`

**REQ-DEM-S07-01** — `examples/demo/assent-demo-platform/` is a complete repo root: governed
content, `.assent/{config.yaml,bindings.yaml,packs/**,tests/**,providers/*.json}`.
**REQ-DEM-S07-02** — `assent lint .` clean and `assent test .` green; `assent test . --coverage`
shows every rule covered in **both** polarities.
**REQ-DEM-S07-03** — README leads with the **60-second tier-1 path** (clone → `assent test .`),
then the tier-2 live path, then the L0→L3 user-resolution ladder with the two-file L2→L3 diff
shown inline.
**REQ-DEM-S07-04** — Prepared demo branches, each named for its outcome and each an entry in
the test suite so it cannot silently stop reproducing: at minimum
`demo/approve-add-dev-topic`, `demo/review-prod-partition-shrink`, `demo/block-foreign-team-acl`,
`demo/block-prod-app-delete`.

---

## Wave C — repo 2: terraform (GitHub)

### DEM-S08 — Raw `.tf` shape + HCL structuring truth `[autonomous]`

**REQ-DEM-S08-01** — Generated-generic Terraform stacks: a blessed-module instantiation per
env, with `source`/`version` pins and a small set of governed inputs. D-002 as in S04.
**REQ-DEM-S08-02** — **Written truth about what the fact model can structure in HCL** — which
constructs yield bound entries and which fall back to opaque. Determined by running the code,
not by reading it, and recorded with the commands used.
**REQ-DEM-S08-03** — The **opaque-change fallback is pinned in both polarities**: a structured
`.tf` edit evaluates on bound fields; an edit the model cannot structure routes to REVIEW and
**never** to APPROVE.

> This story is deliberately allowed to discover that raw HCL structures worse than expected.
> If so, the finding is recorded and repo 2 leans harder on `.tfvars` for structured rules
> while keeping `.tf` as the opaque-fallback demonstration — which is itself worth showing.
> **Silently dropping `.tf` is not an option**; it was explicitly requested.

### DEM-S09 — `tf-module-instance` class + rules `[autonomous]`

**REQ-DEM-S09-01** — Rules: module `source` within the allow-listed registry; `version` pinned
exactly (no range) and within the allowed band; instance-size / replica-count within the
per-environment band; a prod change exceeding the bounded-change budget → REVIEW.
**REQ-DEM-S09-02** — Both polarities per rule.
**REQ-DEM-S09-03** — Environment split proven: the identical diff APPROVEs in `dev` and REVIEWs
or BLOCKs in `prod`, from one changeset — the clearest single demonstration of binding scope.
**Depends on DEM-S00 and is currently impossible in either tier**: `run` fails closed on >1
binding, and `test` collapses a multi-binding document to its strictest (D-060). A
decision-*flipping* environment split has never been exercised by any shipped example — D-060
recorded the corpus split as decision-neutral — so this requirement is also S00's sharpest
acceptance test, not merely a demo nicety.

### DEM-S10 — `tf-backend` ungoverned → REVIEW `[autonomous]`

**REQ-DEM-S10-01** — `stacks/<env>/backend.tf` is matched by **no** class. The two cases have
**different governing mechanisms and must be cited separately** — conflating them is how a
guarantee gets over-claimed:
- **Delete** — an unmatched whole-file delete escalates fail-safe to REVIEW
  (**D-063**, `aggregate.unmatchedDelete` / `fileEvent.unmatchedDelete`). Operator-confirmed;
  no relaxation to APPROVE authorized.
- **Edit** — governed by **ADR-0008 §1**'s implicit `unclassified` class (`classify.go:18-20`),
  which no vouch rule
  may match, so an unmatched edit cannot be vouched for.

**REQ-DEM-S10-02** — **What actually happens to an unmatched EDIT at the `run` seam is
DETERMINED BY RUNNING THE CODE, not asserted** — the same discipline as S08's HCL truth, and
required here because `run.go:487-492` records `unclassified` routing as *"not wired in this
lane"*. The observed behaviour is recorded with the commands used. If it is anything other than
a refusal or REVIEW, **that is a finding, not a demo feature**, and it is logged rather than
worked around. Depends on DEM-S00, which owns the same seam (REQ-DEM-S00-05).

**REQ-DEM-S10-03** — 🔴 **RESCOPED TO TIER 2 — as originally written this was unbuildable.**
The intent stands: in a changeset touching **both** a governed `.tfvars` and `backend.tf`, the
ungoverned file must dominate and an otherwise-clean governed change must not carry the
ungoverned one to APPROVE. But per REQ-DEM-S05-04's note the evaluation unit is **one file**, so
no single tier-1 evaluation can contain both — and there is no unmatched-**edit** analogue of
the delete escalation (`internal/core/aggregate/coverage.go:251-253` gates on
`ch.Path == "" && ch.Kind == delete`). Demonstrate this at tier 2 (DEM-S14) against a real MR
touching both files, or state the single-subject constraint and drop the claim. **Anti-tautology
clause:** two separate single-file evaluations run side by side do **not** demonstrate
domination — domination is a property of aggregating them, which is the thing that does not
exist. Building that aggregation is engine work this epic explicitly fences out; if S10 finds it
necessary, that is a **finding to log**, not a demo feature to add.

### DEM-S11 — Repo 2 assembly `[autonomous]`

**REQ-DEM-S11-01** — `examples/demo/assent-demo-terraform/` complete, `lint` clean, `test`
green, both-polarity coverage, as S07.
**REQ-DEM-S11-02** — README leads with tier 1 and **states in its first screenful that the live
GitHub tier is pending E10-S18**, linking the epic. No workflow file that would appear to run
assent on PRs may be committed before the adapter exists.
**REQ-DEM-S11-03** — Prepared demo branches as S07, including a `demo/review-opaque-backend-change`.

---

## Wave D — make it real

### DEM-S12 — Demo CI + mirror-drift gate `[autonomous]`

**REQ-DEM-S12-01** — Both demo trees run `assent lint` + `assent test` under `task check` and in
`verify`. A demo that stops reproducing **reds assent's own build**.
**REQ-DEM-S12-02** — Every prepared demo branch's expected outcome is an adopter-test case, so
the README's promised decisions are gated, not asserted.
**REQ-DEM-S12-03** — Once S13 lands, a check diffs the in-tree trees against the published
repos and fails on drift; until then it is a no-op with the reason recorded (**no silent
skip** — a gate that cannot fail must announce that it is inert).

### DEM-S13 — Publish the repositories `[operator]`

**REQ-DEM-S13-01** — 🔴 Operator creates `PlatformRelay/assent-demo-platform` (mirrored to
GitLab) and `PlatformRelay/assent-demo-terraform`, seeds `main` from the in-tree trees, pushes
the demo branches. **Requires explicit authorization** — judgment call (d).
**REQ-DEM-S13-02** — `README.md` and the docs site link both repos with the tier each supports.
**REQ-DEM-S13-03** — The mirror procedure is a script in `hack/`, not a remembered sequence.

### DEM-S14 — Live tier-2 proof on GitLab `[infra-gated · operator]`

**REQ-DEM-S14-01** — assent runs on real MRs in `assent-demo-platform` producing at least one
APPROVE (with a real SHA-pinned merge), one REVIEW (with a resolvable thread), and one BLOCK.
**REQ-DEM-S14-02** — DecisionRecords retained as evidence under `docs/decisions/evidence/`,
mirroring the D-042 shape.
**REQ-DEM-S14-03** — GitHub tier 2 is **explicitly deferred to E10-S18**, which retargets its
live adoption proof at `assent-demo-terraform` instead of a throwaway repository.

---

## Exit gate

The epic is done when: `(class, environment)` routing is wired with ambiguity still failing
closed and D-060's collapse deleted (S00); both demo trees are `lint`-clean and `test`-green
under `task check`
with both-polarity coverage on every rule (S12); a reader with **no forge account, no token,
and no network** can clone either repo and see real decisions (tier 1); every `providers:` key
in the repository has a resolving host declaration and the path is documented (S01/S02); the
reference broker provider passes its hermetic suite against the frozen response schema (S03);
`hack/check-sanitization.sh` is green over all newly authored shapes (D-002); the E11 tier-1
ceiling record exists (S05); both repositories are published and linked (S13); and the GitLab
live tier-2 proof is recorded (S14) with the GitHub tier honestly marked pending E10-S18.
