# ADR-0009: Execution modes: CI, local/dry-run, explain, webhook service

| | |
| --- | --- |
| **Status** | Accepted (partial: one-shot arming restrictions per ADR-0017 §4; P2-E5). **Substantially unimplemented as of 2026-08-09** — only the `run` mode exists (no `--dry-run`/`explain`/`serve`/`scan`/`stats`), and the challenge-resolution amendment's deferred forge auto-merge was never built; see the two *Implementation status* notes below and [D-135](../decisions/decisions.md). |
| **Date** | 2026-07-21 |
| **Deciders** | Konrad Heimel |
| **Context links** | [ADR-0005 forge](0005-forge-abstraction-gitlab-first.md) · [ADR-0007 effects](0007-rule-effects-decision-aggregation.md) |

## Context

The same decision pipeline must run: in CI per MR (primary); on a developer's machine before
pushing ("what would the gate say?"); in a debugging session ("*why* did it say that?"); and —
for orgs that prefer event-driven operation over per-repo CI jobs — as a webhook receiver.
The v1 scaffold said "no long-lived service"; that hardens into: **the core is a one-shot
pipeline; the service is a thin wrapper around it**, so serve-mode support is an architecture
constraint now even if the wrapper ships later.

## Decision (proposed)

One core pipeline (ingest → classify → evaluate → aggregate → publish), four entrypoints:

| Mode | Command | Publisher | Notes |
| --- | --- | --- | --- |
| CI | `run` | real forge writes | reads MR context from CI env vars (GitLab CI first) |
| Local / dry-run | `run --dry-run` (also default when no MR context) | **recorder**: prints decision, findings, and every action it *would* take | works against local branch vs target ref; no token needed unless providers require it |
| Explain / debug | `explain` (or `run --explain`) | recorder + full trace | per-change: detected classes, routed packs/bindings, matched rules, predicate results, score arithmetic, aggregation path |
| Webhook service | `serve` | real forge writes | long-lived; subscribes to MR events, clones/checks out the branch per event (ADR-0008 §4), then runs the identical pipeline |
| Historical scan | `scan --since <date> \| --mrs <range>` | recorder only | replays past (even merged) MRs through the current policy set: backtesting a pack before enabling automerge, calibration ("what % would have automerged?"), regression checks after policy changes. Emits one JSON report per MR |
| Statistics | `stats <reports-glob>` | n/a | aggregates JSON reports (from `run` or `scan`) into automerge rate, outcome distribution, top firing rules, score histograms — flat files, **no database for now** (ADR-0012) |

Side effects are isolated behind the **Publisher** port; dry-run swaps in a recorder — the
decision core cannot tell the difference. The JSON report is emitted in every mode and is
byte-identical between a dry-run and a real run on the same input (determinism gate).

Shipping order: `run`/`--dry-run`/`explain` in v1; `serve` in v1.x once the CI path is proven
(OQ-14) — but ports and CLI structure assume it from day one.

> **Implementation status (2026-08-09, [D-135](../decisions/decisions.md)) — of the six modes
> above, only `run` exists.** A fact about the code, not a change to the decision: the table
> and the shipping order are untouched.
>
> The shipped dispatch table is `run`, `doctor`, `lint`, `test`, `compare`, `catalogue`,
> `render`, `eval-input`, `version`, `help`. **There is no `--dry-run` flag** — `assent run
> --dry-run` exits `2` with `flag provided but not defined: -dry-run`, and a repo-wide search
> for `dry.run` across `cmd/` and `internal/` returns nothing. `explain`, `serve`, `scan` and
> `stats` are likewise undispatched and exit `2`. Fail-safe direction: an unknown flag or
> command refuses before the forge is contacted, so nothing is written.
>
> Two claims above are therefore not true of the tool today: *"Side effects are isolated behind
> the Publisher port; dry-run swaps in a recorder"* — there is no recorder to swap in — and, in
> Consequences, *"Every doc example can show the dry-run first — the adoption path starts with
> zero risk."* **There is no zero-risk adoption path via a mode switch.** The only way to run
> `assent` without approve/merge writes is to leave one of the forge-probed arming
> preconditions unmet; `docs/usage/cli.md` §*How to keep assent advisory* states this, and
> explains why a pack's `spec.phase` must not be used as a substitute (with no `require:`
> obligations declared, an `observe` or `off` ceiling turns a BLOCK into an approve+merge).
>
> This gap is *why* the CLI reference previously recommended `phase: observe` as the advisory
> lever: the sanctioned mechanism was never built, so the docs invented one. Same open options
> as the amendment below, and the same owner — build the recorder mode, or retract it from this
> ADR and re-derive the adoption story. Tracked in [D-135](../decisions/decisions.md); this
> note picks neither. The project already states this honestly elsewhere — see
> [the walkthrough](../usage/walkthrough.md)'s "**Planned — `assent explain` does not exist**".

## Consequences

- CI env parsing (which vars identify the MR) is adapter code, not core; adding GitHub
  Actions later touches only that layer + forge adapter.
- `serve` introduces state concerns (event dedup, re-evaluation on thread resolution) that the
  one-shot modes don't have; these must be spec'd before it ships, not bolted on.
- Every doc example can show the dry-run first — the adoption path starts with zero risk.

## Counterpoints considered

- *"Webhook-first like a bot framework."* — Event-driven is operationally heavier (state,
  HA, secrets custody) and most target orgs can add a CI job trivially; CI-first keeps the
  trust story simple ("it runs in *your* pipeline with *your* token").

## Amendment (2026-07-21, adversarial review F2/F3): the challenge-resolution mechanism

One-shot CI cannot observe thread resolution (forges do not trigger pipelines on it), so the
original "merges after all threads are resolved *and* re-evaluation passes" promise had no
mechanism. Fixed as follows — **the forge, not assent, enforces resolution**:

1. On `challenge` findings (and no block), assent posts the resolvable threads, records the
   decision, **approves conditionally and arms forge auto-merge pinned to the evaluated SHA**
   (GitLab: "merge when pipeline succeeds" + all-discussions-resolved merge gate, `merge?sha=`;
   GitHub mapping per OQ-7). The forge merges when every thread is resolved.
2. **Resolution alone does not re-run assent** — this is now an explicit, documented
   property, compensated by: any new push cancels the armed merge and re-evaluates
   (forge-native), the SHA pin (ADR-0015 §2) guarantees only the evaluated commit can merge,
   and fact staleness is bounded by `facts.max_age` (ADR-0015 §3).
3. Repos that need genuine re-evaluation on resolution (e.g. re-checking facts at merge
   time) use `serve` mode (v1.x) — that is its primary justification.

Adoption prerequisite (per ADR-0015 §4): the repo's forge settings must enable the
all-threads-resolved merge gate; `assent doctor` verifies this and the protected-pipeline
topology before the tool arms any auto-merge.

> **Implementation status (2026-08-09, [D-135](../decisions/decisions.md)) — point 1's
> deferred forge auto-merge is NOT implemented.** This note records a fact about the code.
> It is not a change to the decision above: the amendment's normative text is untouched and
> the decision remains open.
>
> As shipped, a `challenge` finding — like any REVIEW or BLOCK — posts the resolvable thread
> and the run's summary comment, then stops. Nothing is approved, nothing is armed with the
> forge for later, and resolving the thread merges nothing. Verified against the code:
> `buildDesired`'s REVIEW/BLOCK branch returns a zero-valued `forge.Preconditions{}` (no
> `Approve`, no `Merge`, no `ArmEligible`); the `forge.Forge` port has **no deferred-merge
> verb at all** — `Approve` and `MergeCAS` are its only write verbs, and `MergeCAS` issues an
> immediate SHA-pinned `PUT .../merge?sha=`; and `Thread.Resolved` is read only by assent's
> own thread-idempotence logic, never by the decision engine, so thread resolution is not
> evidence. Measured end-to-end: a REVIEW run, followed by resolving every thread and running
> again, leaves the decision REVIEW with zero approvals and zero merges. An MR merges only on
> a later run that reaches APPROVE on its own inputs, and **assent**, not the forge, performs
> that merge.
>
> Consequently point 1's "the forge, not assent, enforces resolution" mechanism, and point 2's
> compensating "any new push cancels the armed merge", describe a design with no counterpart
> in the tool today.
>
> **Two honest resolutions are open and this note picks NEITHER** — choosing is an
> architecture decision for the operator, tracked as an open item in
> [D-135](../decisions/decisions.md): **(a)** build the deferred arming this amendment
> specifies (GitLab C11 — `PUT .../merge` with `auto_merge=true` combined with `sha=`, per the
> forge dossier), or **(b)** retract the amendment and re-derive the challenge-resolution
> story around the immediate-merge behaviour that exists. Until one is chosen,
> [the walkthrough](../usage/walkthrough.md) Step 6 is the accurate description of shipped
> behaviour.

## Amendment 2 (2026-07-21, second review P2-12): scan honesty

`scan` also records each historical MR's **actual outcome** (merged / closed / reverted) and
`stats` reports the decision-vs-outcome confusion matrix — "61% would-have-automerged" only
proves self-consistency; "of the MRs we would have automerged, how many did humans merge
unchanged?" is the number that earns trust. Batch *apply* over past MRs stays out of scope
(recorder-only; OQ-20).
