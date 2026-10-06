# Review brief — E10 branch-review ROUND 2 (delta review)

You are one of the round-2 reviewers. Round 1 (four legs, all REQUEST_CHANGES) produced
fixes; review the DELTA since the fixes plus the whole branch's remaining state. READ-ONLY.

## Delta to review

Commits `git log 75089a2~1..HEAD --oneline` (the round-1 fixes: one-read-chain pin,
forge-consistent endpoint resolution + TestForgeSelection, merge-result capability-gated pin,
honest capability grading, rate-limit-on-success, PKCS#8, Approve commit_id pin, ResolveThread
thread-id mapping, catalog integrity incl. TestEveryRowHasAdapterDisposition + the two S00 Q4
sentinel cases on both factories, changed_files cross-check, dead code/comments/nil-port).

## Round-1 findings already accepted-and-fixed (verify the fix, don't re-derive)

1. GetMR fills the pin cache (one read chain) — internal/forge/{github/snapshot.go,gitlab/gitlab.go}.
2. --forge github endpoint resolution forge-consistent; TestForgeSelection — cmd/assent/forge_select*.go.
3. Merge-result record capability-gated with the adapter-owned gap reason — cmd/assent/run.go.
4. deferred-merge-arming graded unknown; threads-block-merge reason truthful — capability.go.
5. Approve commit_id pin; ResolveThread thread-id mapping.
6. Catalog: duplicates gone, TestEveryRowHasAdapterDisposition, two sentinel cases on both
   factories, cited deferrals on the five trust-boundary rows.

## What to hunt in round 2

- Fix-quality: does each fix actually close its finding, or is it a patch that re-opens
  another axis (locking errors, TOCTOU shapes, vacuous tests)?
- Anything the fixes changed that no longer satisfies the epic spec REQs / S00 model.
- The five-delta fail-closed table's integrity after the capability re-grading.
- Anything CRITICAL you can name with an anchor.

## Output format

`SEVERITY | file:anchor | finding | recommended fix`, then `VERDICT: APPROVE |
REQUEST_CHANGES | BLOCK`. Maximum 8 findings, most serious first. Cite what you read.
