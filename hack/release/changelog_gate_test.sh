#!/usr/bin/env bash
# REQ-AUD-S02-01/02 — the CHANGELOG drift gate is REGENERATED, WIRED, and FIRES.
#
# `hack/release/verify-changelog.sh` has existed since E9-S03 and was invoked by
# nothing: no `task` target listed it, no workflow ran it. A gate nobody calls is
# a comment, which is how `CHANGELOG.md` came to have no `[0.1.0]` section at all
# after the v0.1.0 tag. This script pins the three things that make it a gate:
#
#   1. content   — CHANGELOG.md carries the released `## [0.1.0] - 2026-08-05`
#                  section (REQ-AUD-S02-01).
#   2. wiring    — `task check` runs `changelog-verify`, and the `verify:` job in
#                  verify.yaml runs it too (REQ-AUD-S02-02). The same wiring
#                  assertions cover the three OTHER gates that shipped unwired and
#                  are wired by this lane: `docs-gates` (D-124) and
#                  `lint-depguard-test` (AUD-S07 / D-123).
#   3. polarity  — a stale CHANGELOG.md makes `verify-changelog.sh` exit non-zero.
#
# Anti-vacuity discipline (this epic shipped several gates that could not fail):
#   * every "is it wired" assertion is a FUNCTION over a file path, and each one is
#     re-run against a temp copy with the wired line DELETED — if it does not go red
#     there, the assertion is not testing what it claims and this script fails.
#   * every extraction (task block, workflow job block, workflow step block) is
#     positive-controlled: non-empty AND containing a known-present line AND not
#     containing a line from the NEXT block, so a broken awk range fails loudly
#     instead of silently asserting nothing over an empty string.
#   * no `grep -q` on the read end of a pipe (SIGPIPE 141 under `pipefail`);
#     extractions go to files first.
#   * the drift probe is verified to have LANDED in the file before its red is
#     believed — `verify-changelog.sh` resolves its own repo root from
#     ${BASH_SOURCE}, so a "temp copy of CHANGELOG.md" probe would never reach it.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

TASKFILE="$ROOT/Taskfile.yml"
WORKFLOW="$ROOT/.github/workflows/verify.yaml"

WORK="$(mktemp -d)"
CHANGELOG_BACKUP="$WORK/CHANGELOG.md.orig"
cp "$ROOT/CHANGELOG.md" "$CHANGELOG_BACKUP"
# Restore unconditionally: the drift probe mutates the REAL CHANGELOG.md (see above).
trap 'cp -f "$CHANGELOG_BACKUP" "$ROOT/CHANGELOG.md"; rm -rf "$WORK"' EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

# ---------------------------------------------------------------- extraction --

# extract_block <file> <indent-2 key> — the body of a top-level 2-space-indented
# mapping key (a Taskfile task, or a workflow job), up to the next such key.
# Task names may contain a colon ("tools:git-cliff"), hence the exact-string match.
extract_block() {
  awk -v name="$2" '
    $0 == "  " name ":" { inblk = 1; next }
    inblk && /^  [A-Za-z0-9_.:-]+:[[:space:]]*$/ { inblk = 0 }
    inblk { print }
  ' "$1"
}

# extract_step <file> <job> <regex> — one workflow step (6-space "- " item) whose
# first line matches <regex>, up to the start of the next step.
extract_step() {
  extract_block "$1" "$2" >"$WORK/job.for-step"
  awk -v pat="$3" '
    /^      - / { instep = ($0 ~ pat) }
    instep { print }
  ' "$WORK/job.for-step"
}

# ------------------------------------------------- assertions (file -> status) --

# check_lists_task <taskfile> <task-name> — is <task-name> a sequential command of
# the `check` task? Not "does the task exist" and not "does the name appear in the
# file": deleting the line from `check:` while leaving the task defined must fail.
check_lists_task() {
  extract_block "$1" check >"$WORK/check.block"
  grep -qE "^[[:space:]]+- task: $2\$" "$WORK/check.block"
}

# verify_job_runs_changelog_gate <workflow> — does verify.yaml's `verify:` job run
# `task changelog-verify`?
verify_job_runs_changelog_gate() {
  extract_block "$1" verify >"$WORK/verify.block"
  grep -qE 'task changelog-verify' "$WORK/verify.block"
}

# ------------------------------------------------------- 0. positive controls --

echo "== 0. extraction positive controls =="

extract_block "$TASKFILE" check >"$WORK/check.control"
[[ -s "$WORK/check.control" ]] || fail "Taskfile check: block extracted EMPTY — the awk range is broken, every wiring assertion below would be vacuous"
grep -qE '^[[:space:]]+- task: fmt$' "$WORK/check.control" \
  || fail "Taskfile check: block does not contain the known-present '- task: fmt' — extraction is wrong"
grep -qE '^[[:space:]]+- task: compare-exitgate-test$' "$WORK/check.control" \
  || fail "Taskfile check: block does not contain the known-present '- task: compare-exitgate-test' (D-118 precedent) — extraction is wrong"
if grep -q 'compare-exitgate-test:' "$WORK/check.control"; then
  fail "Taskfile check: block ran past the end of check into the next task definition"
fi
echo "OK: check: block extracted ($(wc -l <"$WORK/check.control" | tr -d ' ') lines), anchored on fmt + compare-exitgate-test, bounded"

extract_block "$WORKFLOW" verify >"$WORK/verify.control"
[[ -s "$WORK/verify.control" ]] || fail "verify.yaml verify: job extracted EMPTY — the awk range is broken"
grep -q 'hack/compare/exitgate_test.sh' "$WORK/verify.control" \
  || fail "verify.yaml verify: job does not contain the known-present compare exit-gate step — extraction is wrong"
if grep -q 'hack/release/exitgate_test.sh' "$WORK/verify.control"; then
  fail "verify.yaml verify: job ran past its end into release-exitgate:"
fi
echo "OK: verify: job extracted ($(wc -l <"$WORK/verify.control" | tr -d ' ') lines), anchored on the compare exit gate, bounded before release-exitgate"

# --------------------------------------------- 1. REQ-AUD-S02-01 (content) --

echo "== 1. REQ-AUD-S02-01: CHANGELOG.md carries the released v0.1.0 section =="
grep -qE '^## \[0\.1\.0\] - 2026-08-05$' "$ROOT/CHANGELOG.md" \
  || fail "CHANGELOG.md has no '## [0.1.0] - 2026-08-05' section — run 'task changelog-write' and commit (REQ-AUD-S02-01)"
grep -qE '^## Unreleased$' "$ROOT/CHANGELOG.md" \
  || fail "CHANGELOG.md has no '## Unreleased' section (REQ-AUD-S02-01)"
# Post-tag commits must sit under Unreleased, i.e. ABOVE the released section.
unrel_line="$(grep -nE '^## Unreleased$' "$ROOT/CHANGELOG.md" | head -1 | cut -d: -f1)"
rel_line="$(grep -nE '^## \[0\.1\.0\] - 2026-08-05$' "$ROOT/CHANGELOG.md" | head -1 | cut -d: -f1)"
[[ "$unrel_line" -lt "$rel_line" ]] \
  || fail "CHANGELOG.md orders [0.1.0] before Unreleased (line $rel_line vs $unrel_line)"
[[ $((rel_line - unrel_line)) -gt 2 ]] \
  || fail "CHANGELOG.md Unreleased section is empty — the post-tag commits are missing (REQ-AUD-S02-01)"
echo "OK: [0.1.0] at line $rel_line, non-empty Unreleased above it at line $unrel_line"

# The D-120 record-consumer warning is generated from cliff.toml's header, so a
# hand-edit of CHANGELOG.md cannot carry it and `changelog-write` cannot wipe it.
grep -q 'pins.toolDigest' "$ROOT/CHANGELOG.md" \
  || fail "CHANGELOG.md carries no pins.toolDigest compatibility note — AUD-S04 changed the value of a published record field with no warning to record consumers (D-120)"
grep -q 'pins.toolDigest' "$ROOT/cliff.toml" \
  || fail "the pins.toolDigest note is in CHANGELOG.md but not in cliff.toml — the next 'task changelog-write' will wipe it"
echo "OK: D-120 toolDigest note present in CHANGELOG.md and sourced from cliff.toml"

# --------------------------------------------- 2. REQ-AUD-S02-02 (wiring) --

echo "== 2. REQ-AUD-S02-02: gates wired into 'task check' =="
# changelog-verify is this story's gate; the other two are the gates that AUD-S06
# (D-124) and AUD-S07 shipped green but unwired — this lane owns Taskfile.yml.
# AUD-S09/S14 adds lint-workflow-pins-test here rather than letting
# workflow_pins_test.sh assert its own wiring: a gate that checks whether
# `task check` invokes it is unreachable precisely when the answer is "no".
# This file is reached through a DIFFERENT check: entry, so it is the only
# non-self-referential proof available.
WIRED_TASKS=(changelog-verify docs-gates lint-depguard-test lint-workflow-pins-test)
for t in "${WIRED_TASKS[@]}"; do
  check_lists_task "$TASKFILE" "$t" \
    || fail "'task check' does not run '$t' — the gate is defined but invoked by nothing"
  extract_block "$TASKFILE" "$t" >"$WORK/def.$t"
  [[ -s "$WORK/def.$t" ]] || fail "'$t' is listed in check: but not defined in Taskfile.yml"
  echo "OK: check runs $t (and $t is defined)"
done

# The task bodies must still invoke the scripts they exist for — a wired task with
# a gutted body is the same defect one level down.
grep -q 'hack/release/verify-changelog.sh' "$WORK/def.changelog-verify" \
  || fail "the changelog-verify task no longer runs hack/release/verify-changelog.sh"
grep -q 'hack/docs/readme_smoke_test.sh' "$WORK/def.docs-gates" \
  || fail "the docs-gates task no longer runs hack/docs/readme_smoke_test.sh (D-124)"
grep -q 'hack/docs/truthlag_pins_test.sh' "$WORK/def.docs-gates" \
  || fail "the docs-gates task no longer runs hack/docs/truthlag_pins_test.sh (D-124)"
grep -q 'hack/lint/depguard_test.sh' "$WORK/def.lint-depguard-test" \
  || fail "the lint-depguard-test task no longer runs hack/lint/depguard_test.sh (D-123/AUD-S07)"
echo "OK: each wired task still invokes its script"

echo "== 2b. the wiring assertion itself can fail (mutation) =="
for t in "${WIRED_TASKS[@]}"; do
  mutant="$WORK/Taskfile.no-$t.yml"
  grep -vE "^[[:space:]]+- task: $t\$" "$TASKFILE" >"$mutant"
  # Prove the mutation landed before believing anything about its result.
  if grep -qE "^[[:space:]]+- task: $t\$" "$mutant"; then
    fail "mutation did not land: '- task: $t' is still in $mutant"
  fi
  if [[ "$(wc -l <"$mutant")" -eq "$(wc -l <"$TASKFILE")" ]]; then
    fail "mutation did not land: $mutant has the same line count as Taskfile.yml"
  fi
  if check_lists_task "$mutant" "$t"; then
    fail "check_lists_task reports '$t' wired in a Taskfile with that line deleted — the assertion is vacuous"
  fi
  echo "OK: deleting '- task: $t' from check: turns the assertion red"
done

echo "== 3. REQ-AUD-S02-02: verify.yaml's verify: job runs the changelog gate =="
verify_job_runs_changelog_gate "$WORKFLOW" \
  || fail "the verify: job in verify.yaml does not run 'task changelog-verify' (REQ-AUD-S02-02)"
grep -q 'go-task/task/v3/cmd/task@' "$WORK/verify.control" \
  || fail "the verify: job runs 'task changelog-verify' but never installs Task"

extract_step "$WORKFLOW" verify 'changelog' >"$WORK/step.changelog"
[[ -s "$WORK/step.changelog" ]] || fail "could not extract the changelog gate step from the verify: job"
grep -q 'task changelog-verify' "$WORK/step.changelog" \
  || fail "the extracted step does not run task changelog-verify — extraction matched the wrong step"
# D-125: the step is deliberately NOT run on pull_request. On that event
# actions/checkout builds refs/pull/N/merge, which also carries every commit landed
# on main since the branch forked — the changelog generated there is a union that no
# CHANGELOG.md committed on the branch can match. D-136 removed the synthetic merge
# SUBJECT from that render but not the extra commits, so the guard still stands:
# without it every PR behind main is red by construction and unfixable by the author.
grep -qF "github.event_name != 'pull_request'" "$WORK/step.changelog" \
  || fail "the changelog gate step has no 'github.event_name != '\''pull_request'\''' guard — the PR merge ref carries commits landed on main since the branch forked, so the generated changelog is a union no committed CHANGELOG.md can match and the step would be red on every PR behind main (D-125, premise restated by D-136)"
echo "OK: verify: job runs task changelog-verify, guarded off pull_request (D-125)"

echo "== 3b. the workflow assertion itself can fail (mutation) =="
mutant="$WORK/verify.no-changelog.yaml"
grep -v 'task changelog-verify' "$WORKFLOW" >"$mutant"
if grep -q 'task changelog-verify' "$mutant"; then
  fail "mutation did not land: $mutant still runs task changelog-verify"
fi
[[ "$(wc -l <"$mutant")" -lt "$(wc -l <"$WORKFLOW")" ]] || fail "mutation did not land: $mutant has the same line count as verify.yaml"
if verify_job_runs_changelog_gate "$mutant"; then
  fail "verify_job_runs_changelog_gate reports the gate present in a workflow with that line deleted — the assertion is vacuous"
fi
echo "OK: deleting the run line from verify.yaml turns the assertion red"

# ------------------------------------------------ 4. drift polarity (in-tree) --

echo "== 4. verify-changelog.sh passes on the committed tree =="
bash "$ROOT/hack/release/verify-changelog.sh" >"$WORK/clean.out" 2>&1 || {
  cat "$WORK/clean.out" >&2
  fail "verify-changelog.sh is red on the committed tree — run 'task changelog-write' and commit"
}
echo "OK: clean polarity green"

echo "== 5. verify-changelog.sh fails closed on a stale CHANGELOG.md =="
printf '\n<!-- AUD-S02 drift probe -->\n' >>"$ROOT/CHANGELOG.md"
grep -q 'AUD-S02 drift probe' "$ROOT/CHANGELOG.md" \
  || fail "drift probe did not land in CHANGELOG.md — the red below would prove nothing"
if bash "$ROOT/hack/release/verify-changelog.sh" >"$WORK/drift.out" 2>&1; then
  cat "$WORK/drift.out" >&2
  fail "verify-changelog.sh exited 0 on a stale CHANGELOG.md — the drift gate does not fire (REQ-AUD-S02-02)"
fi
grep -q 'drift' "$WORK/drift.out" \
  || fail "verify-changelog.sh failed on the stale CHANGELOG.md but did not report drift — it failed for some other reason"
cp -f "$CHANGELOG_BACKUP" "$ROOT/CHANGELOG.md"
if grep -q 'AUD-S02 drift probe' "$ROOT/CHANGELOG.md"; then
  fail "restore failed: the drift probe is still in CHANGELOG.md"
fi
echo "OK: stale CHANGELOG.md is caught, probe restored"

# ------------------------------ 6. the GitHub Release body carries the notes --
#
# Sections 1-5 prove the D-120 compatibility note reaches CHANGELOG.md. It did —
# and still never reached the GitHub Release body, because release.yaml ran
# git-cliff with `--strip header` and the notes live in the changelog HEADER.
# Consumers reading only the Release page got no warning that `pins.toolDigest`
# changed derivation. Flagged as a pre-tag blocker in the session INBOX.
#
# This is a BEHAVIOURAL check, not a text assertion about release.yaml: the
# git-cliff arguments are extracted FROM the workflow and the release body is
# actually rendered with them, so the gate cannot drift away from what CI runs.

echo "== 6. the rendered GitHub Release body carries the compatibility notes =="

RELEASE_WF="$ROOT/.github/workflows/release.yaml"
[[ -f "$RELEASE_WF" ]] || fail "missing $RELEASE_WF"

CLIFF="${GIT_CLIFF_BIN:-$ROOT/bin/git-cliff}"
if [[ ! -x "$CLIFF" ]]; then
  bash "$ROOT/hack/install-git-cliff.sh" v2.13.1 bin/git-cliff >&2
  CLIFF="$ROOT/bin/git-cliff"
fi

# The `args:` of the git-cliff step, scoped to that step: `args:` also appears in
# both goreleaser steps, so a file-wide grep would pick the wrong one.
cliff_args="$(awk '
  /^      - / { inblock = 0 }
  /orhun\/git-cliff-action@/ { inblock = 1 }
  inblock && /^          args: / { sub(/^          args: /, ""); print; exit }
' "$RELEASE_WF")"
[[ -n "$cliff_args" ]] \
  || fail "could not extract the git-cliff step's args: from release.yaml — the extraction broke, so rendering below would prove nothing"
echo "OK: release.yaml renders the body with: git-cliff $cliff_args"

read -r -a cliff_argv <<<"$cliff_args"
"$CLIFF" --config "$ROOT/cliff.toml" "${cliff_argv[@]}" >"$WORK/release-body.md" 2>"$WORK/release-body.err" || {
  cat "$WORK/release-body.err" >&2
  fail "git-cliff failed with release.yaml's own arguments ($cliff_args)"
}

# Positive control on the render: an empty or headingless body would make the
# note grep below meaningless in the wrong direction (absent == "clean").
[[ -s "$WORK/release-body.md" ]] \
  || fail "release body rendered EMPTY with release.yaml's arguments"
grep -qE '^## \[[0-9]+\.[0-9]+\.[0-9]+\]' "$WORK/release-body.md" \
  || fail "rendered release body carries no '## [X.Y.Z]' release section — the render did not produce real release notes"

grep -q 'pins.toolDigest' "$WORK/release-body.md" \
  || fail "the rendered GitHub Release body carries no pins.toolDigest compatibility note — the D-120 warning reaches CHANGELOG.md but NOT the Release page (drop '--strip header' from the git-cliff step in release.yaml)"
echo "OK: rendered release body carries the D-120 compatibility note"

# The Release body is the artifact section 7 exists to protect: `--latest` with no
# `--strip header` publishes whatever git-cliff renders straight onto the Release
# page, where it is read by people who never open CHANGELOG.md. Assert it here, in
# the section that renders with release.yaml's OWN arguments, so the proof is about
# the published artifact and not about a parallel invocation this script invented.
if grep -nE '^- Merge ' "$WORK/release-body.md" >"$WORK/release-body.merges"; then
  cat "$WORK/release-body.merges" >&2
  fail "the rendered GitHub Release body contains merge-commit subjects — they would be published verbatim on the Release page (see section 7)"
fi
echo "OK: rendered release body carries no merge-commit subject"

# Polarity control: re-render WITH the bug. If the note survives `--strip
# header` too, then the grep above passes for some unrelated reason and this
# section is not testing what it claims.
"$CLIFF" --config "$ROOT/cliff.toml" "${cliff_argv[@]}" --strip header \
  >"$WORK/release-body-stripped.md" 2>/dev/null || true
[[ -s "$WORK/release-body-stripped.md" ]] \
  || fail "polarity control rendered empty — cannot conclude anything from its missing note"
if grep -q 'pins.toolDigest' "$WORK/release-body-stripped.md"; then
  fail "polarity control: the note is present even WITH '--strip header', so section 6 does not actually detect the regression it exists to catch"
fi
echo "OK: polarity control — re-adding '--strip header' removes the note again"

# ---------------------- 7. merge commits never reach the changelog (D-136) --
#
# `cliff.toml` sets `conventional_commits = false` + `filter_unconventional =
# false` and ends its parser list with a catch-all `{ message = ".*", group =
# "Other" }` — load-bearing for D-125's drift gate, so it must NOT be removed.
# The consequence is that any subject not explicitly skipped renders, merge
# commits included: three `Merge remote-tracking branch …` lines sat in the
# Unreleased section and three more inside `[0.1.0]`. The only defence was the
# D-125 working rule ("prefix every merge subject with a cliff-skipped form"),
# which depends on every integrator remembering and had already failed twice.
#
# D-136's structural fix is one parser entry keyed on the COMMIT SHAPE rather
# than on its text — `{ field = "merge_commit", pattern = "true", skip = true }`
# — so it holds whatever subject the integrator types and whatever synthetic
# merge CI mints for `refs/pull/N/merge`.
#
# Both polarities, twice over:
#   7a real history — the permanent `Merge …` ancestors of v0.1.0 are a durable
#      anchor. Clean render: no `- Merge …` line. Mutant render (parser entry
#      deleted, mutation proven to have landed): the lines come back, AND the
#      two renders differ by NOTHING ELSE, so the rule cannot be quietly
#      swallowing ordinary commits.
#   7b a sandbox repo — proves the rule is structural, not text-shaped: git's
#      default `Merge branch 'x'` subject, the CI `Merge <sha> into <sha>`
#      shape, and a merge whose author wrote a perfectly conventional gitmoji
#      subject are ALL skipped, while ordinary commits in the same repo render.
#      That third case is D-136's accepted cost, pinned here so it is a decision
#      and not a surprise.

echo "== 7. merge commits never render into the changelog (D-136) =="

MERGE_SKIP_RULE='field = "merge_commit"'

"$CLIFF" --config "$ROOT/cliff.toml" -o "$WORK/clean-full.md" 2>"$WORK/clean-full.err" || {
  cat "$WORK/clean-full.err" >&2
  fail "git-cliff failed rendering the full changelog from cliff.toml"
}
[[ -s "$WORK/clean-full.md" ]] || fail "full changelog rendered EMPTY — every assertion below would be vacuous"
grep -qE '^## \[0\.1\.0\] - 2026-08-05$' "$WORK/clean-full.md" \
  || fail "full render carries no '## [0.1.0]' section — this is not real changelog output"

if grep -nE '^- Merge ' "$WORK/clean-full.md" >"$WORK/clean-full.merges"; then
  cat "$WORK/clean-full.merges" >&2
  fail "the generated changelog renders merge-commit subjects — add '{ field = \"merge_commit\", pattern = \"true\", skip = true }' to cliff.toml's commit_parsers (D-136); do NOT remove the '.*' catch-all, it is load-bearing for D-125"
fi
echo "OK: no merge subject in the generated changelog"

grep -qF "$MERGE_SKIP_RULE" "$ROOT/cliff.toml" \
  || fail "cliff.toml has no '$MERGE_SKIP_RULE' commit parser — merge subjects are only absent because none happens to be in range, which is exactly the recurrence D-136 closes"

echo "== 7a. the assertion can fail (mutation over real history) =="
mutant_cfg="$WORK/cliff.no-merge-skip.toml"
grep -vF "$MERGE_SKIP_RULE" "$ROOT/cliff.toml" >"$mutant_cfg"
if grep -qF "$MERGE_SKIP_RULE" "$mutant_cfg"; then
  fail "mutation did not land: '$MERGE_SKIP_RULE' is still in $mutant_cfg"
fi
if [[ "$(wc -l <"$mutant_cfg")" -eq "$(wc -l <"$ROOT/cliff.toml")" ]]; then
  fail "mutation did not land: $mutant_cfg has the same line count as cliff.toml"
fi
"$CLIFF" --config "$mutant_cfg" -o "$WORK/mutant-full.md" 2>"$WORK/mutant-full.err" || {
  cat "$WORK/mutant-full.err" >&2
  fail "git-cliff failed rendering with the mutant config — the mutation broke the TOML instead of removing the rule"
}
mutant_merges="$(grep -cE '^- Merge ' "$WORK/mutant-full.md" || true)"
[[ "$mutant_merges" -ge 1 ]] \
  || fail "removing the merge-skip parser produced no merge subjects at all — history no longer contains the anchor commits, so the clean-render assertion above proves nothing"
echo "OK: deleting the merge-skip parser puts $mutant_merges merge subject(s) back"

# The rule must remove merge lines and NOTHING else. Every diff line is either a
# hunk header or a deleted `- Merge …` line; anything else means the parser is
# also swallowing ordinary commits.
diff "$WORK/mutant-full.md" "$WORK/clean-full.md" >"$WORK/render.diff" || true
[[ -s "$WORK/render.diff" ]] || fail "clean and mutant renders are identical — the merge-skip parser has no effect"
if grep -vE '^([0-9]+(,[0-9]+)?[acd][0-9]+(,[0-9]+)?|< - Merge |---)' "$WORK/render.diff" >"$WORK/render.diff.extra"; then
  cat "$WORK/render.diff.extra" >&2
  fail "the merge-skip parser changes the render beyond deleting merge subjects — it is over-skipping ordinary commits"
fi
echo "OK: the parser deletes merge subjects and changes nothing else"

echo "== 7b. the rule is structural: any merge commit, any subject =="
SANDBOX="$WORK/sandbox"
mkdir -p "$SANDBOX"
# Fully self-contained git invocations: a global commit.gpgsign, a hooksPath, a
# missing user identity or a non-`main` init default must not decide the result.
sgit() {
  git -C "$SANDBOX" \
    -c user.name='changelog gate' -c user.email='gate@example.invalid' \
    -c commit.gpgsign=false -c core.hooksPath=/dev/null \
    -c init.defaultBranch=main -c advice.detachedHead=false "$@"
}
git init -q "$SANDBOX" >/dev/null 2>&1 || fail "could not git init the sandbox repo"
sgit symbolic-ref HEAD refs/heads/main
printf 'a\n' >"$SANDBOX/a.txt"; sgit add -A; sgit commit -q -m ':sparkles: feat(sandbox): ordinary feature subject still renders'
sgit checkout -q -b side
printf 'b\n' >"$SANDBOX/b.txt"; sgit add -A; sgit commit -q -m ':bug: fix(sandbox): ordinary fix subject on a side branch'
sgit checkout -q main
printf 'c\n' >"$SANDBOX/c.txt"; sgit add -A; sgit commit -q -m ':memo: docs(sandbox): ordinary docs subject on main'
# (i) git's own default merge subject — the shape that reached CHANGELOG.md twice.
sgit merge -q --no-ff side -m "Merge branch 'side'"
# (ii) the refs/pull/N/merge shape actions/checkout mints at CI time (D-125).
sgit checkout -q -b side2
printf 'd\n' >"$SANDBOX/d.txt"; sgit add -A; sgit commit -q -m ':wrench: chore(sandbox): ordinary chore subject on a second branch'
sgit checkout -q main
sha_head="$(sgit rev-parse --short HEAD)"; sha_side="$(sgit rev-parse --short side2)"
sgit merge -q --no-ff side2 -m "Merge ${sha_side} into ${sha_head}"
# (iii) a merge whose author wrote a real, conventional subject: skipped too.
sgit checkout -q -b side3
printf 'e\n' >"$SANDBOX/e.txt"; sgit add -A; sgit commit -q -m ':sparkles: feat(sandbox): ordinary feature subject on a third branch'
sgit checkout -q main
sgit merge -q --no-ff side3 -m ':sparkles: feat(sandbox): integrator wrote a real subject on a merge commit'

"$CLIFF" --config "$ROOT/cliff.toml" --repository "$SANDBOX" -o "$WORK/sandbox.md" 2>"$WORK/sandbox.err" || {
  cat "$WORK/sandbox.err" >&2
  fail "git-cliff failed rendering the sandbox repository"
}
[[ -s "$WORK/sandbox.md" ]] || fail "sandbox render is EMPTY"

# Positive control FIRST: the ordinary commits must be there, otherwise "no merge
# line" would pass for an empty section.
SANDBOX_ORDINARY=(
  '- :sparkles: feat(sandbox): ordinary feature subject still renders'
  '- :bug: fix(sandbox): ordinary fix subject on a side branch'
  '- :memo: docs(sandbox): ordinary docs subject on main'
  '- :wrench: chore(sandbox): ordinary chore subject on a second branch'
  '- :sparkles: feat(sandbox): ordinary feature subject on a third branch'
)
for line in "${SANDBOX_ORDINARY[@]}"; do
  grep -qxF -e "$line" "$WORK/sandbox.md" \
    || fail "sandbox render is missing the ordinary commit '$line' — the merge-skip parser is eating non-merge commits"
done
echo "OK: all ${#SANDBOX_ORDINARY[@]} ordinary sandbox commits render"

SANDBOX_MERGES=(
  "- Merge branch 'side'"
  "- Merge ${sha_side} into ${sha_head}"
  '- :sparkles: feat(sandbox): integrator wrote a real subject on a merge commit'
)
for line in "${SANDBOX_MERGES[@]}"; do
  if grep -qxF -e "$line" "$WORK/sandbox.md"; then
    fail "sandbox render contains the merge subject '$line' — the merge-skip parser is not structural"
  fi
done
echo "OK: none of the ${#SANDBOX_MERGES[@]} merge subjects render (default, CI merge-ref, and hand-written shapes)"

# Sandbox polarity control: without the parser all three come back, so the three
# negatives above are about the rule and not about a render that lost the commits.
"$CLIFF" --config "$mutant_cfg" --repository "$SANDBOX" -o "$WORK/sandbox-mutant.md" 2>/dev/null \
  || fail "git-cliff failed rendering the sandbox with the mutant config"
for line in "${SANDBOX_MERGES[@]}"; do
  grep -qxF -e "$line" "$WORK/sandbox-mutant.md" \
    || fail "polarity control: '$line' is absent even WITHOUT the merge-skip parser — section 7b does not detect what it claims"
done
echo "OK: polarity control — removing the parser brings all three merge subjects back"

echo "PASS: changelog drift gate regenerated, wired into task check + verify.yaml, and proven at both polarities (REQ-AUD-S02-01/02); release body carries the compatibility notes and no merge subject (D-136)"
