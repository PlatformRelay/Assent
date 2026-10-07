# P5-DOC2 — docs audit P2: structure and storytelling — spec delta

**Epic ID / REQ prefix:** `DOC2` / `REQ-DOC2-S0n-nn` (audit-remediation change, same shape
as `p5-xrev-final-remediation`: not a meta-plan epic, it executes audit items 9–11).
**Levels:** L0–L3 per ADR-0006.

This is a change delta over published docs surfaces. The authoritative evidence for every
finding is the origin report (`data/assent-docs-audit/report.md`), re-verified at `3503cc0`:
`docs/index.md` is still the 18-line stub, the Usage nav is still Install → CLI reference →
Walkthrough, and the five cli.md essays still sit at `:98–251` (shifted +1 from the audit's
`:97–250` by E10's `-forge` row). Where a `Test:` artifact is shared by several REQs, the
artifact covers all of them (one artifact per REQ, not one REQ per artifact).

S00 is scope found at re-verification, not an audit item; its evidence is a diff review of
this change.

---

## DOC2-S00 — README quick-start de-duplication [autonomous]

**As a** reader of the README **I want** the quick start to say each thing once **so that**
the E10 merge accident stops doubling the install caveat, and PR 190's DOC-13 version pin
can land green.

**Goal:** delete `README.md:106–125` — the repeated go-install caveat, lint/test block,
"No repo of your own yet?" paragraph and second "Developers:" line — so the section reads:
install → caveat → lint/test → sample-repo note → forge selection → GitHub comment-only
note → "Developers: gates live in the Taskfile" → task-check block. No surviving sentence
is edited.

**Operator input:** none.

**Dependencies:** none.

**Definition of done:** each quick-start block appears exactly once; `readme_smoke_test.sh`
green (it executes the section's bash blocks — now once each, not twice); no other README
byte changes.

Requirements:

- **REQ-DOC2-S00-01** — Given the README's `## Quick start` section, when it is read, then
  the go-install caveat paragraph, the lint/test block, the sample-repo paragraph, the
  forge-selection block, the GitHub comment-only note and the task-check block each appear
  exactly once, in that order.
  - Test: the story's diff review + `hack/docs/readme_smoke_test.sh` as a regression
    check only — the smoke gate executes the section's bash blocks and cannot itself
    redden a prose duplication, which is why this REQ is L0 and closes on the diff review
  - Verify: `task docs-gates`
  - Level: L0

---

## DOC2-S01 — the site home carries the README's story (audit item 9) [autonomous]

**As a** newcomer landing on the published site home **I want** it to answer why, how and
how-fast **so that** the site home is not 18 lines of bare links while
the README carries the whole narrative (GAP-2).

**Goal:** rebuild `docs/index.md` around the README's Why + How-it-works + Quick start +
status banner, links adjusted to docs-relative form, keeping the existing hero block and
H1. **The copy source is this branch's README with PR 190's three P0 hunks applied** —
`:22` `E2–E8`→`E1–E9`, `:45–46` the `.tf` opaque clause, `:76` `@v0.1.0`→`@v0.4.0` —
because both sources alone are wrong: the branch's README still carries the pre-P0
wording, and PR 190's README is based on the pre-E10 tree, so it lacks the forge-selection
block and the GitHub comment-only note that S01-01 mandates (its GitHub-adapter row even
says **Planned**). The branch's E10-era quick-start content is current truth and stays.
"Start here" keeps Vision, walkthrough, ADRs, architecture and the decision log; the
planning links (meta-plan, open questions) move into a "Contributing" tail; index.md's
existing two-sentence intro is replaced by the README's intro paragraph.

**Operator input:** none (audit-recommended structure).

**Dependencies:** S00 (the quick start it copies must not carry the duplication).

**Definition of done:** `docs/index.md` renders under `mkdocs build --strict` with no
unrecognized link; every link resolves; the copied text matches PR 190's README wording
modulo link targets; the REQ-DOCSNAV-S01-04 sanitization read is recorded for the copied
text's new home.

Requirements:

- **REQ-DOC2-S01-01** — Given `docs/index.md`, when it is read, then it carries (top to
  bottom): the status banner (alpha; the GitLab CI path is Core with the E1–E9 range;
  pre-1.0 schema/CLI warning; an API-stability link), the Why section with its three
  bullets, the How-it-works section with the mermaid flowchart and the stateless-per-
  invocation paragraph, and the Quick start with install, caveat, lint/test, sample-repo,
  forge-selection and GitHub comment-only-note content.
  - Test: `mkdocs.yml` build + the story's diff review
  - Verify: `task docs-build`
  - Level: L1
- **REQ-DOC2-S01-02** — Given every link in the new `docs/index.md`, when
  `mkdocs build --strict` runs, then no unrecognized-link or omitted-file warning fires;
  links into `planning/**` resolve (those pages build via `not_in_nav`) and links into
  `adr/**` and `architecture/**` resolve (those pages are navigated).
  - Test: `docs/index.md`
  - Verify: `task docs-build`
  - Level: L1
- **REQ-DOC2-S01-03** — Given the site home's "Start here" list and its "Contributing"
  tail, when they are read, then Vision, the adoption walkthrough, the ADR index, the C4
  pages and the decision log sit in Start here, and the meta-plan and open-questions links
  sit in Contributing — the reader's first click is a product page, not a working document.
  - Test: `docs/index.md`
  - Verify: the story's diff review
  - Level: L0
- **REQ-DOC2-S01-04** — Given the copied banner/quick-start wording, when it is read, then
  it equals the branch's README text with PR 190's three P0 hunks applied — E1–E9 (not
  E2–E8), the `.tf` opaque clause, and `@v0.4.0` in the go-install caveat — and the
  branch's E10-era quick-start content (forge selection, GitHub comment-only note) is
  carried as-is; the P0 corrections are settled text even though PR 190 is still open.
  - Test: `docs/index.md` diffed against the branch README with those three hunks applied
    by hand (they are the only wording differences the copy introduces)
  - Verify: the story's diff review; after both PRs land, DOC-13's scan of
    `docs/index.md` must be green
  - Level: L1

---

## DOC2-S02 — Usage nav order (audit item 10) [autonomous]

**As a** newcomer choosing what to read after installing **I want** the walkthrough before
the 395-line CLI lookup table **so that** the teaching page precedes the reference (GAP-3's
nav-order half).

**Goal:** in `mkdocs.yml`, reorder the Usage section to Install → Walkthrough → CLI
reference. Nothing else in the nav moves.

**Operator input:** none.

**Dependencies:** none (independent of S01 and S03).

**Definition of done:** nav order as above; `task docs-build` green.

Requirements:

- **REQ-DOC2-S02-01** — Given `mkdocs.yml`'s `nav:`, when its Usage section is read, then
  it lists Install, Walkthrough, CLI reference in that order.
  - Test: `mkdocs.yml`
  - Verify: `task docs-build` (nav validity) + the story's diff review
  - Level: L0

---

## DOC2-S03 — extract the trust-model essays into operating-safely (audit item 11) [autonomous · security-relevant]

**As a** reader looking up a `run` flag **I want** the flag table without two pages of
trust boundary interleaved **so that** the reference stays a reference — and **as a**
operator deciding how to run assent **I want** the trust-model material one click away and
unchanged **so that** no caveat is lost in the move (STYLE-1).

**Goal:** move `cli.md`'s five sections — *What gates approve and merge*, *How to keep
assent advisory*, *Symlinks in the checkout tree*, *Known limitation: the checkout is not
bound to the evaluated commit*, *Checkout-less runs and enumeration completeness*
(`:98–251`) — verbatim into a new `docs/usage/operating-safely.md` (as `##` sections under
a short intro linking back to the CLI reference), add the page to the Usage nav
(REQ-DOCSNAV-S01-01) and to the exitgate retired-phrase corpus (see REQ-DOC2-S03-04), and
in `cli.md` leave one-line caveats + links per safety-relevant flag plus a pointer
paragraph where the essays were. Retarget the references that pointed into the essays.
The inventory at `3503cc0` is **five**, not two: (a) `cli.md`'s doctor section ("See
*What gates approve and merge* above"), (b) `walkthrough.md:212`'s
`[How to keep assent advisory](cli.md#how-to-keep-assent-advisory)`, (c) a prose pointer
inside accepted ADR-0009 (`docs/adr/0009-execution-modes.md:55`: "`docs/usage/cli.md`
§*How to keep assent advisory* states this"), (d) a prose pointer inside a frozen
decision-evidence record (`docs/decisions/evidence/p4-e1-s11-adoption/README.md:27`:
"and the CLI reference's *What gates approve and merge*"), and (e) a prose pointer inside
the decision log's D-134 row (`docs/decisions/decisions.md:141`: "The decision matrix
above is now published in `cli.md` rather than summarised"), which this move makes stale.
(c) is dispositioned, not edited: ADRs are immutable once accepted, the pointer names a
section that still exists — on the new page — and it is recorded as a known limitation in
this change's hand-off. (d) is dispositioned like ADR-0009: the record is frozen and left
untouched, the pointer names the section by title, and it is recorded as a hand-off
residual. (e) is dispositioned like the others: the decision-log row is left untouched
(rows are reconciled by their own owner, not as a side effect of a docs move), the
pointer names the matrix that still exists — on the new page — and it is recorded as a
hand-off residual.

Two navigation-only transformations are allowed inside the moved text (listed here so the
verbatim fence stays honest): the advisory essay's deixis "reruns **the CI snippet above**
— which passes no `--pack`" loses its antecedent when the essay leaves `cli.md`, so it is
retargeted to name and link the `assent run` invocation in the CLI reference; and the
doctor section's "above" (whose target moved to another page) is dropped when the link is
retargeted. No other word of the moved sentences changes.

**Operator input:** none.

**Dependencies:** none hard, but lands after S02 so the nav row slots into the final order.

**Definition of done:** the five essays read unchanged in their new home modulo the
navigation-only transformations the Goal names; `cli.md` keeps every flag row with the
XREV-S03-06-pinned `--config` wording intact; every link into the moved sections resolves;
the new page is in the nav and in the retired-phrase corpus; all gates below green.

Requirements:

- **REQ-DOC2-S03-01** — Given `docs/usage/operating-safely.md`, when its five trust/safety
  sections are diffed against their pre-move `cli.md` text, then each sentence, table and
  code block is unchanged, except for the two navigation-only transformations the Goal
  names (heading level `###`→`##`, `#assent-doctor` anchors retargeted to
  `cli.md#assent-doctor`, the advisory essay's "the CI snippet above" deixis, italics
  "see *X* below" cross-references kept verbatim — same page, same order).
  - Test: the story's diff review + `hack/check-sanitization.sh`
  - Verify: `bash hack/check-sanitization.sh && task docs-build`
  - Level: L1
- **REQ-DOC2-S03-02** — Given `cli.md`'s run flag table, when it is read, then the `-arm`,
  `-checkout` and `-pack` rows each carry their one-line safety caveat with a link to the
  moved section in `operating-safely.md` — the rows' existing italic "see *X* below"
  tails are replaced by those links, not left behind — with the `-pack` link targeting
  `operating-safely.md#how-to-keep-assent-advisory` (the anchor leaves `cli.md` in this
  same change); the `--config` row's pinned wording (XREV-S03-06) is untouched; a pointer
  paragraph where the essays were links to the new page.
  - Test: `hack/docs/truthlag_pins_test.sh` (XREV-S03-06) + the story's diff review
  - Verify: `bash hack/docs/truthlag_pins_test.sh && task docs-build`
  - Level: L1
- **REQ-DOC2-S03-03** — Given the references that pointed into the essays, when they are
  read after the change, then `cli.md`'s doctor section targets
  `operating-safely.md#what-gates-approve-and-merge` without the now-false "above", and
  `walkthrough.md:212` targets `operating-safely.md#how-to-keep-assent-advisory`; both
  anchors exist in the built site. ADR-0009's prose pointer is left untouched (immutability)
  and recorded in the hand-off.
  - Test: the built `site/usage/operating-safely/index.html` (anchor presence)
  - Verify: `task docs-build` + grep the built anchors
  - Level: L1
- **REQ-DOC2-S03-04** — Given every existing gate that reads the moved or linked content,
  when it runs after the extraction, then it is green, and the new page does not leave the
  front-of-house sensor set: `docs/usage/operating-safely.md` is added to
  `hack/audit/exitgate_test.sh`'s PHRASE_CORPUS in the same commit as the page (the
  shrunk-corpus mutant removes four corpus members and must still redden: the corpus then
  lists 11 files of which 10 are present — `docs/usage/quickstart.md` is a pre-existing
  dead entry — so removing four leaves 6 present < MIN 8. No grep at `3503cc0` pins a
  sentence inside the five essays (measured: the only `cli.md` pin is XREV-S03-06, which
  targets the flag-table row that stays); the pin co-update duty the origin report
  attaches to item 11 therefore reduces to keeping these gates green. PR 190's DOC-13
  initial-chapter list cannot be co-edited here (open PR, not in this base) — the new
  page's absence from that list is a recorded residual, not a silent hole: the page
  carries no `@vX.Y.Z`/`VERSION=` pin by construction.
  - Test: `task docs-gates`; `go test ./cmd/assent -run TestNoStaleProductClaims`;
    `bash hack/audit/exitgate_test.sh` (branch tip)
  - Verify: the three commands above, at branch tip
  - Level: L1
