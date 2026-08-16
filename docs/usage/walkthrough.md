# Walkthrough — adopting assent on a topic registry

> **Mixed status — read the per-step banner.** This page began life as a UX sketch
> written before any code existed. Most of it now describes the shipped v0.1.0 binary; the steps
> that still describe unbuilt commands are labelled **Planned**, and each one names what
> you can do today instead. The authority for what exists is
> [the CLI reference](cli.md), which is pinned byte-for-byte to `assent --help`.
> Config semantics: [ADR-0010](../adr/0010-config-files-repo-layout.md);
> effects: [ADR-0007](../adr/0007-rule-effects-decision-aggregation.md).

## The repo

`topic-registry` — one YAML file per Kafka topic, map-at-root keyed by topic name.
Trimmed from a real shipped fixture
(`examples/packs/topic-registry/.assent/tests/topics/wildcard-grant/head/topics/prod/billing.invoices.v1.yaml`):

```yaml
# topics/prod/billing.invoices.v1.yaml
billing.invoices.v1:
  owner: billing-team
  partitions: 6
  replication_factor: 3
  retention_hours: 168
  schema:
    format: avro
    subject: billing.invoices.v1-value
    compatibility: BACKWARD
  acl:
    grants:
      billing-team: read
      finance-team: read
```

Today every MR waits for a platform engineer. Goal: routine changes merge themselves.

`partitions`, `replication_factor` and `retention_hours` are scalar fields; `acl.grants`
and (on other topics) `consumers` are nested maps. The starter packs also govern nested
JSON objects, nested tfvars maps, and — opaquely, falling back to REVIEW — HCL `.tf`
blocks. Step 3 below is real `assent test` output across all four.

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
`kafka-topic`), `.assent/bindings.yaml`, `.assent/packs/topics/rules/` — nine rule files
today (`ownership`, `bounded-change`, `non-destructive`, `schema-valid`,
`schema-compatibility`, `list-no-shrink`, `wildcard-grant`, `soft-delete`,
`referenced-resource-ownership`) — and `.assent/tests/topics/` with a passing fixture for
every rule — `assent lint` rejects a rule with no test case, so the two ship together by
construction.

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
PASS topics/list-no-shrink (APPROVE)
PASS topics/list-no-shrink/negative (REVIEW)
PASS topics/ownership (APPROVE)
PASS topics/ownership/negative (REVIEW)
PASS topics/quota-ceiling (APPROVE)
PASS topics/quota-ceiling/facts-omitted (REVIEW)
PASS topics/quota-ceiling/negative (REVIEW)
PASS topics/resource-ownership (APPROVE)
PASS topics/resource-ownership/negative (REVIEW)
PASS topics/schema-compatibility (APPROVE)
PASS topics/schema-compatibility/negative (REVIEW)
PASS topics/schema-valid (APPROVE)
PASS topics/schema-valid/negative (BLOCK)
PASS topics/soft-delete (APPROVE)
PASS topics/soft-delete/negative (REVIEW)
PASS topics/soft-delete/unrelated-modify (APPROVE)
PASS topics/wildcard-grant (APPROVE)
PASS topics/wildcard-grant/negative (BLOCK)
PASS topics/non-destructive (APPROVE)
PASS topics/non-destructive-delete (REVIEW)
```

That is real output from the shipped `topic-registry` starter pack — **YAML** with nested
`acl.grants` and keyed `consumers` maps, not a single flat field. Each case is a directory
with `base/`, `head/`, `facts.yaml` and `expect.yaml` (or an inline `cases.yaml` entry); a
failing case prints the expected and actual decision plus the findings that differ.
`--update` rewrites expectations from the produced actuals (refused when `CI` is set),
`--coverage` is the read-only both-polarity completeness gate. Exit `0` every case
matched; `1` a mismatch, write or load error; `2` usage, discovery, or the CI guard
refusing `--update`.

`topic-registry` is one of **four** governed formats — three of them structurally
diffed, with `.tf` governed only as an opaque whole (below):

```console
$ assent test examples/packs/service-catalog
PASS catalog/allowed-fields (APPROVE)
PASS catalog/allowed-fields/negative (REVIEW)
PASS catalog/context-fresh (APPROVE)
PASS catalog/context-fresh/negative (REVIEW)
PASS catalog/nested-fields (APPROVE)
PASS catalog/nested-fields/negative (REVIEW)
PASS catalog/non-destructive (APPROVE)
PASS catalog/non-destructive/negative (REVIEW)
PASS catalog/ownership (APPROVE)
PASS catalog/ownership/negative (REVIEW)
PASS catalog/privilege-tier (REVIEW)
PASS catalog/privilege-tier/negative (REVIEW)
PASS catalog/schema-valid (APPROVE)
PASS catalog/schema-valid/negative (BLOCK)
PASS catalog/unkeyed-list-opaque (REVIEW)
PASS catalog/file-non-destructive (APPROVE)
PASS catalog/file-non-destructive-delete (BLOCK)
```

`service-catalog` is **JSON** with nested objects (`catalog/nested-fields`) and a tier
allow-list (`catalog/privilege-tier`). `catalog/unkeyed-list-opaque` is a measured, not
wished-for, limit: an *unkeyed* list is opaque to the differ by design (D-061), so a
change inside one falls back to REVIEW rather than a false-precision partial diff.

```console
$ assent test examples/packs/infra-vars
PASS vars/bounded-change (APPROVE)
PASS vars/bounded-change/negative (REVIEW)
PASS vars/max-replicas-change (APPROVE)
PASS vars/max-replicas-change/negative (REVIEW)
PASS vars/min-replicas-change (APPROVE)
PASS vars/min-replicas-change/negative (REVIEW)
PASS vars/nested-map-change (APPROVE)
PASS vars/nested-map-change/negative (REVIEW)
PASS vars/ownership (APPROVE)
PASS vars/ownership/negative (REVIEW)
PASS vars/placement (APPROVE)
PASS vars/placement/negative (REVIEW)
PASS vars/tf-opaque (REVIEW)
PASS vars/companion-delete (REVIEW)
```

`infra-vars` is **tfvars** — keyed `workloads.*` maps, including the deeper
`vars/nested-map-change` case. `vars/tf-opaque` and `vars/companion-delete` are the two
honest edges of the current differ, both pinned as expected **REVIEW**, never a silent
APPROVE:

- **`.tf` is governed but not structurally diffed.** The differ only routes the
  `.tfvars` extension to the HCL parser; a `.tf` file's content — blocks or bare literals
  alike — is opaque and falls back to REVIEW, never a partial parse. Assent does **not**
  understand Terraform expressions or resource blocks; it treats a whole changed `.tf`
  file as one un-provable unit. This is a permanent v1 limitation, not a bug — see
  `examples/README.md`.
- **A companion file outside the pack's class match** (e.g. a `NOTES.md` next to the
  `*.tfvars` files) deleted alongside a real change is caught only by the class-agnostic
  unmatched-whole-file-delete fail-safe — REVIEW, no obligation attached. v1 does not
  correlate "delete A and append B" across files; that is out of engine scope today.

These packs also carry the **REF-EX C1–C8** governance patterns: keyed-map entry removal
(C1, `topics/list-no-shrink`), a tier allow-list (C2, `catalog/privilege-tier`),
wildcard-grant blocking (C3, `topics/wildcard-grant`), soft-delete-as-field-add (C4,
`topics/soft-delete`), a fact-derived quota ceiling (C5, `topics/quota-ceiling`), a
placement allow-list (C6, `vars/placement`), referenced-resource ownership (C7,
`topics/resource-ownership`), and the companion-file-delete REVIEW above (C8,
`vars/companion-delete`) — all runnable today from
[`examples/packs/`](https://github.com/PlatformRelay/assent/tree/main/examples/packs).

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
    # Simplest route, shown here to keep the example self-contained. It stamps the
    # binary `0.0.0-dev`, so every DecisionRecord this job emits carries that in
    # `pins.toolVersion` (`pins.toolDigest` still identifies the build, D-120).
    # For a record that names the real tag, install the checksum-verified release
    # archive instead — the full URL pattern is in the install guide.
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

> **Shipped**, with two exceptions stated where they arise below. The deferred forge
> auto-merge that [ADR-0009](../adr/0009-execution-modes.md)'s challenge-resolution
> amendment specifies — "approves conditionally and arms forge auto-merge pinned to the
> evaluated SHA" — is **not implemented**: assent posts the thread and stops. And
> `assent explain`, at the end of this section, does not exist.

A dev bumps `partitions: 12 -> 24` on their own topic in dev: pipeline runs, the MR gets a
summary comment ("APPROVE — 1 obligation proved, score 1/10"), approval, and merges. Nobody was
interrupted.

The same dev shrinks retention on a prod topic: assent opens a **resolvable thread** —
headline message, then collapsible *"Why this check exists & how to fix"* (with the rule's
`docs.url`) and *"Evaluation details"* sections ([ADR-0012](../adr/0012-presentation-templates-debug.md)),
alongside the run's summary comment. On that path assent does **not** approve, does **not**
merge, and arms **nothing** with the forge for later — there is no deferred-merge call in the
tool at all; its only merge verb is an immediate SHA-pinned one.

They resolve the thread ("intentional, ticket TOPIC-123"). **Resolving it merges nothing by
itself.** It does matter to GitLab — with the project's *all discussions resolved* merge gate
enabled (the C3 setting assent requires before it will arm at all), an unresolved thread
blocks merging on GitLab's side — but assent never reads thread state as evidence, and, as
ADR-0009's own amendment notes, forges do not start a pipeline when a discussion is resolved.
So nothing re-runs until the next push.

When a run does happen it re-evaluates from scratch, and the decision moves only if the
*inputs* moved: an amended change, a policy edit on the target branch, fresher facts, or
forge-proven approval evidence from the approval-rules API. Thread resolution is not one of
them. If that run decides APPROVE and the arming preconditions hold, **assent** performs the
merge itself, immediately, pinned to the commit it just judged — so only the evaluated commit
can merge (ADR-0015 §2), and a push that lands mid-run makes the SHA guard refuse. The
policies that judged the MR came from the *target* branch, so nobody can weaken the rules in
the MR they gate (ADR-0015).

> **What this means in practice:** an MR parked on a `challenge` finding does not merge when
> the last thread is resolved. Someone has to push, or re-run the pipeline, and that run has
> to reach APPROVE on its own inputs. If you are waiting on a merge that never arrives, this
> is why.

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
