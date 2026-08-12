# P5-SEC-SC — OpenSSF Scorecard residuals (fuzzing + Best Practices badge)

**Epic ID / REQ prefix:** `SEC-SC` / `REQ-SEC-SC-Snn-nn`. Cross-cutting hygiene epic (same
naming class as `AUD` / `AUD2` / `PCS`). Does **not** consume E12–E14.

**Problem**: The 2026-08-13 security sweep of `PlatformRelay/assent` found **zero** Dependabot
vulnerability alerts, **zero** CodeQL alerts, **zero** secret-scanning alerts, and SonarCloud
`PlatformRelay_assent` at Quality Gate **OK** with 0 bugs / 0 vulnerabilities / 0 unreviewed
hotspots. The only remaining *open* GitHub code-scanning alerts from OpenSSF Scorecard that
are real work (not solo-maintainer process limits) are:

| Alert | Scorecard check | Disposition |
| --- | --- | --- |
| [#3](https://github.com/PlatformRelay/assent/security/code-scanning/3) | **Fuzzing** (score 0) | **this epic, S01** — no `Fuzz*` targets exist |
| [#6](https://github.com/PlatformRelay/assent/security/code-scanning/6) | **CII-Best-Practices** (score 0) | **this epic, S02** — no Best Practices badge |

Dismissed in the same sweep (not this epic): **Maintained** (#5, false positive: repo <90 days),
**Code-Review** (#4, won't-fix: solo maintainer), **Branch-Protection** (#1, won't-fix: required
approvers would deadlock a solo maintainer; `main` already has required `verify`+CodeQL,
`enforce_admins`, no force-push/delete).

**Why this is not a drive-by Scorecard chase:** Scorecard's Fuzzing check is a *proxy* for
"untrusted bytes have a fuzzer." assent's attack surface is exactly that: YAML/JSON/HCL diffs
and CEL expressions arriving from an MR. Native Go fuzzing is the smallest honest close. The
Best Practices badge is a *process* artefact (human claims on bestpractices.dev) and cannot be
faked in-tree — S02 is operator-gated.

**Not in scope:** OSS-Fuzz / ClusterFuzzLite onboarding; requiring GitHub PR approvers;
raising the Scorecard *overall* score as a release gate; SonarCloud CODE_SMELL hygiene
(`SONAR-SHELL` / `SONAR-GO-*` already on the backlog).

**Lanes:** **A** parser fuzz (`internal/change/**`, optional CEL compile) · **B** docs/badge
(`docs/**`, README).

---

## SEC-SC-S01 — Native Go fuzz targets on untrusted-byte parsers `[autonomous]`

**Lane A.** As an adopter feeding MR diffs into assent, I want the YAML/JSON/HCL differ (and
the CEL compile path) exercised by Go's native fuzzer, so malformed input cannot panic or
fail-open the change model, and so Scorecard's Fuzzing check can detect `func Fuzz…`.

**Depends on:** none. **Do first.**

Acceptance criteria:

- Given the repository, when Scorecard's Fuzzing check (or an equivalent `rg '^func Fuzz'`
  over `**/*_test.go`) runs, then at least **two** Go native fuzz targets exist
  (`func FuzzXxx(f *testing.F)`).
- Given a fuzz target over `internal/change.Diff` (YAML) and one over the JSON and/or HCL
  adapter, when `go test -fuzz=Fuzz -fuzztime=5s` is run on each, then it completes without
  crash and without modifying `internal/core` (fuzz lives in `_test.go` only — rule 7:
  randomness must not enter the decision path).
- Given a corpus seed of valid fixtures already used by `diff_test.go` / `diff_json_test.go`
  / `diff_hcl_test.go`, when the fuzzer starts, then those seeds are `f.Add`'d so the first
  seconds are not wasted on empty input.
- **Edge case** — given input that is not valid YAML/JSON/HCL, when the fuzzer calls Diff,
  then the function returns an error or an opaque/REVIEW-safe ChangeSet; it must **not**
  panic, and it must **not** invent a successful structured diff from garbage.
- **Edge case** — given a panic or `go test` crash during `-fuzztime=5s`, when CI runs the
  smoke job, then the job is red (the smoke is a real gate, not a `|| true`).
- Given CI, when `verify` (or a dedicated short job it already calls) runs, then each fuzz
  target is invoked with a **bounded** `-fuzztime` (≤15s total across targets) so the gate
  stays PR-viable.

**Definition of done:** ≥2 `Fuzz*` functions in-tree; CI smoke reds on crash; Scorecard
Fuzzing can detect them on the next scheduled run; `task check` green; no decision-path
package imports `testing` or `math/rand`.

**Not in scope:** OSS-Fuzz project registration; fuzzing the GitLab HTTP client; mutating
production parsers to "be more fuzzable" beyond crash/fail-open fixes discovered by the
fuzzer (those land in this story if small, else a follow-up fix).

Requirements:

- **REQ-SEC-SC-S01-01** *(YAML differ fuzz)* — `FuzzDiffYAML` (name flexible) calls
  `change.Diff` with `f.Add` seeds from existing YAML fixtures. Test:
  `internal/change/diff_fuzz_test.go`; Verify:
  `go test ./internal/change -run=^$ -fuzz=FuzzDiff -fuzztime=5s`; Level: L0
- **REQ-SEC-SC-S01-02** *(JSON or HCL adapter fuzz)* — a second target covers
  `Diff`/`DiffEntries` on JSON and/or HCL bytes with the same panic/fail-open contract.
  Test: `internal/change/*_fuzz_test.go`; Verify:
  `go test ./internal/change -run=^$ -fuzz=Fuzz -fuzztime=5s`; Level: L0
- **REQ-SEC-SC-S01-03** *(garbage is not a structured success)* — a table of truncated /
  random / NUL-injected inputs either errors or yields opaque/undecidable; never a
  silent well-typed diff. Test: `internal/change/diff_fuzz_test.go`; Verify:
  `go test ./internal/change -run 'TestFuzzGarbage|TestDiffFuzzCorpus' -count=1`; Level: L0
- **REQ-SEC-SC-S01-04** *(CI smoke · adversarial)* — deleting every `func Fuzz` reds a
  documented gate (script or `go test` invocation in `verify.yaml` / `Taskfile.yml`);
  proven in the failing direction, then restored. Test: `hack/lint/fuzz_targets_test.sh`
  (create); Verify: `bash hack/lint/fuzz_targets_test.sh --self-test`; Level: L0
- **REQ-SEC-SC-S01-05** *(rule 7 fence)* — no production file under `internal/core` or
  `internal/change` (excluding `_test.go`) gains `math/rand`, `crypto/rand`, or
  `time.Now` as a result of this story. Test: existing purity / determinism gates;
  Verify: `task check`; Level: L0

---

## SEC-SC-S02 — OpenSSF Best Practices passing badge `[operator-gated]`

**Lane B.** As a downstream Scorecard consumer, I want assent to hold at least a
**passing** [OpenSSF Best Practices](https://www.bestpractices.dev/) badge so the CII check
is no longer a permanent 0, and so the claims we already meet (SECURITY.md, CodeQL,
Scorecard, CI, license) are recorded where the check looks.

**Depends on:** none (parallel with S01). **Cannot start until the operator creates the
project on bestpractices.dev** — that site is an authenticated human claim form; an agent
must not invent a badge URL.

Acceptance criteria:

- Given the operator has created the project entry for `github.com/PlatformRelay/assent`,
  when the in-tree evidence page is read, then every *passing*-tier criterion we already
  satisfy is listed with a path (SECURITY.md, CodeQL workflow, Scorecard workflow,
  Apache-2.0 LICENSE, required CI on `main`, private vuln reporting).
- Given a criterion we do **not** yet meet, when the same page is read, then it is listed
  as a gap with an owner (in-tree story vs operator), never silently ticked.
- Given the badge reaches **passing**, when `README.md` is read, then the Best Practices
  badge sits next to the existing Scorecard badge and the URL is the live project, not a
  placeholder.
- **Edge case** — given the badge is still in-progress, when README is read, then it does
  **not** display a passing/silver/gold badge (no fake green).
- **Edge case** — given a later editor adds a badge URL that 404s, when the docs/hygiene
  gate runs, then it fails (curl or equivalent against the badge JSON API).

**Definition of done:** operator-owned project exists; in-tree evidence page honest;
README badge only after passing; Scorecard CII check can see the badge on the next run.

**Not in scope:** silver/gold criteria; changing SECURITY.md policy; implementing S01.

Requirements:

- **REQ-SEC-SC-S02-01** *(operator project)* — a `D-nnn` row records the bestpractices.dev
  project URL once the operator creates it. Test: `docs/decisions/decisions.md`; Verify:
  `rg 'bestpractices.dev' docs/decisions/decisions.md`; Level: doc
- **REQ-SEC-SC-S02-02** *(honest evidence)* — `docs/planning/oss-best-practices.md` (name
  flexible) maps passing-tier criteria to in-tree evidence or an explicit gap. Test: that
  page; Verify: `bash hack/lint/docs_truth_test.sh` or a dedicated `rg` gate that the
  page exists and contains `Gap:`; Level: doc
- **REQ-SEC-SC-S02-03** *(README only after passing)* — README gains the badge **iff** the
  project reports `passing` or higher; a 404 or `in_progress` URL must not ship as a
  passing badge. Test: README + gate; Verify:
  `bash hack/lint/best_practices_badge_test.sh` (create); Level: L0
- **REQ-SEC-SC-S02-04** *(regression fence)* — pointing the badge at a non-existent project
  reds the gate; proven failing-direction then restored. Test: same script; Verify:
  `bash hack/lint/best_practices_badge_test.sh --self-test`; Level: L0

---

## Exit

S01 lands autonomously and is the slice to start. S02 waits on the operator creating the
Best Practices project (INBOX). The epic is done when Scorecard Fuzzing is no longer 0
(after the next weekly `scorecard.yaml` run) and either the CII badge is passing or S02's
evidence page lists remaining gaps without a fake README badge.
