# Spec-set review register — round 1

Three independent legs on three models (fresh headless opencode sessions, read-only):
`GLM-5.3-Flash`, `DeepSeek-V4.1-Flash`, `Qwen3.8-Flash-Next` — reports at
`reviews/spec-leg-*.md`, committed. All three returned `VERDICT: FIX-FIRST`; the central
finding was unanimous.

## Triage table

| # | Leg(s) | Severity | Finding | Disposition |
| --- | --- | --- | --- | --- |
| 1 | all three | CRITICAL | The twelve S00-minted conformance case IDs are in no task (six in none at all); S01's DoD left them undischarged; T13 was satisfiable vacuously. | **accepted** — new task T0 mints all twelve as catalog rows and implements them per wave; T13's gate asserts the twelve present+executed; proposal records the S01 DoD residue. |
| 2 | all three | MAJOR | Delta's composite `(project, mr)` handle contradicts the epic spec's frozen two-argument REQ text while tasks call the epic REQs "the acceptance bar". | **accepted** — delta now states the amendment explicitly (S00 obligation 2b delegated the choice); the epic spec's REQ-E10-S02-01/-05 signature text corrected; ADR-0021 left unedited (superseded, not contradicted) per S00's own decision. |
| 3 | DeepSeek + Qwen | MAJOR | REQ-E10-S12-03 says "against a real repository" but S18 is fenced — how S12-03 closes autonomously was unstated. | **accepted** — T11 now states the autonomous closure (S00 Q2 table filled with hermetic reported values, OQ-33/34 cited open, live confirmation remains S18's); T14 carries the block-gate. |
| 4 | GLM + DeepSeek | MAJOR | Delta REQs (E10X) traced to no task; the compile-time no-project-argument proof unowned. | **accepted** — E10X REQs cited in T1; mechanical proofs in T1's deliverables. |
| 5 | GLM + DeepSeek | MINOR | Epic `Describe(project, mr)` and `provider_host.go:275` stale anchors (S00-corrected) unowned. | **accepted** — T1 corrects the epic spec anchors; delta records the correction. |
| 6 | GLM | MINOR | T4 (S05) omitted context deadlines (ADR-0021 item 4). | **accepted** — added with a deadline-bounded conformance case. |
| 7 | DeepSeek | MINOR | `ErrUnauthorized` lift had no delta REQ / named case. | **accepted** — REQ-E10X-01-03 added; sentinel cases bound at T1 (fake+GitLab), GitHub factory at T6. |
| 8 | DeepSeek | MINOR | S17's "bare github-deferred rows" undefined. | **accepted** — T13 defines bare = deferred row lacking a cited non-empty reason. |
| 9 | DeepSeek + Qwen | MINOR | REQ-E10-S14-01's "14 non-deferred rows" count is stale (now 16). | **accepted** — T13 uses "every non-deferred row" without a count; no spec-count edits (the epic's count is historical fact of its writing; T13's wording avoids inheriting it). |
| 10 | GLM | MINOR | T11 dropped REQ-E10-S12-03's test anchor + T14 gate. | **accepted** — T11 fills the S00 Q2 table (hermetic values, OQs cited open); T14 carries the block-gate. |
| 11 | GLM + DeepSeek | MINOR | proposal typo "S02–S01" → "S00–S01". | **accepted** — fixed. |

No CRITICAL survives triage — the single CRITICAL is fixed in the plan (T0). Verdict after
fixes: PROCEED into the task loop.
