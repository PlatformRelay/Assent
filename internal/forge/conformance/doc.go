// Package conformance holds the forge-neutral L2 conformance suite (ADR-0005).
//
// The suite is IMPORTABLE (E10-S01): case bodies live in ordinary Go, not in
// `_test.go`, so a second adapter conformance-tests itself by supplying a
// Factory and calling RunSuite — no case is copied, so none can drift. Before
// this, duplication was the only option, which is why D-084's `github-deferred`
// rows were unflippable.
//
// Cases exercise forge.Reconcile through the hermetic fake and gitlab httptest,
// proving executable contract for:
//
//   - ADR-0015 §2 SHA-guard semantics (E4-S07): target/source movement after
//     evaluation fails closed with ErrSHAMoved.
//   - ADR-0019 P3-E5 publication-protocol replay (E4-S09): rerun idempotence,
//     crash-then-rerun gap-fill without duplication, deterministic duplicate
//     repair, and contributor marker spoofing ignored by the author-identity
//     filter on ListBotThreads.
//
// Three gates keep the suite honest, because a conformance suite that has
// stopped proving its property still reports PASS:
//
//   - TestCatalogMatchesExecutedCases — the catalog is checked against OBSERVED
//     EXECUTION, so a case unhooked from the runner cannot stay accounted for.
//   - TestEveryCaseCanFail — every case must go red against a sabotaged backend,
//     on every adapter.
//   - TestEveryObservationIsLoadBearing — corrupting any single value a case
//     reads must flip its verdict, so no observation is dead weight.
package conformance
