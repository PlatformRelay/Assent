# OSS playbook — practices adopted from sibling projects

Distilled 2026-07-21 from an inventory of two sibling OSS repos (a Kubernetes operator with
near-CNCF-grade supply-chain rigor; an inventory operator with a strong product/branding
surface). Ordered backlog for assent; "when" anchors each item to the meta-plan.

## Adopt (prioritized)

| # | Practice | When | Status |
| --- | --- | --- | --- |
| 1 | SECURITY.md with threat-model summary, supported-versions table, private reporting | after the adversarial design review lands (threat model becomes real) | ⏳ |
| 2 | Community files: CODE_OF_CONDUCT (Contributor Covenant 2.1), CONTRIBUTING with a **standards map** (each doc owns exactly one concern), GOVERNANCE with maintainer-continuity note | CoC + templates now; CONTRIBUTING with GUIDELINES.md; GOVERNANCE before going public | 🔶 |
| 3 | README formula: logo → ≤6 badges → one-line value prop + tagline → status callout → why-bullets with ADR links → mermaid hero diagram → quick start → **honest maturity table** (frontends / forges / providers as Core·Beta·Planned) → community/security tables | with first public push (E9) | ⏳ |
| 4 | Release engineering: tag-triggered workflow, git-cliff notes, cosign keyless signing, SLSA provenance + SBOM attestation — **plus goreleaser for CLI binaries** (`go install`, brew, curl+checksum): the one piece the siblings lack and a CLI must have | E9 | ⏳ |
| 5 | CI hardening: gitleaks, CodeQL, OpenSSF Scorecard (+badge), govulncheck, SHA-pinned actions, dependabot+renovate, codecov | E9, incrementally from first code | ⏳ |
| 6 | Branding pack via reproducible source assets and generator script (logos, social cards, favicons) | before going public | ✅ |
| 7 | mkdocs-material site + GH Pages docs workflow (stub `mkdocs.yml` exists) | E9 | ⏳ |
| 8 | Compatibility-promise doc (`API_STABILITY.md` equivalent): what policy schema / decision contract / test-format guarantee per version, graduation criteria — **high trust signal for a gate tool** | Phase 3 (contracts freeze) | ⏳ |
| 9 | Published tiered test strategy + "What CI proves" README table | with ADR-0006/0014 acceptance | ⏳ |
| 10 | Demo assets: VHS-scripted terminal GIFs (ideal for a CLI) + a **live demo repo** (public self-service repo with `.assent/` policies and real auto-merged MRs) | after walking skeleton (Phase 4) | ⏳ |
| 11 | Hygiene configs: .editorconfig, CODEOWNERS, .go-arch-lint.yml (enforces the hexagonal rule in ADR-0011), mockery config | with first code | ⏳ |
| 12 | Issue templates + PR template — **missing in both siblings**; add day one | now | 🔶 |

## Avoid (observed anti-patterns)

1. Referencing demo assets before they exist (empty demo dirs behind README promises).
2. Badge walls (>6 badges reads as noise).
3. Publishing internal working notes on the docs site — keep `docs/planning/` + `openspec/`
   out of the mkdocs nav once the site goes live (only `docs/` product pages published).
4. Commit SHAs inside CHANGELOG entries (squash-merges invalidate them → needs a self-healing
   bot; configure cliff.toml without SHAs instead).
5. Generated output (test binaries, coverage, site/) accumulating at repo root — everything
   goes to `bin/` / `artifacts/` (gitignored) from the start.

## Deliberate divergence

License: assent is **Apache-2.0** (patent grant; Kubernetes/Argo family) while the siblings
are MIT — intentional for a policy engine, documented in D-002.
