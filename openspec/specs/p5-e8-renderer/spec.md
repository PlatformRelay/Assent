# P5-E8 — Renderer & presentation (ADR-0016 tier 0)

**Epic ID / REQ prefix:** `E8` / `REQ-E8-S0n-nn`.

**Problem**: E2–E7 closed the decision engine, provider host, GitLab forge port, adopter harness,
and conformance infra — but **presentation is still a placeholder**. `internal/core/decision`
already emits a schema-valid **`PresentationModel`** companion (`record.go` +
`presentation-model.schema.json`) with no rendered markdown (ADR-0016 §4). `cmd/assent/run.go`
still posts a one-line stub thread body (`buildDesired`, ~L659) and **never implements P3-E5 step 3**
(the per-MR `summary-comment` slot — deferred from E4 per **D-073**). Markers are rendered only
inside `internal/forge/gitlab/marker.go`, not by a central renderer envelope. E5 propagated
`Fact.Sensitive` onto the CEL envelope (**D-068**) with an explicit **E8 redaction handoff**.
ADR-0016 tier 0 (D-012/D-015) names the missing product surface: renderer-owned envelope,
config knobs, CEL message interpolation, `assent render`, default-theme goldens, template lint,
and an `en` locale catalog — **without** tier 1–2 slot/full-template overrides.

**Key ground truth (de-risks the epic):**
- **Reuse frozen contracts:** consume `schemas/decision/v1alpha1/presentation-model.schema.json`
  and `internal/core/decision.PresentationModel` — **do not** invent parallel render types.
  Epic DoD prefers **`git diff schemas/` == 0** except the **one unavoidable** tier-0 config
  extension (judgment **D-088**).
- **CEL activation already exists:** `internal/core/aggregate` + `hack/spikes/cel/activation.go`
  prove the shared env for `assert` leaves; E8 extends the same model to `{{ }}` message
  interpolation (ADR-0016 §2).
- **Markers are frozen:** `docs/contracts/p3-e5-publication-protocol/marker-grammar.schema.json`
  + `internal/forge.Marker` are the correlation contract; the renderer **wraps** marker JSON in
  the envelope region — reconciliation continues to parse via `forge/gitlab.parseMarker`.
- **Safety vs wording split is already frozen:** ADR-0014 / E6 assert structured findings only;
  E8 goldens prove wording changes do not break `assent test` safety counts.
- **`internal/core` stays I/O-free** (`TestCorePurity`); renderer lives in `internal/render/**`
  + `cmd/assent/render.go`; forge summary wiring touches `internal/forge/**` and `run.go` only
  at the edge (late slice).

**Scope**: (S01) renderer package + PresentationModel fixture loader; (S02) presentation config
knobs in `.assent/config.yaml`; (S03) `en` locale catalog; (S04) renderer-owned envelope +
marker region; (S05) markdown/HTML escaping + length clamping; (S06) sensitive-fact redaction
(D-068); (S07) CEL message rendering; (S08) default theme finding-thread layout + wire
`buildDesired` thread bodies; (S09) default-theme golden markdown snapshots; (S10) `assent render`
CLI; (S11) presentation/template lint at load time; (S12) summary-comment slot (P3-E5 step 3,
closes D-073); (S13) autonomous exit gate. **Infra-gated:** none — entire epic is fixture/hermetic
goldens; live forge summary proof mirrors E4-S11 optional pattern but is **not** an E8 story.

**Non-goals** (fenced): **GitHub adapter** (E10, D-012 locked); **release / goreleaser / cosign**
(E9); **PolicyComparisonSuite runner** (D-057 deferred); **tier 1–2 templates** (slot overrides,
full artifact templates — designed seams only, no `.assent/templates/` loader); **Rego backend**
(E11); **`serve` / keyed per-MR lock** (E12); **markdown report artifact** (tier 2); **widening
PresentationModel** beyond additive tolerance already in schema; **replacing adopter safety
assertions with `message~`** (discouraged, stays non-coverage).

**ADRs**: 0012 (layout/lifecycle/redaction — override mechanism superseded), 0016 (tier 0 scope,
envelope invariant, CEL messages, render goldens, locale catalog), 0014 (safety vs render test
split), 0019 (marker grammar — renderer-owned region), 0010 (config.yaml wiring). **Reuse**:
`internal/core/decision`, frozen PresentationModel schema, aggregate CEL env, forge Marker types,
P3-E5 summary fixtures, `examples/contracts/d016-strict-fixture/presentation-model.json`.
**New**: `internal/render/**`, render fixture corpus, `cmd/assent render`, config `presentation:`
block, locale catalog, summary Reconcile path, golden markdown tests.

**Executability**: S01–S13 **`[autonomous]`** — hermetic fixtures, in-memory fake forge for
summary slot, no live GitLab. TDD; determinism `-count=2` on render goldens; `TestCorePurity`
untouched for `internal/core`. **Engine-grade / fail-safety review:** S04 (envelope/markers), S05
(escaping/clamping), S06 (redaction), S07 (CEL messages), S11 (lint), S12 (summary reconcile),
S13 (exit gate).

**Judgment calls (decide-and-log / operator):**
(a) **DECIDED — tier-0 `presentation` block extends `config.schema.json`.** ADR-0016 tier 0
requires repo-level knobs; strict `additionalProperties:false` on Config rejects an unschema'd
block. This is the **one allowed** schema touch for E8 (optional nested object only). Logged
**D-088**.
(b) **DECIDED — safe presentation defaults:** global `verbosity: standard`, `emoji: true`,
`collapseThreshold: 5` (hide detail bodies beyond N same-code findings), `locale: en`; per-environment
overrides optional under `presentation.environments[]` matching Config environment names.
Logged **D-089**.
(c) **DECIDED — sensitive values render as `[redacted]` in all user-visible regions** (headline,
details, debug lines); CEL may still read raw values during evaluation — redaction is render-time
only (D-068 handoff). Logged **D-090**.
(d) **DECIDED — comment field clamp: 500 runes** per interpolated/scalar field in forge-facing
markdown (full values remain in JSON report / ReplayBundle). Logged **D-091**.
(e) **DECIDED — render fixtures live under `examples/render/<case>/`** with committed
`presentation-model.json`, optional `render-context.json` (CEL activation + marker pins + config),
and golden `expect.<artifact>.md`. Logged **D-092**.
(f) **DECIDED — D-073 closes in E8-S12; summary slot is not deferred again.** Hermetic fake +
P3-E5 fixture replay first; live GitLab note upsert is optional follow-up (E4-S11 class), not an
E8 merge blocker. Logged **D-093**.
(g) **DECIDED — marker bytes are owned by `internal/render` envelope; forge/gitlab delegates.**
`render.Envelope(marker, body)` is the single writer of `<!-- assent:marker … -->`; gitlab adapter
stops duplicating marker JSON assembly (wrap-only). Logged **D-094**.
(h) **DECIDED — presentation lint runs at pack/config load (same gate as `assent lint`), not at
render time.** Unknown CEL identifiers in `message`/`debug` lines are hard errors — never `<no value>`
at render (ADR-0016 §2). Logged **D-095**.

**Dependency order**: **S01 → S02 → S03 → S04 → {S05 ∥ S06} → S07 → S08 → {S09 ∥ S10} → S11 →
S12 → S13**. **Closes D-073 (summary slot): S12. Closes D-068 (presentation redaction): S06.
Closes ADR-0016 tier 0: S13.**

**Coordination note:** S08 edits `cmd/assent/run.go` (`buildDesired` body) — same file as E4/E5
orchestration; extend in place, do not fork a second run loop. S12 adds `DesiredReviewState.Summary`
+ early Reconcile step 3 — coordinate with `internal/forge/conformance` P3-E5 replay tests (extend,
do not rewrite E4-S09).

---

## E8-S01 — Renderer scaffold + PresentationModel fixture loader [autonomous]

**As a** renderer implementer **I want** a pure `internal/render` package that decodes frozen
PresentationModel fixtures **so that** every later slice shares one typed consumer seam.

**Goal**: Add `internal/render` with strict JSON decode of `decision.PresentationModel` (or
schema-validated bytes via `schemas.PresentationModelSchema`), a `Fixture` type loading
`examples/render/<case>/presentation-model.json`, and a no-op `RenderFindingThread` stub that
returns an error until S08. Package stays I/O-free except explicit fixture paths in tests.

**Dependencies**: P3-E1 PresentationModel schema, E2 `decision.Build` types.

**Definition of done**: loader tests pass; invalid fixture fails closed; `git diff schemas/` == 0.

Requirements:
- **REQ-E8-S01-01** — strict decode rejects unknown required-field absence and invalid enum
  decision. Test: `internal/render/fixture_test.go`; Verify:
  `go test ./internal/render/... -run TestLoadPresentationFixture`; Level: L0
- **REQ-E8-S01-02** — loader accepts `examples/contracts/d016-strict-fixture/presentation-model.json`
  unchanged. Test: same; Verify:
  `go test ./internal/render/... -run TestLoadD016Fixture`; Level: L0
- **REQ-E8-S01-03** — package doc states PresentationModel is the sole render contract (no
  parallel struct). Test: `internal/render/doc.go`; Verify:
  `grep -q PresentationModel internal/render/doc.go`; Level: doc

---

## E8-S02 — Presentation config knobs (`.assent/config.yaml`) [autonomous]

**As a** pack operator **I want** tier-0 presentation knobs in config **so that** verbosity, emoji,
collapse, and locale are data-driven per ADR-0016 §1.

**Goal**: Extend `schemas/policy/v1alpha1/config.schema.json` with optional top-level `presentation`
(D-088): `verbosity` (`minimal|standard|full`), `emoji` (bool), `collapseThreshold` (int ≥1),
`locale` (string, default `en`), optional `environments[]` overrides keyed by environment name.
Load in config parser; expose `render.Options` resolved for the active environment.

**Dependencies**: E8-S01, ADR-0010 config loader.

**Definition of done**: schema compiles; strict-decode compat tests updated; defaults match D-089.

Requirements:
- **REQ-E8-S02-01** — unknown `presentation` enum values reject at load. Test:
  `schemas/testdata/compat/strict-decode/config/` (new vectors); Verify:
  `go test ./schemas/... -run StrictDecodeConfig`; Level: L0
- **REQ-E8-S02-02** — missing `presentation` block yields D-089 defaults. Test:
  `internal/render/options_test.go`; Verify:
  `go test ./internal/render/... -run TestDefaultPresentationOptions`; Level: L0
- **REQ-E8-S02-03** — per-environment override wins over global for matched env name. Test: same;
  Verify: `go test ./internal/render/... -run TestPresentationEnvOverride`; Level: L0

---

## E8-S03 — `en` locale catalog (chrome strings) [autonomous]

**As a** contributor **I want** fixed renderer chrome in a catalog **so that** lifecycle strings
are translatable data, not template forks (ADR-0016 §5).

**Goal**: Add `internal/render/locale/en.yaml` (or embedded Go map) keyed by stable ids
(`resolve_thread`, `evaluation_details`, `why_this_check`, `summary_headline`, …). Renderer
looks up strings by `Options.Locale`; unknown locale fails closed to `en`.

**Dependencies**: E8-S02.

**Definition of done**: catalog covered by unit tests; only `en` shipped.

Requirements:
- **REQ-E8-S03-01** — every default-theme chrome id used in S08 exists in the `en` catalog.
  Test: `internal/render/locale/catalog_test.go`; Verify:
  `go test ./internal/render/... -run TestEnCatalogComplete`; Level: L0
- **REQ-E8-S03-02** — unknown locale falls back to `en` with a lint warning (not a render panic).
  Test: same; Verify: `go test ./internal/render/... -run TestUnknownLocaleFallback`; Level: L0

---

## E8-S04 — Renderer-owned envelope + marker region [autonomous · engine-grade]

**As a** reconciliation implementer **I want** markers emitted only inside a renderer-owned
envelope **so that** user/content regions can never drop lifecycle metadata (ADR-0016 §1,
ADR-0019).

**Goal**: Implement `render.Envelope(marker forge.Marker, body string) (string, error)` that
prepends/appends the hidden HTML marker comment outside any future customizable region; reject
bodies containing marker sentinels or premature `-->`. Refactor `internal/forge/gitlab/marker.go`
to delegate marker JSON to `render.FormatMarker` (D-094) while keeping parse in gitlab/forge.

**Dependencies**: E8-S01, forge.Marker, marker-grammar schema.

**Definition of done**: round-trip parse stable; envelope tests prove marker cannot be stripped by
body content.

Requirements:
- **REQ-E8-S04-01** — `Envelope` always places exactly one well-formed `<!-- assent:marker … -->`
  outside the markdown body region. Test: `internal/render/envelope_test.go`; Verify:
  `go test ./internal/render/... -run TestEnvelopeMarkerOutsideBody`; Level: L0
- **REQ-E8-S04-02** — body containing `assent:marker` sentinel fails closed at envelope time.
  Test: same; Verify: `go test ./internal/render/... -run TestEnvelopeRejectsEmbeddedSentinel`; Level: L0
- **REQ-E8-S04-03** — gitlab `renderMarker` delegates to render helper; existing gitlab marker
  tests stay green. Test: `internal/forge/gitlab/marker_test.go`; Verify:
  `go test ./internal/forge/gitlab/... -run Marker`; Level: L1

---

## E8-S05 — Markdown/HTML escaping + length clamping [autonomous · engine-grade]

**As a** security reviewer **I want** injection-safe rendering **so that** untrusted values cannot
forge approvals or break `<details>` blocks (ADR-0012 amendment).

**Goal**: Central `render.EscapeMarkdown(s string) string` and `render.Clamp(s string, maxRunes int)`
(default D-091 = 500 runes). Apply to all interpolated scalars before layout assembly.

**Dependencies**: E8-S04.

**Definition of done**: adversarial fixtures prove no raw HTML/script leakage; clamp truncates with
ellipsis.

Requirements:
- **REQ-E8-S05-01** — values containing `<script>`, raw `<details>`, and markdown link injections
  render inert (escaped or stripped). Test: `internal/render/escape_test.go`; Verify:
  `go test ./internal/render/... -run TestEscapeMarkdownAdversarial`; Level: L0
- **REQ-E8-S05-02** — clamp enforces D-091 rune limit with stable ellipsis suffix. Test:
  `internal/render/clamp_test.go`; Verify:
  `go test ./internal/render/... -run TestClamp500`; Level: L0

---

## E8-S06 — Sensitive fact redaction (D-068 handoff) [autonomous · engine-grade]

**As a** operator **I want** sensitive facts redacted in forge markdown **so that** E5's
`Fact.Sensitive` marker never leaks values in comments (ADR-0012 A-08 / D-068).

**Goal**: When render activation includes facts with `sensitive: true`, replace displayed values
with `[redacted]` (D-090) in headline, docs, and evaluation-details sections — while leaving CEL
evaluation untouched. Cover resolved-fact values and debug lines referencing sensitive paths.

**Dependencies**: E8-S05, E5 `provider.ToAggregateFact` / `aggregate.Fact.Sensitive`.

**Definition of done**: golden proves sensitive path never appears in rendered markdown; non-sensitive
facts still visible.

Requirements:
- **REQ-E8-S06-01** — sensitive fact value never appears in rendered finding-thread markdown.
  Test: `internal/render/redact_test.go`; Verify:
  `go test ./internal/render/... -run TestSensitiveFactRedacted`; Level: L0
- **REQ-E8-S06-02** — non-sensitive facts remain visible in evaluation details. Test: same; Verify:
  `go test ./internal/render/... -run TestNonSensitiveFactsVisible`; Level: L0

---

## E8-S07 — CEL message rendering (`{{ }}` over predicate scope) [autonomous · engine-grade]

**As a** rule author **I want** one expression language for assert and messages **so that** load-time
lint catches typos before render (ADR-0016 §2, ADR-0013).

**Goal**: Implement `render.EvalMessage(expr string, activation cel.Activation) (string, error)`
using the same CEL env as aggregate evaluation (reuse/refactor shared env builder — D-095). Support
rule `message`, `docs.summary`, and `debug:` lines from pack metadata + render context. Unknown
fields = compile error.

**Dependencies**: E8-S05, aggregate CEL env, `hack/spikes/cel` patterns.

**Definition of done**: archetype rule messages render deterministically; compile failures surface at
lint (S11), not as `<no value>`.

Requirements:
- **REQ-E8-S07-01** — `{{ old }}`, `{{ new }}`, `{{ facts.quota.max_partitions }}` render correctly
  on a fixed activation fixture. Test: `internal/render/message_test.go`; Verify:
  `go test ./internal/render/... -run TestCELMessageInterpolation`; Level: L0
- **REQ-E8-S07-02** — type/unknown-field expressions fail at compile with located error. Test:
  `internal/render/message_test.go`; Verify:
  `go test ./internal/render/... -run TestCELMessageCompileError`; Level: L0

---

## E8-S08 — Default theme: finding-thread layout + run wiring [autonomous]

**As a** contributor **I want** ADR-0012 default layout in forge threads **so that** MR comments
match dry-run/`assent render` output.

**Goal**: Implement `render.RenderFindingThread(pm PresentationModel, finding, Context) string`
producing ADR-0012 layout (headline, resolve CTA from locale, collapsible docs + evaluation details)
honouring verbosity/emoji/collapse from Options. Replace `buildDesired` stub body in
`cmd/assent/run.go` with renderer output + `render.Envelope`.

**Dependencies**: E8-S03..S07, E4 `buildDesired` marker wiring.

**Definition of done**: run-path thread bodies use renderer; REVIEW/BLOCK still one thread.

Requirements:
- **REQ-E8-S08-01** — default layout includes resolve CTA + evaluation `<details>` for `standard`
  verbosity. Test: `internal/render/theme_default_test.go`; Verify:
  `go test ./internal/render/... -run TestDefaultThemeStandard`; Level: L0
- **REQ-E8-S08-02** — `minimal` verbosity omits evaluation details block. Test: same; Verify:
  `go test ./internal/render/... -run TestDefaultThemeMinimal`; Level: L0
- **REQ-E8-S08-03** — `buildDesired` uses renderer output (no fmt.Sprintf stub). Test:
  `cmd/assent/run_render_test.go`; Verify:
  `go test ./cmd/assent/... -run TestBuildDesiredUsesRenderer`; Level: L1

---

## E8-S09 — Default-theme golden markdown snapshots [autonomous]

**As a** maintainer **I want** committed render goldens **so that** "strong defaults" is enforced
(ADR-0016 §4).

**Goal**: Seed `examples/render/` corpus (D-092) with ≥3 cases (challenge, block, require-review)
each with `presentation-model.json`, `render-context.json`, and `expect.finding-thread.md`.
Tests compare normalized markdown; `-count=2` determinism gate.

**Dependencies**: E8-S08.

**Definition of done**: goldens green; wording-only change fails test until golden update intentional.

Requirements:
- **REQ-E8-S09-01** — golden test renders each fixture case byte-identically to committed expect file.
  Test: `internal/render/golden_test.go`; Verify:
  `go test ./internal/render/... -run TestRenderGoldens`; Level: L1
- **REQ-E8-S09-02** — double-run produces identical markdown (`-count=2`). Test: same; Verify:
  `go test ./internal/render/... -run TestRenderGoldens -count=2`; Level: L1

---

## E8-S10 — `assent render` CLI against fixtures [autonomous]

**As a** pack author **I want** `assent render` **so that** I can preview forge markdown without a
live MR (ADR-0016 §4).

**Goal**: Add `assent render --fixture examples/render/<case> [--artifact finding-thread|summary]
[--presentation-minimal|--presentation-full]` writing markdown to stdout; strict fixture validation;
exit non-zero on render/validation errors.

**Dependencies**: E8-S09.

**Definition of done**: CLI mirrors test renderer; documented in `cmd/assent` help.

Requirements:
- **REQ-E8-S10-01** — `assent render --fixture …` stdout equals golden expect file for default case.
  Test: `cmd/assent/render_test.go`; Verify:
  `go test ./cmd/assent/... -run TestRenderCLI`; Level: L1
- **REQ-E8-S10-02** — invalid fixture path exits non-zero with located error on stderr. Test: same;
  Verify: `go test ./cmd/assent/... -run TestRenderCLIInvalidFixture`; Level: L0

---

## E8-S11 — Presentation/template lint at load time [autonomous · engine-grade]

**As a** pack maintainer **I want** presentation mistakes to fail `assent lint` **so that** broken
messages never reach render (ADR-0016 §4 template lint).

**Goal**: Extend `internal/lint` (or `assent lint`) to compile-check every rule `message`, `docs.summary`,
and `debug:` CEL expression against the predicate scope; reject marker-region violations in any
authored template fragment (tier 0: config-only — flag `.assent/templates/**` as **tier 1 deferred**
with explicit error if present). Wire lint into existing scope tests.

**Dependencies**: E8-S07, E8-S02, E3 lint surface.

**Definition of done**: bad message fails lint; good archetype packs pass.

Requirements:
- **REQ-E8-S11-01** — unknown CEL field in `message` fails lint with rule location. Test:
  `internal/lint/presentation_test.go`; Verify:
  `go test ./internal/lint/... -run TestLintUnknownMessageField`; Level: L0
- **REQ-E8-S11-02** — presence of `.assent/templates/` returns tier-1 deferred error (not silent
  ignore). Test: same; Verify:
  `go test ./internal/lint/... -run TestLintRejectsTier1Templates`; Level: L0

---

## E8-S12 — Summary-comment slot: P3-E5 step 3 + D-073 closure [autonomous · engine-grade]

**As a** reconciliation implementer **I want** the per-MR summary upsert **so that** P3-E5 step 3
and E4 judgment call (a) close without waiting for live GitLab (D-093).

**Goal**: Add `DesiredReviewState.Summary *DesiredSummary` (marker + rendered body). Implement
`reconcileSummary` as Reconcile step 3 (before thread loop): upsert exactly one bot `summary-comment`
artifact in fake + gitlab httptest (edit-in-place, never re-post). Render summary via
`render.RenderSummary(pm, Context)` using default theme. Wire `buildDesired` / run path to populate
Summary for all decision outcomes. Extend P3-E5 replay conformance to assert `summaryUpdated`.

**Dependencies**: E8-S08, E4 Reconcile engine, P3-E5 fixtures (`rerun-idempotence.yaml`, etc.).

**Definition of done**: hermetic tests prove one summary note; rerun idempotence fixture passes step 3;
D-073 marked closed in decisions.md when implemented.

Requirements:
- **REQ-E8-S12-01** — Reconcile creates or updates exactly one `summary-comment` bot note per MR.
  Test: `internal/forge/reconcile_summary_test.go`; Verify:
  `go test ./internal/forge/... -run TestReconcileSummaryUpsert`; Level: L1
- **REQ-E8-S12-02** — identical DesiredReviewState rerun updates summary in place (zero new summary
  notes). Test: same + `internal/forge/conformance/reconciliation_test.go`; Verify:
  `go test ./internal/forge/... -run 'Summary|RerunIdempotence'`; Level: L1
- **REQ-E8-S12-03** — summary body is renderer output (includes decision hash marker region via
  `render.Envelope`). Test: `cmd/assent/run_render_test.go`; Verify:
  `go test ./cmd/assent/... -run TestBuildDesiredSummaryUsesRenderer`; Level: L1

---

## E8-S13 — Exit gate: render goldens green + safety split proven [autonomous · engine-grade]

**As a** maintainer **I want** the autonomous E8 slice proven in CI **so that** E9 release can ship
the renderer product.

**Goal**: (1) all E8 L0/L1 tests green under `task check`; (2) render goldens + `-count=2` wired in
verify determinism job (reuse E7-S04 pattern); (3) test proves an intentional wording golden change
does **not** fail `assent test` safety coverage on the same fixture pack; (4) backlog marks E8 spec
authoritative; (5) schema diff limited to D-088 presentation block only.

**Dependencies**: E8-S01..S12.

**Definition of done**: exit-gate checklist green; D-088–D-095 recorded; D-073 closed.

Requirements:
- **REQ-E8-S13-01** — exit-gate test runs render goldens + lint + summary reconcile slice. Test:
  `internal/render/exitgate_test.go`; Verify:
  `go test ./internal/render/... ./internal/forge/... ./cmd/assent/... -run 'ExitGate|RenderGoldens|ReconcileSummary'`; Level: L1
- **REQ-E8-S13-02** — safety split: `assent test` structured assertions unchanged when render golden
  wording differs (ADR-0014). Test: `cmd/assent/render_exitgate_test.go`; Verify:
  `go test ./cmd/assent/... -run TestE8ExitGateSafetySplit`; Level: L1
- **REQ-E8-S13-03** — backlog + later-phases mark E8 autonomous slice ready/closed per operator merge.
  Test: `openspec/specs/backlog.md`; Verify:
  `grep -q p5-e8-renderer/spec.md openspec/specs/backlog.md`; Level: doc

---

## Epic definition of done

| Gate | Criterion |
| --- | --- |
| **Autonomous (S13)** | S01–S12 + S13 green; render goldens + lint + summary slot hermetic; `assent render` works on fixtures |
| **Schemas** | `git diff schemas/` == presentation block only (D-088); PresentationModel schema unchanged |
| **Safety split** | Wording goldens do not break E6 safety coverage (ADR-0014) |
| **Deferred seams** | Tier 1–2 templates, GitHub, Rego, release — fenced non-goals |
| **Next epic** | E9 release may ship binaries/docs with renderer product |

**Story count:** 13 — **13 autonomous**, **0 infra-gated**.

**Do first:** **E8-S01** — thinnest vertical scaffold (typed PresentationModel consumer) before config/theme work.
