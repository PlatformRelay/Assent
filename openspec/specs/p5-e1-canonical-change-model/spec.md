# E1 — Canonical change model: JSON + YAML (+ HCL/tfvars)

**Problem**: P4-E1 shipped the thinnest possible slice of the change model — a **pure,
modify-only** YAML differ (`internal/change/diff.go`) that diffs exactly one root-mapping
document, emits only `KindModify`, carries no source positions, and is consumed by a minimal
`assent-policy` meta-class classifier and by `cmd/assent`'s single-governed-file adapter. That
was deliberate thinness (P4-E1 non-goals), not the target shape. E1 is where the **full**
canonical change model described in `openspec/config.yaml` ("JSON/YAML/HCL adapters -> value
tree -> structural diff with positions, first-class deletes/renames (fold opt-in), input
resource limits, opaque -> REVIEW") and in ADR-0003/ADR-0017 §5 actually lands, so `internal/
core` (E2/E3) and the forge adapters (E4/E8) can be built against a format-agnostic,
add/delete/rename/positioned `ChangeSet` instead of the modify-only YAML-specific one.

**Scope**: extend the shipped differ to first-class add/delete + source positions on every
`Change` (E1-S01); fold add+delete pairs into an opt-in `rename` (E1-S02, per class, default
`raw`, never laxer than delete); generalize the differ onto a canonical value tree and add a
JSON format adapter over it (E1-S03); add an HCL/tfvars format adapter over the same tree
(E1-S04); derive stable `EntryRef` subjects for map/list collections with a declared identity
key, unkeyed lists rejected (E1-S05); broaden the classifier to the four ADR-0017 §5 matcher
domains that are decidable from a single-file `ChangeSet` alone — `files`, `values.pointers`,
`valueChanges` — plus a new `entryEvents` domain for collection-entry-level add/delete/rename
matching (E1-S05's `EntryRef` add/delete/rename; this is **not** ADR-0003's whole-file
`fileEvents` concept, which this epic explicitly defers — see Non-goals), while preserving the
shipped `assent-policy`/`unclassified` reserved-class dominance (E1-S06); enforce pure
size/depth/entry-count/alias-expansion input ceilings, fail-closed to opaque (E1-S07); and
close the live P4-E1-S10 review follow-up (D-042) by making `cmd/assent` enumerate the MR's
**full** changed-file set instead of one hardcoded governed subject, so classification/routing
(including the `.assent/**` → `assent-policy` BLOCK) runs on the live adapter path, not only
at the engine-golden tier (E1-S08).

**Non-goals** (fenced to their owning epic — do not re-scope here):
- Rego frontend, scoring/points, multi-obligation `require:` AND composition, `require-review`
  authorization, one-shot arming — **E2**.
- The declarative YAML *policy* frontend (envelope loader, CEL `assert` backend, `assent lint`
  hard-error **enforcement** of the reserved-class/catch-all-vouch rules this epic's matcher
  domains make expressible) — **E3**. E1 ships the matcher-domain **primitives** the lint will
  later enforce against; it does not ship the lint pass itself.
- GitLab/GitHub forge adapter internals, real protected-source verification (D-034 hard gate,
  still an INSECURE PLACEHOLDER) — **E4/E8**.
- Provider host, fact resolution — **E5**.
- `assent test` adopter harness — **E6**.
- kind/e2e infra — **E7** (runs alongside E1 per the meta-plan ordering constraint, but is a
  separate epic/spec).
- Distribution/releases — **E9**.
- Whole-file `fileEvents` tracking as **ADR-0003** actually defines it (`docs/adr/
  0003-canonical-change-model.md`, "Deletions and renames are first-class": file-level `added |
  deleted | renamed | modified | opaque` events, with renames detected via **git** rename
  detection preserving identity). No story in this epic implements git-level whole-file
  add/delete/rename tracking — the differ still operates on one already-named file's content per
  call. E1-S06 ships a distinctly-named, collection-entry-scoped `entryEvents` domain instead
  (E1-S05's `EntryRef` add/delete/rename) — it must not be read as satisfying ADR-0017 §5's
  `fileEvents` domain name, which denotes the ADR-0003 whole-file concept. The real `fileEvents`
  domain, and the git-rename-detection it needs, is deferred to a **fast-follow after E1-S08**
  (which already enumerates the MR's full changed-file set and is the natural place whole-file
  add/delete/rename would first become observable) — not scoped into this epic's eight stories.
- A JSON Schema for `ChangeSet`/`Change` (no such schema is frozen today — grep of `schemas/`
  found none; the shape is a Go struct only). If a future epic freezes one, these stories'
  additive fields (Kind values, positions, EntryRef) are additive-compatible by construction
  (new optional fields / new enum members), not breaking — noted so P3-E2's strict-decode
  rules are not silently violated later.

ADRs: 0003 (canonical change model — deletes/renames first-class, opt-in rename fold,
input limits, source positions, opaque → REVIEW), 0003 Amendment/Amendment 2, 0008 (classifier,
`assent-policy`/`unclassified` reserved classes, matcher domains referenced), 0011 (core ports:
`FormatAdapter{Match,Parse}` — a per-format parser onto one `Value` tree, so the structural
differ stays format-agnostic), 0015 §1 (`.assent/**` self-vouch boundary — must keep holding
once the changed-file-set enumeration lands), 0017 §5 (governed subjects / `EntryRef` /
matcher domains: `files`, `values.pointers`, `fileEvents`, `valueChanges`; unkeyed lists
rejected at lint, not guessed). Consumes: `internal/change/diff.go` + `diff_test.go` (P4-E1-S02,
shipped), `internal/core/classify/classify.go` (P4-E1-S07, shipped), `cmd/assent/run.go`
(P4-E1-S10, shipped, live-green D-041/D-042), `examples/repos/service-catalog/**.json`,
`examples/repos/infra-vars/**.tfvars`. D-033 (pins stay out-of-band, not in `EvaluationInput` —
unaffected by this epic), D-034 (protected-source verification stays a placeholder — this epic
does not touch `doctor`/arming), D-039/D-041/D-042 (what shipped, and the exact live gap this
epic's S08 closes).

## Executability classification (autonomous vs infra-gated)

**Every story in this epic is `[autonomous]`.** E1 is pure `internal/change` (+ `internal/
core/classify`) engine code plus the `cmd/assent` CLI/CI-env adapter's changed-file-set
enumeration — all gate-able with unit/golden/property tests against fixtures already checked
into `examples/repos/` and a fake/injected changed-file list, with no live GitLab, no token,
and no real repository. Unlike P4-E1 (which had two infra-gated stories, S10/S11, because it
proved the **adapter's** real thread/approval/merge semantics against a live forge), this epic
never opens a real network connection or forge session — `E1-S08`'s "changed-file set" comes
from the local checkout / an injected file list (ADR-0008 §4: local checkout is mandatory
regardless of live-vs-fake forge), so its test double is a fixture directory or an in-memory
fake, never a live MR. If a future story in this epic is found to need live infra, it must be
split out and re-tagged explicitly — none currently do.

## Judgment calls fixed by this spec (log to the operator, not `decisions.md` — agent-authored spec, not a D-nnn judgment)

- **Canonical value-tree design lands inside the first format-adapter slice (E1-S03), not as
  a separate refactor-only story.** A standalone "generalize the differ" story would have no
  independent user-facing value (INVEST: Valuable) and Go tooling can't gate "no observable
  behaviour change" cheaply as its own increment; bundling the tree generalization with the
  JSON adapter gives it a concrete second client to prove genericity against, exactly as
  P4-E1-S02 was "the smallest thing that produces a real ChangeSet the rest is written
  against." E1-S04 (HCL) then becomes deliberately thin — a parser onto the already-general
  tree, no differ change.
- **Source positions (ADR-0003 Amendment 2 "first-class positions") are folded into E1-S01**
  (the add/delete story, since it already touches every code path that emits a `Change`,
  including retrofitting the shipped P4-E1 modify-only path) rather than treated as a separate
  story or silently dropped. ADR-0003 explicitly warns retrofitting positions after adapters
  exist means rewriting the parsers — so this epic captures them at the one point (E1-S01)
  before JSON/HCL adapters are added in E1-S03/S04, and those two stories inherit
  position-capture as part of "produces the same canonical ChangeSet shape," not as separate
  follow-up work.
- **Parse-time input ceilings stay pure; only the deadline is not.** ADR-0003 Amendment 2 asks
  for both size/depth/entry-count/alias-expansion caps AND a parse deadline. The caps are pure
  (comparisons against parsed structure) and belong in `internal/change` (E1-S07). A wall-clock
  parse deadline needs a clock/context and is barred from `internal/change` by the purity gate
  (GUIDELINES.md §5, `internal/core/purity_test.go`); it is explicitly fenced OUT of E1-S07 and
  left to whichever `cmd/assent`-tier story wants it (not scoped here — no bullet in this
  epic's brief asked for it).
- **`EntryRef` derivation lives in `internal/change`** (a new file alongside `diff.go`), not in
  `internal/core/classify`, because P4-E1's own non-goals list it in the same breath as
  multi-format adapters and matcher-domain breadth as differ-owned capabilities, and because
  `entries:` collection-mode parsing (map/list identity) is a walk-time concern over the same
  value tree E1-S03 introduces, not a post-hoc classification concern.
- **The `assent lint` reserved-class/catch-all-vouch hard-error enforcement is explicitly E3**,
  not E1, even though E1-S06 makes the matcher-domain vocabulary those lint rules will
  eventually check against expressible. E1-S06 preserves and extends the existing
  `classify.ValidateRouting` primitive; it does not build the policy-envelope lint pass that
  calls it against authored packs (that loader does not exist until E3).
- **E1-S08 (changed-file-set enumeration) is scoped as an outcome + constraint, not a new Forge
  port method.** The task follow-up (D-042 F1) is stated as "enumerate the MR's changed-file
  set"; the mechanism (local checkout tree walk vs. an existing/new `internal/forge` listing
  call) is left to the implementer, fenced only by ADR-0008 §4 ("evaluation always runs against
  a local checkout") so this story cannot smuggle in E4 forge-adapter API design.

## Dependency order

```
E1-S01 add/delete diffs + positions ──┬─► E1-S02 opt-in rename fold
                                       │
                                       ├─► E1-S03 canonical value tree + JSON adapter ──► E1-S04 HCL/tfvars adapter
                                       │                          │
                                       │                          ├─► E1-S05 EntryRef derivation (map/list)
                                       │                          │
                                       │                          └─► E1-S07 input resource limits (also needs S04)
                                       │
                                       └─► E1-S06 classifier matcher-domain breadth (also needs S02, S05)

E1-S08 cmd/assent changed-file-set enumeration ── independent; buildable in parallel from day one
```

**First slice: E1-S01 (add/delete diffs + positions).** It is the smallest pure extension of
already-shipped, already-tested code (`internal/change/diff.go`'s `walkMap`/`walk` currently
return an opaque reason for exactly the shapes this story must instead emit as `Change`s), and
almost every other story (rename fold, the value-tree generalization, EntryRef derivation,
matcher-domain breadth) needs `KindAdd`/`KindDelete` to exist first. **E1-S08 is independently
startable in parallel from day one** (it only needs the already-shipped P4-E1 differ + classifier)
and closes a named, already-logged live-adapter security gap (D-042 F1) — recommend starting it
alongside E1-S01 if two lanes are available, per the meta-plan's own bias toward proving trust
boundaries early rather than deferring them.

---

## E1-S01 — Add/delete diffs + source positions on every Change `[autonomous]`

**As a** rule author **I want** a map key added or removed between base and head to produce a
first-class `add`/`delete` `Change` (instead of an opaque fail), each carrying a source
position **so that** `block`/`challenge` rules can match entry deletions directly and forge
adapters can anchor inline comments to the exact changed line.

**Goal**: extend `internal/change`'s YAML walker so a key present on only one side of a
mapping emits `Kind: add` (head-only) or `Kind: delete` (base-only) with the full present-side
value, instead of today's `"key removed/added — add/delete is out of modify-only scope"`
opaque reason; every emitted `Change` (add, delete, and the existing modify) carries a 1-indexed
line/column position for each side where a value exists on that side (nil/absent on the side
an add/delete does not touch). Still **pure**: no clock/env/network/random anywhere in the
codepath.

**Operator input**: no.

**Dependencies**: none (extends the shipped P4-E1-S02 differ in place).

**Definition of done**: an added key and a removed key each produce one `Change` with the
correct `Kind`, full value, and non-nil position on the present side; the existing modify path
now also carries positions on both sides (a backfill of the shipped behaviour, proven by
updating `TestModifyOnlyYAMLDiff`'s golden); a still-genuinely-undecidable shape (mapping-vs-
scalar type flip, alias/anchor, non-string/duplicate key) remains opaque exactly as before —
this story narrows what fails closed, it does not widen what is decidable; every golden
double-runs byte-identical.

**Not in scope**: rename folding (E1-S02), JSON/HCL adapters (E1-S03/S04), sequence/list
add-delete (that is collection-mode `EntryRef` territory, E1-S05, not a bare mapping walk).

Requirements:

- **REQ-E1-S01-01** — Given a YAML mapping with a key present in head but absent in base, when
  the differ runs, then it emits one `Change` with `Kind: add`, `Path` pointing at the added
  key, `Old` empty/absent, `New` = the head value's canonical render, and a non-nil position on
  the head side only — no longer the opaque `"key added ... out of modify-only scope"` result.
  - Test: `internal/change/diff_test.go`
  - Verify: `go test ./internal/change/... -run TestAddedKeyEmitsAddChange`
  - Level: L0
- **REQ-E1-S01-02** — Given a YAML mapping with a key present in base but absent in head, when
  the differ runs, then it emits one `Change` with `Kind: delete`, the base value in `Old`, `New`
  empty/absent, and a non-nil position on the base side only. Adversarial case: a delete at the
  same path as an unrelated modify elsewhere in the document does not get merged/conflated —
  each key change is independently reported, proven by a golden with one add, one delete, and
  one modify in the same document.
  - Test: `internal/change/diff_test.go`
  - Verify: `go test ./internal/change/... -run TestRemovedKeyEmitsDeleteChange`
  - Level: L0
- **REQ-E1-S01-03** — Given the shipped modify-only golden fixture (`orders.events.v1.yaml`,
  `partitions 12 -> 24`), when the differ runs, then the resulting `Change` carries a position
  for both `Old` and `New` pointing at the `partitions:` line/column in the base and head byte
  streams respectively — the existing golden JSON gains position fields without changing
  `File`/`Path`/`Kind`/`Old`/`New`.
  - Test: `internal/change/diff_test.go`
  - Verify: `go test ./internal/change/... -run TestModifyOnlyYAMLDiff`
  - Level: L0
- **REQ-E1-S01-04** — Given a mapping-vs-scalar type flip, an alias/anchor node, or a
  non-string/duplicate mapping key (the shapes `TestStructuralDeltaFailsSafe` and
  `mappingEntries` already cover), when the differ runs, then the result is still opaque with a
  non-empty reason and zero partial changes — this story must not accidentally widen what is
  decidable beyond add/delete/modify. Adversarial case: the existing
  `TestStructuralDeltaFailsSafe`/`TestOpaqueInputFailsSafe` suites are re-run unmodified and
  stay green.
  - Test: `internal/change/diff_test.go`
  - Verify: `go test ./internal/change/... -run TestStructuralDeltaFailsSafe`
  - Level: L0
- **REQ-E1-S01-05** — Given the determinism hard rule (GUIDELINES.md §5), when
  `internal/core/purity_test.go`'s `TestCorePurity` scans `internal/change/**` (it already walks
  `../change` recursively from `internal/core`), then the new add/delete/position code
  introduces no `os.Getenv`/`os.Environ`, `time.Now`, `rand`, or network import, and every new
  golden double-runs to byte-identical output (extending the existing
  `TestDiffDoubleRunStable` pattern to cover add/delete cases).
  - Test: `internal/change/diff_test.go`
  - Verify: `go test ./internal/change/... -run TestDiffDoubleRunStable`
  - Level: L0

## E1-S02 — Opt-in rename fold (delete+add → rename, default raw) `[autonomous]`

**As a** rule author **I want** a same-class delete+add pair with an equal/near-equal value to
be optionally foldable into one `rename` change **so that** a genuine rename (e.g. a topic
renamed, value untouched) does not read as an unrelated deletion plus an unrelated addition —
while a repo that has not opted in keeps seeing the raw, stricter delete+add pair.

**Goal**: implement the fold as a pure post-process over an already-produced `ChangeSet`'s
`Kind: add`/`Kind: delete` pairs: `renames: detect|raw` is a **per-class** switch, default
`raw` (fold never happens unless explicitly requested); when `detect` is active and a delete
and an add in the same class have equal (or configurably near-equal) values, they fold into one
`Kind: rename` entry carrying `oldPath`/`newPath` and the shared value; the engine always
applies the **stricter of the class's configured delete-effect and rename-effect** to the
folded entry (ADR-0003 amendment) — folding can never be used to make a dangerous delete
resolve more leniently than it would unfolded.

**Operator input**: no.

**Dependencies**: E1-S01 (needs `Kind: add`/`Kind: delete` to exist before there is anything to
fold).

**Definition of done**: with `renames: raw` (default), a delete+add pair is reported as two
separate `Change`s exactly as E1-S01 emits them — no behaviour change; with `renames: detect`,
an equal-value delete+add pair in the same class folds into one `Kind: rename` `Change`; an
adversarial near-threshold pair (crafted to sit just above a similarity threshold) is proven to
still resolve to at least as strict an effect as the raw delete would have; every golden
double-runs.

**Not in scope**: JSON/HCL adapters (E1-S03/S04) — this story operates on the already-produced,
format-agnostic `ChangeSet`, so it works unchanged once those adapters exist; wiring `renames:`
into an authored policy envelope's `classes[]` schema (that authoring surface is E3's declarative
policy frontend — E1 ships the fold *function* and its config type, not the YAML envelope
parser for it).

Requirements:

- **REQ-E1-S02-01** — Given `renames: raw` (the default, unset), when a `ChangeSet` containing
  a same-class delete+add pair with an identical value is folded, then the output is
  **unchanged** from the unfolded input — two `Change`s (`delete`, `add`), never a `rename` —
  proving the opt-in default is genuinely raw, not detect-by-default.
  - Test: `internal/change/rename_test.go`
  - Verify: `go test ./internal/change/... -run TestRenameFoldDefaultRaw`
  - Level: L0
- **REQ-E1-S02-02** — Given `renames: detect` and a same-class delete at path A / add at path B
  with an identical value, when the fold runs, then the output replaces the pair with one
  `Kind: rename` `Change` carrying `oldPath: A`, `newPath: B`, and the shared value — proven by
  a golden double-run.
  - Test: `internal/change/rename_test.go`
  - Verify: `go test ./internal/change/... -run TestRenameFoldDetect`
  - Level: L0
- **REQ-E1-S02-03** — Given `renames: detect` and a class whose configured `delete` effect is
  strictly more severe than its configured `rename` effect, when a delete+add pair folds, then
  the engine-visible effect for the folded entry is the **delete** class's (stricter), never
  silently downgraded to the rename effect. Adversarial case (ADR-0003 amendment, "attacker-
  tunable downgrade knob"): a delete+add pair crafted with a value difference sitting just
  above whatever similarity threshold this story's fold implementation uses is proven to still
  fold (or, if the threshold implementation rejects it, to fall back to the raw, stricter,
  delete+add pair) — never silently resolved to a weaker effect than the unfolded delete alone.
  - Test: `internal/change/rename_test.go`
  - Verify: `go test ./internal/change/... -run TestRenameFoldNeverLaxerThanDelete`
  - Level: L0
- **REQ-E1-S02-04** — Given the determinism rule, when the fold runs, then it references no
  clock/env/network/random, `TestCorePurity` stays green over `internal/change/**`, and the
  fold is order-independent — shuffling the input `ChangeSet`'s entry order before folding
  yields a byte-identical folded result after canonical sort.
  - Test: `internal/change/rename_test.go`
  - Verify: `go test ./internal/change/... -run TestRenameFoldOrderIndependent`
  - Level: L0

## E1-S03 — Canonical value tree + JSON format adapter `[autonomous]`

**As a** rule author with a JSON service catalog **I want** a JSON file's changes to produce
the same `ChangeSet` shape a YAML file's changes do **so that** the same policy pack matches
both formats without knowing which one it is looking at.

**Goal**: introduce a canonical, format-neutral value-tree type in `internal/change` (a scalar/
mapping/sequence node carrying a tag-qualified render and a position, generalizing what
`*yaml.Node` gives the walker today) and re-express `walk`/`walkMap`'s comparison logic to
operate over that tree instead of `*yaml.Node` directly (the YAML parser becomes the tree's
first producer, proven by re-running every existing YAML golden — including E1-S01/S02's —
unmodified and getting byte-identical `ChangeSet` output before and after this refactor); then
add a JSON parser that produces the same tree from `encoding/json`-decoded bytes (using
`json.Number`/raw-token decoding, not float64, so the differ's injective tag-qualified
comparison — the property that closes the numeric-collapse fail-open class — holds for JSON
the same way it holds for YAML). `Diff` gains format selection by file extension (`.json` ->
JSON parser, `.yaml`/`.yml` -> YAML parser).

**Operator input**: no.

**Dependencies**: E1-S01 (the tree must carry add/delete + positions from day one, not need a
second retrofit).

**Definition of done**: every existing YAML golden (`diff_test.go`, `rename_test.go`) passes
unmodified after the tree refactor; a new JSON fixture drawn from
`examples/repos/service-catalog/catalog/prod/core-services.json` (e.g. a `services[].tier`
edit) produces a `ChangeSet` with the same field shape (`File`/`Path`/`Kind`/`Old`/`New`/
position) a YAML edit would; an unparseable/oversized/truncated JSON document fails opaque,
never a silent empty diff; a `Change` on `services[]` (a JSON array — this thin slice does
**not** yet derive per-entry `EntryRef` identity, that is E1-S05) surfaces the same way the
current differ surfaces any non-mapping-walkable shape today (opaque, until E1-S05 lands
array/list walking); every golden double-runs.

**Not in scope**: HCL/tfvars adapter (E1-S04); `EntryRef`/collection-mode array walking
(E1-S05 — this story's JSON adapter can diff a top-level JSON **object**'s scalar/nested-object
fields exactly like YAML's mapping walk, but does not yet walk into `services[]` array entries
individually); JSON Schema authoring for the value tree (no such schema is frozen — see epic
Non-goals).

Requirements:

- **REQ-E1-S03-01** — Given the full E1-S01/S02 YAML golden suite, when it is re-run against the
  refactored tree-based walker, then every golden's output is byte-identical to its
  pre-refactor value — the generalization introduces no observable YAML behaviour change.
  - Test: `internal/change/diff_test.go`
  - Verify: `go test ./internal/change/...`
  - Level: L0
- **REQ-E1-S03-02** — Given a JSON document with one scalar field changed at the top level or
  within a nested object (e.g. an `examples/repos/service-catalog/` fixture with
  `"apiVersion"` edited), when the differ runs with `.json` file selection, then it emits a
  `ChangeSet` with the same `Change` shape (`Kind: modify`, JSON-Pointer `Path`, tag-qualified
  `Old`/`New`, positions) the YAML differ produces for an equivalent edit — proven by a golden
  whose only difference from a YAML-sourced golden is the source format.
  - Test: `internal/change/diff_json_test.go`
  - Verify: `go test ./internal/change/... -run TestJSONAdapterModify`
  - Level: L0
- **REQ-E1-S03-03** — Given two JSON numeric literals that would collapse under a lossy
  float64 decode (e.g. an integer >= 2^53 and a neighboring distinct integer, or two
  distinct high-precision decimals), when the differ compares them, then it reports a change
  (injective comparison preserved for JSON, mirroring the YAML `render`/`compareKey`
  discipline) — never a silent miss from `encoding/json`'s default float64 unmarshalling.
  Adversarial case: a fixture pair specifically constructed to collapse under naive
  `float64` decoding is proven to still be detected as changed.
  - Test: `internal/change/diff_json_test.go`
  - Verify: `go test ./internal/change/... -run TestJSONNumericInjective`
  - Level: L0
- **REQ-E1-S03-04** — Given malformed/truncated JSON bytes, or a JSON document whose root is
  not an object (a bare array, a bare scalar), when the differ runs, then it returns an opaque
  `ChangeSet` with a non-empty reason and zero partial changes — never a silent empty diff,
  mirroring `TestOpaqueInputFailsSafe`'s YAML contract for the JSON adapter.
  - Test: `internal/change/diff_json_test.go`
  - Verify: `go test ./internal/change/... -run TestJSONOpaqueFailsSafe`
  - Level: L0
- **REQ-E1-S03-05** — Given the determinism rule, when the tree refactor and JSON adapter land,
  then `TestCorePurity` stays green over `internal/change/**` (no `encoding/json` decode path
  introduces env/clock/network/random), and every new JSON golden double-runs byte-identical.
  - Test: `internal/change/diff_json_test.go`
  - Verify: `go test ./internal/change/... -run TestJSONAdapterDoubleRunStable`
  - Level: L0

## E1-S04 — HCL/tfvars format adapter `[autonomous]`

**As a** rule author with a `.tfvars` infra-vars repo **I want** a tfvars value edit to produce
the same `ChangeSet` shape JSON/YAML changes do **so that** one policy pack governs all three
formats.

**Goal**: add an HCL/tfvars parser producing the E1-S03 canonical value tree, scoped to
**tfvars' literal-only subset** (ADR-0003's explicit HCL caveat: full HCL with expressions is
represented but expression *evaluation* is out of scope for v1) via `hashicorp/hcl`; `.tfvars`
file selection routes to this adapter.

**Operator input**: no.

**Dependencies**: E1-S03 (the shared value tree this adapter parses into).

**Definition of done**: a fixture drawn from `examples/repos/infra-vars/envs/prod/
compute.tfvars` (e.g. a `workloads.orders-api.min_replicas` edit) produces a `ChangeSet` with
the same field shape the YAML/JSON adapters produce; a non-literal HCL expression (interpolation,
function call, a reference to another variable) is explicitly opaque, not silently evaluated or
silently dropped; malformed HCL bytes fail opaque; every golden double-runs.

**Not in scope**: full HCL expression evaluation (explicitly out of v1 scope per ADR-0003);
non-`.tfvars` HCL file kinds (`.tf` resource blocks) — this story is tfvars-literal only, as
named in the epic title and ADR-0003's caveat.

Requirements:

- **REQ-E1-S04-01** — Given a tfvars document with one literal value changed inside a nested
  object (e.g. `workloads.orders-api.min_replicas` edited from an `examples/repos/infra-vars/`
  fixture), when the differ runs with `.tfvars` file selection, then it emits a `ChangeSet`
  with the same `Change` shape (`Kind: modify`, pointer-style `Path`, tag-qualified `Old`/`New`,
  positions) the YAML/JSON adapters produce.
  - Test: `internal/change/diff_hcl_test.go`
  - Verify: `go test ./internal/change/... -run TestHCLAdapterModify`
  - Level: L0
- **REQ-E1-S04-02** — Given a tfvars value expressed as a non-literal HCL expression
  (interpolation `"${var.x}"`, a function call, a variable reference), when the differ
  encounters it, then the result is opaque with a reason naming the unsupported construct —
  never silently evaluated (which could hide the real value) and never silently dropped
  (fail-safe direction, GUIDELINES §2).
  - Test: `internal/change/diff_hcl_test.go`
  - Verify: `go test ./internal/change/... -run TestHCLNonLiteralFailsSafe`
  - Level: L0
- **REQ-E1-S04-03** — Given malformed HCL bytes, when the differ runs, then it returns an
  opaque `ChangeSet` with a non-empty reason and zero partial changes.
  - Test: `internal/change/diff_hcl_test.go`
  - Verify: `go test ./internal/change/... -run TestHCLOpaqueFailsSafe`
  - Level: L0
- **REQ-E1-S04-04** — Given the determinism rule, when the HCL adapter lands, then
  `TestCorePurity` stays green over `internal/change/**` and every new HCL golden double-runs
  byte-identical.
  - Test: `internal/change/diff_hcl_test.go`
  - Verify: `go test ./internal/change/... -run TestHCLAdapterDoubleRunStable`
  - Level: L0

## E1-S05 — EntryRef derivation for map and list collections `[autonomous]`

**As a** rule author governing a keyed list of services (JSON) or a keyed map of workloads
(tfvars) **I want** each collection entry to have a stable identity-derived `EntryRef` **so
that** a rule can say "this specific service/workload changed," and a rename-fold or a
forge-anchored comment can refer to the same entry across a reorder.

**Goal**: add `entries: {mode: document|list|map, root, identity.pointer}` collection-mode
parsing over the E1-S03 value tree: `document` mode is the already-shipped single-root-mapping
behaviour (unchanged, default); `map` mode treats an object's top-level (or `root`-pointed)
keys as entries, each keyed by its own map key (fixture:
`examples/repos/infra-vars/envs/prod/compute.tfvars`'s `workloads` map); `list` mode treats a
JSON/YAML array's elements as entries, each keyed by the value at `identity.pointer` within the
element (fixture: `examples/repos/service-catalog/catalog/prod/core-services.json`'s
`services[]`, keyed by `/name`). Each entry's changes are tagged with a runtime `EntryRef`
(e.g. `service:orders-api`, `workload:orders-api`) alongside the existing file-pointer `Path`.
An unkeyed `list` (no `identity.pointer` declared, or a declared pointer missing/non-unique
across entries) is **rejected**, not guessed — this story raises that rejection as a differ-
level error (`assent lint`'s hard-error surfacing of the same rule is E3's).

**Operator input**: no.

**Dependencies**: E1-S01 (add/delete — an entry added/removed from a keyed collection is an
add/delete of that `EntryRef`, not a bare mapping key), E1-S03 (the value tree — `list` mode
needs sequence walking the tree already generalizes).

**Definition of done**: a `map`-mode fixture over `compute.tfvars`'s `workloads` produces
per-workload `Change`s tagged with `EntryRef: workload:<key>`; a `list`-mode fixture over
`core-services.json`'s `services[]` produces per-service `Change`s tagged with
`EntryRef: service:<name>`, and reordering the array (no content change) yields **zero**
changes (identity-keyed, not index-keyed) — proving positional diffing is not silently
happening; a `list`-mode declaration with no `identity.pointer` (or one that collides across
two entries) is rejected with a clear reason, never guessed at; `document` mode is unchanged
from the shipped P4-E1 behaviour; every golden double-runs.

**Not in scope**: the `assent lint` **enforcement** surfacing this story's unkeyed-list
rejection as a load-time policy error (E3, needs the envelope loader); nested collections
(a list inside a list, or a map inside a list-mode entry) beyond one level — fenced explicitly
here and, if hit, must fail opaque rather than silently under-derive, proven by an adversarial
REQ below.

Requirements:

- **REQ-E1-S05-01** — Given `entries: {mode: map, root: /workloads}` over the
  `compute.tfvars` fixture and a change to one workload's `min_replicas`, when the differ
  derives entries, then the resulting `Change` carries `EntryRef: workload:orders-api` (or the
  equivalent map-mode identity string) alongside its existing `Path`, and no other workload's
  entries are touched.
  - Test: `internal/change/entryref_test.go`
  - Verify: `go test ./internal/change/... -run TestMapModeEntryRef`
  - Level: L0
- **REQ-E1-S05-02** — Given `entries: {mode: list, root: /services, identity: {pointer:
  /name}}` over the `core-services.json` fixture and a change to one service's `tier`, when the
  differ derives entries, then the resulting `Change` carries `EntryRef: service:orders-api`
  (identity-key-derived, not index-derived). Adversarial case: reordering `services[]` in head
  with no content change yields **zero** `Change`s — a naive index-positional diff would report
  every reordered element as a false modify; this proves identity-keyed, not index-keyed,
  comparison.
  - Test: `internal/change/entryref_test.go`
  - Verify: `go test ./internal/change/... -run TestListModeEntryRefIdentityNotIndex`
  - Level: L0
- **REQ-E1-S05-03** — Given a `list`-mode declaration with no `identity.pointer` (or one whose
  resolved value collides across two or more entries in the same document), when entry
  derivation runs, then it is **rejected** with a clear reason (mapped by the caller to opaque/
  REVIEW) — never silently falling back to index-based identity. Adversarial case: two entries
  sharing the same `/name` value (a duplicate identity) is rejected, not silently resolved to
  "first match wins."
  - Test: `internal/change/entryref_test.go`
  - Verify: `go test ./internal/change/... -run TestUnkeyedOrDuplicateIdentityRejected`
  - Level: L0
- **REQ-E1-S05-04** — Given a document-mode fixture identical to the shipped P4-E1-S02 golden
  (no `entries:` declaration, or an explicit `mode: document`), when the differ runs, then its
  output is byte-identical to the pre-E1-S05 behaviour — `document` mode remains the unchanged
  default, proving this story is additive.
  - Test: `internal/change/entryref_test.go`
  - Verify: `go test ./internal/change/... -run TestDocumentModeUnchanged`
  - Level: L0
- **REQ-E1-S05-05** — Given a nested collection beyond one level (a list-mode entry whose value
  itself contains a further list, or a map nested inside a list-mode entry's identity path),
  when entry derivation encounters it, then the result is opaque with a reason naming the
  unsupported nesting depth — never a silent partial/incorrect `EntryRef` derivation.
  - Test: `internal/change/entryref_test.go`
  - Verify: `go test ./internal/change/... -run TestNestedCollectionFailsSafe`
  - Level: L0
- **REQ-E1-S05-06** — Given the determinism rule, when entry derivation runs, then
  `TestCorePurity` stays green over `internal/change/**`, `EntryRef` assignment is
  order-independent (shuffling array/map iteration order yields byte-identical output after
  canonical sort), and every golden double-runs.
  - Test: `internal/change/entryref_test.go`
  - Verify: `go test ./internal/change/... -run TestEntryRefOrderIndependent`
  - Level: L0

## E1-S06 — Classifier matcher-domain breadth (`files`/`values.pointers`/`entryEvents`/`valueChanges`) `[autonomous]`

**As a** policy pack author **I want** to match changes by file glob, by JSON-pointer value
path, by collection-entry-level event (added/deleted/renamed/modified), or by value-level event
independently **so that** I can write "any topic file under `topics/**`" and "any change to a
`replicas` field" as two different, precise matcher domains instead of one overloaded
path-as-glob-and-pointer matcher.

**Goal**: extend `internal/core/classify` with four matcher-domain evaluation primitives over
a `ChangeSet`: `files` (path glob against `Change.File`/collection root) and `values.pointers`
(JSON-Pointer glob/exact match against `Change.Path`) are two of ADR-0017 §5's four named
domains; `valueChanges` (predicate-free structural match: "any modify," "any delete," etc., at
the entry/value level, distinct from a specific pointer) is the third. The fourth primitive
this story ships is **`entryEvents`** (add/delete/rename/modify at the collection-**entry**
level — needs E1-S01/S02's `Kind`s and E1-S05's `EntryRef` add/delete/rename to be meaningful) —
a deliberately **different name** from ADR-0017 §5's `fileEvents`, because `entryEvents` matches
collection-entry identity churn (a keyed map/list entry added/removed/renamed within one file),
not ADR-0003's whole-file git-detected add/delete/rename that `fileEvents` actually denotes (see
epic Non-goals). The shipped `assent-policy`/`unclassified` reserved-class dominance
(`classify.Classify`, `classify.ValidateRouting`) is preserved unchanged — these four domains
are additive matcher vocabulary a class's routing rule can use, not a replacement for the
reserved-class short-circuit.

**Operator input**: no.

**Dependencies**: E1-S01 (add/delete `Kind`s the `entryEvents`/`valueChanges` domains match
against), E1-S02 (rename `Kind` for `entryEvents`), E1-S05 (`EntryRef` add/delete/rename —
`entryEvents` matches collection-entry events, so it needs entry derivation to exist first;
REQ-E1-S06-03's fixture is literally an E1-S05 collection-entry delete).

**Definition of done**: each of the four domains has at least one positive and one negative
match golden; a matcher combining two domains (e.g. `files: topics/**` AND `valueChanges:
delete`) narrows correctly; the shipped `TestAssentPolicyBlockGolden` /
`TestAssentPolicyDominatesMixedChangeSet` / `TestNonPolicyChangeIsUnclassified` goldens are
re-run unmodified and stay green — this story does not weaken the self-vouch boundary; every
golden double-runs.

**Not in scope**: `assent lint`'s enforcement that a `vouch` rule must be scoped to a
non-catch-all matcher (ADR-0008 amendment (a)) — that enforcement needs the E3 policy-envelope
loader; this story ships the matcher-domain evaluation functions the future lint pass will call.
ADR-0003's whole-file `fileEvents` (git-detected whole-file add/delete/rename) is explicitly
**not** implemented by this story's `entryEvents` domain — see the epic's Non-goals for the
fencing and deferred owner.

Requirements:

- **REQ-E1-S06-01** — Given a `files` domain matcher (e.g. `topics/**`) and a `ChangeSet`
  containing a change under `topics/orders.yml` and a change under `catalog/services.json`,
  when the matcher runs, then only the `topics/orders.yml` change is selected.
  - Test: `internal/core/classify/matcher_test.go`
  - Verify: `go test ./internal/core/classify/... -run TestFilesDomainMatch`
  - Level: L0
- **REQ-E1-S06-02** — Given a `values.pointers` domain matcher (e.g. `/partitions`) and a
  `ChangeSet` with changes at `/partitions` and `/owner`, when the matcher runs, then only the
  `/partitions` change is selected — proving `values.pointers` matches the field pointer, not
  the file glob (the overload ADR-0017 §5 explicitly ends).
  - Test: `internal/core/classify/matcher_test.go`
  - Verify: `go test ./internal/core/classify/... -run TestValuesPointersDomainMatch`
  - Level: L0
- **REQ-E1-S06-03** — Given an `entryEvents` domain matcher (e.g. `deleted`) and a `ChangeSet`
  containing an E1-S05 collection-entry delete and an unrelated modify, when the matcher runs,
  then only the delete event is selected. This domain matches collection-**entry** identity
  churn only — it does not detect or match whole-file add/delete/rename (that is ADR-0003's
  `fileEvents`, out of scope per this epic's Non-goals).
  - Test: `internal/core/classify/matcher_test.go`
  - Verify: `go test ./internal/core/classify/... -run TestEntryEventsDomainMatch`
  - Level: L0
- **REQ-E1-S06-04** — Given a `valueChanges` domain matcher (e.g. `modify`) and a mixed
  `ChangeSet` (one add, one delete, one modify), when the matcher runs, then only the modify
  entry is selected — proving `valueChanges` matches structurally on `Kind`, independent of
  path.
  - Test: `internal/core/classify/matcher_test.go`
  - Verify: `go test ./internal/core/classify/... -run TestValueChangesDomainMatch`
  - Level: L0
- **REQ-E1-S06-05** — Given the shipped P4-E1-S07 trust-boundary golden suite
  (`assent_policy_golden_test.go`), when it is re-run after this story's matcher-domain
  additions, then `TestAssentPolicyBlockGolden`, `TestAssentPolicyDominatesMixedChangeSet`, and
  `TestNonPolicyChangeIsUnclassified` all stay green unmodified — the four new matcher domains
  are additive and do not weaken or bypass the `.assent/**` self-vouch boundary. Adversarial
  case: a matcher combining `files: .assent/**` with any `valueChanges` domain still routes to
  `assent-policy`, never to a class that could vouch.
  - Test: `internal/core/classify/assent_policy_golden_test.go`
  - Verify: `go test ./internal/core/classify/... -run TestAssentPolicyBlockGolden`
  - Level: L0
- **REQ-E1-S06-06** — Given the determinism rule, when the matcher domains run, then
  `TestCorePurity` stays green over `internal/core/classify/**`, matching is order-independent
  over the input `ChangeSet`, and every golden double-runs.
  - Test: `internal/core/classify/matcher_test.go`
  - Verify: `go test ./internal/core/classify/... -run TestMatcherDomainOrderIndependent`
  - Level: L0

## E1-S07 — Input resource limits (size/depth/entry-count/alias-expansion ceilings) `[autonomous]`

**As a** platform operator **I want** every format adapter to enforce hard ceilings on input
size, nesting depth, entry count, and alias/anchor expansion **so that** a maliciously or
accidentally oversized/deeply-nested/alias-bombing file fails closed to REVIEW instead of
consuming unbounded memory/CPU or (worse) silently mis-diffing.

**Goal**: centralize pure, parse-time ceilings — max file size (bytes), max nesting depth, max
total entry/node count, and max alias/anchor expansion factor (generalizing the shipped
billion-laughs YAML rejection, `TestOpaqueInputFailsSafe`'s alias case) — in the E1-S03 shared
value-tree parse/walk entry point, so YAML/JSON/HCL adapters inherit one enforcement point
instead of three duplicated checks. A breach of any ceiling yields an opaque `ChangeSet` with a
reason naming which ceiling was breached, exactly like every other opaque path in this package
— never a crash, never a partial diff, never a silent skip. Ceilings are **pure** (comparisons
against already-parsed/parsing structure); a wall-clock parse **deadline** is explicitly out of
scope (judgment call, see epic header) because it needs a clock the purity gate bars from this
package.

**Operator input**: no.

**Dependencies**: E1-S03 (JSON adapter + shared tree — this is where the one enforcement point
lives), E1-S04 (HCL adapter — must inherit the same ceilings, proven by its own adversarial
golden).

**Definition of done**: an over-size, over-depth, over-entry-count, or over-alias-expansion
fixture in each of YAML/JSON/HCL fails opaque with a reason naming the breached ceiling; a
fixture just under every ceiling parses and diffs normally (proving the ceilings are not so
tight they reject legitimate fixtures); the shipped billion-laughs golden
(`TestOpaqueInputFailsSafe`'s "billion-laughs alias expansion" case) still passes, generalized
under the new shared enforcement rather than YAML-only ad hoc rejection; every golden
double-runs.

**Not in scope**: a wall-clock parse deadline (fenced to a future `cmd/assent`-tier story, not
this epic's brief); making the ceiling values operator-configurable via an authored policy
envelope (E3 — this story ships fixed, documented default ceilings; a config surface for them
is a later, separately-scoped concern if ever needed). Also not in scope: ADR-0003 Amendment
2's **symlink/path-traversal-name rejection** and an **aggregate (whole-MR) file-count
ceiling** — distinct from this story's PER-FILE size/depth/entry-count ceilings above. A
single-file adapter call (what `internal/change` sees) cannot observe the MR's file set or its
filesystem entry types; those checks are properties of the *enumerated* changed-file set, not
of one file's parsed content, and this matters concretely because E1-S08 introduces enumerating
the MR's full changed-file set — unbounded file count / a symlinked path first becomes
exploitable there. The natural owner is **E1-S08 or a fast-follow to it**; this story does not
implement them and this gap must not be read as already covered by the per-file ceilings above.

Requirements:

- **REQ-E1-S07-01** — Given a YAML/JSON/HCL fixture exceeding the maximum input byte size, when
  the differ runs, then it returns an opaque `ChangeSet` whose reason names the size ceiling —
  proven for all three formats with one shared fixture-generation helper.
  - Test: `internal/change/limits_test.go`
  - Verify: `go test ./internal/change/... -run TestOversizeInputFailsSafe`
  - Level: L0
- **REQ-E1-S07-02** — Given a fixture whose nesting depth exceeds the maximum, when the differ
  runs, then it returns opaque with a reason naming the depth ceiling, for all three formats.
  - Test: `internal/change/limits_test.go`
  - Verify: `go test ./internal/change/... -run TestExcessiveDepthFailsSafe`
  - Level: L0
- **REQ-E1-S07-03** — Given a fixture whose total node/entry count exceeds the maximum, when the
  differ runs, then it returns opaque with a reason naming the entry-count ceiling, for all
  three formats.
  - Test: `internal/change/limits_test.go`
  - Verify: `go test ./internal/change/... -run TestExcessiveEntryCountFailsSafe`
  - Level: L0
- **REQ-E1-S07-04** — Given the shipped billion-laughs YAML alias-expansion fixture
  (`TestOpaqueInputFailsSafe`'s case) and an equivalent constructed JSON/HCL alias/anchor-style
  expansion bomb where the format supports it (YAML anchors; a HCL/tfvars construct is not
  expected to have an equivalent — documented as such), when the differ runs, then it returns
  opaque with a reason naming the alias-expansion ceiling, and the shipped YAML case's existing
  test keeps passing unmodified. Adversarial case: a fixture crafted to sit just under the
  expansion-factor ceiling but still consume disproportionate memory (a wide-but-shallow
  expansion) is proven to also be caught by the entry-count ceiling (REQ-E1-S07-03), so the two
  ceilings jointly close the class, not depth/expansion alone.
  - Test: `internal/change/limits_test.go`
  - Verify: `go test ./internal/change/... -run TestAliasExpansionFailsSafe`
  - Level: L0
- **REQ-E1-S07-05** — Given a fixture just under every ceiling (size, depth, entry count,
  expansion), when the differ runs, then it parses and diffs normally — a non-opaque
  `ChangeSet` — proving the ceilings do not false-positive on legitimate large-but-bounded
  fixtures.
  - Test: `internal/change/limits_test.go`
  - Verify: `go test ./internal/change/... -run TestUnderCeilingsParsesNormally`
  - Level: L0
- **REQ-E1-S07-06** — Given the determinism rule, when the ceilings run, then `TestCorePurity`
  stays green over `internal/change/**` (the ceilings are pure comparisons, no clock/context
  introduced), and every new golden double-runs byte-identical.
  - Test: `internal/change/limits_test.go`
  - Verify: `go test ./internal/change/... -run TestLimitsDoubleRunStable`
  - Level: L0

## E1-S08 — `cmd/assent`: enumerate the MR's full changed-file set `[autonomous]`

**As a** platform operator **I want** `assent run` to classify and route **every** file the MR
changed, not just the one file named as the policy binding's governed subject **so that** an MR
that smuggles a `.assent/**` policy edit alongside an unrelated governed-file change is caught
by the live adapter path, not only by the engine-tier golden.

**Goal**: close the named live-adapter gap logged at D-042 ("S10 review F1"): `cmd/assent/
run.go` today derives the single file it diffs from `binding.Subject` (`governed :=
strings.TrimPrefix(binding.Subject, "file:")`, `run.go:151`) — a hardcoded single-file diff.
This story changes the CLI/CI-env adapter to enumerate the **full set of files the MR's source
branch changed relative to the target ref** (from the local checkout, per ADR-0008 §4 — no new
Forge port method is prescribed by this story; the mechanism is left to the implementer), diff
each changed file, classify the union of resulting `ChangeSet`s, and route/evaluate across all
of them — so a mixed MR (one governed-entry change + one `.assent/**` change) is classified to
`assent-policy` → BLOCK on the real adapter path, complementing (not replacing) the existing
`internal/core/classify` engine-tier golden (P4-E1-S07-01). **Mechanism fence**: the enumeration
and every per-file content read this story performs MUST come from the **local checkout only**
(ADR-0008 §4: "No API-only file fetching") — no additional per-file `client.FileAtRef`-style API
call for the newly-enumerated files. This is a deliberate constraint, not a style preference:
the already-shipped `run.go` fetches its one governed file's content via `client.FileAtRef`
(API-only), which is itself the exact pattern ADR-0008 §4 prohibits; this story must not
inherit or multiply that pre-existing violation by adding an API call per enumerated file. Fixing
the shipped single-file `FileAtRef` read is not this story's job (out of scope, unchanged
below) — only the *new* changed-file-set enumeration this story adds must be checkout-sourced.

**Operator input**: no — this story is gated against an injected/fixture changed-file list or
an in-memory fake checkout, never a live GitLab session; the mechanism this story adds is
provably correct against fixtures, and any live-MR proof of the same behaviour belongs to
E4/E7, not here.

**Dependencies**: none (uses the already-shipped P4-E1 differ + classifier; independently
buildable and startable from day one, in parallel with E1-S01..S07).

**Definition of done**: given an injected list of changed files (a fixture directory or
in-memory checkout double) including both a governed entry and a `.assent/**` file, `assent
run` diffs and classifies **all** of them, and the run BLOCKs exactly as
`TestAssentPolicyDominatesMixedChangeSet` predicts at the engine tier — now proven on the
adapter path (`cmd/assent`); a changed-file set containing **only** non-policy files behaves
exactly as the shipped single-governed-file path did (no regression); the existing
`cmd/assent/run_test.go` suite (governed-subject-only fixtures) is re-run and stays green,
proving this is additive, not a breaking rewrite of the run path.

**Not in scope**: any live-GitLab proof of the same behaviour (E4/E7); a new/changed `internal/
forge` port method for listing changed files (mechanism is implementer's choice, fenced only by
"local checkout," per the epic's judgment-call note); the D-034 protected-source verification
hard gate (untouched by this story — arming/`doctor` preconditions are unrelated to which files
get diffed).

Requirements:

- **REQ-E1-S08-01** — Given an injected changed-file list containing a governed-entry file
  (e.g. `topics/orders.yml`) and an unrelated `.assent/packs/topic.yml` file, when `assent run`
  executes, then it diffs and classifies **both** files, the union routes to `assent-policy`,
  and the run **BLOCK**s — never APPROVE/vouch — exercised on the live-adapter code path
  (`cmd/assent`), not only the engine-tier `classify` golden.
  - Test: `cmd/assent/run_test.go`
  - Verify: `go test ./cmd/assent/... -run TestRunEnumeratesChangedFileSetAndBlocksOnPolicyEdit`
  - Level: L1
- **REQ-E1-S08-02** — Given an injected changed-file list containing **only** non-policy files
  (the shipped single-governed-file shape), when `assent run` executes, then its behaviour is
  unchanged from the pre-E1-S08 adapter — the existing `run_test.go` fixtures and assertions
  pass without modification, proving this story is additive.
  - Test: `cmd/assent/run_test.go`
  - Verify: `go test ./cmd/assent/...`
  - Level: L1
- **REQ-E1-S08-03** — Given a changed file that the differ cannot decide (opaque per any of
  E1-S01/S03/S04/S07's fail-closed paths), when it is one entry among several enumerated
  files, then the overall run still fails safe (opaque routes to REVIEW/BLOCK exactly as a
  single-file opaque result would) — one undecidable file among many enumerated files does not
  get silently dropped from the union while the others proceed as if nothing were wrong.
  Adversarial case: an opaque `.assent/**` file plus a clean governed-entry file still routes
  the whole run through the fail-safe path, never approves on the strength of the clean file
  alone.
  - Test: `cmd/assent/run_test.go`
  - Verify: `go test ./cmd/assent/... -run TestRunOpaqueFileAmongManyFailsSafe`
  - Level: L1
- **REQ-E1-S08-04** — Given the determinism rule, when the changed-file-set enumeration runs,
  then it is proven against an injected/fixture changed-file list or in-memory checkout double
  only — no live network call, no `os.Getenv` read outside `cmd/assent`'s existing CI-env
  adapter boundary (REQ-P4-E1-S01-03's guard is unaffected, since `internal/core`/`internal/
  change` still read only pinned values) — and the resulting decision path double-runs
  byte-identical.
  - Test: `cmd/assent/run_test.go`
  - Verify: `go test ./cmd/assent/... -run TestRunChangedFileSetDoubleRunStable`
  - Level: L1
