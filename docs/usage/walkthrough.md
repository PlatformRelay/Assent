# Walkthrough — adopting assent on a topic registry

> **Mixed status — read the per-step banner.** This page began life as a UX sketch
> written before any code existed. Most of it now describes the shipped v0.1.0 binary; the steps
> that still describe unbuilt commands are labelled **Planned**, and each one names what
> you can do today instead. The authority for what exists is
> [the CLI reference](cli.md), which is pinned byte-for-byte to `assent --help`.
> Config semantics: [ADR-0010](../adr/0010-config-files-repo-layout.md);
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

## Step 1 — scaffold a policy tree

> **Planned — `assent init` does not exist.** There is no scaffolding subcommand in the
> shipped binary. Copy a starter pack instead; the shipped ones are complete, linted, and
> covered by their own tests.

```bash
git clone https://github.com/PlatformRelay/assent
cp -R assent/examples/packs/topic-registry/.assent .assent
assent lint .
```

That gives you `.assent/config.yaml` (environments prod/dev by path, class
`kafka-topic`), `.assent/bindings.yaml`, `.assent/packs/topics/` (ownership,
bounded-change, non-destructive, schema-valid) and `.assent/tests/topics/` with a passing
fixture for every rule — `assent lint` rejects a rule with no test case, so the two ship
together by construction.

Committed starter packs: [`examples/packs/`](https://github.com/PlatformRelay/assent/tree/main/examples/packs)
(`topic-registry`, `service-catalog`, `infra-vars`).

## Step 2 — make a rule yours

> **Shipped.** Policy authoring is the frozen `assent.dev/v1alpha1` surface; `assent lint`
> is the gate over it.

Edit the starter pack (`.assent/packs/topics/rules/bounded-change.yaml`), e.g. cap
partitions via your quota provider and challenge retention shrinks — see the full rule
file example in ADR-0010. Wire your company's permission source in `config.yaml`:

```yaml
providers:
  author: { type: builtin/gitlab-groups, failure: closed }   # http/exec too — ADR-0004
```

Re-run `assent lint .` after every edit: unknown fields, unknown enums, duplicate
collection IDs, a rule without a test, and an unsupported `match.fileEvents` kind are all
hard errors, not warnings.

## Step 3 — test the policy like code

> **Shipped** — `assent test <repo>` runs `.assent/tests/**` against the real engine.

```console
$ assent test .
PASS topics/bounded-change (APPROVE)
PASS topics/bounded-change/negative (REVIEW)
PASS topics/ownership (APPROVE)
PASS topics/ownership/negative (REVIEW)
PASS topics/schema-valid (APPROVE)
PASS topics/schema-valid/negative (BLOCK)
PASS topics/non-destructive (APPROVE)
PASS topics/non-destructive-delete (REVIEW)
```

That is real output from the shipped starter pack. Each case is a directory with
`base/`, `head/`, `facts.yaml` and `expect.yaml` (or an inline `cases.yaml` entry); a
failing case prints the expected and actual decision plus the findings that differ.
`--update` rewrites expectations from the produced actuals (refused when `CI` is set),
`--coverage` is the read-only both-polarity completeness gate. Exit `0` every case
matched; `1` a mismatch, write or load error; `2` usage, discovery, or the CI guard
refusing `--update`.

## Step 4 — backtest before trusting it

> **Planned — `assent scan` and `assent stats` do not exist.** There is no historical
> backtest over closed MRs in v0.1.0, and no "would-have-automerged %" report.

What ships instead is **`assent compare`**: replay a comparison suite of a *baseline*
versus a *candidate* policy over a fixed corpus and apply the promotion gates, so you can
see what a policy change would do before you enforce it.

```bash
assent compare --suite compare-suite/
```

Use it the way you would use a backtest for *policy edits*. For first-adoption evidence
on live MRs, set the pack's rollout `phase: observe` (ADR-0018) — rules evaluate and land
in `findings.observed`, structurally excluded from the decision — and read the emitted
`DecisionRecord`s rather than expecting a scan report.

## Step 5 — wire CI (GitLab first)

> **Shipped** — `assent run` is the CI entry point. One caveat below: **no container
> image is published**, so the job installs the binary.

```yaml
# included from a PROTECTED source (compliance pipeline / protected include) — the assent
# job definition must not be editable from the MR branch (ADR-0015 §4); `assent doctor`
# verifies this and the required forge settings (all-threads-resolved merge gate) at setup.
# One-publisher-per-MR (ADR-0019 §3): resource_group keyed per MR IID serializes concurrent
# assent jobs for the same MR. Without it, duplicates converge only on the next reconcile.
assent:
  image: golang:1.25            # no ghcr.io/…/assent image is published yet — install in-job
  rules: [{ if: $CI_MERGE_REQUEST_IID }]
  resource_group: assent-mr-$CI_MERGE_REQUEST_IID
  before_script:
    # Or fetch + checksum-verify a release archive — see the install guide.
    - go install github.com/PlatformRelay/assent/cmd/assent@v0.1.0
  script:
    - assent run --project "$CI_PROJECT_ID" --mr "$CI_MERGE_REQUEST_IID"
        --subject "file:$SUBJECT" --bot-author "$ASSENT_BOT"
```

`GITLAB_TOKEN` comes from a masked CI variable and is never a flag. Run `assent doctor`
once during setup: it reports whether this environment can arm auto-merge and why not
when it cannot. See [Install](install.md) for the checksum-verified archive route, and
[CLI reference](cli.md) for every `assent run` flag.

## Step 6 — the contributor experience

> **Shipped**, except the `assent explain` block at the end of this section.

A dev bumps `partitions: 12 -> 24` on their own topic in dev: pipeline runs, the MR gets a
summary comment ("APPROVE — 1 obligation proved, score 1/10"), approval, and merges. Nobody was
interrupted.

The same dev shrinks retention on a prod topic: assent opens a **resolvable thread** —
headline message, then collapsible *"Why this check exists & how to fix"* (with the rule's
`docs.url`) and *"Evaluation details"* sections ([ADR-0012](../adr/0012-presentation-templates-debug.md)).
They resolve the thread ("intentional, ticket TOPIC-123"). assent had already armed the
forge's auto-merge, pinned to the evaluated commit — so the moment the last thread is
resolved, **GitLab itself** merges (ADR-0009 amendment). Any new push cancels that and
re-evaluates from scratch; the policies that judged this MR came from the *target* branch,
so nobody can weaken the rules in the MR they gate (ADR-0015).

> **Planned — `assent explain` does not exist.** Today the same information is in the
> `DecisionRecord` that every run emits (`assent run --emit record.json`): matched and
> unmatched rules, obligation results, and the aggregation that produced the decision.
> The sketch below is the intended local ergonomics, not a shipped command.

```console
$ assent explain --mr 481
change topics/prod/orders.yaml /retentionMs modify 604800000 -> 86400000
  class kafka-topic · env prod · binding -> packs [topics, topics-strict] threshold 4
  ✓ matched retention-shrink-challenge
      prove: {obligation: bounded-change, when: "new < old"} = true
      onFailure: {effect: challenge, code: bounded-change.out-of-band}
  ✗ not matched partition-increase
      (valueChanges pointer /partitions did not match this change)
aggregation: no block · 1 unresolved challenge -> REVIEW
```

## Status summary

| Step | Command | Status |
| --- | --- | --- |
| 1 — scaffold | `assent init` | **Planned** — copy `examples/packs/<sample>/.assent` |
| 2 — author | `assent lint <repo>` | **Shipped** |
| 3 — test | `assent test <repo>` | **Shipped** |
| 4 — backtest | `assent scan` / `assent stats` | **Planned** — `assent compare` covers policy-change replay |
| 5 — CI | `assent run` · `assent doctor` | **Shipped** (no published container image) |
| 6 — explain | `assent explain` | **Planned** — read the emitted `DecisionRecord` |

The full dispatched command set — including `catalogue`, `render`, `eval-input` and
`version` — is in the [CLI reference](cli.md).
