# P5-E9 — Distribution & release (oss-playbook)

**Epic ID / REQ prefix:** `E9` / `REQ-E9-S0n-nn`.

**Problem**: E2–E8 closed the engine, forge, provider, adopter harness, conformance infra, and
renderer — but **adopters still cannot install a stamped binary, verify its signatures, or read a
product-focused docs site**. Release tooling is pre-alpha: `cmd/assent/main.go` exposes
`version = "0.0.0-dev"` with no link-time injection; there is **no** `.goreleaser.yaml`, **no**
tag-triggered release workflow, **no** cosign/SLSA/SBOM attestation on artifacts, and **no**
install script. Partial oss-playbook coverage already landed (D-044/D-045): MkDocs Material +
`docs.yaml` GH Pages deploy, CodeQL, OpenSSF Scorecard, govulncheck (push + weekly), gitleaks,
SHA-pinned Actions, Dependabot, `SECURITY.md`, `CODEOWNERS`, `cliff.toml` (minimal), and
`API_STABILITY.md`. E9 **extends what exists** — it does not greenfield duplicate security jobs.

**Key ground truth (de-risks the epic):**
- **Security CI mostly done:** `.github/workflows/{verify,codeql,scorecard,vulncheck}.yaml` are
  pinned and green (D-045); S04 is an **audit + residual gap** lane, not a second CodeQL install.
- **Docs pipeline live:** `mkdocs.yml` + `docs/requirements-docs.txt` + `task docs-build` +
  `docs.yaml` deploy to `https://platformrelay.github.io/assent/` (D-044) — but nav still exposes
  `docs/planning/*` (oss-playbook anti-pattern #3); S08 fences product-only pages.
- **Changelog seed exists:** root `cliff.toml` with gitmoji parsers; no `CHANGELOG.md` sync CI yet.
- **Sibling patterns:** `kollect-render/.goreleaser.yaml` (CLI ldflags + cross-compile) and
  `mkurator/.github/workflows/release.yaml` (cosign + git-cliff + attestations) are the templates.
- **Version surface:** single `main.version` var in `cmd/assent/main.go`; goreleaser ldflags target
  `-X main.version={{.Version}}` (D-099).
- **Autonomous close path:** S01–S04 + **S07a** (install script + `install.md`) + S08 → S09 +
  S10 (tape sources) + S11 (OQ-2 decision) + S12 (verify harness on S02 `--snapshot` checksums;
  cosign branch **skip-when-no-signature**) close without publish credentials. **Infra-gated:**
  S05–S06 (publish + signing), **S07b** (Homebrew tap push), S10 GIF optional, S13 tagged release
  proof.

**Scope**: (S01) semver ldflags + `assent version`; (S02) goreleaser config + local snapshot;
(S03) git-cliff changelog without SHAs + `CHANGELOG.md` sync; (S04) CI hardening audit + residual
gaps (actionlint, README badges, oss-playbook checklist — **no duplicate** CodeQL/Scorecard);
(S05) tag-triggered release workflow + goreleaser publish; (S06) cosign keyless + SLSA + SBOM on
release artifacts; (**S07a**) curl+checksum install script + `docs/usage/install.md`; (**S07b**)
Homebrew tap wiring + goreleaser `brews` publish; (S08) MkDocs product-only nav + install page;
(S09) README maturity formula + honest post-E8 status; (S10) VHS demo tape sources; (S11) OQ-2
GitLab dogfood mirror decide-and-log; (S12) release artifact verify harness (S02 snapshot);
(S13) exit gate. **Infra-gated:** S05, S06, S07b, S10 GIF optional, S13 publish half.

**Non-goals** (fenced): **GitHub Actions adapter** (E10, D-012 locked); **Rego backend** (E11);
**PolicyComparisonSuite full runner** (D-057 — separate epic after E9 or parallel if file-disjoint;
E6-S09 seed stays untouched); **`serve` / E12**; **remote packs** (E13); **domain/apiVersion
rename** (OQ-1 / D-028 — mechanical lane, not release); **API_STABILITY.md rewrite** (exists —
link only); **branch protection** (operator residual from D-045); **live demo repo** with real
auto-merged MRs (oss-playbook #10 second half — post-E9 optional); **container/OCI image** (CLI
only in v1).

**ADRs / decisions**: D-044 (MkDocs), D-045 (security batch), oss-playbook #3–#5, #10.
**Reuse**: existing workflows, `cliff.toml`, `Taskfile.yml`, `SECURITY.md`, `API_STABILITY.md`.
**New**: `.goreleaser.yaml`, `.github/workflows/release.yaml`, `hack/release/**`, install script,
Homebrew formula template, VHS tapes, product docs nav trim.

**Executability**: S01–S04, S07a, S08, S09, S10 (tapes), S11, S12 **`[autonomous]`** — snapshot
artifacts, checksum verify (cosign optional/skip-when-absent), docs/nav/README, decision log.
**Infra-gated:** S05, S06, S07b (publish credentials), S10 (GIF render optional), S13 (tagged
release + live install proof). **Engine-grade / supply-chain review:** S06 (signing/SBOM/SLSA),
S12 (verify harness), S13 (exit gate).

**Judgment calls (decide-and-log / operator):**
(a) **DECIDED — link-time version via `-X main.version={{.Version}}`.** Dev builds keep
`0.0.0-dev`; goreleaser/release inject semver from tag (`v0.1.0` → `0.1.0`). Logged **D-099**.
(b) **DECIDED — goreleaser v2 cross-compile:** `linux/darwin/windows` × `amd64/arm64`, skip
`windows/arm64`; `CGO_ENABLED=0`, `-trimpath`, `-s -w` ldflags. Logged **D-100**.
(c) **DECIDED — git-cliff changelog omits commit SHAs** (oss-playbook #4); body is categorized
subject lines only; `CHANGELOG.md` synced on main via CI or task. Logged **D-101**.
(d) **DECIDED — CI hardening S04 is audit-first:** extend gaps only (actionlint workflow, Scorecard
README badge, oss-playbook status doc); **do not** add second CodeQL/Scorecard/govulncheck jobs.
Logged **D-102**.
(e) **DECIDED — MkDocs nav = product pages only:** Home, Vision, ADRs, Architecture, Usage,
API stability, Decisions; **exclude** `docs/planning/**` and `openspec/**` from nav (files may
remain in-repo for contributors). Logged **D-103**.
(f) **DECIDED — README maturity table** (oss-playbook #3): honest Core/Beta/Planned rows post-E8
(GitLab forge Core, assert/CEL Core, renderer Core, GitHub Planned, Rego Locked, serve Designed).
Status callout moves from "design phase" to "alpha — GitLab CI path". Logged **D-104**.
(g) **DECIDED — OQ-2 GitLab mirror DEFERRED:** GitHub (`PlatformRelay/assent`) remains canonical
origin; a GitLab read-only mirror is an optional operator infra task, not an E9 merge blocker.
Logged **D-105**.
(h) **DECIDED — VHS: commit `.tape` sources under `docs/assets/demos/`; GIF/WebM publish optional
infra-gated; README promises demos only after committed GIF assets exist (oss-playbook #1).
Logged **D-106**.
(i) **DECIDED — Homebrew via goreleaser `brews`** targeting `PlatformRelay/homebrew-tap` (infra-
gated push); in-repo formula template for review before first tap commit. Logged **D-107**.
(j) **DECIDED — first public tag `v0.1.0`** (first installable alpha reflecting E2–E8 product).
Logged **D-108**.
(k) **DECIDED — supply-chain stack on release artifacts:** goreleaser cosign + SBOM where
supported; supplement with `actions/attest` SLSA provenance on checksums/archives (mkurator
pattern). Logged **D-109**.
(l) **DECIDED — curl install script verifies SHA256 checksums before extract (always fail-closed
on mismatch); cosign verify runs when signature bundles are present, skipped when absent
(snapshot/autonomous path). `--require-signature` flag fails closed if bundles missing (post-S06
releases). Logged **D-110**.

**Dependency order**: **S01 → {S02, S04 ∥}**; **S02 → S03**; **S02 → S07a → S08 → S09**;
**S02 → S12** (checksum verify on snapshot; cosign branch optional); **S02 → S05 → S06 → S07b**;
**S10 ∥**, **S11 ∥** (early); **S13** last. **Closes oss-playbook #4–#5, #7 nav fence, #10 tapes:
S02–S07a/b, S08, S10. Next epic after E9: PolicyComparisonSuite runner (D-057) or E11/E12 per
operator priority.**

**Coordination note:** S05/S06 touch `.github/workflows/release.yaml` only — do not modify
`verify.yaml` security steps (D-086). S08 edits `mkdocs.yml` nav only; `docs/planning/**` stays
in-tree for contributors.

---

## E9-S01 — Semver ldflags + `assent version` contract [autonomous]

**As a** release engineer **I want** link-time version injection **so that** shipped binaries report
the tag they were built from.

**Goal**: Add `-ldflags "-X main.version=…"` to `Taskfile.yml` `build` (default `0.0.0-dev`) and
document the contract in `cmd/assent/main.go`. Extend `assent version` to print semver only (no
git dirty suffix in v1 — tag is source of truth). Test: build with injected version, assert stdout.

**Dependencies**: none.

**Definition of done**: `task build` still works; injected version test passes; no schema changes.

Requirements:
- **REQ-E9-S01-01** — `go build -ldflags "-X main.version=1.2.3-test"` yields `assent version` →
  `assent 1.2.3-test`. Test: `cmd/assent/version_test.go`; Verify:
  `go test ./cmd/assent/... -run TestVersionLdflags`; Level: L0
- **REQ-E9-S01-02** — default dev build prints `assent 0.0.0-dev`. Test: same; Verify:
  `go test ./cmd/assent/... -run TestVersionDevDefault`; Level: L0
- **REQ-E9-S01-03** — `Taskfile.yml` `build` passes `-X main.version=${ASSENT_VERSION:-0.0.0-dev}`.
  Test: `Taskfile.yml`; Verify: `grep -q 'main.version' Taskfile.yml`; Level: doc

---

## E9-S02 — Goreleaser config + local snapshot dry-run [autonomous]

**As a** maintainer **I want** goreleaser config and snapshot builds **so that** release artifacts
are reproducible locally without publish credentials.

**Goal**: Add `.goreleaser.yaml` (v2) for `./cmd/assent`: cross-platform binaries (D-100),
`archives` with LICENSE+README, `checksum` SHA256 manifest, `snapshot` naming. Add
`task release-snapshot` → `goreleaser release --snapshot --clean --skip=publish`. Output under
`dist/` (gitignored). No GitHub publish in this slice.

**Dependencies**: E9-S01.

**Definition of done**: snapshot produces `dist/**` with per-OS archives + checksums file; CI-free.

Requirements:
- **REQ-E9-S02-01** — `goreleaser release --snapshot --clean --skip=publish` exits 0 and emits
  ≥1 archive + checksums for `assent`. Test: `hack/release/snapshot_test.sh`; Verify:
  `task release-snapshot && test -f dist/checksums.txt`; Level: L1
- **REQ-E9-S02-02** — built binary reports semver from snapshot version (not `0.0.0-dev`). Test:
  same script; Verify: `dist/**/assent version | grep -qv 0.0.0-dev`; Level: L1
- **REQ-E9-S02-03** — `.goreleaser.yaml` documents `project_name: assent` and ldflags
  `-X main.version={{.Version}}`. Test: `.goreleaser.yaml`; Verify:
  `grep -q 'main.version={{.Version}}' .goreleaser.yaml`; Level: doc

---

## E9-S03 — git-cliff changelog without SHAs + CHANGELOG.md sync [autonomous]

**As a** adopter **I want** human-readable release notes **so that** I know what changed without
stale commit SHAs (oss-playbook #4).

**Goal**: Extend root `cliff.toml` — ensure body template emits **no commit SHAs** (subject lines
only, grouped per existing gitmoji parsers). Add `task changelog` (preview) + `task changelog-write`
(update `CHANGELOG.md`). Add `hack/release/verify-changelog.sh` or CI step ensuring
`CHANGELOG.md` is fresh vs `cliff.toml` output (mkurator `changelog-sync` pattern, lightweight).
Seed `CHANGELOG.md` with Unreleased section.

**Dependencies**: E9-S02 (tag pattern aligned).

**Definition of done**: `CHANGELOG.md` exists; cliff output contains no `[a-f0-9]{7,}` SHA tokens.

Requirements:
- **REQ-E9-S03-01** — `git-cliff --unreleased` output contains no full/commit-short SHA patterns.
  Test: `hack/release/changelog_test.sh`; Verify:
  `! task changelog | grep -qE '[0-9a-f]{7,40}'`; Level: L0
- **REQ-E9-S03-02** — `CHANGELOG.md` parses and includes Unreleased + at least one historical stub.
  Test: same; Verify: `test -f CHANGELOG.md && grep -q Unreleased CHANGELOG.md`; Level: doc
- **REQ-E9-S03-03** — release workflow will consume git-cliff output (document hook point in
  `hack/release/README.md`). Test: `hack/release/README.md`; Verify:
  `grep -q git-cliff hack/release/README.md`; Level: doc

---

## E9-S04 — CI hardening audit + residual gaps (extend, don't duplicate) [autonomous]

**As a** security reviewer **I want** an honest inventory of CI gates **so that** E9 closes only
real gaps without duplicating D-045 jobs.

**Goal**: Add `docs/planning/ci-hardening-status.md` (or section in `hack/release/README.md`)
listing each oss-playbook #5 item with **Exists / Gap / Owner**: CodeQL ✅, Scorecard ✅,
govulncheck ✅ (verify + weekly), gitleaks ✅, SHA-pinned actions ✅, Dependabot ✅. Close
residual gaps only: (1) `actionlint` workflow on `.github/workflows/**`; (2) OpenSSF Scorecard
badge on README (if score publishes); (3) optional `ossf/scorecard-action` pin bump if drift.
**Explicit non-actions:** no second CodeQL matrix, no duplicate govulncheck on push.

**Dependencies**: E9-S01 ∥ (parallel after S01).

**Definition of done**: audit doc committed; actionlint workflow green; README badge only if
workflow exists.

Requirements:
- **REQ-E9-S04-01** — audit doc marks CodeQL, Scorecard, govulncheck, gitleaks as **shipped**
  with workflow paths. Test: `docs/planning/ci-hardening-status.md`; Verify:
  `grep -q codeql.yaml docs/planning/ci-hardening-status.md`; Level: doc
- **REQ-E9-S04-02** — `.github/workflows/actionlint.yaml` runs actionlint on workflow changes.
  Test: workflow file; Verify: `actionlint .github/workflows/*.yaml`; Level: L1
- **REQ-E9-S04-03** — exactly one CodeQL workflow file exists (D-045 shipped; E9 must not add a
  second). Test: `hack/release/ci_audit_test.sh`; Verify:
  `test "$(grep -l 'name: CodeQL' .github/workflows/*.yaml | wc -l)" -eq 1`; Level: L0

---

## E9-S05 — Tag-triggered release workflow + goreleaser publish [infra-gated]

**As a** operator **I want** pushing `v*.*.*` tags to publish GitHub Release assets **so that**
adopters download stamped binaries.

**Goal**: Add `.github/workflows/release.yaml` on `push.tags: v*.*.*` + `workflow_dispatch` (retag
test). Job: checkout (fetch-depth 0), setup-go, goreleaser-action (pinned), publish to GitHub
Releases with `contents: write`. Wire git-cliff release notes (S03). **Autonomous half:** workflow
syntax + dry-run job on PR that runs `goreleaser release --snapshot --skip=publish`.

**Dependencies**: E9-S02, E9-S03.

**Definition of done**: workflow_dispatch on a test tag publishes assets; PR snapshot job green.

Requirements:
- **REQ-E9-S05-01** — tag push `v0.0.0-test` (or workflow_dispatch) runs goreleaser publish with
  archives attached to GitHub Release. Test: manual/CI; Verify:
  `gh release view v0.0.0-test --json assets`; Level: infra
- **REQ-E9-S05-02** — release workflow uses SHA-pinned actions only (matches verify.yaml pins).
  Test: `.github/workflows/release.yaml`; Verify:
  `grep -q 'actions/checkout@' .github/workflows/release.yaml`; Level: doc
- **REQ-E9-S05-03** — PR CI runs goreleaser snapshot without publish credentials. Test:
  workflow or `hack/release/snapshot_test.sh`; Verify: `task release-snapshot`; Level: L1

---

## E9-S06 — cosign keyless + SLSA provenance + SBOM [infra-gated · engine-grade]

**As a** adopter **I want** signed release artifacts with attestations **so that** I can verify
supply-chain integrity before installing.

**Goal**: Extend release workflow (S05): `id-token: write`, `attestations: write`;
`sigstore/cosign-installer`; goreleaser `signs` + `sboms` (syft/spdx) on archives/checksums;
`actions/attest` SLSA provenance on release artifacts (mkurator pattern). Update `SECURITY.md`
release verification section with cosign verify commands. **Autonomous half:** document verify
commands + test against snapshot sigs if goreleaser snapshot signing enabled locally.

**Dependencies**: E9-S05.

**Definition of done**: published release has `.sigstore.json` bundles + SBOM; SECURITY.md documents
verification.

Requirements:
- **REQ-E9-S06-01** — release artifacts include cosign signature bundles verifiable with
  `cosign verify-blob` or `cosign verify`. Test: manual post-release; Verify:
  `cosign verify-blob --certificate-oidc-issuer https://token.actions.githubusercontent.com …`;
  Level: infra
- **REQ-E9-S06-02** — SBOM SPDX JSON attached per archive (goreleaser sbom or syft step). Test:
  release assets; Verify: `gh release download … -p '*.spdx.json' | head -1`; Level: infra
- **REQ-E9-S06-03** — `SECURITY.md` lists enforced release gates (cosign, SBOM, SLSA) with copy-
  paste verify commands. Test: `SECURITY.md`; Verify:
  `grep -q cosign SECURITY.md`; Level: doc

---

## E9-S07a — curl+checksum install script + install docs [autonomous]

**As a** adopter **I want** a checksum-verified install path **so that** I can install from local
snapshot or release artifacts without trusting transport alone.

**Goal**: (1) **`hack/install.sh`** — given `--archive` + `--checksums` (S02 snapshot or release),
verify SHA256 (D-110, always fail-closed on mismatch), install to `/usr/local/bin` or
`~/.local/bin`. Cosign verify: **skip when no `.sigstore.json` bundles** (snapshot/autonomous);
verify when present; **`--require-signature`** fails closed if bundles absent (for post-S06
releases). (2) **`docs/usage/install.md`** documents `go install`, curl script usage (local
`dist/` + GitHub release URL pattern), and Homebrew as "coming soon" until S07b. Script tested
against S02 snapshot — **no S06 dependency**.

**Dependencies**: E9-S02.

**Definition of done**: install.sh passes against snapshot; install.md committed; cosign skip path
documented.

Requirements:
- **REQ-E9-S07a-01** — install.sh verifies SHA256 before extract; rejects mismatch exit 1. Test:
  `hack/release/install_test.sh`; Verify: `bash hack/release/install_test.sh`; Level: L1
- **REQ-E9-S07a-02** — without signature bundles, install.sh skips cosign and succeeds on valid
  checksum. Test: same; Verify: `bash hack/release/install_test.sh snapshot-no-sig`; Level: L0
- **REQ-E9-S07a-03** — `docs/usage/install.md` documents go install + curl script. Test: doc;
  Verify: `grep -q 'go install' docs/usage/install.md`; Level: doc

---

## E9-S07b — Homebrew tap wiring + goreleaser brews publish [infra-gated]

**As a** macOS adopter **I want** `brew install assent` **so that** I can consume assent via
Homebrew after the tap lands.

**Goal**: goreleaser `brews` section (D-107) targeting `PlatformRelay/homebrew-tap`; in-repo
formula template for review; tap push infra-gated on tagged release (S05/S06). README/Homebrew
claim only after tap commit lands.

**Dependencies**: E9-S05, E9-S06.

**Definition of done**: formula template in-repo; tap receives commit on first tagged release.

Requirements:
- **REQ-E9-S07b-01** — `.goreleaser.yaml` includes `brews` with tap owner/repo (D-107). Test:
  config; Verify: `grep -q brews .goreleaser.yaml`; Level: doc
- **REQ-E9-S07b-02** — tagged release publishes formula to homebrew-tap (manual/CI verify). Test:
  infra; Verify: `brew tap PlatformRelay/tap && brew install assent`; Level: infra
- **REQ-E9-S07b-03** — install.md Homebrew section upgraded from "coming soon" after tap lands.
  Test: doc; Verify: `grep -q Homebrew docs/usage/install.md`; Level: doc

---

## E9-S08 — MkDocs product-only nav + install page [autonomous]

**As a** visitor **I want** a product docs site **so that** planning/openspec noise stays out of
the published nav (oss-playbook #3, D-103).

**Goal**: Trim `mkdocs.yml` `nav:` to product pages: Home, Vision, ADRs, Architecture, Usage
(incl. new install.md from S07), API stability (`API_STABILITY.md` via mkdocs include or
`docs/api-stability.md` symlink/copy), Decisions. Remove Planning subtree from nav (files remain
on disk). Add `docs/usage/install.md` to nav. `mkdocs build --strict` stays green.

**Dependencies**: E9-S07a (install doc content).

**Definition of done**: site builds strict; nav has no `planning/` entries; install page linked.

Requirements:
- **REQ-E9-S08-01** — `mkdocs build --strict` passes after nav trim. Test: CI/docs task; Verify:
  `task docs-build`; Level: L1
- **REQ-E9-S08-02** — `mkdocs.yml` nav excludes `planning/`. Test: `mkdocs.yml`; Verify:
  `! grep -q 'planning/meta-plan' mkdocs.yml`; Level: L0
- **REQ-E9-S08-03** — Usage section includes install page. Test: `mkdocs.yml`; Verify:
  `grep -q install docs/usage/install.md mkdocs.yml`; Level: doc

---

## E9-S09 — README maturity formula + honest alpha status [autonomous]

**As a** prospective adopter **I want** an honest README **so that** I know what is production-
ready vs planned (oss-playbook #3, D-104).

**Goal**: Rewrite README per oss-playbook formula: keep ≤6 badges (add Scorecard if S04); replace
"design phase" with alpha status post-E8; add **maturity table** (Policy lint/test Core, GitLab
forge Core, Provider builtins Core, Renderer Core, GitHub adapter Planned, Rego Locked, serve
Designed, Remote packs Locked); link docs site, install doc, `API_STABILITY.md`, SECURITY.md;
mermaid hero optional if already present. Do **not** link VHS GIFs until S10 assets exist.

**Dependencies**: E9-S08 (docs links).

**Definition of done**: README has maturity table + alpha callout; badges match real workflows.

Requirements:
- **REQ-E9-S09-01** — README contains maturity table with GitLab=Core and GitHub=Planned. Test:
  `README.md`; Verify: `grep -q 'GitLab' README.md && grep -q Planned README.md`; Level: doc
- **REQ-E9-S09-02** — README badge count ≤6 (oss-playbook #3) and each badge maps to existing
  workflow/metadata. Test: same; Verify:
  `test "$(grep -c 'badge.svg' README.md)" -le 6`; Level: doc
- **REQ-E9-S09-03** — Status callout no longer says "design phase" exclusively. Test: same;
  Verify: `grep -qi alpha README.md`; Level: doc

---

## E9-S10 — VHS demo tape sources [autonomous · optional GIF infra-gated]

**As a** docs maintainer **I want** reproducible terminal demos **so that** CLI UX can be shown
without manual re-recording (oss-playbook #10, D-106).

**Goal**: Add `docs/assets/demos/` with VHS `.tape` files for: `assent test` on example pack,
`assent render --finding`, `assent lint`. Document `vhs <tape>` in `docs/assets/demos/README.md`.
Optional infra-gated: CI job renders GIF on release or manual dispatch; commit GIFs only when
operator approves asset weight. **Do not** add README demo section until GIFs committed.

**Dependencies**: none (parallel).

**Definition of done**: tapes run locally with `vhs` installed; no broken README promises.

Requirements:
- **REQ-E9-S10-01** — ≥3 `.tape` files exist under `docs/assets/demos/`. Test: glob; Verify:
  `ls docs/assets/demos/*.tape | wc -l | awk '{exit ($1>=3)?0:1}'`; Level: doc
- **REQ-E9-S10-02** — demo README documents vhs version pin + render command. Test:
  `docs/assets/demos/README.md`; Verify: `grep -q vhs docs/assets/demos/README.md`; Level: doc
- **REQ-E9-S10-03** — README does not embed demo GIFs until `docs/assets/demos/*.gif` exist
  (guard test or comment in S09). Test: `hack/release/demo_guard_test.sh`; Verify:
  `! grep -q 'demos/.*\.gif' README.md || test -n "$(ls docs/assets/demos/*.gif 2>/dev/null)"`;
  Level: L0

---

## E9-S11 — OQ-2 GitLab dogfood mirror decision [autonomous · decide-and-log]

**As a** operator **I want** an explicit OQ-2 disposition **so that** E9 does not block on an
ambiguous hosting mirror.

**Goal**: Document options in `docs/planning/open-questions.md` (OQ-2 row) + log **D-105**:
**Option A (Recommended): defer** — GitHub canonical, GitLab mirror optional follow-up; **Option B:**
read-only GitLab mirror via push mirror (operator infra); **Option C:** dual-primary (reject — drift
risk). No mirror workflow in E9 unless operator overrides D-105 in INBOX. If defer: add one-line
note to README "Canonical repo: GitHub".

**Dependencies**: none (early).

**Definition of done**: D-105 recorded; OQ-2 row updated; no unowned mirror work in E9 scope.

Requirements:
- **REQ-E9-S11-01** — `docs/decisions/decisions.md` contains D-105 with defer recommendation.
  Test: decisions log; Verify: `grep -q D-105 docs/decisions/decisions.md`; Level: doc
- **REQ-E9-S11-02** — `docs/planning/open-questions.md` OQ-2 references D-105 disposition.
  Test: open-questions; Verify: `grep -q D-105 docs/planning/open-questions.md`; Level: doc
- **REQ-E9-S11-03** — E9 spec non-goals exclude mandatory GitLab mirror. Test: this spec; Verify:
  `grep -q 'OQ-2' openspec/specs/p5-e9-distribution/spec.md`; Level: doc

---

## E9-S12 — Release artifact verify harness [autonomous · engine-grade]

**As a** maintainer **I want** automated verification of snapshot/release artifacts **so that**
checksum and signature regressions fail CI before adopters see them.

**Goal**: Add `hack/release/verify-artifacts.sh`: given a `dist/` directory, verify (1) checksums
match archives (required — S02 snapshot), (2) cosign bundles validate **when present**, skip when
absent (D-110 autonomous path), (3) `assent version` in each archive matches expected semver.
Wire into `task release-verify` and PR CI after `release-snapshot`. Post-S06: optional extended
test fixture with real sig bundles (not an S12 hard dependency). Document in `SECURITY.md`.

**Dependencies**: E9-S02 only (autonomous checksum path; cosign branch optional).

**Definition of done**: verify script exits non-zero on tampered checksum; passes on snapshot
without signatures.

Requirements:
- **REQ-E9-S12-01** — verify script passes on fresh `task release-snapshot` output (no sigs). Test:
  `hack/release/verify_test.sh`; Verify: `task release-snapshot && task release-verify`; Level: L1
- **REQ-E9-S12-02** — tampered archive fails checksum verify (negative test). Test: same; Verify:
  `hack/release/verify_test.sh negative`; Level: L0
- **REQ-E9-S12-03** — when sig bundles present, invalid signature fails closed; when absent, verify
  skips cosign branch. Test: same; Verify:
- **REQ-E9-S12-04** — script documented in `hack/release/README.md` with cosign skip-when-absent
  behaviour. Test: README; Verify: `grep -q verify-artifacts hack/release/README.md`; Level: doc

---

## E9-S13 — Exit gate: tagged release + channels + docs live [infra-gated · engine-grade]

**As a** maintainer **I want** the E9 autonomous slice proven and a tagged release published **so
that** the Phase-5 distribution gate closes.

**Goal**: **Autonomous half:** S01–S04 + S07a + S08 → S09 + S12 green under `task check`; snapshot +
checksum verify in CI; docs site builds product nav; backlog marks E9 spec authoritative.
**Infra-gated half:** tag `v0.1.0` (D-108); S05–S06 publish signed assets; S07b tap (if ready);
install via curl script + `go install` + Homebrew; cosign/SBOM verified live;
`https://platformrelay.github.io/assent/` live with install page. Record **D-111** (E9 exit gate
closed) when both halves done.

**Dependencies**: E9-S01..S12.

**Definition of done**: exit checklist green; D-099–D-110 cited; D-111 on infra completion.

Requirements:
- **REQ-E9-S13-01** — autonomous exit-gate test runs snapshot + verify + docs-build. Test:
  `hack/release/exitgate_test.sh`; Verify:
  `task release-snapshot && task release-verify && task docs-build`; Level: L1
- **REQ-E9-S13-02** — infra: tagged release installs via curl script and reports signed semver
  binary. Test: manual post-tag; Verify:
  `curl -fsSL …/hack/install.sh | bash -s -- --version 0.1.0 && assent version`; Level: infra
- **REQ-E9-S13-03** — backlog + later-phases mark E9 SPEC READY → IMPLEMENTING/CLOSED per operator.
  Test: `openspec/specs/backlog.md`; Verify:
  `grep -q p5-e9-distribution/spec.md openspec/specs/backlog.md`; Level: doc

---

## Epic definition of done

| Gate | Criterion |
| --- | --- |
| **Autonomous (S13 half)** | S01–S04 + S07a + S08→S09 + S11 + S12 green; snapshot checksum verify; D-105 logged |
| **Infra (S13 half)** | Tag `v0.1.0` publishes signed assets; S07b tap optional; curl + go install work |
| **Supply chain** | cosign + SBOM + SLSA on release artifacts (S06); verify harness green (S12) |
| **No dup CI** | CodeQL/Scorecard/govulncheck not duplicated (D-102) |
| **Deferred** | GitLab mirror (D-105), PolicyComparisonSuite (D-057), E10/E11/E12 |
| **Next epic** | D-057 PolicyComparisonSuite runner (recommended) or E11 Rego per operator |

**Story count:** 14 — **10 autonomous** (S01–S04, S07a, S08, S09, S10 tapes, S11, S12), **4
infra-gated** (S05, S06, S07b, S13 publish half; S10 GIF optional within S10).

**Do first:** **E9-S01** — thinnest vertical slice (stamped `assent version`) before goreleaser or
workflows.
