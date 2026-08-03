# Vision and intended use case

## The problem

Platform teams run **self-service repositories**: a git repo holds structured configuration
(YAML topic definitions, JSON service catalogs, tfvars for infrastructure modules), and teams
request changes via merge requests. The repo *is* the API. This pattern is everywhere —
Kafka topic self-service, tenant onboarding, DNS zones, IAM group files, Terraform variable
sets — and it always develops the same bottleneck:

**every MR needs a human reviewer, but most MRs are routine.**

A team bumps the partition count of *their own* topic within quota. A service adds itself to a
catalog file following the schema. Someone raises a memory limit inside an approved band. The
reviewer's job is not judgment — it is *reconstruction*: what changed, is it production, does
the author own this entry, is anything destructive hiding in the diff. That reconstruction is
mechanical, repeatable, and therefore automatable.

Teams that automate it today write a **bespoke bot per repo** — a pile of pipeline scripts,
regexes over diffs, and hard-coded permission lookups that nobody wants to touch. The logic is
untested, invisible to the people governed by it, and dies with its author.

## The product

**assent** is a generic, open-source auto-merge gate that any self-service repo can adopt:

1. **Install**: add one job to the repo's pipeline (GitLab CI first; GitHub Actions next) and a
   policy directory (e.g. `.assent/`) to the repo.
2. **Describe**: write rules in **Rego** or a **Kyverno-style declarative YAML** against a
   canonical model of the change — not against raw diff text.
3. **Trust**: assent evaluates every MR/PR deterministically and acts like a reviewer:
   resolvable review threads for findings, comments explaining the decision, approve/deny, and
   auto-merge when the decision is APPROVE and the platform's own gates (CI green, discussions
   resolved) are met.
4. **Verify**: policies ship with tests. The built-in harness runs fixture changes against the
   policy set and asserts the expected decision — locally and in CI.

### One decision, explained

For each MR the engine produces exactly one decision — `APPROVE`, `REVIEW` (human required),
or `BLOCK` — aggregated from per-rule **effects**: informational comments, resolvable
"are you sure?" challenge threads, hard blocks, positive vouches that make changes
automerge-eligible, and **risk points** summed against per-environment thresholds
(ADR-0007). Findings render with expandable docs/debug sections (ADR-0012) so every decision
explains itself. Determinism is a hard requirement: the same diff, repo state, and facts
always produce the same decision. No LLM in the decision path.

### Modes

The same pipeline runs as: a **CI job** (primary), a **local dry-run** ("what would the gate
say?"), **explain** (full per-rule trace), a **historical scan** over past MRs (backtesting a
policy before trusting it, feeding `stats` — no database, just report artifacts), and later a
**webhook service** for orgs that prefer event-driven operation (ADR-0009).

## What makes it different

| Capability | Typical bespoke bot | assent |
| --- | --- | --- |
| Change understanding | regex on diff lines | canonical field-level change model for JSON / YAML / HCL-tfvars |
| Rule language | imperative script | Rego or declarative YAML, versioned in the governed repo |
| Permission checks | hard-coded HTTP calls | pluggable providers: Keycloak, LDAP, GitLab/GitHub groups, ownership files, custom plugins |
| Review UX | pipeline pass/fail | resolvable review threads, comments, approve/deny, auto-merge |
| Testing | none | fixture-based policy tests, required by lint |
| Platform | one forge | GitLab + GitHub behind one forge-neutral port |

## Personas

- **Platform engineer (adopter)** — owns a self-service repo; wants routine MRs merged without
  a human, with an audit trail; writes and tests the policy set.
- **Contributor (governed user)** — opens MRs against the repo; gets an instant, explained
  decision instead of waiting for a reviewer in another timezone.
- **Rule author / plugin developer (extender)** — integrates a company-specific permission
  source or fact provider without forking the core.
- **Auditor** — reads policies and decision logs; can replay any historical decision.

## Example rule archetypes (generic)

These generalize the rules a real production merge gate needs; concrete samples live in
[`examples/`](https://github.com/PlatformRelay/assent/tree/main/examples):

- **Ownership**: the author may only modify entries whose `owner` (group/team) they belong to —
  membership resolved via a permission provider (Keycloak, LDAP, forge groups, ownership file).
- **Bounded change**: numeric fields may change only within a band (e.g. `partitions` may
  increase up to a quota, never decrease).
- **Allow-listed fields**: only a named set of fields may change for automerge; anything else
  → human review.
- **No destruction**: deletions of whole entries/files always require human review or a
  second approval.
- **Environment split**: changes touching `prod/**` need stricter rules than `dev/**`.
- **Schema validity**: the changed file must still validate against the repo's schema.
- **Freshness/context facts**: e.g. the referenced cost center or on-call rotation must exist
  in an external system — resolved by a fact-provider plugin.

## E2E and samples strategy

Real-forge behaviour (threads, approvals, merge) can only be proven against a real forge.
The repo ships:

- a **kind cluster setup** (`hack/kind/`) that can host a GitLab instance for e2e tests, and/or
  a **GitLab testcontainer** profile for CI (trade-off tracked in ADR-0006);
- **generated sample repos** (topic-style YAML, catalog-style JSON, tfvars) seeded into that
  GitLab, used both as e2e fixtures and as user-facing documentation examples;
- GitHub e2e via a dedicated test org once the GitHub adapter lands.

## North star

> A platform team makes a repo automerge-capable in **under one hour**: install the CI job,
> copy a sample policy, adapt it, run the policy tests, merge. From then on ≥70% of routine
> MRs merge without human attention — with every decision explained and replayable.

## Non-goals

- Not a general CI system, linter aggregator, or code-review LLM.
- Not a replacement for the forge's own protections (branch protection, approval rules) — it
  composes with them.
- No probabilistic/LLM component in the decision path (an advisory layer may come later, but
  never gating).

## Naming note

**assent** — "to give assent": formal, considered approval. Chosen 2026-07-21 (D-009 in
[`docs/decisions/decisions.md`](decisions/decisions.md)); the public repo is created as a
separate, explicit operator step (D-001).
