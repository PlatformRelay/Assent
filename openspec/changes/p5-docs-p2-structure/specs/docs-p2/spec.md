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
  - Test: `hack/docs/readme_smoke_test.sh` (executes the section) + the story's diff review
  - Verify: `bash hack/docs/readme_smoke_test.sh` and `task docs-gates`
  - Level: L0

---

## DOC2-S01 — the site home carries the README's story (audit item 9) [autonomous]

**As a** newcomer landing on `platformrelay.github.io/Assent` **I want** the front door to
answer why, how and how-fast **so that** the site home is not 18 lines of bare links while
the README carries the whole narrative (GAP-2).

**Goal:** rebuild `docs/index.md` around the README's Why + How-it-works + Quick start +
status banner, links adjusted to docs-relative form, keeping the existing hero block and
H1. The banner and quick-start wording are sourced from PR 190's corrected README (E1–E9;
the `.tf` opaque clause; `@v0.4.0`) so the two surfaces agree once both PRs land. "Start
here" keeps Vision, walkthrough, ADRs, architecture and the decision log; the planning
links (meta-plan, open questions) move into a "Contributing" tail.

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
  invocation paragraph, and the Quick start with install, caveat, lint/test, sample-repo
  and forge-selection content.
  - Test: `mkdocs.yml` build + the story's diff review
  - Verify: `task docs-build`
  - Level: L1
- **REQ-DOC2-S01-02** — Given every link in the new `docs/index.md`, when
  `mkdocs build --strict` runs, then no unrecognized-link or omitted-file warning fires;
  links into `planning/**` and `adr/**` resolve (those pages build via `not_in_nav`).
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
  it carries PR 190's corrected facts (E1–E9, not E2–E8; the `.tf` opaque clause;
  `@v0.4.0`) — the P0 corrections are settled text even though PR 190 is still open.
  - Test: `docs/index.md` vs PR 190's `README.md` hunks
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
(REQ-DOCSNAV-S01-01), and in `cli.md` leave one-line caveats + links per safety-relevant
flag plus a pointer paragraph where the essays were. Retarget the two references that
pointed into the essays: `cli.md`'s doctor section ("See *What gates approve and merge*
above") and `walkthrough.md:212`'s `[How to keep assent advisory](cli.md#...)` link.

**Operator input:** none.

**Dependencies:** none hard, but lands after S02 so the nav row slots into the final order.

**Definition of done:** the five essays read byte-identical in their new home modulo
heading level and retargeted in-page anchors; `cli.md` keeps every flag row with the
XREV-S03-06-pinned `--config` wording intact; every link into the moved sections resolves;
all gates below green.

Requirements:

- **REQ-DOC2-S03-01** — Given `docs/usage/operating-safely.md`, when its five trust/safety
  sections are diffed against their pre-move `cli.md` text, then each sentence, table and
  code block is unchanged (heading level `###`→`##`, internal links to `#assent-doctor`
  retargeted to `cli.md#assent-doctor`, italics "see *X* below" cross-references kept
  verbatim — same page, same order).
  - Test: the story's diff review + `hack/check-sanitization.sh`
  - Verify: `bash hack/check-sanitization.sh && task docs-build`
  - Level: L1
- **REQ-DOC2-S03-02** — Given `cli.md`'s run flag table, when it is read, then the `-arm`,
  `-checkout` and `-pack` rows each carry their one-line safety caveat with a link to the
  moved section in `operating-safely.md`; the `--config` row's pinned wording
  (XREV-S03-06) is untouched; a pointer paragraph where the essays were links to the new
  page.
  - Test: `hack/docs/truthlag_pins_test.sh` (XREV-S03-06) + the story's diff review
  - Verify: `bash hack/docs/truthlag_pins_test.sh && task docs-build`
  - Level: L1
- **REQ-DOC2-S03-03** — Given the two references that pointed into the essays —
  `cli.md`'s `assent doctor` section and `walkthrough.md:212` — when they are read, then
  they target `operating-safely.md`'s anchors, which exist in the built site.
  - Test: the built `site/usage/operating-safely/index.html` (anchor presence)
  - Verify: `task docs-build` + grep the built anchors
  - Level: L1
- **REQ-DOC2-S03-04** — Given every existing gate that reads the moved or linked content
  (`truthlag_pins_test.sh`, `readme_smoke_test.sh`, the exitgate phrase corpus over
  `docs/usage/cli.md` and `docs/index.md`, `TestNoStaleProductClaims`, `check-sanitization.sh`,
  `task docs-build`), when it runs after the extraction, then it is green — the pin
  co-update duty the origin report attaches to item 11 reduces to keeping these green,
  because no grep at `3503cc0` pins a sentence inside the five essays (measured: the only
  `cli.md` pin is XREV-S03-06, which targets the flag-table row that stays).
  - Test: `task docs-gates`; `go test ./cmd/assent -run TestNoStaleProductClaims`;
    `bash hack/audit/exitgate_test.sh` (branch tip)
  - Verify: the three commands above, at branch tip
  - Level: L1
