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
#
# Anchor on the note's HEADER SENTENCE, never on the bare `pins.toolDigest`
# token. The token is NOT header-unique: two commit subjects inside the [0.2.0]
# section carry it verbatim ("warn record consumers that pins.toolDigest changed
# value", "derive pins.toolDigest from Go build info"), and cliff.toml names it
# again in the D-121 note. A bare-token grep therefore matches the rendered BODY
# and passes whether or not the header note survives at all — which is precisely
# the vacuity section 6's polarity control failed closed on. Keep this phrase in
# sync with cliff.toml's `[changelog] header`; do not simplify it back.
NOTE_ANCHOR='pins.toolDigest` changes value after'

grep -qF "$NOTE_ANCHOR" "$ROOT/CHANGELOG.md" \
  || fail "CHANGELOG.md carries no pins.toolDigest compatibility note — AUD-S04 changed the value of a published record field with no warning to record consumers (D-120)"
grep -qF "$NOTE_ANCHOR" "$ROOT/cliff.toml" \
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
# The step is deliberately NOT run on pull_request. D-125's reason (the merge ref's
# synthetic merge subject renders) died with D-136, and the successor reason drafted
# with D-136 was measured and is false — see OQ-30. This assertion therefore pins the
# guard's PRESENCE as the recorded state of the repository, not a mechanism: the guard
# is retained pending evidence and must not be dropped as dead-premise cleanup while
# OQ-30 is open. Enabling the PR placement is a deliberate change that closes OQ-30
# and updates this assertion with it.
grep -qF "github.event_name != 'pull_request'" "$WORK/step.changelog" \
  || fail "the changelog gate step has lost its 'github.event_name != '\''pull_request'\''' guard — the guard is retained pending OQ-30 (D-125's reason is dead, its successor measured false); removing it is a CI-gating change that must close OQ-30, not a side effect"
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

grep -qF "$NOTE_ANCHOR" "$WORK/release-body.md" \
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
if grep -qF "$NOTE_ANCHOR" "$WORK/release-body-stripped.md"; then
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

# The rule must remove merge subjects and NOTHING else. Stated over the MULTISET
# of rendered bullets, not as a line diff: a raw diff also shows structural churn
# that is a consequence, not a side effect — when a group's last member was a
# merge line the whole `### Other` heading disappears with it, and re-grouping
# (REL-14) moves bullets between sections. The honest claim is: no bullet appears
# that was not there before, and the only bullets that disappear are merge
# subjects.
grep -E '^- ' "$WORK/clean-full.md" | sort >"$WORK/clean.bullets"
grep -E '^- ' "$WORK/mutant-full.md" | sort >"$WORK/mutant.bullets"
[[ -s "$WORK/clean.bullets" ]] || fail "clean render has no bullets at all — the comparison below would be vacuous"
comm -13 "$WORK/clean.bullets" "$WORK/mutant.bullets" >"$WORK/only-in-mutant"
comm -23 "$WORK/clean.bullets" "$WORK/mutant.bullets" >"$WORK/only-in-clean"
[[ -s "$WORK/only-in-mutant" ]] || fail "clean and mutant renders carry the same bullets — the merge-skip parser has no effect"
if [[ -s "$WORK/only-in-clean" ]]; then
  cat "$WORK/only-in-clean" >&2
  fail "the merge-skip parser makes bullets APPEAR that the mutant render does not have — it cannot add content, so the comparison is wrong"
fi
# D-136's rule is keyed on commit SHAPE (`merge_commit`), never on subject text,
# so the honest assertion is "every bullet only the mutant renders belongs to a
# MERGE COMMIT" — not "every such bullet starts with `- Merge `". A merge whose
# author wrote a conventional subject is skipped correctly, and that is exactly
# the accepted cost 7b pins in the sandbox; real history now carries one
# (`41a3072 :twisted_rightwards_arrows: chore(aud-s18): merge AUD-S13 (PR #35)`,
# landed with PR #38), which a subject-prefix check misreads as an over-skip.
# Keying on `git log --merges` is also STRICTLY STRONGER in the other direction:
# an ordinary commit merely titled "Merge …" would have satisfied the old grep
# and now fails, because it is not a merge commit.
git -C "$ROOT" log --merges --format=%s | sort -u >"$WORK/merge-subjects"
[[ -s "$WORK/merge-subjects" ]] \
  || fail "git log --merges lists no merge commits — the membership check below would pass vacuously"
sed -e 's/^- //' "$WORK/only-in-mutant" | sort -u >"$WORK/only-in-mutant.subjects"
comm -23 "$WORK/only-in-mutant.subjects" "$WORK/merge-subjects" >"$WORK/only-in-mutant.extra"
if [[ -s "$WORK/only-in-mutant.extra" ]]; then
  cat "$WORK/only-in-mutant.extra" >&2
  fail "the merge-skip parser removes bullets that are not merge commits — it is over-skipping ordinary commits"
fi
echo "OK: multiset-identical apart from $(wc -l <"$WORK/only-in-mutant" | tr -d ' ') merge subject(s), which only the mutant renders"

echo "== 7b. the rule is structural: any merge commit, any subject =="
SANDBOX="$WORK/sandbox"
mkdir -p "$SANDBOX"
# Fully self-contained git invocations: a global commit.gpgsign, a hooksPath, a
# missing user identity or a non-`main` init default must not decide the result.
# The `env -u` prefix is not decoration: `-c` overrides CONFIG, but GIT_DIR /
# GIT_WORK_TREE / GIT_INDEX_FILE / GIT_OBJECT_DIRECTORY come from the ENVIRONMENT
# and beat `-C`. `task check` can be reached from a git hook or a CI wrapper that
# exports them, and there this sandbox would silently operate on the REAL
# repository — turning section 7b from a proof into a repo-corrupting no-op.
sgit() {
  env -u GIT_DIR -u GIT_WORK_TREE -u GIT_INDEX_FILE -u GIT_OBJECT_DIRECTORY \
    -u GIT_ALTERNATE_OBJECT_DIRECTORIES -u GIT_COMMON_DIR \
    git -C "$SANDBOX" \
    -c user.name='changelog gate' -c user.email='gate@example.invalid' \
    -c commit.gpgsign=false -c core.hooksPath=/dev/null \
    -c init.defaultBranch=main -c advice.detachedHead=false "$@"
}
env -u GIT_DIR -u GIT_WORK_TREE -u GIT_INDEX_FILE -u GIT_OBJECT_DIRECTORY \
  -u GIT_ALTERNATE_OBJECT_DIRECTORIES -u GIT_COMMON_DIR \
  git init -q "$SANDBOX" >/dev/null 2>&1 || fail "could not git init the sandbox repo"
# The sandbox must be its OWN repository, never the one under test.
[[ -d "$SANDBOX/.git" ]] || fail "sandbox has no .git — git init landed somewhere else (a GIT_DIR in the environment?)"
# Compare physical paths: on macOS $TMPDIR is under /var, a symlink to /private/var,
# and `rev-parse --show-toplevel` reports the resolved form.
sandbox_expected="$(cd "$SANDBOX" && pwd -P)"
sandbox_root="$(cd "$(sgit rev-parse --show-toplevel)" && pwd -P)"
[[ "$sandbox_root" == "$sandbox_expected" ]] \
  || fail "sandbox git commands resolve to '$sandbox_root', not '$sandbox_expected' — the environment is redirecting them at another repository"
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

# ------------- 8. gitmoji subjects reach their real group, not Other (REL-14) --
#
# `cliff.toml`'s original parser list matched eight gitmoji shortcodes and, as a
# fallback, conventional types at the START of the subject (`^fix`, `^ci`, ...).
# In this project the shortcode always comes FIRST, so those `^type` alternatives
# could never fire: every subject whose shortcode was outside the eight fell
# through the `.*` catch-all into "Other". The published v0.2.0 Release body
# therefore filed a real user-facing fix — `:ambulance: fix(forge): skip
# malformed bot markers ...` — under Other, alongside 18 `ci(...)` commits.
#
# D-137's fix maps on the CONVENTIONAL TYPE the author declared after the
# shortcode, not on the emoji's dictionary meaning, because the emoji is the
# unreliable half: `:lipstick: fix(provider): ...` is a fix, not a UI change, and
# `:art:` is used for both `style(...)` and `refactor(...)`. Two things stay in
# Other on purpose — `revert(...)`, which has no group to go to, and one
# malformed `:test(release):` subject that declares no parseable type.
#
# Polarities: the `:ambulance:` fix must render under Fixes, and stripping the
# new parser entries must put it back under Other. The second assertion is
# structural rather than a snapshot, so it keeps holding as history grows: NO
# line in Other may declare a type this repo knows how to file.
#
# REDMAIN-N2 / D-168 — THE DETECTOR WAS FAIL-OPEN FOR THE SHAPE THAT CAUSES THE
# BUG. Until this lane it read `^- :[a-z0-9_]+: ($FILEABLE_TYPES)[(:]`, which
# REQUIRES an ASCII shortcode. `dfdae69`'s entry
# `- 👷 ci(docs): stop uploading the Pages artifact on pull requests` declares the
# fileable type `ci` and sits in Other — precisely what this section forbids —
# but leads with a LITERAL EMOJI, so the pattern could not see it. Other went
# 2 -> 3 entries while the OK line below still printed its hardcoded prose
# "revert + one malformed subject". The prefix is now optional and admits three
# spellings (shortcode / literal emoji / nothing at all); §8b pins the
# regression by showing the OLD pattern miss the very probe the new one catches.
#
# Re-filing `dfdae69`'s rendered entry OUT of Other needs a `cliff.toml` parser
# entry, which this lane's fence does not include (tracked as REDMAIN-N3). It is
# therefore exempted here — by the SAME SHA list `commit_subject_gate.sh` uses,
# read through its `--legacy-subjects` mode so there is ONE authority and not two
# that can drift. The exemption is self-retiring: an exempt subject that no
# longer renders under Other reds this section and says to delete it.

echo "== 8. gitmoji subjects reach their real group, not Other (REL-14 / D-137) =="

# group_lines <rendered changelog> -> "GROUP<TAB>- subject" per rendered bullet.
group_lines() {
  awk '
    /^### / { grp = substr($0, 5); next }
    /^## /  { grp = "" }
    /^- / && grp != "" { print grp "\t" $0 }
  ' "$1"
}

# The anchor: a real fix an adopter would look for under Fixes.
AMBULANCE='- :ambulance: fix(forge): skip malformed bot markers with a warning instead of bricking reconcile (AUD-S12, REL-06)'
# Types this repo files somewhere. `revert` is deliberately absent: no group
# fits it, and inventing one is a changelog-structure change, not this fix.
FILEABLE_TYPES='feat|fix|docs|specs|refactor|style|test|chore|build|ci|perf|security'

# The detector (REDMAIN-N2 / D-168). The gitmoji prefix is OPTIONAL and has two
# recognised spellings, so a fileable type is seen behind an ASCII shortcode,
# behind a literal emoji, or behind nothing at all:
#   `- :bug: fix(a): …`   `- 👷 ci(docs): …`   `- ci(docs): …`
# `[^ -~]` is a BYTE class under LC_ALL=C — every byte outside printable ASCII,
# which is every lead byte of a UTF-8 emoji. It is spelled that way because
# `[:ascii:]` is a PCRE extension GNU grep does not implement, and because a
# locale-dependent class would make this gate's verdict depend on the runner's
# LANG. Every use of these two patterns therefore goes through `LC_ALL=C grep`.
OTHER_MAPPABLE_RE="^- (:[a-z0-9_+-]+: |[^ -~][^ ]* )?($FILEABLE_TYPES)[(:]"
# The pre-D-168 pattern, kept ONLY as §8b's regression control. It is what
# fail-open looked like; nothing outside §8b may use it.
OTHER_MAPPABLE_RE_PREFIX_REQUIRED="^- :[a-z0-9_]+: ($FILEABLE_TYPES)[(:]"

group_lines "$WORK/clean-full.md" >"$WORK/clean.groups"
[[ -s "$WORK/clean.groups" ]] || fail "group extraction produced no lines — section 8's assertions would all be vacuous"
distinct_groups="$(cut -f1 "$WORK/clean.groups" | sort -u | wc -l | tr -d ' ')"
[[ "$distinct_groups" -ge 5 ]] \
  || fail "group extraction found only $distinct_groups distinct group(s) — the awk range is broken"
anchor_hits="$(grep -cF -e "$AMBULANCE" "$WORK/clean.groups" || true)"
[[ "$anchor_hits" -ge 1 ]] \
  || fail "the REL-14 anchor commit is not in the rendered changelog at all — section 8 is testing nothing (subject: $AMBULANCE)"
echo "OK: $distinct_groups groups extracted, anchor present $anchor_hits time(s)"

anchor_group="$(grep -F -e "$AMBULANCE" "$WORK/clean.groups" | head -1 | cut -f1)"
[[ "$anchor_group" == "Fixes" ]] \
  || fail "the ':ambulance: fix(forge): ...' hotfix renders under '$anchor_group', not 'Fixes' — a user-facing fix is mis-filed on the published Release page (REL-14)"
echo "OK: the :ambulance: hotfix renders under Fixes"

awk -F'\t' '$1 == "Other" { print $2 }' "$WORK/clean.groups" >"$WORK/clean.other"
# Non-empty on BOTH sides of the exemption subtraction below: an empty Other
# block would make the detector pass for the wrong reason, and an empty exemption
# list would make the "still renders under Other" assertion pass vacuously.
[[ -s "$WORK/clean.other" ]] \
  || fail "no bullets extracted for the Other group — the exemption and detector assertions below would both be vacuous (the awk range or the render is broken)"

SUBJECT_GATE="$ROOT/hack/release/commit_subject_gate.sh"
[[ -f "$SUBJECT_GATE" ]] || fail "missing $SUBJECT_GATE — §8's exemption and §9 both read it (REDMAIN-N1/N2, D-168)"
bash "$SUBJECT_GATE" --legacy-subjects >"$WORK/legacy.subjects" 2>"$WORK/legacy.err" || {
  cat "$WORK/legacy.err" >&2
  fail "'commit_subject_gate.sh --legacy-subjects' failed — §8 cannot establish which Other entries are exempt published history"
}
[[ -s "$WORK/legacy.subjects" ]] \
  || fail "'commit_subject_gate.sh --legacy-subjects' printed nothing while LEGACY_ALLOW_SHAS is expected non-empty — the single authority for the exemption is broken, so the subtraction below would be a no-op that nobody notices"

: >"$WORK/other.exempt"
while IFS= read -r legacy_subject; do
  [[ -n "$legacy_subject" ]] || continue
  grep -qxF -e "- $legacy_subject" "$WORK/clean.other" \
    || fail "the published-history exemption '- $legacy_subject' no longer renders under Other — cliff.toml now files it correctly (REDMAIN-N3), so the exemption is dead scaffolding: drop that SHA from LEGACY_ALLOW_SHAS in hack/release/commit_subject_gate.sh (REQ-REDMAIN-N2-02)"
  printf '%s\n' "- $legacy_subject" >>"$WORK/other.exempt"
done <"$WORK/legacy.subjects"
[[ -s "$WORK/other.exempt" ]] \
  || fail "the exemption file is empty after reading a non-empty --legacy-subjects list — the read loop is broken"
grep -v -x -F -f "$WORK/other.exempt" "$WORK/clean.other" >"$WORK/clean.other.checked" || true
n_other="$(wc -l <"$WORK/clean.other" | tr -d ' ')"
n_exempt="$(wc -l <"$WORK/other.exempt" | tr -d ' ')"
n_checked="$(wc -l <"$WORK/clean.other.checked" | tr -d ' ')"
[[ "$n_checked" -eq $((n_other - n_exempt)) ]] \
  || fail "exemption subtraction removed $((n_other - n_checked)) line(s) for $n_exempt exemption(s) — the filter is not matching whole lines"
echo "OK: $n_exempt published-history exemption(s) still render under Other, $n_checked line(s) left to check"

if LC_ALL=C grep -nE "$OTHER_MAPPABLE_RE" "$WORK/clean.other.checked" >"$WORK/clean.other.mappable"; then
  cat "$WORK/clean.other.mappable" >&2
  fail "the lines above declare a conventional type this repo files, yet render under Other — extend cliff.toml's type-keyed parsers (REL-14). A literal-emoji prefix is no longer a way past this check (REDMAIN-N2 / D-168)"
fi
echo "OK: nothing in Other declares a fileable type ($n_other line(s) in Other: $n_exempt exempt published-history entr(y/ies) + $n_checked by design — revert, which has no group, and one malformed ':test(release):' subject that declares no parseable type)"

echo "== 8a. the grouping assertions can fail (mutation) =="
mutant_cfg2="$WORK/cliff.no-type-parsers.toml"
grep -v '# REL-14' "$ROOT/cliff.toml" >"$mutant_cfg2"
if grep -q '# REL-14' "$mutant_cfg2"; then
  fail "mutation did not land: '# REL-14' entries are still in $mutant_cfg2"
fi
removed=$(( $(wc -l <"$ROOT/cliff.toml") - $(wc -l <"$mutant_cfg2") ))
[[ "$removed" -ge 2 ]] \
  || fail "mutation did not land: only $removed line(s) removed from cliff.toml — the REL-14 parsers are not tagged '# REL-14'"
"$CLIFF" --config "$mutant_cfg2" -o "$WORK/mutant-groups.md" 2>"$WORK/mutant-groups.err" || {
  cat "$WORK/mutant-groups.err" >&2
  fail "git-cliff failed with the type-parser mutation — the mutation broke the TOML instead of removing the entries"
}
group_lines "$WORK/mutant-groups.md" >"$WORK/mutant.groups"
mutant_anchor_group="$(grep -F -e "$AMBULANCE" "$WORK/mutant.groups" | head -1 | cut -f1)"
[[ "$mutant_anchor_group" == "Other" ]] \
  || fail "removing the $removed REL-14 parser entries leaves the hotfix under '$mutant_anchor_group' — section 8 does not detect the regression it exists to catch"
echo "OK: removing the $removed REL-14 parser entries puts the hotfix back under Other"

# The parsers must only RE-FILE lines, never add or drop one.
cut -f2 "$WORK/clean.groups" | sort >"$WORK/clean.subjects"
cut -f2 "$WORK/mutant.groups" | sort >"$WORK/mutant.subjects"
if ! cmp -s "$WORK/clean.subjects" "$WORK/mutant.subjects"; then
  fail "the REL-14 parsers change WHICH subjects render, not just where they are filed — they must only re-group"
fi
echo "OK: identical subject multiset before and after grouping — the parsers only re-file"

# --- 8b. the Other detector sees a fileable type behind ANY prefix (REDMAIN-N2) --
#
# §8's clean assertion is an ABSENCE, and an absence is only evidence when the
# detector that produced it is known to detect. The exempt line is subtracted
# before §8's grep, so the real Other block no longer exercises the literal-emoji
# path at all — this section is where that path is actually proved, on a probe
# whose expected verdict is written down line by line.
#
# It also pins the REGRESSION, not just the fix: the pre-D-168 pattern is run
# against the same probe and must MISS the literal-emoji line. If someone
# reverts OTHER_MAPPABLE_RE to the prefix-required form, the two assertions here
# collapse into each other and this section reds.

echo "== 8b. the Other detector sees a fileable type behind any prefix (REDMAIN-N2 / D-168) =="
cat >"$WORK/other.probe" <<'PROBE'
- 👷 ci(docs): literal-emoji prefix, fileable type — the REDMAIN-N2 defect
- :bug: fix(forge): ASCII shortcode prefix, fileable type
- ci(docs): no prefix at all, fileable type
- :rewind: revert(kind): revert is deliberately not fileable — must NOT be flagged
- :test(release): malformed subject declaring no parseable type — must NOT be flagged
- Merge pull request #1 from org/branch — not a conventional subject at all
PROBE
[[ "$(wc -l <"$WORK/other.probe" | tr -d ' ')" -eq 6 ]] \
  || fail "the §8b probe did not land with its 6 lines — the heredoc is broken and every count below is meaningless"

LC_ALL=C grep -nE "$OTHER_MAPPABLE_RE" "$WORK/other.probe" >"$WORK/probe.hits" || true
[[ "$(wc -l <"$WORK/probe.hits" | tr -d ' ')" -eq 3 ]] \
  || { cat "$WORK/probe.hits" >&2; fail "the Other detector flags $(wc -l <"$WORK/probe.hits" | tr -d ' ') of the 6 probe lines, want exactly 3 (the three fileable ones)"; }
for want in 'ci(docs): literal-emoji prefix' 'fix(forge): ASCII shortcode prefix' 'ci(docs): no prefix at all'; do
  grep -qF -e "$want" "$WORK/probe.hits" \
    || fail "the Other detector does NOT flag the probe line containing '$want' — a mis-filed entry of that shape would render on the Release page unseen"
done
for reject in 'revert is deliberately not fileable' 'declaring no parseable type' 'not a conventional subject at all'; do
  if grep -qF -e "$reject" "$WORK/probe.hits"; then
    fail "the Other detector flags the probe line containing '$reject' — it is over-firing on entries that belong in Other, which would make this gate unfixable"
  fi
done
echo "OK: 3/6 probe lines flagged — shortcode, literal emoji and bare type all seen; revert/malformed/merge all left alone"

LC_ALL=C grep -nE "$OTHER_MAPPABLE_RE_PREFIX_REQUIRED" "$WORK/other.probe" >"$WORK/probe.hits.old" || true
[[ "$(wc -l <"$WORK/probe.hits.old" | tr -d ' ')" -eq 1 ]] \
  || { cat "$WORK/probe.hits.old" >&2; fail "the pre-D-168 pattern flags $(wc -l <"$WORK/probe.hits.old" | tr -d ' ') probe line(s), want exactly 1 — the regression control is not reproducing the old behaviour, so the 'this was fail-open' claim below is unproved"; }
if grep -qF -e 'literal-emoji prefix' "$WORK/probe.hits.old"; then
  fail "the pre-D-168 pattern flags the literal-emoji line — then REDMAIN-N2 was not a real fail-open and OTHER_MAPPABLE_RE has been reverted to the prefix-required form"
fi
echo "OK: regression control — the pre-D-168 pattern sees 1 of the 3, and is blind to the literal-emoji entry that caused REDMAIN-N2"

# ------- 9. a literal-emoji commit subject is rejected by a gate (REDMAIN-N1) --
#
# The other half of the SAME defect. §8/§8b make a mis-filed entry visible in the
# rendered changelog; this section makes the commit that produces one impossible
# to land unnoticed. `GUIDELINES.md` § Repository discipline mandates the ASCII
# shortcode and, until D-168, NOTHING enforced it: there was no commit-message
# linter anywhere in hack/** or .github/workflows/**, which is how `dfdae69`
# reached published history and, through `cliff.toml`'s shortcode-keyed parsers,
# the `### Other` group of the published Release page.
#
# The gate lives in `hack/release/commit_subject_gate.sh` and is reached two ways:
#   * this script, i.e. `task check` stage `release-changelog-gate-test` (a 22nd
#     stage was NOT added: `CHECK_STAGES` in hack/audit/exitgate_test.sh asserts
#     the Taskfile's check: list is EQUAL to it, so adding one is a change to that
#     pin — see D-168);
#   * a step of verify.yaml's `verify:` job, which is NOT guarded off
#     `pull_request` (unlike the changelog drift gate, D-125/OQ-30), so a lane's
#     literal-emoji commit reds its own PR instead of reddening main after merge.
#
# Polarities proved here, in order: green on real history; green on an all-ASCII
# sandbox; RED the moment a literal-emoji commit is added to that sandbox; RED on
# real history with the published-history exemption stripped.

echo "== 9. a literal-emoji commit subject is rejected by a gate (REDMAIN-N1 / D-168) =="

bash "$SUBJECT_GATE" >"$WORK/subject.self" 2>&1 || {
  cat "$WORK/subject.self" >&2
  fail "hack/release/commit_subject_gate.sh is RED on this repository's own history — a new commit subject leads with a literal emoji, or the published-history exemption has gone stale"
}
grep -q '^OK: ' "$WORK/subject.self" \
  || fail "commit_subject_gate.sh exited 0 without its OK line — it returned success without reporting a scan (output: $(head -1 "$WORK/subject.self"))"
cat "$WORK/subject.self"

echo "== 9a. wiring: verify.yaml's verify: job runs the commit-subject gate on pull_request =="
extract_step "$WORKFLOW" verify 'commit subject' >"$WORK/step.subject"
[[ -s "$WORK/step.subject" ]] \
  || fail "could not extract a 'commit subject' step from verify.yaml's verify: job — the gate is defined but nothing runs it on a pull request (REQ-REDMAIN-N1-02)"
grep -qF 'bash hack/release/commit_subject_gate.sh' "$WORK/step.subject" \
  || fail "the extracted verify: step does not run 'bash hack/release/commit_subject_gate.sh' — extraction matched the wrong step"
# No repo argument: the argument form is the sandbox/foreign mode, which skips the
# published-history exemption self-checks. CI must run the self mode.
grep -qE 'bash hack/release/commit_subject_gate\.sh[[:space:]]*$' "$WORK/step.subject" \
  || fail "verify.yaml runs commit_subject_gate.sh WITH an argument — that is foreign mode, which skips the exemption self-checks; CI must run it with no argument"
if grep -qF "github.event_name" "$WORK/step.subject"; then
  fail "the commit-subject gate step carries an event guard — it must run on pull_request, which is the only place a bad subject can still be reworded (hard rule 2 forbids rewriting it afterwards)"
fi
echo "OK: verify: job runs the commit-subject gate, unguarded, in self mode"

echo "== 9b. the wiring assertion itself can fail (mutation) =="
mutant_wf_subject="$WORK/verify.no-subject-gate.yaml"
grep -vF 'bash hack/release/commit_subject_gate.sh' "$WORKFLOW" >"$mutant_wf_subject"
if grep -qF 'bash hack/release/commit_subject_gate.sh' "$mutant_wf_subject"; then
  fail "mutation did not land: $mutant_wf_subject still runs the commit-subject gate"
fi
[[ "$(wc -l <"$mutant_wf_subject")" -lt "$(wc -l <"$WORKFLOW")" ]] \
  || fail "mutation did not land: $mutant_wf_subject has the same line count as verify.yaml"
extract_step "$mutant_wf_subject" verify 'commit subject' >"$WORK/step.subject.mutant"
if grep -qF 'bash hack/release/commit_subject_gate.sh' "$WORK/step.subject.mutant"; then
  fail "the wiring assertion reports the gate present in a workflow with that run line deleted — §9a is vacuous"
fi
echo "OK: deleting the run line from verify.yaml turns the wiring assertion red"

echo "== 9c. both polarities over a sandbox repository =="
SUBJ_SANDBOX="$WORK/subject-sandbox"
mkdir -p "$SUBJ_SANDBOX"
# Same self-containment discipline as §7b: GIT_DIR and friends come from the
# ENVIRONMENT and beat `-C`, so a `task check` reached from a git hook or a CI
# wrapper that exports them would otherwise have this block commit into the REAL
# repository.
ssgit() {
  env -u GIT_DIR -u GIT_WORK_TREE -u GIT_INDEX_FILE -u GIT_OBJECT_DIRECTORY \
    -u GIT_ALTERNATE_OBJECT_DIRECTORIES -u GIT_COMMON_DIR \
    git -C "$SUBJ_SANDBOX" \
    -c user.name='commit subject gate' -c user.email='gate@example.invalid' \
    -c commit.gpgsign=false -c core.hooksPath=/dev/null \
    -c init.defaultBranch=main -c advice.detachedHead=false "$@"
}
env -u GIT_DIR -u GIT_WORK_TREE -u GIT_INDEX_FILE -u GIT_OBJECT_DIRECTORY \
  -u GIT_ALTERNATE_OBJECT_DIRECTORIES -u GIT_COMMON_DIR \
  git init -q "$SUBJ_SANDBOX" >/dev/null 2>&1 || fail "could not git init the subject sandbox"
[[ -d "$SUBJ_SANDBOX/.git" ]] \
  || fail "subject sandbox has no .git — git init landed somewhere else (a GIT_DIR in the environment?)"
subj_expected="$(cd "$SUBJ_SANDBOX" && pwd -P)"
subj_actual="$(cd "$(ssgit rev-parse --show-toplevel)" && pwd -P)"
[[ "$subj_actual" == "$subj_expected" ]] \
  || fail "subject sandbox git commands resolve to '$subj_actual', not '$subj_expected' — the environment is redirecting them at another repository"
ssgit symbolic-ref HEAD refs/heads/main

# The legitimate shapes this gate must NEVER reject: the project convention, and
# the two bot/forge subjects that carry no shortcode at all.
printf 'a\n' >"$SUBJ_SANDBOX/a.txt"; ssgit add -A
ssgit commit -q -m ':sparkles: feat(sandbox): the project convention, ASCII shortcode first'
printf 'b\n' >"$SUBJ_SANDBOX/b.txt"; ssgit add -A
ssgit commit -q -m 'build(deps): bump some/action from 1.2.3 to 1.2.4'
ssgit checkout -q -b side
printf 'c\n' >"$SUBJ_SANDBOX/c.txt"; ssgit add -A
ssgit commit -q -m ':bug: fix(sandbox): a fix on a side branch'
ssgit checkout -q main
ssgit merge -q --no-ff side -m 'Merge pull request #1 from org/side'

bash "$SUBJECT_GATE" "$SUBJ_SANDBOX" >"$WORK/subject.sandbox.clean" 2>&1 || {
  cat "$WORK/subject.sandbox.clean" >&2
  fail "the commit-subject gate is RED on an all-ASCII sandbox — it rejects Dependabot's 'build(deps): …', GitHub's 'Merge pull request …', or the project convention itself, which would make it unusable"
}
sandbox_n="$(ssgit rev-list --count HEAD)"
[[ "$sandbox_n" -eq 4 ]] \
  || fail "the sandbox has $sandbox_n commits, expected 4 — it was not built as intended and the red below would not mean what it says"
grep -qF "$sandbox_n commit subject(s) scanned" "$WORK/subject.sandbox.clean" \
  || fail "the gate reported success without scanning all $sandbox_n sandbox commits (said: $(cat "$WORK/subject.sandbox.clean")) — a green that skipped the history proves nothing"
echo "OK: green on a $sandbox_n-commit all-ASCII sandbox (convention + dependabot + merge subjects all accepted)"

printf 'd\n' >"$SUBJ_SANDBOX/d.txt"; ssgit add -A
ssgit commit -q -m '👷 ci(docs): stop uploading the Pages artifact on pull requests'
bad_sha="$(ssgit rev-parse HEAD)"
if bash "$SUBJECT_GATE" "$SUBJ_SANDBOX" >"$WORK/subject.sandbox.bad" 2>&1; then
  cat "$WORK/subject.sandbox.bad" >&2
  fail "the commit-subject gate exited 0 on a sandbox containing a literal-emoji subject — REDMAIN-N1 is NOT closed (this is the exact shape of dfdae69)"
fi
grep -qF "$bad_sha" "$WORK/subject.sandbox.bad" \
  || fail "the gate went red but never named the offending commit $bad_sha — an author cannot act on it (output: $(head -3 "$WORK/subject.sandbox.bad"))"
grep -qF 'shortcode' "$WORK/subject.sandbox.bad" \
  || fail "the gate's failure message does not tell the author to use the ASCII shortcode — it reds without a remedy"
echo "OK: adding ONE literal-emoji commit to the same sandbox turns the gate red and names it"

echo "== 9d. the published-history exemption is load-bearing, not decoration =="
# If the gate were vacuous over real history, deleting the exemption would change
# nothing. It must red, and it must red naming dfdae69 — the REDMAIN-N1 commit.
LEGACY_ANCHOR='dfdae69143c3bd5b4819df106bf6fbbad18eb4fc'
grep -qF "$LEGACY_ANCHOR" "$SUBJECT_GATE" \
  || fail "the REDMAIN-N1 anchor $LEGACY_ANCHOR is not listed in $SUBJECT_GATE — the exemption this section mutates does not exist, so the mutation below would prove nothing"
mutant_gate="$WORK/commit_subject_gate.no-exemption.sh"
grep -v "$LEGACY_ANCHOR" "$SUBJECT_GATE" >"$mutant_gate"
if grep -qF "$LEGACY_ANCHOR" "$mutant_gate"; then
  fail "mutation did not land: $LEGACY_ANCHOR is still in $mutant_gate"
fi
[[ "$(wc -l <"$mutant_gate")" -lt "$(wc -l <"$SUBJECT_GATE")" ]] \
  || fail "mutation did not land: $mutant_gate has the same line count as $SUBJECT_GATE"
if bash "$mutant_gate" "$ROOT" >"$WORK/subject.no-exemption" 2>&1; then
  cat "$WORK/subject.no-exemption" >&2
  fail "with the exemption deleted the gate STILL passes on this repository — it is not actually scanning published history, so its green above is vacuous"
fi
grep -qF "$LEGACY_ANCHOR" "$WORK/subject.no-exemption" \
  || fail "the exemption-free gate reds on this repository but does not name $LEGACY_ANCHOR — it is failing for some other reason"
echo "OK: deleting the exemption reds the gate on real history, naming $LEGACY_ANCHOR"

echo "PASS: changelog drift gate regenerated, wired into task check + verify.yaml, and proven at both polarities (REQ-AUD-S02-01/02); release body carries the compatibility notes and no merge subject (D-136); every fileable subject reaches its real group (REL-14 / D-137) behind any prefix shape, and a literal-emoji commit subject is rejected by a gate rather than by a human (REDMAIN-N1/N2 / D-168)"
