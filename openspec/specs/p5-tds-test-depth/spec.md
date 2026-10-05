# P5-TDS — Test depth signals (fuzz in CI, mutation measurement, benchmarks)

**Epic ID / REQ prefix:** `TDS` / `REQ-TDS-Snn-nn`. Cross-cutting hygiene epic. Gate wiring: see
the [gate-wiring rule](../README.md) (D1).

**Problem.** Coverage (>=91%) shows lines ran, not that tests can fail. Verified 2026-10-05:
`rg '^func Fuzz'` finds **zero** targets; **SEC-SC-S01** (native fuzz targets + bounded smoke) is
specified but unimplemented, so this epic does **not** duplicate it. No mutation measurement and
no benchmark of the decision path (`internal/core/decision`, `aggregate`). Prior art: attune
`scripts/run-fuzz.sh`.

**Dependency on SEC-SC-S01.** TDS-S01 changes only *how* S01's targets run in CI; if S01 already
wired a <=15s smoke (REQ-SEC-SC-S01-04), TDS-S01 supersedes that invocation, recorded as a spec
note on SEC-SC.

**Not in scope:** writing the fuzz targets; OSS-Fuzz; mutation as a **gate**; benchmark
thresholds.

---

## TDS-S01 — CI fuzz runner with flake classifier `[blocked-by SEC-SC-S01]`

- Given each `func Fuzz*`, when CI runs it, then each target gets `-fuzztime=30s`; the runner
  enumerates targets and **zero targets is red**.
- Retry rule (D7): **at most once**, only when the output contains the Go deadline flake
  (golang/go#75804) **and none of** `panic:`, `fatal error:`, `Failing input written`,
  `signal: killed`, or new files under `testdata/fuzz/`. Note the genuine flake output itself
  contains `--- FAIL: FuzzX`, so `FAIL` is **not** a deny marker.
- **Adversarial** — any deny marker with the flake text present is red, no retry; two flake
  failures in a row is red.

Requirements:

- **REQ-TDS-S01-01** — runner enumerates and runs every target at 30s; zero targets red. Test:
  `hack/ci/run_fuzz_test.sh` (create); Verify: `bash hack/ci/run_fuzz_test.sh`; Level: L0
- **REQ-TDS-S01-02** — classifier table (pure flake -> one retry; flake + each deny marker ->
  red; clean -> green). Test: same; Verify: `bash hack/ci/run_fuzz_test.sh --classify`; Level: L0
- **REQ-TDS-S01-03** *(adversarial)* — retry cap and marker list can't be loosened silently
  (self-test mutates them and expects red). Test: same; Verify:
  `bash hack/ci/run_fuzz_test.sh --self-test`; Level: L0

**Counterpoint.** 30s x N targets per PR may be heavy; trigger (PR-path-filtered vs nightly) is
decided at implementation by measured cost, the 30s budget is the spec value.

---

## TDS-S02 — Nightly report-only mutation testing `[deferred until NIT-S02 shipped]`

- Own workflow `mutation.yaml` (own issue via NIT-S02, cannot mask regressions). One pinned
  mutation tool (candidate `gremlins`) **confirmed to support the `go.mod` Go**; if not, pick
  another or stay deferred — never pin unverified.
- Runs on the pure core (`internal/core/...`: decision, aggregate, classify); score and **lived**
  mutants go to the job summary/artifact. **Measurement, not a gate**: only a tool crash or an
  empty report is red; no threshold flag.
- **Control (D8).** A fixture module under `hack/testdata/mutation-control/` (own `go.mod`; never
  in `internal/core`, which would distort coverage and trip `purity_test.go`) whose test
  **executes** the function but **asserts nothing**. (Gremlins reports *untested* code as NOT
  COVERED, never LIVED, so an untested control proves nothing.) The control mutant **must report
  LIVED**; if it is killed or NOT COVERED the report is invalid and the job is red.
- The tool works on a copy and never edits tracked files; no tooling enters the decision path.

Requirements:

- **REQ-TDS-S02-01** — report parser yields score + lived list from a fixture report. Test:
  `hack/ci/mutation_report_test.sh` (create); Verify:
  `bash hack/ci/mutation_report_test.sh`; Level: L0
- **REQ-TDS-S02-02** *(control)* — control LIVED required; killed or NOT COVERED is red. Test:
  same; Verify: `bash hack/ci/mutation_report_test.sh --self-test`; Level: L0
- **REQ-TDS-S02-03** *(forbidden outcome)* — no threshold flag/exit-on-score in the workflow.
  Test: `hack/lint/nightly_wiring_test.sh`; Verify:
  `bash hack/lint/nightly_wiring_test.sh`; Level: L0

---

## TDS-S03 — Benchmarks for the decision path, `workflow_dispatch` only `[autonomous]`

- Given `internal/core/decision` and `aggregate`, then at least one benchmark per package exists
  on realistic existing fixtures, in `_test.go` only.
- A manual `workflow_dispatch` workflow runs `-bench=. -benchmem -count=10` on base and head and
  appends `benchstat` output to the job summary. No PR job, no schedule, no threshold; hence no
  wiring guard script.

Requirements:

- **REQ-TDS-S03-01** — benchmarks exist and run. Test:
  `internal/core/decision/decision_bench_test.go`, `internal/core/aggregate/aggregate_bench_test.go`
  (create); Verify: `go test -run=^$ -bench=. -benchtime=1x ./internal/core/...`; Level: L0

---

## Exit

S01 after SEC-SC-S01; S02 after NIT-S02; S03 anytime. New scripts follow the
[gate-wiring rule](../README.md): pinned `verify` steps and `task check` stages with
`STAGE_BODY_PINS` entries for every mode.
