-		// path consults the same report it renders).
+		// path consults the same report it renders). Its arming-path entries
+		// are not proven (the arming consult walks armingConsultSet in order),
+		// so the v1 report refuses at the FIRST unproven member —
+		// deferred-merge-arming, whose allow_auto_merge field is unverified
+		// (S00 Q2 row 6, REQ-E10-S09-02). The revocation delta keeps its own
+		// isolated refusal row below.
[0m
[0m$ [0mrm -rf "$TMPDIR/opencode/e10base"
(no output)
[0m
MAJOR | cmd/assent/forge_select.go:39-42 + forge_select_test.go:47-51 | `selectForge("", "")` returns `(KindGitLab, "https://gitlab.com", nil)` — the no-forge-named + no-endpoint shape is exactly "cannot be unambiguously detected" where REQ-E10-S13-01 (openspec/specs/p5-e10-github-forge/spec.md:688) demands an error, not the GitLab default; the named gate then cements it: the row called `autodetect_gitlab_com` passes an EMPTY endpoint (it never passes the gitlab.com host), and no row covers the real flag-default shape `--forge github --gitlab-endpoint https://gitlab.com` | error on empty kind+empty endpoint; fix the test row to endpoint `https://gitlab.com`; add the github+flag-default-endpoint row

MAJOR | internal/forge/gitlab/gitlab.go:497-498,607-616 | the round-1 pin fix added a `pinMu` to the GitHub client "because a client may be shared across goroutines… the GitLab mirror keeps the same discipline" (github.go comment) — it does not: GetMR and mrPinned write/read `mrPinnedProject`/`mrPinnedInfo` with no lock on the very struct that documents itself goroutine-shareable (warnMu comment, gitlab.go:64); unexercised by today's sequential tests, false claim in the fix's own comment | add `pinMu` to `gitlab.Client` mirroring `github.Client`, or correct the claim

MAJOR | internal/forge/conformance/backends_test.go:217 | `task check`'s test task is `go test -race ./...` (Taskfile.yml:30-32) and it is RED: `TestConformanceDeadlineBounded/gitlab` deterministically fails with a data race on `h.mrReads++` (retried slow-MR read served by two handler goroutines). I reproduced it at HEAD and at `75089a2~1`, so it pre-exists the delta — but AGENTS rule 4 says `task check` green before every commit, and all three delta commits landed over it | guard the harness counters with a mutex/atomic and re-run `task check`; do not blame the delta, but do not land over a red gate either

MINOR | cmd/assent/run.go:395-407,423 | the round-1 comment-fix stacked the superseded "6. Build the DecisionRecord" block above the new one (same text twice), and the fallback gap reason literally hardcodes "gitlab plain-merge…" in cmd while the comment above it asserts "cmd never names a forge" (E10-S03) | delete the stale block; move the fallback text into the fake fixtures' capability entries instead of naming a forge in cmd

MINOR | cmd/assent/run.go:409 | the capability gate `Supported && mergeDigest != ""` silently degrades a SUPPORTED-but-empty-digest report to a GAP record — the contradiction between the adapter's capability entry and its digest becomes a mislabelled gap (and, for a supported entry whose reason is empty, the gitlab-hardcoded text); it should fail closed | return an error when `State==Supported && mergeDigest==""` instead of taking the gap branch

MINOR | internal/forge/github/snapshot.go changedFilesVerdict | the count cross-check is one-directional: `entries > reported` yields COMPLETE, and `reported==0` is treated as "unreported" — a forge reporting `changed_files:0` while serving entries, or pages enumerating past the pinned read's count, passes the cross-check that exists precisely to catch read-vs-enumeration mismatch (int decodes absent and zero alike) | decode `changed_files` as `*int`; grade `entries > reported` incomplete; add the missing rows to the pure-function table test

MINOR | internal/forge/github/write.go:488-494 | the Approve fix left a duplicated, dead second `if err != nil` after `json.Marshal` — exactly the dead-code class round 1 asked to remove (also: `detectForgeFromEndpoint`, forge_select.go:74, is now referenced nowhere, including tests) | delete the duplicate check and the dead function

MINOR | internal/forge/conformance/catalog.yaml ratelimit-403 row + cmd/assent/capability_gap_record_test.go:114 | the S00 Q4 row note carries duplicated fragments ("is a transport error is a transport error", "a direct entry test a direct entry test") and both now-executed sentinel rows keep `forge: github-deferred`; the sibling gate test was relaxed from prefix-matching the gap text to `gapA == ""`, so any non-empty string satisfies "still records merge result" | fix the prose, relabel the executed rows' forge, pin the gap assertion to the fixture's own adapter reason

VERDICT: REQUEST_CHANGES

Read: brief; `git log/diff 75089a2~1..HEAD` (all 32 files); forge_select.go + test, run.go, capability.go, snapshot.go(+test), write.go(+test), graphql.go, auth.go, github.go, gitlab.go(+limits test), catalog(.go/yaml/_test), sentinel_cases_test.go, reconciliation/suite_test.go, capability_gap_record_test.go, run_test.go, spec REQ-E10-S13-01, Taskfile.yml; ran `go build`, `go vet`, `go test -race ./internal/forge/... ./cmd/...`, and reproduced finding 3 at base in a temp tree. Not verified: golangci-lint/`task check` full run locally, CI logs, live-API behaviour of any probe.
