# ADR acceptance review (P2-E5 — Phase-2 gate)

**Date**: 2026-07-21  
**Scope**: ADR-0002 through ADR-0017 (ADR-0001 already Accepted).  
**Evidence base**: Phase-1 dossiers/prior-art/archetypes + Phase-2 spikes A–C + secure-setup  
topology (P2-E4). **P2-E6 deferred** (D-018) — does not gate this round.  
**Authority**: decide-and-log (agent-loop-local); operator ratifies after the fact.

## Recommended verdicts (summary)

| ADR | Recommended verdict | Rationale (one line) |
| --- | --- | --- |
| 0002 | **Accepted** | Envelope + pluggable backends confirmed by prior-art (Kyverno/conftest) + ADR-0013 hybrid |
| 0003 | **Accepted** (partial: ADR-0017 §5) | Semantic change model confirmed by Renovate; EntryRef/matcher domains reshape path overload |
| 0004 | **Accepted** | Spike C proves typed HTTP/exec + token isolation; gRPC deferred (D-012 / OQ-4) |
| 0005 | **Accepted** (partial: ADR-0017 §1/§7) | GitLab-first + dossiers; merge-result / Reconcile reshape adapter obligations |
| 0006 | **Accepted** | Spike B: testcontainer CI default; pyramid + adopter tests (ADR-0014) intact |
| 0007 | **Accepted** (partial: ADR-0017 §2/§3) | Tri-state/onFail/points stand; **vouch coverage replaced** by named obligations |
| 0008 | **Accepted** | Classification/routing/scope + fail-safe amendment; no spike contradiction |
| 0009 | **Accepted** (partial: ADR-0017 §4) | Modes + forge-native challenge gate (D-018); one-shot arming restricted for expiring facts |
| 0010 | **Accepted** (partial: ADR-0017 §2/§5) | Layout stands; envelopes gain `prove`/`require` when schemas land |
| 0011 | **Accepted** (partial: ADR-0017 §1/§7) | Ports draft → Reconcile + serialized schemas are the API (no v1 Go SDK) |
| 0012 | **Accepted** (partial: ADR-0016) | Default layout/lifecycle/redaction stand; **override mechanism superseded** by theming |
| 0013 | **Accepted** | Spike A clears residual CEL risks; D-019 CEL direction + hybrid syntax |
| 0014 | **Accepted** (partial: ADR-0017 §8 / P2-2) | Public test contract stands; `expect.yaml` moves to obligation/exact defaults |
| 0015 | **Accepted** (partial: ADR-0017 §1/§4/§6) | Trust backbone confirmed by dossiers + Spike B SHA-guard + Spike C isolation |
| 0016 | **Accepted** | D-015; Spike A proves one CEL activation for `{{ }}` messages |
| 0017 | **Accepted** | D-016/D-018; spikes + dossiers supply mechanism evidence; north-star timing still PENDING |

No ADR in 0002–0017 is fully **Superseded** — ADR-0017 reshapes clause sets inside Accepted ADRs;
ADR-0016 supersedes only ADR-0012's override mechanism.

---

## Evidence matrix

Columns per REQ-P2-E5-S01-01.

| ADR | Evidence | Superseded clauses | Recommended verdict |
| --- | --- | --- | --- |
| **ADR-0002** | [prior-art](prior-art.md) §2 Kyverno CEL + §1 conftest (lessons 10, 13, 15); D-006; ADR-0013 decision closes OQ-11/12; archetypes use YAML envelope | None (v2 already superseded the two-frontend draft internally) | **Accepted** |
| **ADR-0003** | [prior-art](prior-art.md) §5 Renovate (lesson 8); amendments F12 + A-05; ADR-0017 §5 EntryRef / matcher domains; OQ-15 committed | **Partial — ADR-0017 §5**: `path`-as-glob-and-pointer overload ends; classes declare `entries` → `EntryRef` and split matcher domains (`files`, `values.pointers`, `fileEvents`, `valueChanges`). Core old/new/delete/rename model **stands** | **Accepted** (with §5 note) |
| **ADR-0004** | [Spike C](spikes/spike-c-provider.md) REQ contract + isolation; ADR-0015 §7; ADR-0017 §6 typed/minimized protocol; D-012 (no gRPC in v1); OQ-4 | None fully — Spike C **implements** the HTTP/exec tier of this ADR under 0017 §6 shapes | **Accepted** |
| **ADR-0005** | [forge-dossier-gitlab.md](forge-dossier-gitlab.md) §1–7; [forge-dossier-github.md](forge-dossier-github.md) §1/§3; OQ-7/OQ-18 leading answers; D-012 GitLab-first | **Partial — ADR-0017 §1/§7**: adapters must expose merge-train/queue or refuse deferred auto-merge; forge boundary is `Snapshot → Resolve → Reconcile → PublicationReceipt`. GitLab-first sequencing **stands** | **Accepted** (with §1/§7 note) |
| **ADR-0006** | [Spike B](spikes/spike-b-e2e.md) (OQ-6); ADR-0014 public test contract; prior-art conftest `verify` (lesson 12); D-010 coverage bar | None | **Accepted** |
| **ADR-0007** | Spike A tri-state (`TestTristate`); ADR-0017 §2/§3; D-018 obligations confirmed; archetypes `prove`/`onFailure` DRAFT markers | **Partial — ADR-0017 §2**: replaces Effects row `vouch` + Aggregation step 3 coverage check + amendment 2 `coverage: exclusive` with **named required obligations** (`require:` / `prove:` / `onFailure`). **Partial — ADR-0017 §3**: `challenge` = acknowledgement only; authorization is new `require-review`. **Partial — ADR-0017 §9**: `score` is not an effect (points remain contributions). Tri-state, onFail, per-firing points, block/challenge aggregation **stand** | **Accepted** (vouch/coverage superseded by 0017 §2; challenge authz by §3) |
| **ADR-0008** | Archetypes inventory routing; ADR-0017 §5 matcher domains align with scope; fail-safe amendment P2-10 | Matcher domain split lives in 0017 §5 (see 0003); routing/bindings model **stands** | **Accepted** |
| **ADR-0009** | D-018 forge-native all-threads-resolved trade; dossier C3/C11; ADR-0017 §4 one-shot arming; OQ-14 serve→v1.x; Tide prior-art (lesson 3) | **Partial — ADR-0017 §4**: one-shot may not arm deferred merge on expiring authorization facts; `facts.max_age` is an **arming precondition**. Mode set + challenge amendment **stand** | **Accepted** (with §4 note) |
| **ADR-0010** | Examples `.assent/` layout; ADR-0013 shorthand samples; ADR-0017 §2/§5 resource shapes; OQ-5 local-only v1 (remote packs designed-for) | **Partial — ADR-0017 §2/§5**: rule files gain `prove`/`onFailure`; bindings gain `require:`; entry identity declarations. Directory layout + `config.yaml`/`bindings.yaml` split **stand** | **Accepted** (with prove/require note) |
| **ADR-0011** | Spike B smoke exercises Approve/Merge SHA-guard; Spike C FactQuery shapes; ADR-0017 §1/§6/§7; D-016 no v1 Go SDK | **Partial — ADR-0017 §1**: Decision pins include **target SHA + merge-result digest**, not source alone. **Partial — ADR-0017 §7**: `Publisher` becomes `Reconcile(DesiredReviewState, Preconditions) → PublicationReceipt`; **serialized schemas are the public API** — Go interfaces stay internal. Draft struct names **stand as internal sketches** | **Accepted** (with Reconcile/schema note) |
| **ADR-0012** | ADR-0016 §1–3 + D-015; Spike A message interpolation; redaction amendment | **Partial — ADR-0016 §1–2**: repo-level Go `text/template` overrides **superseded** by tiered knobs/slots/templates + CEL `{{ }}`. Default layout, finding-lifecycle state machine, redaction **stand** (already noted in ADR-0012) | **Accepted** (override → ADR-0016) |
| **ADR-0013** | [Spike A](spikes/spike-a-cel.md) coercion/trace/cost/purity/interpolation; appendix gallery; D-019 CEL confirmed; prior-art Kyverno ValidatingPolicy (lesson 10); OQ-11/12 | ADR-0017 §7 tempers reversibility: **CEL semantics are the commitment**; only the Go CEL implementation is swappable — not a supersession of the hybrid syntax | **Accepted** |
| **ADR-0014** | D-010; prior-art conftest (lesson 12); ADR-0016 amendment (safety vs presentation asserts); ADR-0017 §8 contract fixture + P2-2 exact default | **Partial — ADR-0017 consequences / §8**: `expect.yaml` moves to obligation / predicate / finding-code assertions with **exact** as the safety default; fixture becomes Phase-3 gate. Directory/shorthand runner shape **stands** | **Accepted** (with expect.yaml note) |
| **ADR-0015** | [Spike B](spikes/spike-b-e2e.md) SHA-guard 409; [Spike C](spikes/spike-c-provider.md) §7 isolation; [Spike secure-setup](spikes/spike-secure-setup.md) topology ↔ §4/§5/§8; dossiers C10/C17; prior-art Mergify/Renovate/Tide (lessons 4–7) | **Partial — ADR-0017 §1**: source-only `merge?sha=` insufficient for deferred auto-merge. **Partial — ADR-0017 §4**: `facts.max_age` demoted from advisory to arming precondition. **Partial — ADR-0017 §6**: provider protocol becomes typed/minimized (Spike C). Trust-boundary principles **stand** | **Accepted** (with §1/§4/§6 notes) |
| **ADR-0016** | D-015; Spike A `TestInterpolation` (one activation); ADR-0012 supersession note; roast P1-6 four-record split | None — this ADR **is** the superseding theming decision | **Accepted** |
| **ADR-0017** | D-016/D-018; dossiers §2–5 (merge trains, require-review evidence); Spike C §6; Spike B merge-result primitives; Spike secure-setup topology (ADR-0017 §9); prior-art Bors/queues (lessons 1, 16); named obligations veto closed | N/A (this is the reshaping ADR). **Residual**: §9 north-star &lt;1h **not confirmed** — OQ-24 timed run PENDING (do not treat as proven) | **Accepted** (north-star aspirational until operator timed run) |

---

## Counterpoints defended this round

| Pressure | Source | Acceptance stance |
| --- | --- | --- |
| Fail-closed-only vs integrity escape hatches (Mergify `skip_intermediate_results`, Renovate `ignoreTests`) | [prior-art](prior-art.md) consolidated #6 | **Accept the UX cost**: capability gaps refuse deferred auto-merge; no silent skip of merge-result pinning or approval evidence. Throughput escapes are out of v1 scope (ADR-0017 §1 counterpoint stands). |
| Obligations ceremony vs anonymous vouch | ADR-0017 counterpoints; D-018 | **Accepted**: named `require:` is the lintable safety composition; veto window closed. |
| Merge-result pinning unavailable on Free | dossier C13/C14; Spike secure-setup | **Fail closed**: Free = advisory-only; Premium+ trains required for deferred auto-merge. |
| Dyn CEL `==` silent-false | Spike A residual | Production facts/entry use NativeTypes (or schema validation); documented hazard, not a reopen of ADR-0013. |
| Env scrubber silent-drop of configured TOKEN/SECRET names | Spike C INBOX note | **Freeze preference for Phase 3**: loud refusal at config load (like `trusted-full-content`), not silent drop — residual for host implementation, not ADR reopen. |

---

## Open questions tagged for ADR accept — proposed resolutions

| OQ | Was tagged | Proposed resolution (this round) | Disposition |
| --- | --- | --- | --- |
| **OQ-4** | ADR-0004 accept | Defer gRPC/`go-plugin` to post-v1; HTTP+exec + builtins only (Spike C scope + D-012). | **Resolve** with ADR-0004 Accepted |
| **OQ-5** | ADR-0010 accept | Local `.assent/` only in v1; remote packs remain designed-for seams (D-012). | **Resolve** with ADR-0010 Accepted |
| **OQ-6** | ADR-0006 accept | CI default = GitLab CE **testcontainer**; kind for local/demo ([Spike B](spikes/spike-b-e2e.md)). | **Resolve** (already leading; strike through) |
| **OQ-7** | ADR-0005 accept | GitHub: parity for the **gate** (required-conversation-resolution), not the device; `REQUEST_CHANGES` reserved for block ([forge-dossier-github.md](forge-dossier-github.md) §3). Live E10 checks residual. | **Resolve** for acceptance; residual → Phase 5 / E10 |
| **OQ-11** | ADR-0013 accept | **cel-go** backend ([Spike A](spikes/spike-a-cel.md) + ADR-0013). | **Resolve** |
| **OQ-12** | ADR-0013 accept | **Hybrid** `all`/`any`/`not` + CEL leaves + per-leaf `message` (ADR-0013 + gallery). | **Resolve** |
| **OQ-13** | ADR-0007 accept | Points + per-binding thresholds only in v1; no effect escalation via score. | **Resolve** with ADR-0007 Accepted |
| **OQ-14** | ADR-0009 accept | `serve` in **v1.x / E12** (post-Phase-4); architecture-ready from day 1 (D-012/D-017). | **Resolve** with ADR-0009 Accepted |
| **OQ-15** | ADR-0003 accept | Fold-to-rename **opt-in per class, default raw**; rename never laxer than delete (ADR-0003 amendment). Residual: similarity metric itself → Phase 3. | **Resolve** core; residual → Phase 3 |
| **OQ-16** | (ratify in P2-E5) | Ratify leading corpus: **kafka/org + JulieOps + octoDNS** ([examples/repos/corpus.md](https://github.com/PlatformRelay/assent/blob/main/examples/repos/corpus.md)). | **Resolve** |
| **OQ-17** | (reframed) | Adopt Spike C host defaults: principal/membership **1h**, boolean authz **1h**, registry **24h**, `sensitive: true` **15m**; capped by `facts.max_age` 24h; expiry blocks arming (ADR-0017 §4). Exact schema freeze → Phase 3 contract fixture. | **Partial resolve**; residual defaults → Phase 3 |
| **OQ-18** | ADR-0005 accept | GitHub arm-and-wait parity **on paper** with three deltas (dismissal, auto-merge revoke, merge queue) — [forge-dossier-github.md](forge-dossier-github.md). | **Resolve** for acceptance; implement → E8/E10 |
| **OQ-24** | north-star / ADR-0017 §9 | Topology **Accepted** (Premium + external CI config + …). Timed clean-room run **still PENDING** — do **not** claim &lt;1h confirmed. | Topology resolved; **north-star residual** → Phase 4 / operator task |

OQs already process-resolved outside ADR accept (OQ-1 name, OQ-3 frontends, OQ-19/20/21/23/25) keep their existing resolutions; residuals retagged in `open-questions.md`.

---

## Verify hooks (P2-E5)

- Review doc: `grep -q "ADR-0017" docs/planning/adr-acceptance-review.md && grep -q "Recommended verdict" docs/planning/adr-acceptance-review.md`
- Index clean of Proposed: `! grep -q "Proposed" docs/adr/README.md`
- Decision log: `grep -qi "acceptance round" docs/decisions/decisions.md`
