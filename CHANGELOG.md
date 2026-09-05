# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Release notes are generated from gitmoji-conventional commits on the default branch using
[git-cliff](https://git-cliff.org/).

## Compatibility notes

Long-lived notes for consumers of released artifacts. They live in `cliff.toml`'s changelog
header because `CHANGELOG.md` is regenerated in full from commit history and a hand-edit here
would be silently overwritten by the next `task changelog-write`.

- **`pins.toolDigest` changes value after `v0.1.0` (D-120).** DecisionRecords emitted by
`v0.1.0` and earlier pin `toolDigest` as sha256 over the *tool version string*, so every
build stamped with the same version shared one digest; records carrying that derivation are
identifiable by exactly it. Builds after D-120 derive `toolDigest` from the binary's
canonical Go build info instead (main module path/version/sum, dependency checksums, VCS
revision and dirty flag), falling back to `sha256("buildinfo-unavailable\n" + toolVersion)`
when that build info is absent or does not identify the main module's content (no module sum
and no VCS revision — `go build -buildvcs=false`, test binaries). The field, its type and the
frozen `v1alpha1` schema are
unchanged — records published with `v0.1.0` remain schema-valid — but the *value* is not
comparable across the boundary: a mismatch between a pre-D-120 and a post-D-120 record means
"derived differently", not "different build".

- **Every `replayBundleDigest` changes value after `v0.1.0` (D-121).** The comparison corpus
published at `v0.1.0` pins each case with an undomained `sha256:<hex>` over the re-marshalled
bundle. D-121 codifies the byte-vs-document split: a `ReplayBundle` is a schema-owned JSON
*document* that consumers re-parse and re-verify, so its digest is now the domain-separated
`assent-jcs-v1` digest — canonical JSON hashed under the replay-bundle schema `$id`
(ADR-0017 §9) — rendered as bare lowercase hex with no `sha256:` tag, because the value is no
longer sha256 over bytes and must not claim to be. Digests over BYTE artifacts are unaffected
and stay raw `sha256:<hex>`: `pins.policySha`, `pins.toolDigest`, and the ADR-0019 marker
occurrence/decision digests. **Action required if you pinned, cached, or recomputed a
`replayBundleDigest`: regenerate it.** A stale pin does not silently pass — `assent compare
--suite` rejects it with a fail-closed digest mismatch before any evaluation runs. The corpus
identity itself is unchanged: no `caseId` was reused or retired and no bundle byte changed, so
D-113 immutability holds — only the algorithm computing the pin moved, versioned by D-121.

- **Ordering operators refuse text operands after `v0.1.0` (D-131).** Through `v0.1.0` a
relational operator (`<`, `<=`, `>`, `>=`) whose operands bound as CEL text compared them
LEXICALLY and returned a boolean, so a quoted numeric in adopter YAML sorted as a string: a
`partitions: "12"` -> `"6"` shrink evaluated `new >= old` to true, proved non-destructive and
reached APPROVE, while the legitimate grow `"6"` -> `"12"` evaluated false and BLOCKed. Builds
after D-131 (ADR-0013 Amendment 1) treat an ordering operator whose operand actually evaluates
to a CEL `string` or `bytes` value as an EVALUATION ERROR. `bytes` is refused deliberately:
`bytes(a) < bytes(b)` performs the identical byte-wise compare, so exempting it would re-admit
the defect as a one-token workaround. Nothing else moves — `int`, `uint`, `double`, `bool`,
`duration` and `timestamp` still order normally, and `==`, `!=`, `in`, `startsWith`, `matches`
and `size()` are unaffected. The failure direction is safe: the affected leaf errors, the rule
routes to `predicate.error`, and the decision degrades to REVIEW — no policy gets a wrong
APPROVE out of this change, only a decision someone must look at. **Action required if your
policy orders text on purpose** — ISO-8601 dates written as strings, version-ish strings,
sorted enum tiers: those leaves now REVIEW instead of deciding, and the fix is to coerce
first. Use `int(...)` or `double(...)` for quoted numerics, and `timestamp(a) < timestamp(b)`
for ISO-8601 dates, which the evaluation environment supports. Ordering raw text is no longer
expressible in tier-1 `assert`.

- **`assent run --checkout` refuses any repository containing any symlink (D-133).** Builds
after D-133 (ADR-0008 Amendment 2) contain every checkout read to the checkout root and refuse
a path reached through a symlink instead of following it: with `--checkout` the local tree is
the sole authority and `head/` is the merge-request head, contributor-authored content under
judgment, so a link at a governed path could otherwise substitute an off-tree file for the
document being judged. The scope is wider than "the merge request shipped a symlink": ANY
symlink ANYWHERE under `base/` or `head/` stops the run — including one that predates the
branch, lives in a directory unrelated to the governed subject and to `.assent/**`, and that
the merge request never touches. A plain `LICENSE -> LICENSES/Apache-2.0.txt` on the BASE side
is enough. The failure is fail-closed and loud, never a wrong verdict: exit `1`, zero forge
writes (no thread, no approval, no merge), and an error naming the offending path, the
checkout side, and the reason. Non-regular files (FIFOs, sockets, devices) under either side
are refused the same way. **Action required if your repository contains a symlink and you pass
`--checkout`: run WITHOUT the flag**, in which case the forge snapshot enumerates the
changed-file set and none of this applies — or provision a symlink-free `base/` and `head/`.
The two side directories may themselves be symlinks; only what lies beneath them may not.
Loosening this will mean folding the refusal into the opaque, fail-safe REVIEW path so such a
repository still gets a decision, never by following the link; no release carries that yet.

## Unreleased

### Chores
- build(deps): bump github/codeql-action/init from 4.37.7 to 4.37.8
- build(deps): bump github/codeql-action/analyze from 4.37.7 to 4.37.8
- build(deps): bump github/codeql-action/upload-sarif
- :arrow_up: build(deps): bump all three github/codeql-action refs to v4.37.9
- :arrow_up: build(deps): bump sbom-action to v0.24.2 and action-gh-release to v3.0.3
- :arrow_up: build(deps): bump fast-uri from 3.1.5 to 3.1.7 in schemas-validator
- :arrow_up: chore(deps): bump golang.org/x/mod to v0.40.0

### Documentation
- :memo: docs(changelog): stamp the v0.3.0 section after tagging
- :memo: docs(spec): decompose the 2026-08-18 audit Next wave as epic P5-AUD2 (D-152)
- :memo: docs(spec): AUD2-S02 names resolveRunFacts, not the nonexistent loadProviderHosts
- :memo: docs(openspec): amend AUD2-S03 to the shipped pin and log D-153 (SEC-03)
- :memo: docs(audit): document the AUD2 gate and correct stale line counts (F4, F6)
- :memo: docs(backlog): mark P5-AUD2 complete (S01-S05 landed)
- :memo: docs(e10): GitHub addressing & representation model (E10-S00)
- :memo: docs(e10): demote GitHub eligible-approval-evidence to unknown (review fixes)
- :memo: docs(e11): record the tier-1 CEL ceiling and narrow E11 (E11-S01)
- :memo: docs(e11): strike set-difference unconditionally; make the graph shape contingent
- :memo: docs(e11): Shape D justifies E11 unconditionally — an encoded adjacency CEL cannot decode (F01)
- :memo: docs(e11): Shape D rests on no-recursion alone — the decode barrier was false (F-01)
- :memo: docs(e11): delete the false cost claim; reason 1 carries Shape D alone (F3-01)
- :memo: docs(ci): D-159 recorded a residual that does not exist (ORPH-01..04)
- :memo: docs(release): correct the residual the gate documents (R3-01)
- :memo: docs(ci): withdraw three falsifiable claims from the PR-reach reader (D-157)
- :memo: docs(ci): withdraw the third copy of the retracted claim (D-157)
- :memo: docs(e10): record the operator LGTM that satisfies E10-S00's GOVERNANCE marker (D-161)
- :memo: docs(planning): record the 2026-08-25 operator rulings and withdraw the stale rule-7 block (D-162..D-164)
- :memo: docs(forge): name the catalog's overstated-coverage defect and log the coverage-floor headroom (D-165)
- :memo: docs(changelog): regenerate CHANGELOG.md for the dependabot bumps
- :memo: docs(e11): re-derive the §6 verdict table from the body, in both copies (F4-01)
- :memo: docs(adr): the rego tier is folds and graph reachability, not cross-entry checks (E11-R01)
- :memo: docs(e11): repoint the missed §1.2 anchor to §1.3 (F4-02)
- :memo: docs(e11): §1.2's headline is about iteration LEVELS, not iteration count (F4-03)
- :memo: docs(e11): §5's cost figures are not reproducible from this record (F4-04)
- :memo: docs(decisions): record D-166 — the ADR-0002 rego claim is withdrawn (E11-R01, F4-01..04)
- :memo: docs(decisions): amend D-156 — it still carried both withdrawn claims verbatim (P1)
- :memo: docs(e11): close three surviving completeness claims in the ceiling record (P1, P2-A, P2-B)
- :memo: docs(adr): the syntax gallery still named the struck tier boundary (P2-C)
- :memo: docs(adr): the rego bullet overclaimed what the input carries (P3-2)
- :memo: docs(decisions): bound D-166's completeness claim to what this lane owns (P1, second pass)
- :memo: docs(adr): correct the third struck-claim survivor and the stale gate sentence (P1, P2-A)
- :memo: docs(decisions): report the sweep as a classification, not a completeness claim (P1, P2-B, P3)
- :memo: docs(e11): last present-tense copy of the closed Test:-list gap (P2-A, third copy)
- :memo: docs(e11): correct D-166's handed-on line states and drop its unbackable count (final round)
- :memo: docs(dem): Wave A REQs gain the Test/Verify/Level triple, and the precondition is discharged
- :memo: docs(dem): Wave B REQs gain the triple, and two of them are honestly downgraded to L3
- :memo: docs(dem): Wave C REQs gain the triple; the env-split proof gets its anti-tautology half
- :memo: docs(dem): Wave D REQs gain the triple, and the operator stories stop pretending to be gateable
- :memo: docs(backlog): the DEM annotation precondition is discharged, and the five L3 REQs are named
- :memo: docs(decisions): D-166 records the execution of D-162 and the five honest downgrades
- :pencil2: docs(decisions): renumber this lane's row D-166 -> D-167 after a collision, and move its two citations with it
- :memo: docs(dem): disclose the four uncovered halves the review found, and correct the Level provenance
- :memo: docs(dem): stop claiming mkdocs proves nav linkage, and stop naming two tests that do not exist
- :memo: docs(dem): retract a coverage gap asserted from a symbol-name grep, and arm the docs claim correctly
- :memo: docs(spec): specify published-site reachability for docs/** (DOCSNAV-S01)
- :memo: docs(backlog): close SONAR-SHELL — 52 of its 54 findings were already gone (D-171)
- :memo: docs(backlog): track the D-154 guard-idiom S7688 residual as SONAR-SHELL-R01
- :memo: docs(decisions): correct SONAR-SHELL to 54 of 54 and withdraw two overstated figures (D-171)
- :memo: docs(decisions): repoint D-166 at the shifted E11 backlog lines (:594->:602, :613->:621)
- :memo: specs(redmain): the literal-emoji commit subject defect, both halves (REDMAIN-N1/N2)
- :memo: docs(decisions): D-168 records the REDMAIN-N1/N2 fix, its narrowness and its hand-off
- :memo: specs(redmain): N2-02 states the decoupled retire path; add N1-04 for the allowlist pin
- :memo: docs(decisions): D-168 (d) is corrected — one authority was the wrong instinct here
- :memo: specs(redmain): N2-02 stops claiming coverage the script does not have; file REDMAIN-N4
- :memo: docs(decisions): D-168 (f) records the unexercised paths and points at REDMAIN-N4
- :memo: docs(backlog): correct SONAR-SHELL to 53 of 54 and re-measure R01 as 12
- :memo: docs(decisions): record D-173 -- withdraw "54 of 54", require an instrument per count
- :memo: docs(backlog): file BASH32-N5N6 — a sentinel that proved the wrong thing, and a completeness claim that outran its sweep
- :memo: docs(backlog): close SONAR-GO-MISC — all 15 findings already fixed (D-172)
- :memo: docs(openspec): DOCSNAV-S02 — the nav gate needs local pre-merge evidence (D-174)
- :memo: docs(decisions): D-174 — nav gate local evidence, four stale stage counts, the published AUD2 floor, two withdrawn-claim backlog rows
- :memo: docs(gates): name the two costs the docs-build stage imposes (D-174)
- :memo: docs(decisions): D-174 records the review round — the ordinal it got wrong, the sites it missed, the costs it left unstated
- :memo: docs(decisions): D-174 records the line-joined sweep and the completeness claim that was false twice
- :memo: docs(adr): ADR-0022 — publish a container image and register it on Artifact Hub
- :memo: docs(adr): add ADR-0022 to the MkDocs nav
- :memo: docs(agents): track AGENTS.md; withdraw DOC-03's false premise
- :memo: docs(decisions): drop the drifting occurrence count from D-176

### Features
- :sparkles: feat(audit): measure REL-03/REL-07 by real mutation run, not by name
- :sparkles: docs(nav): navigate the 25 product pages, name the 31 exclusions, and gate both (D-170)

### Fixes
- :bug: docs(decisions): reference the AUD2 spec as a path, not an mkdocs link
- :bug: fix(audit): scope the REL-03 branch assertion to the guard block (F1)
- :bug: fix(audit): measure the REL-03 guard's terminal path, not "a return appears"
- :bug: fix(audit): assert no fall-through, not where the return sits
- :bug: fix(audit): require an ALL-of FAIL quorum and refuse bash < 4.3
- :bug: fix(taskfile): -count=1 on the cache-blind dogfood-comparison and e2e gates
- :bug: fix(ci): -count=1 on verify.yaml's comparison corpus dogfood
- :bug: fix(ci): run the -count=1 wiring pin in the PR-visible verify job
- :bug: fix(ci): pin COUNT1-F01 and refuse a merged gate step
- :bug: fix(ci): bump golangci-lint to v2.13.1 for Go 1.27 (D-158)
- :bug: fix(ci): close two PR-reach fail-opens the review found end-to-end (D-157)
- :bug: fix(ci): read flush block sequences and whole branch names (D-157)
- :bug: fix(ci): strip CR, and close the one fail-open direction by construction (D-157)
- :bug: fix(ci): refuse globs on branches-ignore, grade disarm keys by name (D-157)
- :bug: fix(lint): readarray -d needs bash 4.4, not 4.0 — readarray IS mapfile
- :bug: fix(lint): a redirection is not a version guard — require an adjacent comparison
- :bug: fix(lint): the include check missed 'source', one of its two spellings
- :bug: fix(lint): drop the reversed-form guard pattern — it opened a fresh fail-open
- :bug: fix(release): decouple the two exemption lists so REDMAIN-N3 has a green state (D-168)
- :pencil2: fix(lint): the replacement rule omitted the predicate's no-hash-before-the-token anchor, and prose restating a regexp does not certify itself (D-169)
- :pencil2: fix(lint): the population-coupling control could return no verdict at all, and the rule never named its operator set (D-169)
- :pencil2: fix(docs): four published `task check` stage counts said 19/14 while the list held 21 (D-174)
- :pencil2: fix(backlog): two rows still restated the ADR-0002 claim D-166 withdrew (D-174)
- :pencil2: fix(docs): a wrong ordinal this lane published, and four more stale stage counts (D-174)
- :pencil2: fix(backlog): give D-166's surviving residual its own OPEN row (D-174)
- :pencil2: fix(audit): a wrong stage ordinal survived three sweeps because it was split by a line wrap (D-174)
- :pencil2: fix(docs): the positional claims the numeric sweep could not see (D-174)

### Other
- 👷 ci(docs): stop uploading the Pages artifact on pull requests

### Refactoring
- :recycle: refactor(audit): pin REL-03 and REL-07 by test name, demote source shape
- :recycle: refactor(forge): extract the conformance suite into an importable package with an explicit observation surface (E10-S01)
- :art: style(hack): add the explicit return decode_b64 was missing (shelldre:S7682)

### Security
- :lock: fix(cmd): discriminate absent provider declaration from forge failure (REL-03)
- :lock: fix(release): pin cosign signer identity and issuer in install.sh (SEC-03)
- :lock: fix(release): widen the cosign identity pin to the real signer casing (SEC-03)
- :lock: fix(provider): bound exec stdout, set WaitDelay, capture stderr
- :lock: fix(release): pin cosign signer identity on the maintainer verify path (AUD2-F01)
- :lock: fix(hack): refuse to run gate scripts under a too-old bash (BASH32-F01)
- :lock: fix(release): grade EVERY cosign invocation, not just some (UNIV-COSIGN, D-160)
- :lock: fix(release): grade per OCCURRENCE, not per line (UC-01..UC-04, D-160)
- :lock: fix(release): stop the gate overclaiming; close the quoted-spelling hole (UC-05..UC-09)

### Testing
- :white_check_mark: test(cmd): assert the REL-03 error wrap as one contiguous substring
- :test: test(compare): kill the surviving EffectChallenge intervention mutant (TEST-02)
- :white_check_mark: test(audit): pin the AUD2 remediations behind a PR-visible exit gate
- :white_check_mark: test(audit): assert verify.yaml still triggers on pull_request
- :white_check_mark: test(hack): gate that hack/** bash 4+ features declare a floor (BASH32-F01)
- :white_check_mark: test(hack): the bash-3.2-clean assertion could not fail (BASH32-F01)
- :white_check_mark: test(hack): close two gate fail-opens found by review (BASH32-F01)
- :white_check_mark: test(ci): wire release-verify-test into task check (D-159)
- :white_check_mark: test(ci): share PR-reach and cross-pin the three PR-visible gates (D-157)
- :white_check_mark: test(ci): grade the shared PR-reach helper itself, on fixtures (D-157)
- :white_check_mark: test(lint): pin all eleven detection patterns, not the four the tree happens to use
- :white_check_mark: test(lint): give the non-probe absence assertions an anti-vacuity partner
- :white_check_mark: test(release): reject literal-emoji commit subjects, and let §8 see them (D-168)
- :white_check_mark: fix(lint): the bash-guard gate's absence assertions now name a file the scan is shown to have enumerated (D-169)
- :white_check_mark: test(gates): the nav-completeness gate now reds `task check`, not only CI (D-174)
- :white_check_mark: test(audit): assert the published AUD2 control floor instead of restating it (D-174)
## [0.3.0] - 2026-08-18

### Chores
- build(deps): bump github/codeql-action/analyze from 4.37.4 to 4.37.6 (#33)
- build(deps): bump raven-actions/actionlint from 2.1.2 to 2.2.0
- build(deps): bump astral-sh/setup-uv from 6.1.0 to 9.0.0
- build(deps): bump github/codeql-action/upload-sarif from 4.37.4 to 4.37.6
- build(deps): bump github.com/santhosh-tekuri/jsonschema/v6 from 6.0.2 to 6.0.3
- build(deps): bump actions/attest from 4.2.1 to 4.2.2
- build(deps): bump github.com/google/cel-go from 0.30.0 to 0.31.0
- build(deps): bump goreleaser/goreleaser-action from 6.4.0 to 7.2.3
- :fire: chore(deps): drop dormant Renovate; Dependabot is the updater
- build(deps): bump github/codeql-action/upload-sarif
- build(deps): bump golang.org/x/text from 0.40.0 to 0.41.0
- build(deps): bump github/codeql-action/init from 4.37.6 to 4.37.7
- build(deps): bump github/codeql-action/analyze from 4.37.6 to 4.37.7
- build(deps): bump astral-sh/setup-uv from 9.0.0 to 10.0.1

### Documentation
- :memo: docs(decisions): record D-126 (D-031 domain deferred) and D-127 (SEC-05 PAT rotation scheduled)
- :memo: docs(decisions): unlock E10 + E11 (D-140/D-141) and record ADR-0021
- :memo: docs(openspec): decompose E10 and E11 into INVEST stories
- :memo: docs(openspec): fix two P0 design defects found by adversarial review
- :memo: docs(openspec): correct the E10 story count to 19 after adding S00
- :memo: docs(openspec): fix 3 P1 design defects found by independent review of PR #45
- :memo: docs(openspec): design P5-DEM public demo repositories (D-142, OQ-32)
- :memo: docs(openspec): add the missing P5-DEM backlog rows and correct the story count
- :memo: docs(openspec): close 5 P2/P3 findings from the PR #45 re-review
- :memo: docs(openspec): close H1/H2 — the FileAtRef enumeration was not exhaustive
- :memo: docs(openspec): make the cmd/assent port boundary an enforced invariant (I1-I3)
- :memo: docs(openspec): fix 4 P1 design defects found by independent review of PR #47
- :memo: docs(openspec): close F7/F8/F10/F11/F17/F18 from the PR #47 re-review
- :memo: docs(openspec): specify Scorecard fuzzing and Best Practices residuals
- :memo: specs(p5-ex): decompose complex in-tree examples epic
- :memo: docs(decisions): record 8 operator rulings from 2026-08-16 session
- :memo: docs(truth): annotate unenforced writes:false and retract built-in IdP claims
- :memo: docs(adr): add ADR-0011 Amendment 4 — Rego/OPA rule-7 capability boundary
- :memo: docs(adr): add ADR-0015 host-side credential resolver amendment
- :memo: docs(adr): retcon ADR narrative to read as planned phases
- :memo: docs(changelog): regenerate CHANGELOG.md for PR #59's operator-ruling commits
- :memo: docs(changelog): regenerate CHANGELOG.md from git-cliff
- :memo: docs(changelog): regenerate CHANGELOG.md for the EX-S05 lane
- :memo: docs(openspec): correct test attribution in the REQ-EX-S05-05 amendment
- :memo: docs(changelog): regenerate CHANGELOG.md after rebase onto origin/main
- :memo: docs(planning): mark REF-EX C1-C8 landed; archetype-goldens manifest v2
- :memo: docs(changelog): regenerate CHANGELOG.md for the EX-S07 lane
- :memo: docs(examples): state C5's measured zero incremental --coverage credit
- :memo: docs(examples): note infra-vars has no fileEvents rule at all (C8)
- :memo: docs(openspec): amend REQ-EX-S07-03/04 to match the measured implementation
- :memo: docs(changelog): regenerate CHANGELOG.md for the EX-S07 lane follow-ups
- :memo: docs(changelog): regenerate CHANGELOG.md for the EX-S07 lane
- :memo: docs(walkthrough): pin the walkthrough to measured multi-format CLI output (EX-S09)
- :memo: docs(walkthrough): close review gaps in the EX-S09 pins and repo snapshot
- :memo: docs(changelog): regenerate CHANGELOG.md for the EX-S09 lane
- :memo: docs(walkthrough): fix Step 3 lede self-contradiction on .tf format count
- :memo: docs(changelog): regenerate CHANGELOG.md for the EX-S09 lane follow-ups
- :memo: docs(changelog): regenerate CHANGELOG.md for the EX-S09 F1 fix
- :memo: docs(changelog): regenerate CHANGELOG.md for the EX-S10 lane
- :memo: docs(changelog): regenerate CHANGELOG.md for the EX-S10 F2/F5 fixes
- :memo: docs(changelog): regenerate CHANGELOG.md for the EX-S10 F6 fix
- :memo: docs(changelog): regenerate CHANGELOG.md for the AUD-S18 CHECK_STAGES fix
- :memo: docs(changelog): regenerate CHANGELOG.md for the check_coverage_bar narrowing fix
- :memo: docs(changelog): regenerate CHANGELOG.md for the stale-'15' cosmetic fix
- :memo: docs(backlog): track the D-145 writes-gate lane as WG-S01
- :memo: docs(changelog): regenerate CHANGELOG.md for the dependabot bumps and audit fixes

### Features
- :sparkles: feat(docs): gate example pack and format claims against dogfood
- :sparkles: feat(examples): thicken service-catalog with nested sla/runtime objects
- :sparkles: feat(examples): thicken topic-registry nested YAML and nested-pointer rules
- :sparkles: feat(examples): thicken infra-vars nested tfvars maps (EX-S04)
- :sparkles: feat(examples): close REF-EX C1-C4 in topic-registry/service-catalog
- :sparkles: feat(examples): EX-S05 HCL honesty — govern .tf, pin the measured opaque decision
- :sparkles: feat(examples): close REF-EX C6 placement allow-list (infra-vars)
- :sparkles: feat(examples): close REF-EX C7 referenced-resource-ownership (gap demo)

### Fixes
- :bug: fix(ci): pin ci-audit-test in the AUD-S18 check-stage list
- :bug: specs(p5-ex): make S10 schema freeze and S07 fence non-vacuous
- :bug: fix(docs-gates): fail on unmapped format tokens and de-confound the tests-dir mutation (EX-S01 review)
- :bug: fix(examples): make privilege-tier's proving fixture load-bearing
- :bug: fix(docs): pin case+decision pairs, not just case names, in the walkthrough (F1)
- :bug: fix(examples): scope format extraction to match: lines, not the whole classes: block (F2)
- :bug: fix(examples): close the same-line trailing-comment gap in format extraction (F6)
- :bug: fix(audit): sync AUD-S18 CHECK_STAGES pin with Taskfile check task
- :bug: fix(audit): narrow check_coverage_bar to avoid dogfood-examples' per-pack coverage lines

### Refactoring
- :recycle: refactor(examples): discover dogfood packs instead of a hardcoded 3-name loop
- :art: style(audit): de-stale hardcoded '15 stages' literals in AUD-S18 gate

### Security
- :lock: build(go): pin toolchain go1.26.6 so builds ship a patched stdlib

### Testing
- :white_check_mark: test(release): anchor the D-120 note check on its header sentence, not the bare token
- :white_check_mark: test(release): key the merge-skip proof on commit shape, not subject prefix
- :white_check_mark: test(examples): close REQ-EX-S05-05 non-vacuity gap for infra-vars .tf governance
- :white_check_mark: test(examples): close REQ-EX-S05-05's other disjunct (fixture deletion)
- :white_check_mark: test(examples): close REF-EX C5 quota-ceiling-from-fact explicitly
- :white_check_mark: test(examples): pin REF-EX C8 companion-file delete as REVIEW (known limitation)
- :white_check_mark: test(examples): add the P5-EX epic exit gate (EX-S10)
## [0.2.0] - 2026-08-09

### Chores
- :construction_worker: ci(lint): depguard deny-rules for the D-123 pure tree (REQ-AUD-S07-01)
- :closed_lock_with_key: ci(release): gate the release job on verify green at the tag SHA
- :construction_worker: ci(release): enforce the verify-tag gate's own test in CI and document it
- :construction_worker: ci(release): run the verify-tag gate's test in task check
- :construction_worker: ci(schemadrift): fence the D-120 toolDigest annotation edit
- :wrench: chore(changelog): render version headings in Keep-a-Changelog bracket form
- :construction_worker: ci(release): run the CHANGELOG drift gate in task check and on main CI
- :construction_worker: ci(docs): wire the AUD-S06 docs truth-lag gates into task check (D-124)
- :construction_worker: ci(lint): wire the depguard polarity proof into task check (AUD-S07)
- :construction_worker: ci(release): wire the changelog gate test itself into task check
- :wrench: chore(gate): raise the D-010 coverage floor 90% -> 91% (D-128)
- :wrench: chore(gate): single-source the coverage floor so CI and local cannot skew (D-128)
- :wrench: fix(release): skip merge commits in cliff.toml so they never reach the Release body

### Documentation
- :memo: docs(decisions): close D-111 E9 exit gate after v0.1.0
- :memo: docs(release): Homebrew tap operator runbook and honest install
- :memo: docs(install): document live Homebrew tap install
- :memo: docs(decisions): record D-111 Homebrew Formula published
- :memo: docs(openspec): Formula live; residual is PAT rotate
- :memo: docs(adr): ADR-0020 forge snapshot changed-file completeness (REL-07 P1)
- :memo: docs(decisions): D-119..D-123 audit-remediation designs
- :memo: docs(openspec): P5-AUD audit-remediation epic — 18 stories, 3 release conditions
- :memo: docs(adr): drop AI-provenance marker + review polish (D-019)
- :memo: docs(openspec): review polish — S01 lane coordination, SEC-08 alias, RELSE-08 residual
- :memo: docs(conformance): point each AUD-S01 catalog row at the test that proves it
- :memo: docs(adr): ADR-0011 Amendment 3 -- boundary enforcement mechanism made true (D-123)
- :memo: docs(release): note the jq dependency and fix the release-tooling list numbering
- :memo: docs(usage): CLI reference pinned to the binary's help output (REQ-AUD-S05-02)
- :memo: docs(usage): cover the two compare exit codes outside the gate contract (REQ-AUD-S05-02)
- :memo: docs(schemas): publish the D-120 toolDigest description (annotation only)
- :memo: docs(cli): correct the `assent run` step order to emit-before-reconcile (D-122)
- :memo: docs(install): `go install` binaries report 0.0.0-dev, not a stamped version (DOC-11)
- :memo: docs(contract): fileEvents ships add/delete — retire the "not yet implemented" note (DOC-06)
- :memo: docs(walkthrough): per-step Shipped/Planned banners replace the design-fiction header (DOC-09)
- :memo: docs(meta-plan): renumber the Phase-5 epic table to the epics that executed (DOC-10)
- :memo: docs(adr): ADR-0020 is Accepted — its contract shipped in AUD-S01
- :memo: docs(examples): drop the pre-alpha banner and the "once it exists" harness caveat
- :memo: docs(usage): document the checkout-less enumeration contract on the -checkout flag
- :memo: docs(release): mandate patch tags over in-place asset replacement (SEC-07)
- :memo: docs(install): narrow the 0.0.0-dev consequence to the version string (D-120)
- :memo: docs(decisions): record D-124 — the AUD-S06 docs gates are unwired, Lane B owns the wiring
- :memo: docs(decisions): fold three unfixed residuals into D-124
- :memo: docs(changelog): warn record consumers that pins.toolDigest changed value (D-120)
- :memo: docs(decisions): record D-125 — CHANGELOG drift gate placement and its cost
- :memo: docs(release): the changelog drift gate is in task check now, not outside it
- :memo: docs(changelog): name the second toolDigest fallback branch
- :memo: docs(architecture): redraw the C4 diagrams from the real go list graph (AUD-S17)
- :memo: docs: mark the rego backend and GitHub adapter as planned outside the C4 pages
- :memo: docs: narrow the composition-root claim and hedge the planned modes in vision.md
- :memo: docs(compare): state the D-121 digest change where consumers will read it
- :memo: docs(adr): record the AUD-S12 malformed-marker behaviour change in ADR-0019 (review F3)
- :memo: docs(adr): correct the convergence mechanism for a skipped bot marker (review F8)
- :memo: docs(planning): mark E10 design-note steps 1-2 shipped by AUD-S15
- :memo: docs(decisions): correct D-128's margin arithmetic and its stale ownership line
- :memo: docs(open-questions): widen OQ-27 to the full string-operand class, deepen OQ-28 (review F4/F5)
- :memo: docs(aud-s18): mark AUD AUTONOMOUS COMPLETE and hand over the four operator residuals
- :memo: docs(aud-s18): renumber this lane's decision row D-131 -> D-132 (D-131 taken by the relational-string lane)
- :memo: docs: renumber this lane's decision row D-129 -> D-131 (D-129/D-130 claimed by the containment lane)
- :memo: docs(engine): the text-order refusal sentence reaches no adopter surface
- :memo: docs: record the review close-out for the three non-blocking notes in D-131
- :memo: docs(engine): narrow the bytes-binding claim to what was verified
- :memo: docs(aud-s13): retarget the TEST-02 spec arm at the D-131 refusal and close OQ-27
- :memo: docs(aud-s18): mark the OQ-27 post-audit blocker CLOSED and unpin its mutation control
- :memo: docs(aud-s18): stop the gate's prose from calling OQ-27 an open tag blocker
- :memo: docs: record D-133 and ADR-0008 Amendment 2 — the checkout is content under judgment
- :memo: docs(checkout): state the truncation mechanism precisely — WalkDir is happy, the read is not
- :memo: docs(cli): document the --checkout symlink limitation where adopters hit it
- :memo: docs(decisions): correct D-133's falsifiability claim, and record the doc carriers
- :memo: docs(release): add compatibility notes for D-133 symlinks and D-131 ordering
- :memo: docs(aud-s13): re-measure D-128 against the current denominator and de-contradict its headline
- :memo: docs(aud-s18): flip OQ-28 to CLOSED in the post-audit blocker record
- :memo: docs: truth the D-129 checkout residuals, D-130's shadow claim, OQ-28
- :memo: docs: point the OQ-28 decision-log link at docs/decisions/
- :memo: docs: truth D-130 — a non-absence registry error aborts the run, not REVIEW
- :memo: docs: record the sibling host-declaration skip in D-130
- :memo: docs: correct the D-130 abort/REVIEW comparison and the resource-owner fixture
- :memo: docs: narrow D-130's arming-posture and approval-evidence claims to what was measured
- :memo: docs(decisions): amend D-129 with the D-133 subsumption and the re-arming condition
- :memo: docs(oq): close OQ-28's residual and relocate its proof pointer after D-133
- :memo: docs(aud-s13): re-measure D-128 again after main moved (7513d79)
- :memo: docs(aud-s18): retire the 'PR #36 pending' caveats now that D-129 is on main
- :memo: docs(cli): stop claiming --arm gates approve/merge (D-134)
- :memo: docs(usage): correct the --arm safety claim in the CLI reference (D-134)
- :memo: docs(decisions): D-134 — --arm is advisory-only, superseding D-041
- :memo: docs: narrow the walkthrough claim in D-134 to what was measured
- :memo: docs(usage): the REVIEW path arms no deferred auto-merge (D-135)
- :memo: docs(usage): say which "arming" assent doctor reports (D-135)
- :memo: docs(adr): record that ADR-0009's deferred auto-merge is unimplemented (D-135)
- :memo: docs: fix ARM-08 advisory advice, annotate the dry-run drift, log OQ-29
- :memo: docs(usage): say that an advisory run still writes (ADV-01)
- :memo: docs(decisions): retire D-136's PR-guard premise instead of replacing it (CL-02/CL-03)
- :memo: docs(cli): record that --checkout is not bound to the evaluated commit (SEC-01)
- :memo: docs(adr): correct four ADR claims the code does not support
- :memo: docs: retract the unsafe advice at D-134's stopping point; dry-run is not shipped
- :memo: docs: record the reliability + security deferrals (D-138, D-139, OQ-31); OQ-29 to P1

### Features
- :sparkles: feat(cli): dispatch-table help listing the real subcommands (REQ-AUD-S05-01)

### Fixes
- :bug: fix(ci): install uv on release-exitgate for docs-build
- :bug: fix(release): do not skip=publish so Homebrew tap can push
- :bug: fix(release): use a POSIX class, not \t, in the gate-step if: guard
- :bug: fix(cli): show GITLAB_TOKEN in the run usage form (REQ-AUD-S05-01)
- :bug: docs(readme): pass the repo root to lint/test, and execute the quick-start (DOC-07)
- :bug: docs(readme): point the ADR-0014 link at the file that exists (DOC-05)
- :bug: fix(docs-gates): the scripts claimed a wiring that does not exist
- :bug: docs(examples): starter packs advertised a subcommand that does not exist
- :bug: fix(release): stop stripping the changelog header from the GitHub Release body
- :ambulance: fix(forge): skip malformed bot markers with a warning instead of bricking reconcile (AUD-S12, REL-06)
- :bug: fix(forge): treat an over-limit body as deterministic, not retryable (AUD-S10 x S11)
- :bug: fix(forge): carry reconcile warnings on refusal paths too (review F1)
- :bug: test(aud-s18): remove the three banned pipe shapes from the audit exit gate
- :bug: fix(engine): relational compare over string-bound operands must fail safe, not lexically
- :bug: fix(engine): ordering a bytes operand is text ordering too — refuse it (D-131)
- :bug: fix(checkout): keep a rooted --subject working, and unshadow the path package
- :bug: fix(release): file gitmoji subjects by conventional type, not by emoji (REL-14, D-137)
- :bug: fix(docs): point every docs-site URL at the case-sensitive Pages path (DOC-02)
- :bug: fix(render): stop minting a 404 documentation link into every MR thread (DOC-03)

### Refactoring
- :recycle: fix(forge): retry idempotent GitLab reads with bounded jittered backoff (AUD-S11, REL-04)
- :recycle: refactor(forge): lift MRInfo/ErrNotFound onto the forge port (ARCH-02)

### Security
- :lock: fix(forge): prove changed-file enumeration completeness or declare a gap
- :lock: fix(run): degrade a checkout-less run to REVIEW when enumeration is incomplete
- :lock: fix(release): reject a tag whose only verify run is a pull_request run
- :lock: fix(run): derive pins.toolDigest from Go build info (D-120)
- :lock: fix(run): emit the DecisionRecord before forge reconcile (D-122)
- :lock: security(ci): pin the Task version in verify.yaml via a single workflow env (AUD-S09 / SEC-04)
- :lock: security(ci): lockfile-pin the ajv validator and scrub checkout credentials (AUD-S14 / SEC-01 + SEC-03)
- :lock: fix(lint): close the comment-blind fail-opens in the workflow-pin gate (review F1/F3/F4/F5/F6)
- :lock: security(deps): override fast-json-patch to ^3.1.1, clearing GHSA-8gh8-hqwg-xf34 (review F2)
- :lock: fix(lint): match what EXECUTES, not what the line mentions (review N1/N2/N4)
- :lock: fix(compare): domain-separate the replay-bundle digest per D-121 (ARCH-04)
- :lock: fix(forge): bound response reads and cap pagination loops (AUD-S10, REL-03/SEC-08)
- :lock: fix(forge): make retry-body safety structural, not conventional (review F5)
- :lock: fix(lint): isolate the real CI step, and enforce command_view's scalar precondition (review N5/N6)
- :lock: fix(test): report leaked credential names, never their values (review F1/F2)
- :lock: test(aud-s18): close the schema-freeze baseline disarm the gate left open
- :lock: test(aud-s18): close the five step-isolation and baseline-override disarms
- :lock: fix(checkout): never return a truncated tree with a nil error
- :lock: fix(checkout): contain checkout reads to the checkout root
- :lock: fix(provider): contain repo-file reads to a symlink-safe root
- :lock: fix(provider): load the resource-owner registry from the target ref
- :lock: fix(provider): fall back to the checkout registry on absence alone

### Testing
- :white_check_mark: test(forge): model truncation and diff-endpoint failure in the fake
- :white_check_mark: test(cmd): serve the paginated diffs cassette in the run-path fakes
- :white_check_mark: test(conformance): require the changed-file-completeness cases
- :white_check_mark: test(core): extend the purity walk to evaldecode, compare and schemas (REQ-AUD-S07-02)
- :white_check_mark: test(lint): fail the depguard gate on an unmapped deny target (REQ-AUD-S07-01)
- :white_check_mark: test(release): scope the gate-step wiring assertions to the gate step
- :white_check_mark: test(release): make the all-runs-green rule discriminate, and pin the gate step armed
- :white_check_mark: test(release): bind the query string, drop the pipelines, and control the negatives
- :white_check_mark: test(cli): prove each dispatch name reaches its own handler (REQ-AUD-S05-01)
- :white_check_mark: test(cli): make the binding probe unsatisfiable by the usage listing (REQ-AUD-S05-01)
- :white_check_mark: test(cli): walk the whole cmd/ tree in the stale-claim pin (REQ-AUD-S05-01)
- :white_check_mark: test(schemadrift): derive the D-120 baseline anchor instead of pinning it
- :white_check_mark: test(run): pin the atomic --emit replace by target file mode (D-122)
- :white_check_mark: test(run): pin the emit-before-reconcile invariant on stdout too (D-122)
- :white_check_mark: test(docs): pin the retired truth-lag claims so they cannot come back (DOC-05/06/09/10/11)
- :white_check_mark: test(release): pin the CHANGELOG drift gate content, wiring and polarity
- :white_check_mark: test(cmd): pin policySha to raw policy bytes (D-121 byte-vs-document split)
- :test: test(provider): add the at-limit boundary control that kills the surviving mutant (review F2)
- :white_check_mark: test(lint): close the aliased-import evasion in the ARCH-02 gate
- :white_check_mark: fix(test): make the exec-timeout tests deterministic under load
- :white_check_mark: test(aud-s18): audit exit gate — 2026-08-06 conditions closed, gates green at the new bar
- :white_check_mark: test(aud-s13): TEST-02/05/06 — toCEL overflow, reconcileClearSlot branches, repo_file containment + expiry
- :white_check_mark: test(aud-s13): drop an unreachable assertion in the containment table (review F6)
- :white_check_mark: test(aud-s18): fix four vacuity defects the gate's own controls caught
- :white_check_mark: test(engine): pin how an unrepresentable numeric renders in {{ }}
- :white_check_mark: test(aud-s13): realign TEST-02 with the D-131 refusal contract
- :white_check_mark: test(checkout): make every containment guard falsifiable
- :white_check_mark: test(checkout): unmask the two collectFS guards and pin the Lstat side check
- :white_check_mark: test(aud-s18): close four disarm/vacuity vectors in the exit gate itself
- :white_check_mark: test(provider): pin the hard error on an unopenable --checkout
- :white_check_mark: test(provider): point the leak scan at the refusal reason
- :white_check_mark: test(provider): pin the registry forge error out of resolveRunFacts
- :white_check_mark: test: deduplicate the requireSymlinks helper across the merged lanes
- :white_check_mark: test(provider): reshape the D-129 live-path proof around D-133's enumeration refusal
- :white_check_mark: test(provider): guard the containment subtests at the parent, not per case
- :white_check_mark: test(release): contain the changelog gate's sandbox against an inherited GIT_DIR
- :white_check_mark: test(run): pin the advisory-only --arm summary token (ARM-04)
## [0.1.0] - 2026-08-05

### Chores
- :tada: chore: scaffold repository — vision, ADRs, C4, meta-plan, specs skeleton, Go module, examples
- :truck: chore: rename project to assent (D-009) — folder, module path, CLI, .assent/, apiVersion
- :construction_worker: ci: golangci-lint, govulncheck, gitleaks, coverage gate, dependabot (A-10)
- :green_heart: ci: run on latest stable Go toolchain — govulncheck requires go>=1.25
- :green_heart: ci: run gitleaks CLI directly — gitleaks-action needs a paid license for org repos
- :green_heart: ci: migrate .golangci.yml to golangci-lint v2 config schema
- :green_heart: ci: bump golangci-lint-action to v9.3.0 — v6 installs golangci-lint v1, incompatible with the v2 config
- :green_heart: ci: scope gitleaks to current-branch history — lane branches must not red main
- build(deps): bump actions/setup-go from 5.6.0 to 7.0.0
- build(deps): bump actions/checkout from 4.4.0 to 7.0.1
- :construction_worker: ci(hack): add migration-invariant guard before schema validation (P3-E3-S04)
- :construction_worker: ci(schemas): add stock Draft 2020-12 validator over schemas + contract fixtures (P3-P1-3)
- :wrench: chore: gitignore session .worktrees/ (agent-loop-local lanes)
- build(deps): bump actions/setup-node from 5.0.0 to 7.0.0
- build(deps): bump github.com/google/cel-go from 0.29.2 to 0.30.0
- :see_no_evil: chore: gitignore references/ (third-party self-service repos, never committed)
- build(deps): bump github/codeql-action/upload-sarif
- build(deps): bump mkdocs-material from 9.7.6 to 9.7.7 in /docs
- build(deps): bump github.com/zclconf/go-cty from 1.16.3 to 1.19.0
- build(deps): bump github/codeql-action/init from 4.37.3 to 4.37.4
- build(deps): bump github/codeql-action/analyze to 4.37.4 to match init
- build(deps): bump golang.org/x/text from 0.39.0 to 0.40.0
- :construction_worker: ci(verify): wire compare and release exit gates

### Documentation
- :memo: docs: design round 2 — effects/routing/modes/config/ports ADRs, Apache-2.0, walkthrough, naming
- :memo: docs: ADR-0014 adopter test format + D-010 TDD/90% coverage gate (Taskfile-enforced)
- :memo: docs: architecture review cycle — ADR-0013 (CEL hybrid), ADR-0015 (trust boundaries), security amendments, GUIDELINES, OSS hygiene
- :memo: docs(adr): onFail branches, per-firing points, content-keyed facts, finding lifecycle + stale-consent, lint hard-errors, scan confusion matrix
- :broom: docs: fix spec drift — openspec context, examples README, C4 to envelope+CEL architecture (P2-8)
- :memo: docs: D-012 adoption-gated scope, D-013 second-review record, new OQs, meta-plan adoption gate
- :memo: docs: D-014 public repo created with security settings; OQ-1 residual narrowed to domain
- :memo: docs(adr): ADR-0016 presentation theming — tiered customization, CEL messages, PresentationModel contract (D-015)
- :memo: docs(adr): ADR-0017 contract model — obligations, merge-result pinning, require-review, EntryRef, typed facts (D-016)
- :memo: docs: D-016, OQ-23..25, Phase-3 contract-fixture gate, README scope aligned to D-012
- :card_index_dividers: specs: backlog index — phase/epic map, gates, reading order
- :memo: specs: Phase-1 epics P1-E1..E4 — corpus, archetype inventory, forge dossier, prior art (full stories + REQs)
- :memo: specs: Phase-2 epics P2-E1..E5 — CEL/e2e/provider/secure-setup spikes + ADR acceptance (full stories + REQs)
- :memo: specs: Phase 3-5 epics — contracts, walking skeleton, E1-E9 + locked E10-E13 (D-012)
- :memo: planning: named-consumer disposition (D-017) — compat review folded in, OQ-19/20/21/23 re-scoped
- :memo: specs: P2-E6 Spike D — Kubernetes CRD/CR validation feasibility (D-017 B11)
- :memo: specs: P3-E4 lifecycle + P3-E5 publication protocol; E11/E12 unlocked, E14 added (D-017)
- :card_index_dividers: specs: backlog index — P2-E6 row, Phase 3-5 summary per D-017
- :memo: docs: D-018 operator ratifications — reference-use-case wording, Spike D deferred to Phase-3 window, named obligations confirmed
- :memo: planning: operator inbox round — D-018/D-019 ratifications, reference-use-case wording, Spike D deferred, P1-E1 sources provided
- :memo: docs(planning): prior-art review — eight tools + implications table
- :memo: docs(examples): pin OSS corpus and resolve OQ-16 (P1-E1-S02)
- :memo: docs(planning): author rule-archetype inventory (P1-E2-S01)
- :memo: docs(examples): archetype base/head fixtures (P1-E2-S02)
- :memo: docs(planning): define success metric and close OQ-25 (P1-E2-S03)
- :memo: docs(planning): add GitLab forge dossier — threads, approvals, merge preconditions, tiers (P1-E3-S01/S02)
- :memo: docs(planning): add GitHub parity dossier for the Forge-port seam (P1-E3-S03)
- :memo: docs(planning): choose Premium secure-setup topology and draft doctor checklist (P2-E4 / OQ-24)
- :memo: docs(spike): Spike C report — contract, isolation evidence, maxAge defaults
- :memo: docs(e2e): Spike B measurements — CI default is testcontainer (OQ-6)
- :memo: docs(p2-e5): consolidate ADR acceptance evidence matrix
- :bookmark: docs(p2-e5): accept ADR-0002..0017 (Phase-2 gate)
- :card_index_dividers: specs: mark Phase 1–2 epics Done — both gates CLOSED
- :memo: docs(specs): add P3-E1 schema fixture stories
- :memo: docs(specs): add P3-E2 versioning compat stories
- :memo: specs(p3-e5): author publication reconciliation protocol stories
- :card_index_dividers: specs(p3-e4): author policy-lifecycle stories (phase/profiles/comparison)
- :memo: docs(specs): add P3-E3 pack-migration stories
- :memo: docs(provider): freeze maxAge defaults table for P2-E5
- :memo: docs(publication): freeze marker grammar for P3-E5-S01
- :memo: docs(specs): amend P3-E2-S01 verify paths off internal/core (D-016)
- :memo: docs(decisions): log D-021/D-022 for roast P1-1/P1-2 schema fixes
- :memo: docs(schemas): soften residual 'exact is the safety default' phrasing (review F1)
- :memo: docs(decisions): close D-016 Phase-3 exit gate; log D-023/D-024
- :memo: docs(decisions): reframe D-016 gate as STAGED, not closed (F1/F3/F4)
- :memo: docs(publication): freeze reconciliation state table for P3-E5-S02
- :memo: docs(decisions): confirm D-016 Phase-3 exit gate CLOSED on main (D-025)
- :memo: docs(decisions): record D-026 operator bulk-ratification
- :memo: docs(publication): freeze duplicate-repair + one-publisher topology for P3-E5-S03
- :memo: docs(adr): author ADR-0019 publication marker reconciliation (P3-E5-S04)
- :memo: docs(rego): document escape-hatch quarantine marker (P3-E3-S03)
- :memo: docs(planning): document phase transitions and lint no-implicit-enforce-phase
- :memo: docs(api): publish API_STABILITY.md and do-not-generalize catalog
- :memo: docs(policy): document profile precedence and single-writer lint
- :memo: docs(usage): migrate walkthrough and sample READMEs to prove vocabulary (P3-E3-S04)
- :memo: docs(planning): register archetype golden corpus as assent test seed manifest (P3-E3-S04)
- :memo: docs(adr): author ADR-0018 policy lifecycle phase profile comparison (P3-E4-S04)
- :memo: docs(decisions): record D-027–D-032 inbox answers and accept ADR-0018/0019
- :memo: docs(planning): decompose P4-E1 walking skeleton into INVEST stories (autonomous vs infra-gated)
- :memo: docs(decisions): record D-033–D-036 (S01 pins, S05 placeholder, S10/S11 park, gitleaks fix)
- :memo: docs(decisions): record D-037 gitlab.com assent-lab + GITLAB_TOKEN location
- :memo: docs(decisions): record D-039 (P4-E1 autonomous slices complete) + D-040 (provider -race deadline)
- :memo: docs: record D-012 adoption SATISFIED (P4-E1-S11) + S10 live-green (D-041/D-042) with evidence
- :memo: docs: decompose Phase 5 E1 into 8 INVEST stories (canonical change model)
- :memo: docs: reconcile audit doc-drift findings D-02/D-03/D-04
- :memo: docs(spec): decompose Phase-5 E2 (decision engine + CEL backend) into INVEST stories (P5-E2)
- :memo: docs(spec): scope E2-S01 to loader-only, move run.go re-seat + policy.go retire to S02 (P5-E2-S01)
- :memo: docs(spec): scope E2-S02 to per-change eval primitive beside skeleton; move run.go re-seat to S04 (P5-E2-S02)
- :memo: docs(site): add branded MkDocs publishing
- :memo: docs(backlog): add SonarCloud maintainability remediation plan (4 lanes)
- :memo: docs: log D-045 security/OpenSSF hardening; mark P4-CODEQL + P4-SEC-OSSF done
- :memo: docs(backlog): record reference-repo coverage findings + example candidates (C1-C8, fact-provider gaps)
- :memo: docs(spec): decompose Phase-5 E3 (assent lint + rule catalogue) into 8 INVEST stories + lane C
- :memo: chore: log D-047 (lint decide-and-log) + extend TestCorePurity to internal/lint (E3-S01 review)
- :memo: docs(spec): defer REQ-E3-S07-03 deprecation metadata to OQ (no v1alpha1 lifecycle field)
- :memo: chore: log D-050 (lint controlling-provider reinterpretation, E3-S05 review)
- :memo: chore: log D-052 (topic-registry fileEvents gap — document-mode whole-file-delete non-destructive)
- :memo: docs(spec): decompose P5-E6 adopter-test harness spec-first (assent test + compare seed)
- :memo: docs(spec): add REQ-E6-S02-07 — full-entry reconstruction fail-safety (Part-A review F2)
- :memo: docs(spec): decompose P5-E-FILEEVENTS spec-first (whole-file add/delete match domain)
- :memo: docs(spec): mark REQ-E2-S01-03 superseded by EFE-S01 + repoint stale pin-test references (TestExamplesPacksKnownBlockers -> TestTopicRegistryLoadsAndLintsClean)
- :memo: docs(decisions): D-063 operator-confirmed — REVIEW default for ungoverned whole-file delete
- :memo: docs(decisions): D-064 — governed means enforce-effective only (not observe)
- :memo: docs(spec): decompose P5-E5 provider host + builtins spec-first
- :memo: docs(spec): E5 — maxAge exceed rejects (not clamps); argv hygiene REQ; seed sync
- :memo: docs(spec): E5-S02 — omit maxAge is load-time error (provider-contract)
- :memo: test(catalogue): include topic-registry in exitgate after D-052 close
- :memo: docs(decisions): defer E5-S09 ownership-file (D-070)
- :memo: docs(decisions): close E5-S10 exit gate (D-071/D-072)
- :memo: docs(openspec): add P5-E4 GitLab forge INVEST spec
- :memo: docs(openspec): tighten E4-S06 arming + checkout composition
- :docs: docs(decisions): D-079 records task check green at ≥90% coverage
- :memo: docs(openspec): add P5-E7 E2E conformance INVEST spec
- :memo: docs(openspec): E7-S03 catalog-index E4 pipeline arming
- :memo: docs(openspec): mark E7 autonomous slice closed (D-087)
- :memo: docs(e8): add renderer tier-0 INVEST spec
- :memo: docs(e8): address spec review on dependencies and summary port
- :memo: docs(decisions): close D-073 summary-comment slot
- :memo: docs(openspec): mark E8 autonomous complete (D-098)
- :memo: docs(e9): add distribution & release INVEST spec
- :memo: docs(e9): address review — deps, S07 split, counts
- :memo: docs(e9): restore E8 marker + changelog drift REQ
- :memo: docs(e9-s11): log OQ-2 GitLab mirror defer (D-105)
- :memo: docs(release): align install.md Go version with go.mod
- :memo: docs(e9-s11): mark OQ-2 resolved like peer rows
- :memo: docs(docs): product-only MkDocs nav + install page (E9-S08)
- :memo: docs(readme): maturity table + alpha status (E9-S09)
- :memo: docs(pcs): PolicyComparisonSuite full runner epic spec (D-057)
- :memo: docs(pcs): fix spec review — purity, catalogue extraction, D-112/115
- :memo: docs: refresh internal package map and PCS decision rows

### Features
- :sparkles: examples: onFail branches — one predicate, both outcomes, quota case explained (P1-7)
- :seedling: feat(examples): add three generic governed sample repos (P1-E1-S01)
- :alembic: feat(cel): spike A CEL residual-risk harness
- :sparkles: feat(spike): typed provider request/response schemas (P2-E3-S01)
- :sparkles: feat(schemas): add policy v1alpha1 authored schemas
- :sparkles: feat(schemas): add decision v1alpha1 runtime record schemas
- :sparkles: feat(schemas): add ApprovalEvidence v1alpha1 schema
- :sparkles: feat(schemas): add testfixture v1alpha1 adopter test-expectation schema
- :sparkles: feat(schemas): enforce unique names in Config/MergePolicy collections
- :sparkles: feat(schemas): add lint hard-error catalogue + schema-validation CI job
- :sparkles: feat(schemas): add D-016 strict + named-consumer-compat fixtures
- :sparkles: feat(provider): add major-version negotiation matrix (P3-E2-S04)
- :sparkles: feat(schemas): additive-tolerant reports with unique collection keys
- :sparkles: feat(hash): add canonical JSON digests with schema-version domain separation
- :sparkles: feat(schemas): add rollout phase field and DecisionRecord findings split
- :sparkles: feat(examples): add per-shape starter packs (P3-E3-S02)
- :sparkles: feat(schemas): add PolicyProfile writes field and Config precedence table
- :sparkles: feat(schemas): add comparison delta taxonomy and PolicyComparisonSuite
- :sparkles: feat(compat): observe/enforce twin and refused auto-merge fixtures (P3-E4-S05)
- :sparkles: feat(cmd): CLI + CI-env adapter assembling pinned EvaluationInput (P4-E1-S01)
- :sparkles: feat(cmd): doctor precondition arming refuses unprotected pipelines (P4-E1-S05)
- :sparkles: feat(change): modify-only YAML differ producing canonical ChangeSet (P4-E1-S02)
- :sparkles: feat(kind): add long-lived local GitLab kind lab
- :sparkles: feat(core): minimal obligations aggregation (P4-E1-S03)
- :sparkles: feat(core): DecisionRecord + PresentationModel report artifact (P4-E1-S04)
- :sparkles: feat(forge): Reconcile thread + approve/SHA-pinned merge against in-memory fake (P4-E1-S06/S08/S07-02)
- :sparkles: feat(forge): rerun-idempotence + deterministic duplicate-repair replay + determinism gate (P4-E1-S12)
- :sparkles: feat: GitLab forge adapter + assent run orchestration (P4-E1-S10)
- :sparkles: feat(change): first-class add/delete diffs + source positions (E1-S01)
- :sparkles: feat(change): opt-in rename fold (delete+add -> rename, default raw) (E1-S02)
- :sparkles: feat(change): JSON format adapter over the canonical value tree (E1-S03)
- :sparkles: feat(cmd): enumerate MR changed-file set + block on smuggled .assent edit (E1-S08)
- :sparkles: feat(change): HCL/tfvars format adapter (literal-only) over the value tree (E1-S04)
- :sparkles: feat(change): EntryRef derivation for map and list collections (E1-S05)
- :sparkles: feat(change): pure input resource ceilings, fail-closed (E1-S07)
- :sparkles: feat(classify): matcher-domain breadth (files/values.pointers/entryEvents/valueChanges) (E1-S06)
- :sparkles: feat(policy): frozen-contract loader — MergePolicy/RulesetBinding/Config/Pack strict decode via reused schemas, fileEvents rejected, structural assertTree (P5-E2-S01)
- :sparkles: feat(aggregate): per-change CEL activation + single-leaf eval primitive over EvaluationInput, full frozen predicate scope, typed old/new, fail-safe (P5-E2-S02)
- :sparkles: feat(aggregate): multi-obligation AND coverage across subjects over EvaluationInput (P5-E2-S04)
- :sparkles: feat(aggregate): per-firing risk points + per-binding threshold gate (P5-E2-S06)
- :sparkles: feat(aggregate): satisfy require-review from injected ApprovalEvidence — stale-sha/self-bot/none-capability fail-safe (P5-E2-S07)
- :sparkles: feat(aggregate): phase off/observe/enforce split + pack ceiling; thread observed into record (P5-E2-S08)
- :sparkles: feat(aggregate): all/any/not combinator walker + per-leaf message attribution (P5-E2-S03)
- :sparkles: feat(aggregate): profile resolution + single-writer authority (P5-E2-S09)
- :sparkles: feat(run): value-typing decoder for live-diff EvaluationInput + real-diff numeric-shrink gate (P5-E2-S04 REQ-06)
- :sparkles: feat(lint): assent lint scaffold + tolerant ingestion + obligation-coverage hard error (P5-E3-S01)
- :sparkles: feat(catalogue): generated additive-tolerant rule catalogue + assent catalogue subcommand (P5-E3-S07)
- :sparkles: feat(lint): tests-per-rule static presence hard error (P5-E3-S06)
- :sparkles: feat(test): assent test scaffold + directory-case loader + facts→envelope + single-case decision (P5-E6-S01)
- :sparkles: feat(aggregate): per-EntryRef entry-object binding in bindLeafActivation, fail-safe scalar fallback (P5-E6-S02 Part A)
- :sparkles: feat(adoptertest): whole-pack entry-tree replay + MR/approval seam (P5-E6-S02-B)
- :sparkles: feat(adoptertest): expectation matcher — findings must-contain/exact, absent, score, message~, fail-closed (P5-E6-S03)
- :sparkles: feat(compare): assent compare seed — one ReplayBundle, baseline↔candidate, one delta classified, one gate (P5-E6-S09)
- :sparkles: feat(adoptertest): failure diff UX — expected/actual + finding-level diff + ready-to-copy actual block (P5-E6-S04)
- :sparkles: feat(adoptertest): assent test --update golden-refresh — comment-preserving in-place write + CI guard (P5-E6-S05)
- :sparkles: feat(adoptertest): inline cases.yaml shorthand — alternate front-end over the shared assembler+matcher (P5-E6-S06)
- :sparkles: feat(adoptertest): assent test --coverage per-rule both-polarity gate (P5-E6-S07)
- :sparkles: feat(test): whole-pack corpus replay machinery + infra-vars green/covered (P5-E6-S08)
- :sparkles: feat(examples): rebuild service-catalog to evaluate green + both-polarity (P5-E6-S08)
- :sparkles: feat(core): load+match match.fileEvents over hand-built whole-file event — loader accept (kinds ⊆ add|delete, modify/rename load-reject), engine matcher + both-way domain disjointness, E6 mirror, pin repoint (EFE-S01)
- :sparkles: feat(core): mint whole-file events from one-sided presence + unmatched-delete REVIEW escalation (P5-EFE-S02)
- :sparkles: feat(cmd): mint FileEvent from live-checkout one-sided presence (EFE-S03)
- :sparkles: feat(examples): topic-registry provable fileEvents non-destructive (D-052)
- :sparkles: feat(examples): service-catalog file-delete BLOCK (D-061)
- :sparkles: feat(provider): E5-S01 promote ResolveFacts classifier + negotiation
- :sparkles: feat(provider): E5-S02 projection-minimized BuildQuery + maxAge load gates
- :sparkles: feat(provider): E5-S02 declaration cross-check refuse mismatch
- :sparkles: feat(provider): E5-S03 HTTP+exec transports, ScrubEnv/argv, digest-pin
- :sparkles: feat(provider): E5-S04 sensitive 15m maxAge + Fact.Sensitive handoff
- :sparkles: feat(cmd): E5-S05 wire provider host into assent run
- :sparkles: feat(provider): E5-S06 builtin gitlab-groups / forge-groups hermetic
- :sparkles: feat(provider): E5-S07 builtin/repo-file most-specific-first (REF-GAP-2)
- :sparkles: feat(provider): add resource-owner builtin (E5-S08)
- :sparkles: feat(provider): add repoFile/resourceOwner host declaration fields
- :sparkles: feat(cmd): wire builtin providers into assent run resolve path
- :sparkles: feat(forge): E4-S01 Snapshot/Resolve ports + hermetic fakes
- :sparkles: feat(forge): E4-S02 GitLab Snapshot L2 cassettes
- :sparkles: feat(forge): E4-S04 reconcile supersession, clear-slot, rescan
- :sparkles: feat(forge): E4-S03 GitLab Resolve ApprovalEvidence L2 cassettes
- :sparkles: feat(doctor): E4-S05 forge-probed arming preconditions
- :sparkles: feat(run): wire forge Snapshot/Resolve on assent run (E4-S06)
- :sparkles: feat(run): assent-policy self-edit BLOCK skips Reconcile (E4-S08)
- :sparkles: feat(e7): Spike-B e2e-vet task and operator docs (E7-S01)
- :sparkles: feat(e7): sample-repo seed generator dry-run (E7-S02)
- :sparkles: feat(e7): conformance catalog + adversarial arming gates (E7-S03)
- :sparkles: feat(ci): add E7-S04 determinism gate with local task mirror
- :sparkles: feat(e7): wire sanitization check into verify CI (E7-S05)
- :sparkles: feat(render): scaffold PresentationModel fixture loader (E8-S01)
- :sparkles: feat(schemas): add presentation block to config schema (E8-S02)
- :sparkles: feat(render): resolve presentation options from config (E8-S02)
- :sparkles: feat(render): add en locale chrome catalog
- :sparkles: feat(forge): add summary UpsertComment port and Reconcile preamble (E8-S12)
- :sparkles: feat(render): add envelope and delegate marker formatting (E8-S04)
- :sparkles: feat(aggregate): shared CEL message template compile (E8-S07)
- :sparkles: feat(render): EvalMessage CEL interpolation with redaction (E8-S07)
- :sparkles: feat(render): default finding-thread theme (E8-S08)
- :sparkles: feat(run): wire buildDesired to finding-thread renderer (E8-S08)
- :sparkles: feat(render): commit E8-S09 finding-thread goldens
- :sparkles: feat(cmd/assent): add assent render CLI (E8-S10)
- :sparkles: feat(lint): presentation lint extends E3-S04 (E8-S11)
- :sparkles: feat(render): add RenderSummary default theme + goldens
- :sparkles: feat(run): wire buildDesired Summary and render CLI
- :sparkles: feat(cmd): semver ldflags and version contract tests
- :sparkles: feat(ci): E9-S04 hardening audit, actionlint, Scorecard badge
- :package: feat(release): goreleaser v2 snapshot config and verify harness
- :sparkles: feat(release): git-cliff tasks, verify gate, and cliff.toml polish
- :sparkles: feat(release): checksum-verified install.sh and install docs
- :clapper: feat(demos): add VHS tape sources for CLI demos (E9-S10)
- :sparkles: feat(release): add artifact verify harness (E9-S12)
- :sparkles: feat(compare): classify missed and stricter intervention deltas (PCS-S02)
- :sparkles: feat(compare): classify obligation uncovered and score threshold
- :sparkles: feat(compare): emit schema-valid ComparisonRecord per case
- :sparkles: feat(compare): five-gate evaluator with acceptedDeltas allowlist (PCS-S05)
- :sparkles: feat(compare): PolicyComparisonSuite loader and RunSuite (PCS-S06)
- :sparkles: feat(compare): CLI suite mode and ADR-0018 exit codes (PCS-S07)
- :sparkles: feat(compare): adversarial corpus + CI dogfood (PCS-S08)
- :sparkles: feat(compare): PCS-S09 exit gate closes D-057 deferred scope
- :rocket: feat(release): tag-triggered workflow with goreleaser publish
- :sparkles: feat(release): cosign keyless + SBOM + SLSA on release (E9-S06)
- :sparkles: feat(release): wire Homebrew tap via goreleaser brews (E9-S07b)

### Fixes
- :bug: specs(p3-e4): fix reviewer P1 (vacuous verify) + align schema/fixture paths
- :bug: specs(p3-e3): fix reviewer P1 (vacuous verify)
- :bug: fix(schemas): amend ApprovalEvidence for roast P1-A/B and P2-C
- :bug: fix(schemas): give RulesetBinding.require an authored home (roast P1-1)
- :bug: fix(schemas): pin exact default to must-contain (roast P1-2)
- :bug: fix(schemas): strip only final extension in fileStem stem guard
- :bug: fix(contracts): pad rerun-idempotence entry-owner digest to 64 hex
- :bug: fix(change): reword differ doc comment to clear gitleaks false positive
- :bug: fix(ci): use golangci v2 linters.exclusions.paths for .worktrees
- :bug: fix(spikes): give provider contract-test exec deadline -race headroom (1s->5s)
- :bug: fix(change): key rename fold on (file,value) + cover co-occurring change (E1-S02 review F1/F2)
- :bug: fix(change): HCL unary-negated numeric literals diff instead of opaque (audit D-01)
- :bug: fix(fixture): correct d016 partitions rule — in-scope CEL `new >= old` + authored `points: 10` (P5-E2-F)
- :bug: fix(aggregate): match value pointers as globs + fail-safe guards (P5-E2-S04 review)
- :bug: fix(run): decode capitalized go-yaml bool spellings (True/TRUE/False/FALSE) as bool, not lexical string (P5-E2-S04 REQ-06 review F1)
- :bug: fix(policy): context-fresh expired fact arms to require-review, not block (E3-S08 review F2/F3)
- :bug: fix(test): fail-safe REVIEW on empty (non-opaque) changeset in adoptertest.Evaluate (P5-E6-S01)
- :bug: fix(core): unmatched-delete escalation suppress only on enforce-effective fileEvents (D-063)
- :bug: fix(cmd): keep sibling whole-file delete fold-opaque (EFE-S03 P1)
- :bug: fix(examples): dogfood topic-registry + sync sample repo map-at-root (EFE-S04 review)
- :bug: fix(provider): tighten golden-updater file modes for gosec (E5-S01)
- :bug: fix(provider): reject unknown declaration types at maxAge load (E5-S02)
- :bug: fix(provider): nosec G304 on digest-pin and maliciousexec reads (E5-S03)
- :lipstick: fix(provider): add Package builtin comment for revive
- :lipstick: fix(provider): add Package builtin comment for revive
- :bug: fix(provider): drop duplicate fixedAsOf after S06 rebase
- :bug: fix(cmd): stateful fakeGitLab discussions for post-write rescan
- :bug: fix(schemadrift): repair D-088 test vectors post-presentation
- :bug: fix(forge): fail-closed summary preamble ordering (E8-S12 review)
- :bug: fix(forge): wire publication writes through render.Envelope (E8-S04)
- :bug: fix(render): backslash-escape markdown link specials (E8-S05)
- :bug: fix(render): satisfy gosec on golden refresh helper
- :bug: fix(compare): classify points-only arithmetic as score-threshold
- :bug: fix(aggregate): apply celCostBudget on live evalRule path
- :bug: fix(release): correct SLSA verify subjects in SECURITY.md (E9-S06)
- :bug: fix(release): correct brews archive id and url_template (E9-S07b)

### Other
- :rewind: revert(kind): defer durable lab; authorize via D-038
- :test(release): add CI audit gate for single CodeQL workflow

### Refactoring
- :rotating_light: style(spike): satisfy revive/errcheck/gosec in spike package
- :truck: refactor(schemas): promote provider envelope to schemas/provider/v1alpha1
- :recycle: refactor(examples): migrate policies and archetypes to prove/onFailure
- :recycle: refactor(change): project YAML into a format-neutral value tree (E1-S03 step 1)
- :art: style(brand): add decision gate identity
- :art: style(hack): shell-script hygiene per SonarCloud shelldre rules
- :art: style(go): clear SonarCloud maintainability smells (SONAR-GO-MISC)
- :art: refactor(aggregate): group coverSubject params into coverCtx (SonarCloud go:S107)
- :recycle: refactor(run): re-seat orchestrate onto frozen MergePolicy/RulesetBinding loader + live-diff EvaluationInput + CoverWithApproval; delete toy policy.go (P5-E2-S04 REQ-06)
- :recycle: fix(catalogue): drop fabricated phase-off deprecation; report authored + ceiling-capped effective phase (P5-E3-S07 review)
- :recycle: refactor(lint): reverse fact-model convention to Option B (value at .value) — D-051 supersedes D-049
- :recycle: refactor(examples): conform pack corpus to strict loader + D-051 facts (P5-E3-C)
- :art: style(aggregate): relocate bindLeafActivation godoc off entryOr (S02-A review F1)
- :recycle: refactor(forge/conformance): move S09 harness into *_test.go
- :recycle: refactor(release): keep changelog-verify out of task check
- :recycle: refactor(pcs): extract catalogue loaders and profile activation
- :recycle: refactor(compare): extract intervention classifiers to classify_intervention.go

### Security
- :lock: docs(adr): provider trust model, authority matrix, resource limits, positions (security review A-03/A-04/A-05, P2-11)
- :lock: ci: SHA-pin actions and tool versions (roast P2-7)
- :lock: feat(hack): add sanitization check gate (P1-E1-S01-02)
- :lock: fix(deps): bump golang.org/x/text to v0.39.0 — GO-2026-5970 via hclsimple in CEL spike
- :lock: ci(schemas): add --ignore-scripts to ajv-cli install — SonarCloud S6505 (block lifecycle-script exec on npm install)
- :lock: docs(security): add SECURITY.md policy + CODEOWNERS review routing
- :lock: ci(security): add CodeQL SAST, OpenSSF Scorecard, scheduled govulncheck
- :lock: feat(core): fact tri-state fail-safe + controlling-fact fail-open rejection (P5-E2-S05)
- :lock: fix(aggregate): count DISTINCT eligible non-author approver IDs — close duplicate-approver approval bypass (P5-E2-S07 F1/F4)
- :lock: fix(aggregate): fail-safe empty all/any combinators + depth/malformed/error tree tests (P5-E2-S03)
- :lock: feat(lint): fact-model Option A (auto-unwrap, D-049) + AST-based facts-reference lint — closes the E2-S05 posture-scan evasion (raw-string/bracket/whitespace) sound-by-construction (P5-E3-S03)
- :lock: feat(lint): structural hard errors — reserved-class, no-implicit-enforce-phase (surgical phase-only schema-invalid dedupe), unkeyed-list (P5-E3-S02)
- :lock: feat(lint): predicate-scope + {{ }} message-template lint — undeclared-predicate-scope over when/cel leaves + message-template-scope, via new exported aggregate.CompileCheck reusing the frozen 11-field env (P5-E3-S04)
- :lock: feat(lint): config-posture hard errors — fail-open (widened) + single-writer-profile (P5-E3-S05)
- :lock: fix(schemadrift): structural D-088 allowlist vs origin/main
- :lock: feat(render): EscapeMarkdown and Clamp helpers (E8-S05)
- :lock: feat(render): sensitive fact redaction for markdown (E8-S06)
- :lock: fix(render): redact sensitive facts on .value accessors (E8-S07)

### Testing
- :test_tube: test(spike): contract parity HTTP vs exec toy provider (REQ-P2-E3-S01-01)
- :test_tube: test(spike): fail-closed fact state classification (REQ-P2-E3-S01-02)
- :test_tube: test(spike): token isolation vs malicious exec provider (REQ-P2-E3-S02-01)
- :test_tube: test(spike): projection minimization + capability refusal (REQ-P2-E3-S02-02)
- :test_tube: test(e2e): Spike B boot scripts + product-surface smoke (P2-E2)
- :test: test(schemas): fail Config/RulesetBinding/MergePolicy schema cases
- :test: test(schemas): fail ApprovalEvidence schema cases
- :test: test(schemas): fail testfixture expect.yaml/cases.yaml schema cases
- :test: test(schemas): add P3-E2-S01 strict-decode compat fixture suite
- :test: test(schemas): add P3-E1-S07 exit-gate + named-consumer-compat fixture tests
- :test: test(schemas): demonstrate content-derived identity across rename (F2, F5)
- :test_tube: test(schemas): P3-E2-S02 report tolerance and collection identity
- :white_check_mark: test(core): trust-boundary goldens — assent-policy BLOCK + tokenless (P4-E1-S07-01/03)
- :construction: test(e2e): wire skeleton e2e harness under e2e build tag, skip without infra (P4-E1-S09)
- :white_check_mark: test(aggregate): reproduce frozen D-016 DecisionRecord end-to-end (P5-E2-S10)
- :white_check_mark: test(lint): E3-S08 exit gate — hard-error corpus + archetype Cover gate + catalogue generation (P5-E3-S08)
- :white_check_mark: test(adoptertest): drive finding-diff deltas through RenderFailure (F1, P5-E6-S04)
- :white_check_mark: test(cmd): hermetic CI env in TestUpdateLeavesPassingCasesUntouched (S05 review F1)
- :white_check_mark: test(adoptertest): white-box table test for ruleMatchesAny (S07 review F1)
- :white_check_mark: test(test): P5-E6-S08 exit gate — corpus green + broken-pack diff + dogfood CI
- :white_check_mark: test(test): fail-closed guard tests for the D-060 combine + binding-collapse (P5-E6-S08 review F1)
- :white_check_mark: test(cmd): unpin topic-registry green corpus + reconcile D-061
- :white_check_mark: test(cmd): EFE-S05 exit gate — create/delete fixtures + coverage + determinism
- :test_tube: test(cmd): add E5 exit gate resolved-facts hermetic test
- :test: test(forge): E4-S03 bot exclusion + gap cassettes (review fix)
- :test: test(doctor): E4-S05 review fixes — forge wins over env
- :test: feat(conformance): add SHA-guard rejection goldens (E4-S07)
- :test: test(forge/conformance): P3-E5 reconciliation replay (E4-S09)
- :test: test(cmd/assent): E4 autonomous exit gate (E4-S10)
- :test: test(conformance): E7-S08 exit gate checklist
- :test: test(schemadrift): allow D-088 presentation-only schema drift
- :test: test(forge): raise internal coverage after E8-S12
- :test: render(E8-S09): add golden test and render fixture corpus
- :test: test(cmd/assent): add render CLI tests (E8-S10)
- :test: test(forge): P3-E5 replay asserts rendered summary bodies
- :test: test(render): E8-S14 exit gate + safety split
- :white_check_mark: test(release): changelog_test.sh for REQ-E9-S03-01..03
- :test_tube: test(release): E9-S13 autonomous exit gate
## [0.0.0] - 2026-08-04

Pre-release development history before the first tagged alpha (`v0.1.0`, D-108). Milestone ADRs
and phase specs live under `docs/decisions/` and `openspec/specs/`.
