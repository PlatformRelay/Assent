# Imagined walkthrough — adopting assent on a topic registry

> **This is design fiction.** Nothing below is implemented; it exists so we can *feel* the
> UX before freezing contracts (meta-plan Phase 3). Where a command or field survives review,
> it becomes a spec REQ. Config semantics: [ADR-0010](../adr/0010-config-files-repo-layout.md);
> effects: [ADR-0007](../adr/0007-rule-effects-decision-aggregation.md).

## The repo

`topic-registry` — one YAML file per Kafka topic:

```yaml
# topics/prod/orders.yaml
name: orders
owner: team-orders
partitions: 12
retentionMs: 604800000
```

Today every MR waits for a platform engineer. Goal: routine changes merge themselves.

## Step 1 — init (5 min)

```console
$ assent init --sample topic-registry
created .assent/config.yaml        (environments: prod/dev by path; class: kafka-topic)
created .assent/bindings.yaml      (kafka-topic -> pack "topics", thresholds dev=10 prod=4)
created .assent/packs/topics/      (starter rules: ownership, bounded-change, no-deletion)
created .assent/tests/topics/      (passing fixtures for every starter rule)
next: assent test && assent scan --since 90d
```

## Step 2 — make a rule yours

Edit the starter pack (`.assent/packs/topics/rules/safety.yaml`), e.g. cap partitions via
your quota provider and challenge retention shrinks — see the full rule file example in
ADR-0010. Wire your company's permission source in `config.yaml`:

```yaml
providers:
  author: { type: builtin/gitlab-groups }        # swap for http/exec/grpc — ADR-0004
```

## Step 3 — test the policy like code

```console
$ assent test
PACK topics
  ✓ partition-increase-ok            APPROVE            (2 vouched, score 1/10)
  ✓ partition-decrease-challenged    REVIEW: challenge  retention-shrink-challenge
  ✓ topic-delete-blocked             BLOCK              no-topic-deletion
  ✗ foreign-topic-edit               expected REVIEW, got APPROVE
      ownership: facts.author.groups fixture lists team-orders; entry owner is team-billing
      -> did you mean expect: REVIEW (uncovered)?  see .assent/tests/topics/foreign-topic-edit/
4 fixtures, 3 passed, 1 failed
```

## Step 4 — backtest before trusting it

```console
$ assent scan --since 90d --out reports/
scanned 214 MRs (2026-04-22..2026-07-21)
$ assent stats reports/
outcome      count   %          top rules firing
APPROVE        131   61%        bounded-change/partition-increase (88)
REVIEW          71   33%        retention-shrink-challenge (24), uncovered-change (31)
BLOCK           12    6%        no-topic-deletion (12)
would-have-automerged: 61% · median score 2 · 0 nondeterministic re-runs
⚠ facts resolved live (today), not as of each MR — treat the % as an estimate
```

61% automerge on day one, and the 12 blocks are all real topic deletions. Ship it.

## Step 5 — wire CI (GitLab first)

```yaml
# included from a PROTECTED source (compliance pipeline / protected include) — the assent
# job definition must not be editable from the MR branch (ADR-0015 §4); `assent doctor`
# verifies this and the required forge settings (all-threads-resolved merge gate) at setup.
# One-publisher-per-MR (ADR-0019 §3): resource_group keyed per MR IID serializes concurrent
# assent jobs for the same MR. Without it, duplicates converge only on the next reconcile.
# (serve mode: use a keyed per-MR lock instead — multi-replica HA is unsupported.)
assent:
  image: ghcr.io/<org>/assent:v0
  rules: [{ if: $CI_MERGE_REQUEST_IID }]
  resource_group: assent-mr-$CI_MERGE_REQUEST_IID
  script: [assent run]        # MR context from CI env; least-privilege token from CI variable
```

## Step 6 — the contributor experience

A dev bumps `partitions: 12 -> 24` on their own topic in dev: pipeline runs, the MR gets a
summary comment ("APPROVE — 1 change vouched, score 1/10"), approval, and merges. Nobody was
interrupted.

The same dev shrinks retention on a prod topic: assent opens a **resolvable thread** —
headline message, then collapsible *"Why this check exists & how to fix"* (with the rule's
`docs.url`) and *"Evaluation details"* sections ([ADR-0012](../adr/0012-presentation-templates-debug.md)).
They resolve the thread ("intentional, ticket TOPIC-123"). assent had already armed the
forge's auto-merge, pinned to the evaluated commit — so the moment the last thread is
resolved, **GitLab itself** merges (ADR-0009 amendment). Any new push cancels that and
re-evaluates from scratch; the policies that judged this MR came from the *target* branch,
so nobody can weaken the rules in the MR they gate (ADR-0015).

Something weird? Anyone can ask locally:

```console
$ assent explain --mr 481
change topics/prod/orders.yaml /retentionMs modify 604800000 -> 86400000
  class kafka-topic · env prod · binding -> packs [topics, topics-strict] threshold 4
  ✓ matched retention-shrink-challenge   assert "new < old" = true -> effect challenge
  ✗ not matched partition-increase      (path mismatch)
aggregation: no block · 1 unresolved challenge -> REVIEW
```

## What this walkthrough commits us to

`init` with runnable samples · fixture tests with decision-level asserts and helpful failure
hints · `scan`/`stats` for evidence-based rollout · one-line CI install · rendered comments
with expandable docs/debug · `explain` that answers "why" without reading Go code.
