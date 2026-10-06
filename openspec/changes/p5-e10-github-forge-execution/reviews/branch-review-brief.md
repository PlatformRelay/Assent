# Review brief — E10 GitHub forge execution (final branch review)

You are one of FOUR independent reviewers. You have NOT seen the author's context; judge
only the artifacts. READ-ONLY — do not edit any file.

## Subject

The branch `fm/assent-github-support` (vs base `main`, i.e. `git diff origin/main...HEAD`).
It implements the E10 GitHub forge adapter epic, change
`openspec/changes/p5-e10-github-forge-execution/` (read proposal.md + tasks.md + delta spec).

Commits to review: `git log origin/main..HEAD --oneline` (the seam S02/S03, capability model
S04 + D-186, transport cases S05, GitHub adapter S06-S08, conformance factory + fail-closed
deltas S10-S12, forge selection S13, action + docs + exit gate S15/S16/S17). The diff is
large; prioritise the trust boundaries and the fail-closed axes over style.

## Target questions (the lens per your leg is in your assignment)

1. **Compliance (spec lens)**: does the implementation satisfy the epic spec's REQs
   (openspec/specs/p5-e10-github-forge/spec.md S02–S17) and the S00 addressing model
   (docs/planning/github-addressing-model.md)? Which REQs hold, which are partial,
   contradicted, or absent?
2. **Adversarial**: find where the fail-closed guarantees have holes — anywhere arming could
   happen on an unproven capability, a policy load could reach MR-relative content, a
   forbidden read could render as absent, a merge could happen on unevaluated bytes, a
   token could leak into an error/log, or a conformance case could pass vacuously
   (unfailable assertions, positive controls missing).
3. **Security**: the trust boundaries the branch touches — target-ref policy loads (six
   sites), the resource-owner registry (D-130), the marker filter (author identity, not
   "any bot"), the content-scope probe's soundness, GraphQL injection surfaces, the App
   JWT mint, secrets hygiene (REQ-E10-S06-03), the depguard boundary.
4. **Fitness**: which architectural characteristic did this branch move (the forge port
   became forge-neutral; the capability model is a new port surface); is it guarded by a
   function that would have failed had it got it wrong; what is the smallest function worth
   adding as a ratchet?

## Output format (return exactly this)

For each finding: `SEVERITY: CRITICAL|MAJOR|MINOR | file:anchor | finding | recommended fix`.
Then a one-line verdict: `VERDICT: APPROVE` or `VERDICT: REQUEST_CHANGES` or `VERDICT: BLOCK`.
Cite file paths and anchors you actually read. If you verified something is fine, do not
list it. Maximum 12 findings, most serious first.
