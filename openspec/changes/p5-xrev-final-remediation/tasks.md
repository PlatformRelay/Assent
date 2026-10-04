# P5-XREV — tasks

Vertical slices, dependency-ordered. Each task: add the failing test first, then implement;
`task check` green before the commit. One logical change per commit,
`:gitmoji: type(scope): summary`.

## T001 — S02: share the match predicate (D3)
1. Add `internal/adoptertest/match_parity_test.go`: drive files/values/valueChanges/fileEvents
   through the shared export against an independently-encoded per-domain expectation table,
   plus an engine-agreement test observing `aggregate.Cover`'s selection.
2. Export `aggregate.MatchesAny` wrapping `matchChanges`; delete `adoptertest.ruleMatchesAny`
   + `matchesAnyGlob` + `containsStr`; call the export.
3. Verify: `go test ./internal/adoptertest/... ./internal/core/aggregate/...`

## T002 — S04: hygiene one-liners (D16, D17, D18)
1. `schemas/fixtures_validate_test.go`: parameterise the walk root; absent/empty/no-doc →
   `t.Fatal`; add temp-tree cases.
2. `hack/spikes/provider/maliciousexec/main.go`: add `//go:build ignore`; add
   `hack/spikes/buildtag_test.go` parsing the constraint.
3. `cmd/assent/provider_host.go`: replace `testing/fstest` with a local `fs.FS` adapter;
   existing loader tests stay green.
4. Verify: `go test ./schemas/... ./hack/spikes/... ./cmd/assent/... && go build ./...`

## T003 — S03: doc-truth batch (D4, D5, D10-vision, D12, D13, D14)
1. Extend `hack/docs/truthlag_pins_test.sh`: `E10|E11`+`D-012` pairing detector with a
   temp-file positive control; README maturity rows vs meta-plan; `--config` help vs
   `cli.md`; vision kind claim; internal/README; c4 hash row; go.mod/rego provenance.
2. Fix `README.md` rows + `task check` comment; `cmd/assent/run.go` flag help;
   `docs/usage/cli.md`; `docs/vision.md`; `internal/README.md`;
   `docs/architecture/c4-container.md`; `go.mod`; `examples/policies/rego/bounded_change.rego`.
3. Verify: `bash hack/docs/truthlag_pins_test.sh`

## T004 — S01: guarded engine entry (D2, folds D6)
1. Add `internal/core/aggregate/decide_test.go` and
   `internal/compare/decide_vacuity_test.go` (failing: `compare` currently APPROVEs on empty
   `require`).
2. Add `Decide`/`DecideRequest`; route `run.decide`, `compare.evaluate`, and
   `adoptertest.Evaluate`'s decidable path; keep `adoptertest`'s bare-REVIEW undecidable path.
3. Delete `Aggregate`/`failSafe`/`newCELEnv`/`bindActivation`/`evalRule` and
   `Rule`/`Binding`/`OnFailure`; retarget the classify golden and the tokenless field scan.
4. Verify: `go build ./... && go test ./internal/core/... ./internal/adoptertest/... ./internal/compare/... ./cmd/assent/... && task dogfood-examples`

## Deferred (recorded, not implemented here)

- D1 (adoption path — needs DEM-S00 routing + run-path entry reconstruction); D7; D8; D9;
  D10 doctor half; D11; D15; D19.
