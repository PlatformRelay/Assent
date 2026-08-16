# P5-EX — Complex in-tree examples, adopter tests, and docs truth

**Epic ID / REQ prefix:** `EX` / `REQ-EX-S0n-nn`.

**Origin:** operator instruction (2026-08-15) — better and more *complex* examples, tests, and
documentation (multi-field nested resources; YAML + JSON + HCL + tfvars), not a single-field
YAML happy path. Recorded as **D-143**. The engine already has E1 format adapters; the
*examples / tests / docs* are the thin layer.

**Vehicle:** extend `examples/packs/`, `examples/repos/` (layouts only), `examples/archetypes/`,
existing `assent test` fixtures, and product docs under `docs/`. Close **REF-EX C1–C8** where
they fit existing archetypes. Do **not** invent a parallel example system.

---

## Problem

Shipped packs (`examples/packs/{topic-registry,service-catalog,infra-vars}`) are dogfood-green
under `assent test` / `--coverage`, but they are thin:

- **topic-registry (YAML):** `schema{}` exists (`format`, `subject`) but no rule matches nested
  pointers; `schema-valid` is a fact stub (`facts.schema.valid.value`), not a nested-field proof.
- **service-catalog (JSON):** pack fixtures are flat `{name, owner, tier, oncall}`. Nested
  `endpoints` / `tags` were **stripped** (D-061): unkeyed nested lists make the differ opaque.
  `examples/repos/service-catalog` still shows those lists; the *pack* does not govern them.
- **infra-vars (tfvars):** keyed object `workloads.*` with scalar `min_replicas` / `max_replicas`
  / `memory_mb`. No deeper nested maps. **No `.tf` pack.** Class match is `envs/**/*.tfvars` only
  (`examples/packs/infra-vars/.assent/config.yaml`).
- **HCL:** E1-S04 is **literal-only**. `parseHCL` (`internal/change/diff_hcl.go`) refuses HCL
  *blocks* (`resource` / `module`) as not-tfvars, fail-safe opaque, **zero partial changes**
  (`TestHCLStructuralGuardsFailSafe`). tfvars attribute objects **are** structured. There is no
  adopter-visible fixture that demonstrates the `.tf` opaque path; infra-vars README comments
  the fallback but does not run it.
- **Dogfood wiring (DEM-S12 finding, still true):** `task dogfood-examples` exists and is **not**
  a `task check` stage. CI `verify.yaml` hardcodes the same three-pack loop. `greenExamplePacks`
  in `cmd/assent/test_corpus_test.go` pins the names. Adding a pack requires three edits that
  can skew.
- **Docs:** `docs/usage/walkthrough.md` still demos a one-file flat topic; `examples/README.md`
  lists three packs and does not claim HCL. AUD-S06 truth-lag gates exist (`task docs-gates`)
  but do not pin format coverage.

`examples/repos/**` remain **layouts without `.assent/` trees** (D-142 ground truth). This epic
does not turn them into demo repos.

---

## Why not P5-DEM (D-143)

**P5-DEM** (`openspec/specs/p5-dem-demo-repos/spec.md`, D-142) is a *different goal*: public
org demo repos, **(class, environment) routing (DEM-S00, engine-grade)**, provider brokers,
operator-gated publish (DEM-S13). Its REQs have **zero** `Test:` / `Verify:` / `Level:`
annotations — that is why this operator ask is **not** implemented as DEM.

EX **steals** DEM's *honest HCL truth* (DEM-S08/S10: determine unmatched / opaque behaviour by
running, do not invent a parser) and DEM-S12's *wiring* (edit both `Taskfile.yml` and
`verify.yaml`) without demo-repo scope, without DEM-S00, and without publishing.

---

## Non-goals

- **Do not re-spec or implement P5-DEM.** Leave the DEM table in the backlog.
- **Do not invent a parallel example system** (no `examples/demo/**`, no public org repos).
- **Do not duplicate E1 adapters.** No new HCL parser; no expression evaluation.
- **Do not start E10 / E11 / P5-SEC-SC / AUD2.**
- **Do not wire DEM-S00** `(class, environment)` routing. Packs stay **single-class**.
  Multi-class in one pack fails closed today (`selectBinding` / `selectBindingForTest`).
- **Do not call live providers.** `assent test` stays `facts.yaml`-stubbed (ADR-0014).
- **Do not change `internal/core`.** Engine/decision-path changes only if a fixture cannot run
  without a one-line harness *discovery* fix (prefer fixture/docs).
- **Schema freeze is ref-relative (AUD-S18 / D-132):** `git diff --name-status v0.1.0 -- schemas` over `*.json`, **not** a working-tree `git diff schemas/` (that is silent on committed edits — PCS vacuity). EX adds no schema JSON beyond the AUD-S18 permitted description-string. Base ref is the release tag; substituting `HEAD` reddens.
- **Do not raise `COVERAGE_MIN`** (91%, D-010/D-128). Example-only lanes that add no
  `internal/` Go: coverage is N/A.
- **No live GitLab, public org repos, or PAT rotation.** Park as non-goals / operator-gated
  (none expected).
- **D-002:** generic invented names only (orders-team, payments-gateway, …). No employer
  names, internal systems, or verbatim private material.
- **Do not re-introduce unkeyed nested lists** into service-catalog pack fixtures (D-061).
  Nested **objects / maps** and identity-keyed collections only.

---

## ADRs and reuse

**ADRs:** 0003 (opaque→REVIEW; HCL literal-only caveat), 0006 (dogfood examples in CI), 0008
(unclassified / unmatched), 0010 (repo layout), 0014 (`assent test` / `--coverage` / facts
stubs), 0017 §5 (EntryRef, class-slice `changes`). **D-061** (nested-list opacity), **D-063**
(unmatched whole-file delete → REVIEW), **D-071** (C5–C7 demonstrators deferred to this
authoring lane), **D-086** (`verify.yaml` is CI superset; this epic *does* add dogfood to
`task check` — a deliberate local-gate extension, logged in judgment (a)).

**Reuse:** `change.Diff` / `parseHCL`, `assent test` / `assent lint` / `--coverage`,
`greenExamplePacks` + `TestDogfoodScriptsIncludeGreenExamplePacks`, `task docs-gates` /
AUD-S06 pin-script pattern, `hack/check-sanitization.sh`.

---

## Judgment calls (decide-and-log)

**(a) Pack discovery vs hardcoded list — DECIDED: discover, pin the contract.**
Runtime dogfood walks `examples/packs/*/` directories that contain `.assent/tests/` (skip
dirs without adopter tests; never pick up `testdata/broken-pack`). A **shared script**
(`hack/dogfood-examples.sh`) is the single loop `task dogfood-examples` *and* `verify.yaml`
call — no forever-hardcoded three-name `for pack in …` in two files. `greenExamplePacks` in
`cmd/assent/test_corpus_test.go` becomes a **filesystem walk of the same glob**, with a pin
that Taskfile + verify.yaml invoke the script (not a copy of the names). Adding a pack with
`.assent/tests/` automatically enters dogfood; if it is not green, CI fails (desired). A
directory under `examples/packs/` *without* tests is a hard error of the inventory gate
(S01), not silently skipped — packs are complete adopter trees.

**(b) C7 location — DECIDED: topic-registry, not a new pack.**
Referenced-resource ownership is an optional nested `acl` object on the existing
`kafka-topic` class. A new pack would be a fourth tree for a field the topic shape can host.
A second class in the same pack would fail closed (DEM-S00). The C7 rule matches
`topics/**/*.yaml` and proves when `!has(entry.acl) || facts.resource_owner.owner.value == entry.acl.owner`
(vacuous for existing topics without `acl`), so current goldens stay green. Dedicated
archetype seed: `examples/archetypes/referenced-resource-ownership/` (D-071). **Do not** put
C7 in a multi-class ACL pack.

**(c) `.tf` opaque demo — DECIDED: case under infra-vars, not a fourth pack.**
`infra-vars` is tfvars-classed today; a `.tf` file in that pack with the current class match
is **ungoverned** (unclassified → fail-safe REVIEW) and would **not** exercise `parseHCL`'s
block guard. EX-S05 **extends the single class** `infra-vars` match paths to
`["envs/**/*.tfvars", "envs/**/*.tf"]` so a `resource`/`module` block is *parsed*, goes
opaque, and yields REVIEW with **zero partial changes**. Still one class; existing tfvars
fixtures do not include `.tf` files. **Determine** the adopter-test decision by running
`assent test`, do not pre-assert a fantasy (DEM-S08/S10 honesty). Document the known
limitation next to the fixture.

**(d) Coverage floor — DECIDED: do not raise.** Example-only lanes may add no `internal/`
Go. If a lane adds no production Go, D-010 is N/A. `cmd/assent` tests (discovery walk) sit
outside the `internal/…` denominator (TEST-04 / D-132).

**(e) `task check` vs D-086 — DECIDED: add `dogfood-examples` to `task check`.**
D-086 said verify is the CI superset and `task check` stays local fmt/vet/lint/test/coverage/build.
DEM-S12 and this operator ask contradict that for *example dogfood*: a pack that is only
green in CI (or only via a manual `task dogfood-examples`) rots locally. EX-S08 adds
`task: dogfood-examples` as a sequential `check:` stage (same pattern as `docs-gates`,
D-124 — not a parallel `deps:` entry, because dogfood `build`s a binary). D-086's "do not
fold verify-only steps into check" still holds for gitleaks/govulncheck/e2e-vet; dogfood is
an adopter-facing local gate, not a verify-only scan.

---

## Executability

**Every story `[autonomous]`.** No network, no forge, no PAT, no live provider. Facts from
authored `facts.yaml` → resolved envelope. D-002 sanitization on every new fixture.
TDD: a fixture/rule that is deleted must redden `assent test` / `assent test --coverage` /
`assent lint` (non-vacuity). Every REQ has `Test:` / `Verify:` / `Level:`.

**Dependency order:** **S01** (inventory paper-gate) → **S08** (wire dogfood into `task check`
+ shared discovery; startable after S01, valuable immediately) ∥ **S02 / S03 / S04** (thicken
three packs, file-disjoint) → **S05** (HCL honesty on infra-vars, after S04 so nested tfvars
goldens exist) → **S06** (C1–C4; after S02/S03 so nested fields exist to hang rules on) →
**S07** (C5–C8; after S04/S06) → **S09** (walkthrough byte-pinned to real CLI output) →
**S10** (exit gate). **Do first: S01.**

---

## Story index

| ID | Story | Execution | Depends on | Gate contribution |
| --- | --- | --- | --- | --- |
| EX-S01 | Docs + format-coverage inventory (paper-gate, both polarities) | **[autonomous]** | none | **do first** — claims cannot outrun dogfood |
| EX-S02 | Thicken topic-registry (YAML nested multi-field + nested-pointer rules) | **[autonomous]** | none | YAML complexity; nested pointers both polarities |
| EX-S03 | Thicken service-catalog (JSON nested objects/maps; D-061-safe) | **[autonomous]** | none | JSON complexity; nested fields both polarities |
| EX-S04 | Thicken infra-vars (tfvars deeper nested maps) | **[autonomous]** | none | tfvars complexity; nested maps both polarities |
| EX-S05 | HCL honesty: structured tfvars + `.tf` block → opaque → REVIEW | **[autonomous]** | S04 | fourth format represented; known-limitation fixture |
| EX-S06 | REF-EX C1–C4 in-tree fixtures | **[autonomous]** | S02, S03 | list-no-shrink, privilege-tier, wildcard-grant, soft-delete |
| EX-S07 | REF-EX C5–C8 (facts stubs + C8 known-limitation REVIEW) | **[autonomous]** | S04, S06 | closes D-071 demonstrators; C8 documented REVIEW |
| EX-S08 | Discover packs; wire `dogfood-examples` into `task check` + verify.yaml | **[autonomous]** | S01 | DEM-S12 wiring without demo-repo scope |
| EX-S09 | Product docs walkthrough byte-pinned to real complex-case CLI output | **[autonomous]** | S02–S07 | AUD-S06-style truth for the new surface |
| EX-S10 | Exit gate: four formats, C1–C8, docs gates, schemas frozen | **[autonomous]** | S01–S09 | **the EX exit gate** |

---

## EX-S01 — Docs + format-coverage inventory paper-gate [autonomous]

**As a** maintainer **I want** a gate that fails when `examples/README.md` or product docs
claim a pack or input format that is not dogfooded — and when a dogfooded pack is missing
from those docs **so that** examples that don't run cannot be documented as if they do.

**Goal:** a both-polarity inventory script (AUD-S06 pin pattern) that (1) enumerates
`examples/packs/*` directories containing `.assent/`; (2) records each pack's governed
format from class `match.paths` extensions (`.yaml` / `.json` / `.tfvars` / `.tf`); (3)
asserts `examples/README.md` names **exactly** those pack directory names; (4) asserts every
claimed format in the README format sentence is actually present on a dogfood pack; (5)
fails if a pack directory exists without `.assent/tests/` (incomplete tree). Wire into
`task docs-gates` so `task check` runs it once S08 lands; until then the script is invoked
from `docs-gates` in this story (docs-gates already runs on check — adding the script there
is the paper-gate, not the pack-loop wiring).

**Operator input:** none (D-143).

**Dependencies:** none. **Do first.**

**Definition of done:** flipping README to claim `cue` or a pack `kafka-acl` reddens the
script; deleting `topic-registry` from the README pack list while the directory exists
reddens the other polarity; `hack/check-sanitization.sh` still green.

**Not in scope:** thickening fixtures (S02–S05); C-series (S06–S07); changing the dogfood
loop (S08); HCL claims (README must **not** claim `.tf` / HCL until S05 adds the fixture and
updates the sentence).

Requirements:

- **REQ-EX-S01-01** — Given the three shipped packs, when the inventory script runs, then it
  exits 0 and reports packs `infra-vars`, `service-catalog`, `topic-registry` with formats
  yaml / json / tfvars (no `.tf` yet).
  - Test: `hack/docs/example_format_inventory_test.sh`
  - Verify: `bash hack/docs/example_format_inventory_test.sh`
  - Level: L1
- **REQ-EX-S01-02** — Given `examples/README.md` names a pack directory that does not exist
  under `examples/packs/`, when the script runs, then it exits non-zero (claimed-but-missing
  polarity).
  - Test: `hack/docs/example_format_inventory_test.sh` (mutation: extra name in a temp copy)
  - Verify: `bash hack/docs/example_format_inventory_test.sh`
  - Level: L1
- **REQ-EX-S01-03** — Given a real pack directory omitted from the README pack list, when
  the script runs, then it exits non-zero (dogfooded-but-undocumented polarity).
  - Test: `hack/docs/example_format_inventory_test.sh`
  - Verify: `bash hack/docs/example_format_inventory_test.sh`
  - Level: L1
- **REQ-EX-S01-04** — Given the README claims an input format whose extension is not present
  on any pack class match, when the script runs, then it exits non-zero.
  - Test: `hack/docs/example_format_inventory_test.sh`
  - Verify: `bash hack/docs/example_format_inventory_test.sh`
  - Level: L1
- **REQ-EX-S01-05** — Given `task docs-gates` is run, when the inventory script is deleted
  from that task, then a pin in `hack/docs/truthlag_pins_test.sh` or the inventory script's
  own wiring check fails (non-vacuity).
  - Test: `hack/docs/truthlag_pins_test.sh` (or inventory script self-wiring stanza)
  - Verify: `task docs-gates`
  - Level: L1

---

## EX-S02 — Thicken topic-registry (YAML nested multi-field) [autonomous]

**As a** policy author **I want** the YAML starter pack to govern a realistic nested topic
document (not one scalar `partitions`) **so that** nested JSON Pointers and both polarities
are proven under `assent test --coverage`.

**Goal:** extend `topics/prod/orders.events.v1.yaml` (and siblings used by existing cases)
with nested fields the schema object already sketches: e.g. `schema.compatibility`,
`schema.references` as a **map** (not an unkeyed list), `retention.hours` / `retention.ms`
as a nested object *or* keep `retention_hours` and add `compaction.strategy` nested. Add a
rule whose `valueChanges.pointers` target a **nested** pointer (`/*/schema/compatibility`
or `/*/schema/format`) with proving + negative cases. Existing obligations
(ownership / bounded-change / non-destructive / schema-valid) must stay green — new fields
must not flip those decisions. Prefer editing existing fixtures over a new pack.

**Operator input:** none.

**Dependencies:** none (parallel with S03/S04).

**Definition of done:** `assent test examples/packs/topic-registry` and `--coverage` green;
deleting the nested-pointer rule reddens `--coverage` or lint tests-per-rule; `assent lint`
clean; D-002 sanitization green.

**Not in scope:** C1–C8 (S06/S07); `.tf`; JSON pack; engine changes.

Requirements:

- **REQ-EX-S02-01** — Given a topic document with at least three nested object levels under
  the identity key (e.g. `schema.compatibility`, `schema.format`, plus a nested map), when
  `assent test` runs the schema-valid proving case, then the decision stays the pinned
  `expect.yaml` value (existing goldens do not silently change).
  - Test: `examples/packs/topic-registry/.assent/tests/topics/schema-valid/expect.yaml`
  - Verify: `./bin/assent test examples/packs/topic-registry`
  - Level: L1
- **REQ-EX-S02-02** — Given a `valueChanges` rule on a nested pointer (not `/*/partitions`),
  when head modifies only that nested field within policy, then the case `decision: APPROVE`
  (or the pack's existing aggregate) and `--coverage` counts a proving polarity for that rule.
  - Test: `examples/packs/topic-registry/.assent/packs/topics/rules/` (new or extended rule) +
    matching `.assent/tests/topics/` case
  - Verify: `./bin/assent test --coverage examples/packs/topic-registry`
  - Level: L1
- **REQ-EX-S02-03** — Given the same nested-pointer rule, when head sets a disallowed nested
  value, then the negative case is non-APPROVE (`REVIEW` or `BLOCK` per `onFailure`) and
  `--coverage` counts the failing polarity.
  - Test: `examples/packs/topic-registry/.assent/tests/topics/<nested>/negative/expect.yaml`
  - Verify: `./bin/assent test --coverage examples/packs/topic-registry`
  - Level: L1
- **REQ-EX-S02-04** — Given the nested-pointer rule file is deleted, when `assent lint` or
  `--coverage` runs, then the gate is red (tests-per-rule and/or both-polarity non-vacuity).
  - Test: `examples/packs/topic-registry/.assent/packs/topics/rules/` + lint corpus
  - Verify: `./bin/assent lint examples/packs/topic-registry`
  - Level: L1

---

## EX-S03 — Thicken service-catalog (JSON nested objects/maps) [autonomous]

**As a** policy author **I want** the JSON catalog entries to carry nested objects beyond
`name` / `owner` / `tier` **so that** rules on nested fields are proven both polarities
without re-opening D-061 list opacity.

**Goal:** add nested **objects/maps** to catalog entries, e.g. `sla: { slo_percent, window }`
and `runtime: { language, replicas }` — **not** unkeyed `endpoints`/`tags` arrays in the
*pack* fixtures. Optionally allow-list a nested pointer (`path.endsWith("/sla/slo_percent")`)
in addition to `/oncall`. Watch D-061: if a fixture goes opaque, **do not** weaken the engine;
strip or reshape the fixture (maps / keyed lists with `identity.pointer`). Update pack tests
so every proving case still satisfies all required obligations.

**Operator input:** none.

**Dependencies:** none.

**Definition of done:** `assent test` + `--coverage` green on service-catalog; no opaque
changeset on the new nested-object modify cases; deleting the nested rule reddens coverage
or lint.

**Not in scope:** restoring `endpoints`/`tags` unkeyed lists in the pack; C2 privilege-tier
(S06 may add `tier` transition rules on this thickened document); live oncall provider.

Requirements:

- **REQ-EX-S03-01** — Given a catalog JSON entry with nested objects (`sla` and/or `runtime`)
  and **no** unkeyed nested lists in pack `base/`/`head/`, when `assent test` diffs a
  nested-field modify, then the ChangeSet is **not** opaque and the proving case matches
  `expect.yaml`.
  - Test: `examples/packs/service-catalog/.assent/tests/catalog/` (extended or new case)
  - Verify: `./bin/assent test examples/packs/service-catalog`
  - Level: L1
- **REQ-EX-S03-02** — Given a rule matching a nested pointer under `/services/*/<nested>/…`,
  when head changes only the allowed nested field, then proving polarity is counted by
  `--coverage`.
  - Test: `examples/packs/service-catalog/.assent/packs/catalog/rules/` + tests
  - Verify: `./bin/assent test --coverage examples/packs/service-catalog`
  - Level: L1
- **REQ-EX-S03-03** — Given head changes a nested sensitive field (or a field outside the
  allow-list), when the negative case runs, then the decision is non-APPROVE per `onFailure`.
  - Test: `examples/packs/service-catalog/.assent/tests/catalog/<nested>/negative/expect.yaml`
  - Verify: `./bin/assent test examples/packs/service-catalog`
  - Level: L1
- **REQ-EX-S03-04** — Given a dedicated catalog case whose `head` re-introduces an **unkeyed**
  nested list (`endpoints` and/or `tags` as JSON arrays — the D-061 shape), when `assent test`
  runs that case, then the ChangeSet is **opaque** (or the pinned `expect.yaml` decision is
  fail-safe non-APPROVE) and the case goes **red** if the differ starts emitting structured
  field changes for that unkeyed list (silent accept). The S03-01 proving fixtures stay
  keyed maps / nested objects only — this REQ is the both-polarity pin, not a comment.
  - Test: `examples/packs/service-catalog/.assent/tests/catalog/unkeyed-list-opaque/`
  - Verify: `./bin/assent test examples/packs/service-catalog`
  - Level: L1

---

## EX-S04 — Thicken infra-vars (tfvars nested maps) [autonomous]

**As a** policy author **I want** tfvars workloads to carry nested maps (not only scalar
replica/memory) **so that** HCL object-constructor nesting is adopter-visible and gated.

**Goal:** nest e.g. `resources = { cpu = 500, memory_mb = 3072 }` and/or `labels = { team =
"orders-team", tier = "prod" }` under each workload. Point `valueChanges` at
`/workloads/*/resources/memory_mb` (or keep existing scalar rules **and** add nested ones).
Both polarities. Literal-only: no `${}` interpolations in these fixtures (those are S05
opaque). Existing min/max replica cases stay green.

**Operator input:** none.

**Dependencies:** none.

**Definition of done:** `assent test` + `--coverage` green; nested map modify is a structured
diff (not opaque); deleting the nested rule reddens coverage/lint.

**Not in scope:** `.tf` resource blocks (S05); C6 placement (S07); changing `parseHCL`.

Requirements:

- **REQ-EX-S04-01** — Given a tfvars workload with a nested object (`resources` or `labels`)
  of at least two keys, when head modifies one nested scalar, then `assent test` sees a
  non-opaque ChangeSet and the proving `expect.yaml` holds.
  - Test: `examples/packs/infra-vars/.assent/tests/vars/` (extended fixtures)
  - Verify: `./bin/assent test examples/packs/infra-vars`
  - Level: L1
- **REQ-EX-S04-02** — Given a `valueChanges` pointer into the nested map, when the value
  stays in band, then `--coverage` records proving polarity for that rule.
  - Test: `examples/packs/infra-vars/.assent/packs/vars/rules/bounded-change.yaml` (extended)
  - Verify: `./bin/assent test --coverage examples/packs/infra-vars`
  - Level: L1
- **REQ-EX-S04-03** — Given the nested value exceeds the stubbed fact band, when the negative
  case runs, then decision is non-APPROVE (`challenge` → REVIEW).
  - Test: `examples/packs/infra-vars/.assent/tests/vars/<nested>/negative/expect.yaml`
  - Verify: `./bin/assent test examples/packs/infra-vars`
  - Level: L1
- **REQ-EX-S04-04** — Given S04's pack fixtures, when `assent test` runs infra-vars, then at
  least one case diffs a **nested-map** modify (S04-01) as a non-opaque structured change —
  this story may not skip the nested fixture. S04 fixtures contain **no** `.tf` files and
  **no** non-literal HCL (`var.team`, `"${…}"`); those belong to S05 only. Edge: adding a
  `.tf` or expression to an S04 proving `head/` fails this REQ.
  - Test: `examples/packs/infra-vars/.assent/tests/vars/` (nested-map case; glob must not
    match `*.tf` under S04 cases)
  - Verify: `./bin/assent test examples/packs/infra-vars`
  - Level: L1

---

## EX-S05 — HCL honesty: structured tfvars + `.tf` block → opaque → REVIEW [autonomous]

**As an** adopter **I want** an in-tree fixture that shows what assent **can** structure in
HCL (tfvars objects) and what it **cannot** (`.tf` `resource`/`module` blocks) **so that** I
do not assume a Terraform parser that does not exist.

**Goal:** (1) keep S04 structured tfvars as the literal HCL success path. (2) Add
`envs/prod/backend.tf` (or `module.tf`) with a generic `resource "…" "…"` / `module "…"`
block — invented names only. (3) Extend **the same** class `infra-vars` `match.paths` to
include `envs/**/*.tf` (judgment (c)) so the file is governed and `parseHCL` runs. (4) Author
`assent test` base/head that change the `.tf` block. (5) **Run** the case and pin whatever
fail-safe decision the engine actually produces (expected: opaque ChangeSet → REVIEW, zero
partial changes). (6) Document the known limitation in `examples/packs/infra-vars/.assent/config.yaml`
(already comments the fallback) and `examples/README.md` (claim `.tf` / HCL only after this
story). Do **not** spec a new HCL parser.

**Operator input:** none.

**Dependencies:** S04 (nested tfvars goldens exist).

**Definition of done:** one proving structured tfvars case (from S04) plus one `.tf` opaque
case with pinned `expect.yaml`; README inventory (S01) updated so the new format claim
matches dogfood; schema freeze vs `v0.1.0` (REQ-EX-S10-04); still single-class.

**Not in scope:** Terraform plan blast radius (REF-GAP-4); DEM-S09 `tf-module-instance` class;
expression evaluation; a fourth pack.

Requirements:

- **REQ-EX-S05-01** — Given class `infra-vars` matches `envs/**/*.tf` **and** `envs/**/*.tfvars`,
  when `assent lint` runs the pack, then it is clean (single class, bindings unchanged except
  paths).
  - Test: `examples/packs/infra-vars/.assent/config.yaml`
  - Verify: `./bin/assent lint examples/packs/infra-vars`
  - Level: L1
- **REQ-EX-S05-02** — Given a directory case whose only governed change is a `.tf` file
  containing an HCL **block**, when `assent test` runs, then the ChangeSet is opaque (or the
  decision is the fail-safe outcome **measured** on the engine — pin the measured `decision`
  in `expect.yaml`) and findings do not claim structured field diffs inside the block.
  - Test: `examples/packs/infra-vars/.assent/tests/vars/tf-opaque/` (name flexible)
  - Verify: `./bin/assent test examples/packs/infra-vars`
  - Level: L1
- **REQ-EX-S05-03** — Given that `.tf` case, when the `resource`/`module` block is replaced
  with equivalent tfvars attributes in a *different* case, then that other case remains
  structured (proves the adapter still diffs literals). Edge: do not let the opaque case
  poison sibling tfvars cases in the same pack.
  - Test: existing S04 tfvars cases still PASS in the same `assent test` invocation
  - Verify: `./bin/assent test examples/packs/infra-vars`
  - Level: L1
- **REQ-EX-S05-04** — Given `examples/README.md` claims HCL / `.tf`, when S01 inventory runs,
  then it exits 0 (format now dogfooded); claiming HCL before this fixture exists must have
  been red (S01 polarity).
  - Test: `hack/docs/example_format_inventory_test.sh`
  - Verify: `bash hack/docs/example_format_inventory_test.sh`
  - Level: L1
- **REQ-EX-S05-05** — Given the `.tf` fixture or class-path extension is deleted, when
  `--coverage` or inventory runs, then the gate is red (non-vacuity of the honesty case).
  Amendment (round-2 review): `--coverage` is architecturally blind to both mutations —
  `catalogue.Input` deliberately omits `Config` (D-017 B10), so no `assent test`/
  `--coverage` code path ever consults `Config.Classes`/`match.paths`, and `--coverage`
  itself is rule-polarity-level, not case-count-level, so a deleted case simply vanishes
  from the run without reddening anything. Non-vacuity is proven by inventory (the
  class-path-extension disjunct: `hack/docs/example_format_inventory_test.sh` derives
  its `FORMATS` list from `config.yaml`'s declared glob extensions) plus a dedicated Go
  test (the fixture-deleted disjunct: `cmd/assent/examples_infravars_tf_governance_test.go`'s
  `TestInfraVarsTFFixtureIsGovernedByItsClass` — `os.Stat`s the fixture files and
  `glob.Match`s the class's declared paths against the fixture's path, the same matcher
  `internal/core/classify`/`internal/core/aggregate` use) — never by `--coverage` alone.
  - Test: pack `--coverage` + inventory + `go test ./cmd/assent/... -run TestInfraVarsTF`
  - Verify: `./bin/assent test --coverage examples/packs/infra-vars`;
    `bash hack/docs/example_format_inventory_test.sh`;
    `go test ./cmd/assent/... -run TestInfraVarsTF -v`
  - Level: L1

---

## EX-S06 — REF-EX C1–C4 in-tree fixtures [autonomous]

**As a** maintainer **I want** generalized C1–C4 fixtures in existing packs **so that**
reference-derived patterns are dogfooded without private material (D-002).

**Goal:** close the first four REF-EX rows by **extending** packs/archetypes, not new
products:

| ID | Pattern | Host | Shape |
| --- | --- | --- | --- |
| C1 | list-no-shrink | topic-registry | identity-keyed `consumers` **map** (not an unkeyed list); entry delete → non-APPROVE (`require-review` / challenge). Reuses no-destruction. |
| C2 | privilege-tier allow-list | service-catalog | `tier` (already present) + rule: only allow-listed tier values or transitions; both polarities. Reuses allowed-fields. |
| C3 | wildcard-grant block | topic-registry | nested `acl.grants` map; a grant of `"*"` → **BLOCK**. |
| C4 | soft-delete-as-field-add | topic-registry | adding `tombstone: true` (or `status: retired`) treated as destruction → `require-review`, not silent APPROVE. |

New rules that match all topic files must be **vacuous-true** when the new field is absent so
S02 goldens stay green; add the new obligation to `bindings.yaml` `require:` **only if**
every existing proving case still covers it (vacuous proof). Prefer a dedicated
`valueChanges` pointer so unrelated modifies do not fire C3/C4.

**Operator input:** none.

**Dependencies:** S02, S03 (nested documents exist).

**Definition of done:** four named test directories (or inline cases) with both polarities
where the archetype has a failing polarity; `archetype-goldens.md` cross-check still holds
or is updated with new rows; sanitization green.

**Not in scope:** C5–C8 (S07); live resource-owner; companion-file correlation engine.

Requirements:

- **REQ-EX-S06-01 (C1)** — Given a keyed `consumers` map on a topic, when head **removes** a
  key, then decision is non-APPROVE; when head **adds** a key (or leaves the map unchanged in
  a proving sibling), then the C1 rule's proving polarity is counted.
  - Test: `examples/packs/topic-registry/.assent/tests/topics/list-no-shrink/`
  - Verify: `./bin/assent test --coverage examples/packs/topic-registry`
  - Level: L1
- **REQ-EX-S06-02 (C2)** — Given catalog `tier`, when head sets a value outside the allow-list
  (e.g. `tier: 0` or a disallowed promotion), then non-APPROVE; when head sets an allow-listed
  tier (or only `oncall`), then proving polarity holds without breaking D-061.
  - Test: `examples/packs/service-catalog/.assent/tests/catalog/privilege-tier/`
  - Verify: `./bin/assent test --coverage examples/packs/service-catalog`
  - Level: L1
- **REQ-EX-S06-03 (C3)** — Given `acl.grants` contains `"*"`, when the negative case runs,
  then **BLOCK**; a proving case with an explicit principal (invented name) does not block on
  this rule. Edge: absent `acl` must not BLOCK existing topics (vacuous).
  - Test: `examples/packs/topic-registry/.assent/tests/topics/wildcard-grant/`
  - Verify: `./bin/assent test examples/packs/topic-registry`
  - Level: L1
- **REQ-EX-S06-04 (C4)** — Given head **adds** a tombstone/retired field that base lacked,
  when the case runs, then non-APPROVE (soft-delete ≠ silent field add). Edge: modifying an
  unrelated nested field without the tombstone does not fire C4.
  - Test: `examples/packs/topic-registry/.assent/tests/topics/soft-delete/`
  - Verify: `./bin/assent test examples/packs/topic-registry`
  - Level: L1
- **REQ-EX-S06-05** — Given any C1–C4 rule file is deleted, when `assent lint` or `--coverage`
  runs, then the pack gate is red.
  - Test: pack lint + `--coverage`
  - Verify: `./bin/assent lint examples/packs/topic-registry && ./bin/assent lint examples/packs/service-catalog`
  - Level: L1

---

## EX-S07 — REF-EX C5–C8 (facts stubs + C8 known-limitation) [autonomous]

**As a** maintainer **I want** C5–C7 demonstrated with **stubbed** `facts.yaml` (repo-file /
resource-owner *shape*) and C8 pinned as expected REVIEW **so that** D-071 is closed without
live providers and without pretending companion-file correlation exists.

**Goal:**

| ID | Pattern | Host | Shape |
| --- | --- | --- | --- |
| C5 | quota-ceiling-from-fact | topic-registry | already has `facts.quota.max_partitions`; add an explicit C5 case + `examples/archetypes/quota-ceiling/` seed. Stub envelope matches `builtin/repo-file` output names. Do not call HTTP quota. |
| C6 | placement allow-list | infra-vars | `instance_set` (already on workloads) must be in `facts.placement.allowed.value`; both polarities. Archetype `examples/archetypes/placement-allow-list/`. |
| C7 | referenced-resource-ownership | topic-registry (judgment (b)) | optional `acl.owner` / `acl.resource`; stub `facts.resource_owner.owner`; mismatch → require-review. Gap **demo**, not a new builtin. Archetype `referenced-resource-ownership/`. |
| C8 | companion-file delete | infra-vars | a file **outside** class match (e.g. `envs/prod/NOTES.md` or `retired/manifest.yaml`) deleted in head; **expected REVIEW** (unmatched / unclassified — D-063 / ADR-0008 §1). Document **known limitation**: v1 does not correlate "delete A and append B". Out of v1 engine scope (REF-GAP-3). |

C7 rule vacuous when `!has(entry.acl)` (judgment (b)). C8 must **not** add a second class.

**Operator input:** none.

**Dependencies:** S04, S06.

**Definition of done:** C5–C8 named fixtures; C8 docs state REVIEW + out-of-v1; backlog
REF-EX row updated to specified/closed-by-EX; no live provider calls (`test_provider_fence`
stays green).

**Not in scope:** implementing cross-file correlation; DEM-S00; HTTP quota URL.

Requirements:

- **REQ-EX-S07-01 (C5)** — Given `facts.quota.max_partitions` (or nested quota object) in
  `facts.yaml`, when partitions exceed the stub, then challenge/REVIEW; when within ceiling,
  proving polarity. Edge: omitted quota fact must not APPROVE a controlling predicate
  (fail-safe unavailable — existing E2 behaviour; pin via a negative facts-omitted case or
  document reuse of engine tests).
  - Test: `examples/packs/topic-registry/.assent/tests/topics/quota-ceiling/` +
    `examples/archetypes/quota-ceiling/`
  - Verify: `./bin/assent test examples/packs/topic-registry`
  - Level: L1
- **REQ-EX-S07-02 (C6)** — Given `facts.placement.allowed.value` is a list/map of instance
  sets, when head sets `instance_set` to a value not in the stub, then non-APPROVE; when it
  stays on an allowed set, proving polarity.
  - Test: `examples/packs/infra-vars/.assent/tests/vars/placement/` +
    `examples/archetypes/placement-allow-list/`
  - Verify: `./bin/assent test --coverage examples/packs/infra-vars`
  - Level: L1
- **REQ-EX-S07-03 (C7)** — Given `entry.acl` names another team's resource and
  `facts.resource_owner.owner.value` does not match the author groups, when the case runs,
  then `require-review`. Edge: topics **without** `acl` still APPROVE the vacuous branch.
  - Test: `examples/packs/topic-registry/.assent/tests/topics/referenced-ownership/` +
    `examples/archetypes/referenced-resource-ownership/`
  - Verify: `./bin/assent test examples/packs/topic-registry`
  - Level: L1
- **REQ-EX-S07-04 (C8)** — Given head deletes a companion file that **no** class path
  matches, when `assent test` runs, then `expect.yaml` decision is **REVIEW** (measured, not
  wished). Docs (`examples/README.md` or pack config comment + walkthrough S09) call this a
  **known limitation**, not a feature. Edge: deleting a *governed* `.tfvars` file still hits
  existing non-destructive / unmatched-delete rules — C8 is the ungoverned companion only.
  - Test: `examples/packs/infra-vars/.assent/tests/vars/companion-delete/` +
    optional `examples/archetypes/` known-limitation seed
  - Verify: `./bin/assent test examples/packs/infra-vars`
  - Level: L1
- **REQ-EX-S07-05** — Given `cmd/assent/test_provider_fence_test.go`, when C5–C7 fixtures are
  added, then `assent test` still must not construct the live provider host.
  - Test: `cmd/assent/test_provider_fence_test.go`
  - Verify: `go test ./cmd/assent/ -run TestAssentTestNeverCallsProviderHost`
  - Level: L0

---

## EX-S08 — Discover packs; wire dogfood into `task check` + verify.yaml [autonomous]

**As a** maintainer **I want** `task check` to run example dogfood and CI to use the same
discovery script **so that** a new pack cannot be green locally and absent in CI (or the
reverse).

**Goal:** implement judgment (a). Add `hack/dogfood-examples.sh` that builds (or uses
`./bin/assent`) and runs `assent test` + `assent test --coverage` on every
`examples/packs/<name>` that has `.assent/tests`. Point `Taskfile.yml` `dogfood-examples:` at
that script (drop the three-name `for` loop). Point `verify.yaml`'s dogfood step at the same
script. Add `task: dogfood-examples` to `check:` **after** `build` (script may depend on
build; avoid parallel `deps:` with `fmt` — D-124 lesson). Replace `greenExamplePacks` string
slice with a directory walk; keep `TestDogfoodScriptsIncludeGreenExamplePacks` as "script is
invoked" + "walk matches README/inventory" rather than grepping three literals.

**Operator input:** none. Extends D-086 locally for dogfood only (judgment (e)).

**Dependencies:** S01 (inventory contract). Does **not** wait for S05 — discovery of three
packs is enough; S05 stays one pack.

**Definition of done:** `task check` runs dogfood; deleting the `check:` line reddens a pin
test (changelog_gate / new ex pin); adding `examples/packs/orphan/` with `.assent/` but no
tests reddens S01; a fourth green pack would be picked up without editing verify.yaml.

**Not in scope:** `examples/demo/**`; raising coverage; gitleaks into check.

Requirements:

- **REQ-EX-S08-01** — Given `hack/dogfood-examples.sh`, when it runs, then every
  `examples/packs/*/.assent/tests` pack is executed with `assent test` and `--coverage` and
  the process exits 0 on the current corpus.
  - Test: `hack/dogfood-examples.sh`
  - Verify: `bash hack/dogfood-examples.sh`
  - Level: L1
- **REQ-EX-S08-02** — Given `Taskfile.yml` `dogfood-examples` and `verify.yaml` dogfood step,
  when either hardcodes a three-name loop again, then `TestDogfoodScriptsIncludeGreenExamplePacks`
  (or successor) fails — they must call the shared script.
  - Test: `cmd/assent/test_corpus_test.go`
  - Verify: `go test ./cmd/assent/ -run TestDogfoodScriptsIncludeGreenExamplePacks`
  - Level: L0
- **REQ-EX-S08-03** — Given `task check`, when `dogfood-examples` is omitted from `check:`,
  then a wiring pin fails (follow `docs-gates` / `changelog_gate_test.sh` pattern).
  - Test: `hack/release/changelog_gate_test.sh` WIRED_TASKS **or** `hack/docs/example_format_inventory_test.sh` / new `hack/examples/dogfood_wiring_test.sh`
  - Verify: `bash hack/examples/dogfood_wiring_test.sh` (or extended existing pin)
  - Level: L1
- **REQ-EX-S08-04** — Given a new directory `examples/packs/extra/.assent/tests/...` that is
  incomplete/red, when dogfood discovery runs, then the script fails (discovery does not
  skip unknown names). Edge: `cmd/assent/testdata/broken-pack` is **not** discovered.
  - Test: `hack/dogfood-examples.sh` + corpus test walk
  - Verify: `go test ./cmd/assent/ -run TestAllExamplePacksGreenUnderAssentTest`
  - Level: L0
- **REQ-EX-S08-05** — Given `greenExamplePacks` is still a hardcoded three-tuple, when the
  walk successor lands, then the slice is derived from the filesystem (or the pin test
  proves the slice equals `filepath.Glob("examples/packs/*/.assent")`).
  - Test: `cmd/assent/test_corpus_test.go`
  - Verify: `go test ./cmd/assent/ -run TestAllExamplePacksGreenUnderAssentTest`
  - Level: L0

---

## EX-S09 — Product docs walkthrough byte-pinned to complex cases [autonomous]

**As an** adopter **I want** the published walkthrough and `examples/README.md` to show
real `assent test` / `assent lint` output on the complex fixtures **so that** docs cannot
drift to a one-field YAML sketch (AUD-S06 / D-124 pattern).

**Goal:** update `docs/usage/walkthrough.md` (and CLI examples as needed) to copy **measured**
output from `assent test examples/packs/topic-registry` (nested YAML), a JSON catalog
nested case, a tfvars nested case, and the `.tf` opaque / C8 REVIEW case. Pin byte-for-byte
or stable substrings via `hack/docs/truthlag_pins_test.sh` (extend, do not fork). Keep
`examples/README.md` truthful (four formats after S05; C8 known limitation; repos are
layouts). Do not mention unbuilt `assent init` as shipped. D-002 names only.

**Operator input:** none.

**Dependencies:** S02–S07 (output must match the complex corpus).

**Definition of done:** walkthrough Step 3 (and format section) matches a captured CLI run;
mutating the doc's PASS list without updating fixtures reddens the pin; `task docs-gates`
green.

**Not in scope:** mkdocs theme; DEM public READMEs; changing CLI flags.

Requirements:

- **REQ-EX-S09-01** — Given a fresh `assent test examples/packs/topic-registry` run, when
  compared to the walkthrough's console block, then every listed case name exists in actual
  output (pin case names, not flaky timing).
  - Test: `hack/docs/truthlag_pins_test.sh` (new pins) and/or `hack/docs/readme_smoke_test.sh`
  - Verify: `task docs-gates`
  - Level: L1
- **REQ-EX-S09-02** — Given the walkthrough still shows only `partitions: 12` as the sole
  governed field and omits nested YAML/JSON/tfvars/HCL, when S09 pins run, then they fail
  until the page names the four formats honestly.
  - Test: `hack/docs/truthlag_pins_test.sh`
  - Verify: `bash hack/docs/truthlag_pins_test.sh`
  - Level: L1
- **REQ-EX-S09-03** — Given C8 / `.tf` opaque, when docs describe them, then they say
  **REVIEW** / known limitation / literal-only HCL — not "assent understands Terraform".
  Edge: a sentence claiming expression evaluation reddens a grep pin.
  - Test: `hack/docs/truthlag_pins_test.sh`
  - Verify: `bash hack/docs/truthlag_pins_test.sh`
  - Level: L1
- **REQ-EX-S09-04** — Given `examples/README.md`, when S01 inventory runs after the doc edit,
  then it stays green (docs and dogfood still agree).
  - Test: `hack/docs/example_format_inventory_test.sh`
  - Verify: `bash hack/docs/example_format_inventory_test.sh`
  - Level: L1

---

## EX-S10 — Exit gate [autonomous]

**As a** maintainer **I want** one gate that proves the epic's invariants **so that** EX
cannot be marked done with a missing format, missing C-series case, stale docs, or schema
drift.

**Goal:** `hack/examples/ex_exitgate_test.sh` (or Go test) asserts: (1) dogfood discovery
runs four **formats** (yaml, json, tfvars, tf/HCL block case); (2) C1–C8 test directories
exist and their `expect.yaml` decisions match the table (C3 BLOCK, C8 REVIEW, etc.); (3)
S01 inventory + `task docs-gates` green; (4) schema freeze is **ref-relative against
`v0.1.0`** (AUD-S18 / D-132 — not working-tree `git diff schemas/`); (5) `internal/core`
untouched on the lane (`git diff --exit-code origin/main...HEAD -- internal/core`); (6)
`task check` includes dogfood; (7) provider fence still holds; (8) D-002 sanitization
green. Do not raise `COVERAGE_MIN`.

**Operator input:** none.

**Dependencies:** S01–S09.

**Definition of done:** exit-gate script green; deleting any C-series fixture reddens it;
backlog REF-EX marked closed by P5-EX.

**Not in scope:** DEM-S13 publish; E10; coverage floor bump.

Requirements:

- **REQ-EX-S10-01** — Given the three packs after S01–S09, when the exit gate lists formats
  from class match paths plus the `.tf` opaque case, then yaml, json, tfvars, and tf are all
  present.
  - Test: `hack/examples/ex_exitgate_test.sh`
  - Verify: `bash hack/examples/ex_exitgate_test.sh`
  - Level: L1
- **REQ-EX-S10-02** — Given C1–C8 fixture paths, when any one directory is missing, then the
  exit gate exits non-zero.
  - Test: `hack/examples/ex_exitgate_test.sh`
  - Verify: `bash hack/examples/ex_exitgate_test.sh`
  - Level: L1
- **REQ-EX-S10-03** — Given C8 `expect.yaml`, when the gate reads `decision:`, then it is
  `REVIEW` (not APPROVE).
  - Test: `hack/examples/ex_exitgate_test.sh`
  - Verify: `bash hack/examples/ex_exitgate_test.sh`
  - Level: L1
- **REQ-EX-S10-04** — Given `schemas/**/*.json`, when the exit gate runs, then it diffs
  against the immutable release tag **`v0.1.0`** — `git diff --name-status v0.1.0 -- schemas`
  (JSON-only), the same freeze as AUD-S18 / D-132 / `hack/audit/exitgate_test.sh`
  (`SCHEMA_BASE="${ASSENT_AUDIT_SCHEMA_BASE:-v0.1.0}"`). A working-tree `git diff schemas/`
  MUST NOT be the Verify (PCS / `hack/compare/exitgate_test.sh` vacuity: committed schema
  edits are invisible). The only JSON schema delta vs `v0.1.0` that may remain is the
  AUD-S18 permitted description-string on
  `schemas/decision/v1alpha1/decision-record.schema.json`; EX must add, delete, or modify
  **no other** schema JSON. Edge: `SCHEMA_BASE` must match `^v[0-9]+\.[0-9]+\.[0-9]+$` and
  resolve as `refs/tags/v0.1.0`; unsetting it or substituting `HEAD` reddens the gate
  (D-132: a non-tag base compares the tree against itself).
  - Test: `hack/examples/ex_exitgate_test.sh` (must contain the `v0.1.0` / `SCHEMA_BASE`
    pin and `git diff --name-status`; deleting the base-ref reddens)
  - Verify: `bash hack/examples/ex_exitgate_test.sh`
  - Level: L1
- **REQ-EX-S10-05** — Given `task check`, when dogfood or docs-gates or inventory is unwired,
  then the exit gate or existing wiring pins fail.
  - Test: `hack/examples/ex_exitgate_test.sh` + S08 wiring pin
  - Verify: `task check`
  - Level: L1
- **REQ-EX-S10-06** — Given a format claimed in README that is not in the dogfood walk, when
  the exit gate runs S01, then it fails (both polarities still owned by S01, re-invoked here).
  - Test: `hack/docs/example_format_inventory_test.sh`
  - Verify: `bash hack/docs/example_format_inventory_test.sh`
  - Level: L1
- **REQ-EX-S10-07** — Given the EX lane, when the exit gate diffs `internal/core` against
  `origin/main...HEAD` (three-dot, merge-base of the lane), then the diff is empty. Edge:
  a working-tree `git diff internal/core` MUST NOT be the Verify; substituting `HEAD` for
  `origin/main` reddens the pin.
  - Test: `hack/examples/ex_exitgate_test.sh`
  - Verify: `git diff --exit-code origin/main...HEAD -- internal/core`
  - Level: L1
- **REQ-EX-S10-08** — Given new example fixtures and docs, when the exit gate runs D-002
  sanitization, then `hack/check-sanitization.sh` exits 0 (no employer names / internal
  systems / verbatim private material). Edge: a planted employer token in a new fixture
  reddens the script.
  - Test: `hack/check-sanitization.sh`
  - Verify: `bash hack/check-sanitization.sh`
  - Level: L1

---

## Paths owned (file-disjoint guidance)

| Story | Owns (prefer) | Avoid |
| --- | --- | --- |
| S01 | `hack/docs/example_format_inventory_test.sh`, `examples/README.md` (pack/format sentence only), `task docs-gates` invocation line | pack fixtures |
| S02 | `examples/packs/topic-registry/` schema nested fields + nested-pointer rule/tests | C-series test dir names |
| S03 | `examples/packs/service-catalog/` nested objects + `unkeyed-list-opaque` case | C2 dir (S06) |
| S04 | `examples/packs/infra-vars/` tfvars nested maps + bounded-change pointers | `.tf` files; `config.yaml` class paths |
| S05 | `config.yaml` class paths, `envs/**/*.tf` fixtures, README HCL sentence | rewriting S04 tfvars scalars |
| S06 | new rules/tests `list-no-shrink`, `privilege-tier`, `wildcard-grant`, `soft-delete` | quota/placement/C7/C8 |
| S07 | C5–C8 tests + `examples/archetypes/{quota-ceiling,placement-allow-list,referenced-resource-ownership}/` | E5 builtins |
| S08 | `hack/dogfood-examples.sh`, `Taskfile.yml`, `.github/workflows/verify.yaml`, `cmd/assent/test_corpus_test.go` walk | fixture content |
| S09 | `docs/usage/walkthrough.md`, `hack/docs/truthlag_pins_test.sh` pins | engine |
| S10 | `hack/examples/ex_exitgate_test.sh` | product behaviour |

---

## Exit gate (epic)

Four formats in dogfood; REF-EX C1–C8 present with expected decisions (C8 REVIEW); docs
inventory + walkthrough pins green; schema freeze vs `v0.1.0` (not working-tree);
`internal/core` unchanged vs `origin/main...HEAD`; D-002 sanitization green;
`task check` runs `dogfood-examples`; no live GitLab/PAT; D-002 clean.
