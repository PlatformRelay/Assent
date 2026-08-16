# ADR-0015: Trust boundaries and merge-time integrity

| | |
| --- | --- |
| **Status** | Accepted (partial: merge-result / max_age arming / typed providers per ADR-0017 §1/§4/§6; P2-E5) |
| **Date** | 2026-07-21 |
| **Deciders** | Konrad Heimel |
| **Context links** | [ADR-0007 effects](0007-rule-effects-decision-aggregation.md) · [ADR-0008 routing](0008-change-classification-routing-scope.md) · [ADR-0009 modes](0009-execution-modes.md) · [ADR-0010 config](0010-config-files-repo-layout.md) · [ADR-0011 ports](0011-core-ports-and-contracts.md) · adversarial design review 2026-07-21 (findings F1, F3, F4, F5, F13, F14) |

## Context

An adversarial review of the design found that the trust model was implicit — and, as
written, broken: policies were loaded from the very branch they gate (F1), the merge action
was not pinned to the evaluated commit (F3), facts could go stale between evaluation and a
deferred merge (F4), the CI job definition itself is author-editable in self-service repos
(F5), and the webhook mode deferred its security requirements while claiming
architecture-readiness (F13). This ADR fixes the trust boundary explicitly. Principle:
**everything that decides is loaded from a trusted ref; everything that is judged comes from
the untrusted branch; every write action re-verifies what it is about to act on.**

## Decision (proposed)

### 1. Policy is trusted input → loaded from the target ref, never the MR branch

`.assent/**` (config, bindings, packs, referenced Rego, templates, tests' expectations) is
**always loaded from the target/base ref** of the MR. The source branch contributes only the
material under judgment: the diff and the branch state for `scope: branch` rules.

Any MR that touches `.assent/**` routes to the implicit meta-class **`assent-policy`**,
which is `block`-by-default: policy changes always require human review and can never be
vouched by the policies they modify. Repos may relax this only to `challenge`, never to
`vouch`. A mandatory golden e2e case: *"MR edits its own policy → BLOCK."*
(Recommended hardening: forge-level CODEOWNERS/approval rule on `.assent/**` requiring a
human — see §5.)

### 2. Merge is SHA-guarded (no TOCTOU)

`Publisher.Merge` and `Publisher.Approve` carry the evaluated head SHA from `Decision.Pins`
and use the forge's compare-and-swap: GitLab `merge?sha=`, GitHub merge `sha` parameter.
If HEAD has moved, the merge **fails closed** and the new pipeline re-evaluates. A forge
adapter that cannot SHA-guard merges must declare the capability gap, and the engine then
never auto-merges on that forge. This is a required case in the conformance suite (ADR-0005).

### 3. Fact freshness is bounded

`Decision.Pins` records per-provider resolution timestamps and the full resolved fact set
(which also makes replay hermetic — see ADR-0011 amendment). When the merge is deferred
(forge-native gate, ADR-0009), staleness is bounded by: (a) any new push re-runs the
pipeline and re-resolves facts (forge behavior), and (b) a configurable
`facts.max_age` (default: 24h) after which an armed auto-merge is considered expired — the
summary comment states the expiry and a pipeline re-run is required. Repos with
security-sensitive facts should set a short `max_age` or use serve mode (v1.x) which
re-evaluates on events.

### 4. The pipeline running assent must not be author-editable

CI-first only works if the job definition is trusted. Adoption prerequisite (documented in
the walkthrough and CI templates, verified by `assent doctor`): the assent job must come from
a **protected source** — GitLab: pipeline configuration from a protected/included file
outside the MR branch's control (compliance pipeline / instance template) or a
merge-request-approved pipeline model; GitHub: workflows from the target branch
(`pull_request` runs the base-ref workflow for forks; same-repo branches need branch
protection on workflow paths). The token is least-privilege: merge+comment on this project
only. The docs must state plainly: putting the assent job in an author-editable
`.gitlab-ci.yml` with a privileged token is an unsupported, insecure topology.

### 5. Bot approval is the gate — say it out loud

When assent approves and merges, its approval satisfies the forge's "1 approval required"
rule; there is **no residual human gate**. This is intended and must be documented, together
with the defense-in-depth option: a forge approval rule on `.assent/**` (and optionally on
`prod/**` classes) requiring an identity assent cannot provide.

### 6. Serve mode: security requirements fixed now, implementation later

So that v1 port shapes don't foreclose them (F13): webhook signature verification (HMAC /
forge-native), event dedup key (MR id + head SHA + event id), idempotent publishing (findings
carry stable ids; re-posting is an upsert), per-repo token scoping (no instance-wide
credential), and the same target-ref policy loading as §1.

## Consequences

- The classifier (ADR-0008) gains the built-in `assent-policy` meta-class; ADR-0010's layout
  section references §1; ADR-0014's expectation-trust question is resolved the same way:
  expectations for `assent test` in CI come from the target ref when `.assent/**` changed.
- `scan` backtests policies from the ref being tested, not from each historical MR — already
  consistent with §1.
- Publisher port signatures change: `Approve`/`Merge` take the pinned SHA (ADR-0011 amended).
- Threat model becomes documentable: this ADR seeds SECURITY.md.

## Counterpoints considered

- *"Loading policy from the target ref makes policy changes hard to test in their own MR."* —
  No: `assent test` and `--dry-run` run locally against the branch; CI additionally runs the
  branch's own tests *as tests* (L1). Only the *gating* decision uses target-ref policy. The
  policy MR is gated by the old policy + human review — exactly right.
- *"SHA-guard + freshness makes automerge slower."* — It makes automerge *checkable*. The
  fast path (APPROVE at evaluation time, merge immediately, SHA still HEAD) is unaffected.

## Additions (2026-07-21, independent security review A-03/A-04/A-05/A-10)

### 7. Provider/plugin trust model (A-03)

Providers run **without the forge write token** — the provider host passes a narrow FactQuery
and receives facts; the approve/merge credential never enters a provider's environment or
process. Exec/gRPC provider binaries are **digest-pinned** in `config.yaml` (optional cosign
verification); default recommendation is *no in-process third-party code* — subprocess with
narrow JSON RPC (tier 2) or WASM (tier 4) are the sanctioned paths, and the gRPC tier is
documented as elevated-risk until sandboxing lands. Provider timeouts fail closed (facts
`unknown` → fail-safe per ADR-0007 tri-state).

### 8. Execution-authority matrix (A-04)

| Mode / context | May hold write token | May auto-approve/merge |
| --- | --- | --- |
| CI from protected/trusted config (§4) | yes (least-privilege) | yes |
| CI on fork / untrusted-contributor MR | **no** | advisory-only (report, no writes) |
| `serve` (v1.x, §6) | yes (per-repo scoped) | yes |
| `--dry-run` / `explain` / `test` / `scan` | no | never |

`assent doctor` refuses to arm auto-merge when it cannot verify the protected-config
precondition.

### 9. Input resource safety & report retention (A-05, review P1-3)

Parsing and evaluation run under hard limits — max file size/count, max diff bytes, nesting
depth, YAML anchor/alias expansion caps, symlink and path-traversal rejection, parse+eval
deadlines, CEL cost budget — breach fails closed to REVIEW (spec'd with ADR-0003). The
summary comment always embeds the decision hash and a link to the report artifact; docs must
warn that CI artifact retention limits the audit window and recommend a retention policy for
report artifacts.

## Amendment (2026-08-16, D-147 — host-side credential resolver for HTTP providers)

### 10. Host-side credential resolver (D-147 / OQ-32)

**Context.** §7 established that no provider transport carries a credential — `CallHTTP`
sets only `Content-Type: application/json`, and the repo-side provider schema
(`$defs/provider`) is closed over exactly `{type, url, failure}`. That was deliberate: a
hostile-provider isolation proof (`TestIsolation`, Spike C) rests on it. But it forecloses
calling a token-authenticated IdP (Entra ID, Keycloak) directly — every such provider had to
go through a broker. D-147 (resolving OQ-32) rules that a direct credential channel should
exist after all, provided it stays **host-side only**: the credential is configured on the
assent host (env / CLI / host config), keyed by a provider name, and the host — never the
MR — injects the auth header. The operator's stated adoption patterns (GitLab CI / GitHub
Actions job variables as the common case; Vault-agent file injection; a hosted variant with
its own backend) all shape the source list below.

**Decision.**

- Repo-side `$defs/provider` gains exactly **one** new optional field: `credentialRef`
  (string, opaque, non-empty). Nothing else about the schema changes; it stays
  `additionalProperties: false`.
- The host holds a **credential allowlist**, keyed by `credentialRef` name, each entry
  populated from a host-side source and binding, host-side, all of: source, header name +
  scheme (default `Authorization: Bearer <token>`), and the **exact origin
  (scheme+host[:port]) the credential may be attached to**. The origin bind is load-bearing,
  not decorative: repo-side config still supplies `url`, so without it, `credentialRef` paired
  with an attacker-chosen `url` in the same MR-editable tree reproduces exactly the
  exfiltration shape a repo-side `secretRef` would have opened. §1 (policy loaded from the
  target ref, never the MR branch) and the `assent-policy` block-by-default class are a
  second, independent mitigation on the same hole — worth stating explicitly since D-147
  credited them separately, but they gate by ref-trust, not by a runtime check, so they don't
  substitute for the origin bind.
  - **env**: `ASSENT_PROVIDER_TOKEN_<REF>` (normalized ref name) — the GitLab CI / GitHub
    Actions "set a job variable" pattern; expected to be the common case.
  - **file**: a host-config-declared path, read at resolve time — the Vault-agent-injects-a-
    file pattern, and the Kubernetes/CSI secret-mounted-file pattern.
  - **pluggable resolver interface** — a small `CredentialSource` contract so a hosted variant
    can add a managed-secret-store backend later without a transport-contract change.
  - An unknown or unallowlisted `credentialRef` is a **hard error** at provider resolution —
    never silently ignored, never falls back to an unauthenticated call.
- **Attachment**: the host resolves `credentialRef` → secret *before* `CallHTTP` builds its
  request, and sets the allowlisted header (default `Authorization: Bearer <secret>`)
  alongside the existing `Content-Type: application/json`. Credentialed calls additionally: (a)
  refuse (hard error) to attach to a non-`https` URL, and (b) use a client with redirects
  disabled, so a credential is never replayed to a redirect target outside the allowlisted
  origin.
- A provider with `credentialRef` set may never be configured `failure: open` — the same lint
  hard-error idiom the schema already applies to controlling/authorization providers
  (`docs/planning/lint-hard-errors.md`). A resolution failure must fail-safe (fact `unknown`,
  ADR-0007 tri-state), never silently degrade to an unauthenticated call.
- **Exec tier: excluded from this design, on purpose.** `ScrubEnv`/`ScrubArgv` keep stripping
  every env/argv entry matching `(?i)(TOKEN|SECRET)` — no exception list, no bypass flag, for
  exec providers under this amendment. The resolver's own host env-var convention
  (`ASSENT_PROVIDER_TOKEN_<REF>`) is chosen deliberately to align with, not fight, that regex:
  if a resolved value were ever echoed into an exec provider's declared env/argv by
  misconfiguration, the existing scrub still catches it as a `TOKEN`-named canary. Exec stays
  excluded rather than gaining a dedicated non-scrubbed channel because exec is precisely the
  tier where §7's isolation claim rests on the scrub — a spawned child, with an environment and
  argv visible in the host process table — and a bypass there would invert that claim for the
  one tier that needs it most.
- **Never persisted**: the resolved secret is never written to `Decision.Pins`, the evaluation
  report, the summary comment, or debug/explain output — consistent with ADR-0004 Amendment
  2's "providers must never return raw credentials as facts." Resolver errors name the
  `credentialRef`, never the resolved value.
- **Purity boundary**: resolution and attachment live in `internal/provider` (host-facing,
  I/O-permitted), outside the arch-lint-enforced pure set (`internal/core`, `internal/change`,
  etc. — ADR-0011 Amendment 3). This does not reopen AGENTS.md's determinism rule for the
  decision path: resolution is a deterministic function of host state, and happens before
  evaluation, not inside it.

**Explicitly NOT authorized by this amendment:**

- A repo-side literal secret, or a repo-side reference that also carries or selects a
  destination. `credentialRef` is an opaque name only; the host — never the MR — decides what
  it resolves to and where it may be sent.
- A repo-side field that can choose or override header name, scheme, or target origin. Those
  stay host-config-only, inside the allowlist entry.
- Any exec-tier exception to `ScrubEnv`/`ScrubArgv`.

**Consequences.**

- Schema (`$defs/provider`): one new optional string property, `credentialRef`. Still
  `additionalProperties: false`; still no way for one MR-editable document to name a URL and a
  real credential together.
- `transport.go`: `CallHTTP` gains a resolver call ahead of request-header construction, an
  https-only guard, and a redirect-disabled client for credentialed requests. `ScrubEnv`/
  `ScrubArgv` are otherwise unchanged — this amendment restates their exec-tier behavior, it
  does not loosen it.
- Host config gains a new credential-allowlist surface (source + header/scheme + origin bind,
  keyed by ref name) that lives outside `.assent/**` — not repo-editable, not part of the
  frozen repo-side schema tree.
- The hostile-provider isolation proof's core claim is unchanged: the MR-editable tree still
  cannot see, choose, or redirect a real credential — it can only name a reference the host has
  already pre-bound to one destination.
- Follow-up: openspec stories for the resolver, the host allowlist config shape, and the lint
  hard-error pairing `credentialRef` with `failure: open`; provider-author guide (DEM-S02)
  documents the GitLab-CI-variable / GitHub-Actions-secret / Vault-agent-file / hosted-backend
  patterns.
- This amendment touches both a security boundary (§7's isolation proof) and the repo-side
  provider schema — implementation should carry explicit maintainer review before merge, not
  rely on CI-green alone.
