// Package conformance holds L2 conformance goldens for the forge port (ADR-0005).
// Cases exercise forge.Reconcile through the hermetic fake and gitlab httptest,
// proving executable contract for:
//
//   - ADR-0015 §2 SHA-guard semantics (E4-S07): target/source movement after
//     evaluation fails closed with ErrSHAMoved.
//   - ADR-0019 P3-E5 publication-protocol replay (E4-S09): rerun idempotence,
//     crash-then-rerun gap-fill without duplication, deterministic duplicate
//     repair, and contributor marker spoofing ignored by the author-identity
//     filter on ListBotThreads.
package conformance
