# P5-TDS — Test depth signals (fuzz in CI, mutation measurement, decision-path benchmarks)

**Epic ID / REQ prefix:** `TDS` / `REQ-TDS-Snn-nn`. Cross-cutting hygiene epic.

**Problem.** Coverage (>=91%, `COVERAGE_MIN`) says lines ran, not that tests can fail.
Verified 2026-10-05: `rg '^func Fuzz'` finds **zero** targets; `p5-sec-scorecard-residuals`
**SEC-SC-S01** (native Go fuzz targets, with a bounded CI smoke) is specified but **not
implemented**, so this epic does **not** duplicate it (see dependency). There is no mutation
measurement and no benchmark for the hot decision path (`internal/core/decision`,
`internal/core/aggregate`). Prior art: attune `scripts/run-fuzz.sh` (flake classifier).

**Dependency on SEC-SC-S01.** TDS-S01 only upgrades *how* S01's targets run in CI; it is
blocked until S01 lands the targets. If S01's own CI smoke (<=15s total, REQ-SEC-SC-S01-04)
is already wired, S01 here **replaces the runner script only** and supersedes that REQ's
invocation — recorded as a spec-change note on SEC-SC, not silently.

**Not in scope:** writing the fuzz targets (SEC-SC-S01); OSS-Fuzz; mutation testing as a
**gate** (measurement only — mutation scores on policy-engine code are noisy and a hard
threshold would be a gate-weakening magnet); benchmark thresholds as gates.

**Lanes:** **A** fuzz runner · **B** mutation · **C** benchmarks.

---

## TDS-S01 — CI fuzz runner with flake classifier `[autonomous]`

**Depends on:** SEC-SC-S01 (targets exist).

- Given each `func Fuzz*`, when CI runs it, then each target gets a 30s `-fuzztime` budget
  (`hack/ci/run-fuzz.sh` enumerates targets; it fails if it finds zero).
- Given a run that fails **only** with the pure Go flake `context deadline exceeded`
  (golang/go#75804) and nothing else, when classified, then it is retried (max 2) and the
  retry result stands.
- **Adversarial** — given any crash, panic, `FAIL`, new `testdata/fuzz` reproducer, or
  timeout mixed with other output, when classified, then **no retry**, job red.
- **Adversarial** — given the classifier input contains the flake string *and* a crash,
  then red (the flake string must be the entire failure signature).

Requirements:

- **REQ-TDS-S01-01** — runner enumerates and runs every target at 30s; zero targets is red.
  Test: `hack/ci/run_fuzz_test.sh` (create); Verify: `bash hack/ci/run_fuzz_test.sh`; Level: L0
- **REQ-TDS-S01-02** — classifier table: pure flake -> retry; flake+crash -> red; crash ->
  red; clean -> green. Test: same; Verify: `bash hack/ci/run_fuzz_test.sh --classify`; Level: L0
- **REQ-TDS-S01-03** *(adversarial)* — removing retry cap, or widening the flake pattern
  (substring instead of whole-signature), reds the self-test. Test: same; Verify:
  `bash hack/ci/run_fuzz_test.sh --self-test`; Level: L0

**Counterpoint.** 30s x N targets on every PR may be heavy: run on PRs touching
`internal/change/**`/`internal/core/**` and nightly (feeds NIT) — decided at implementation
by measured cost; the budget is a spec value, the trigger is not.

---

## TDS-S02 — Nightly report-only mutation testing on the pure core `[autonomous]`

**Depends on:** NIT-S01 (nightly workflow exists; otherwise a `workflow_dispatch` stub).

- Given the nightly workflow, when it runs, then a mutation tool (e.g. `gremlins`, version
  pinned) runs on `internal/core/...` (decision, aggregate, classify) and writes the
  mutation score and the list of **lived** mutants to the job summary and an artifact.
- Given any score, when the job finishes, then it is green — **measurement, not a gate**.
  Only a tool crash or an empty report is red.
- **Adversarial / evidence rule** — given the tool run, when reviewed, then a **no-op
  control** exists: a deliberately untested trivial mutant must be reported as lived
  (proves the harness can see survivors); if the control mutant is "killed" the report is
  invalid and the job is red.
- The control lives in a **fixture module under `hack/testdata/mutation-control/`** (own
  `go.mod`, one trivial function with no test), never in `internal/core` (it would distort
  the coverage floor and trip `internal/core/purity_test.go`); the nightly job runs the tool
  on that module as well as on `internal/core/...`.
- The tool and version are pinned at implementation (candidate: `gremlins`; the implementer
  must first confirm that release supports Go 1.26 and record it — if not, pick another tool
  or defer, do not pin an unverified one).
- Given rule 7 (determinism), when the tool runs, then it works on a copy and never edits
  tracked files, and no mutation tooling enters the decision path.

Requirements:

- **REQ-TDS-S02-01** — nightly job produces score + survivors summary for the three
  packages. Test: `hack/ci/mutation_report_test.sh` (create; parses a fixture report);
  Verify: `bash hack/ci/mutation_report_test.sh`; Level: L0
- **REQ-TDS-S02-02** *(control)* — control survivor must appear, else red. Test: same;
  Verify: `bash hack/ci/mutation_report_test.sh --self-test`; Level: L0
- **REQ-TDS-S02-03** *(forbidden outcome)* — no threshold flag (`--threshold-*` /
  `exit` on score) in the workflow. Test: `hack/lint/nightly_wiring_test.sh` (create, introduced by NIT-S01); Verify:
  `bash hack/lint/nightly_wiring_test.sh`; Level: L0

**Not in scope:** acting on survivors (each becomes its own test-adding fix); mutating
`internal/change` (parser-heavy, slow; follow-up).

---

## TDS-S03 — Benchmarks for the hot decision path, benchstat in summary `[autonomous]`

**Depends on:** none.

- Given `internal/core/decision` and `aggregate`, when `go test -run=^$ -bench=. -benchmem
  -count=10` runs, then at least one benchmark per package exists on a realistic fixture
  (reuse existing golden inputs; no new fixtures that diverge from the corpus).
- Given a PR, when the benchmark job runs on base and head, then `benchstat` output is
  appended to `$GITHUB_STEP_SUMMARY`; the job is **report-only** (never fails on a
  regression; fails only if benchmarks do not compile/run).
- Given determinism rule 5, then benchmark code lives in `_test.go` and uses no clock/rand
  in the code under test (the benchmark harness's own timer is fine).

Requirements:

- **REQ-TDS-S03-01** — benchmarks exist and run. Test:
  `internal/core/decision/decision_bench_test.go`, `internal/core/aggregate/aggregate_bench_test.go`
  (create); Verify: `go test -run=^$ -bench=. -benchtime=1x ./internal/core/...`; Level: L0
- **REQ-TDS-S03-02** — the benchmark summary job (a `pull_request` job in `verify.yaml`,
  report-only, `-count>=10`) is guarded in the workflow it lives in. Test:
  `hack/lint/bench_job_test.sh` (create); Verify: `bash hack/lint/bench_job_test.sh`; Level: L0
- **REQ-TDS-S03-03** *(forbidden outcome)* — no `benchstat` exit-code or delta threshold gate;
  the guard has a failing-direction case. Test: `hack/lint/bench_job_test.sh`; Verify:
  `bash hack/lint/bench_job_test.sh --self-test`; Level: L0

---

## Exit

S01 green after SEC-SC-S01; S02/S03 produce their reports at least once on `main`. New `hack/lint`/`hack/ci` scripts are hooked into an **existing** `task check` stage where one fits (no `CHECK_STAGES` change, but a `STAGE_BODY_PINS` entry per hooked command, `exitgate_test.sh:207`); a script becomes a new `check:` stage — and then needs a deliberate `CHECK_STAGES` pin in `hack/audit/exitgate_test.sh` — only if no existing stage fits.
