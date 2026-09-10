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

echo "== 1. REQ-AUD-S02-01 (amended by D-180): released v0.1.0 section present, NO Unreleased section, newest tag stamped =="
grep -qE '^## \[0\.1\.0\] - 2026-08-05$' "$ROOT/CHANGELOG.md" \
  || fail "CHANGELOG.md has no '## [0.1.0] - 2026-08-05' section — run 'task changelog-write' and commit (REQ-AUD-S02-01)"
# D-180: the committed file holds RELEASED versions only. An `## Unreleased`
# heading in it means the preview form (a bare `git-cliff -o`) was committed —
# which is exactly the shape that made every ordinary commit stale it.
if grep -nE '^## Unreleased$' "$ROOT/CHANGELOG.md" >"$WORK/committed.unreleased"; then
  fail "CHANGELOG.md carries an '## Unreleased' section (line $(cut -d: -f1 "$WORK/committed.unreleased")) — REQ-AUD-S02-01 as amended by D-180: the committed file holds released versions only; run 'task changelog-write' (it renders released-only) and commit"
fi
# ...and its newest section is the newest tag reachable from HEAD, i.e. the
# post-tag stamp has happened — EXCEPT on the tagged commit itself (D-181).
# This script runs in CI too (release-exitgate → hack/audit/exitgate_test.sh →
# `task check`), and release.yaml's tag gate needs every verify run on the tag
# SHA green, so a red here would lock the tag out exactly like the drift gate
# did. The exception is verify-changelog.sh's OWN allowance, read from its
# output rather than re-implemented, so the two can never disagree: the newest
# tag points at HEAD and CHANGELOG.md lacks only that section → a warning. Any
# commit on top of the unstamped tag still reds here and in the drift gate.
# newest_section_check <repo-root> <label> — the §1 assertion as a function, so
# §10e can run it on sandbox states without a full nested run. Prints OK/WARN
# and returns 0, or prints the cause and returns 1. The three green cases:
#   * the newest section is the newest tag's (the ordinary state);
#   * the newest tag is on HEAD and verify-changelog.sh says "tolerated (D-181)";
#   * verify-changelog.sh is PLAINLY green, i.e. the file is byte-identical to
#     the released-only render of every tag: the newest tag then renders no
#     section at all (all its commits are cliff-skipped, e.g. a tooling-only
#     patch), and nothing is missing (review N2).
newest_section_check() {
  local root="$1" label="$2" tag first msg
  tag="$(git -C "$root" describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || true)"
  if [[ -z "$tag" ]]; then
    echo "no v[0-9]* tag is reachable from HEAD in $root — a shallow or tagless checkout; the released-only assertions need the tags"
    return 1
  fi
  first="$(grep -m1 -E '^## \[' "$root/CHANGELOG.md" || true)"
  if [[ "$first" == "## [${tag#v}] - "* ]]; then
    echo "OK: newest section is the newest tag ($tag)"
    return 0
  fi
  msg="CHANGELOG.md's newest section is '$first' but the newest tag is $tag — the post-tag stamp commit is missing: run 'task changelog-write' and commit it as ':memo: chore(release): stamp the $tag section after tagging' (REQ-AUD-S02-01 as amended by D-180)"
  if ! bash "$root/hack/release/verify-changelog.sh" >"$WORK/allowance.$label" 2>&1; then
    cat "$WORK/allowance.$label"
    echo "$msg"
    return 1
  fi
  if grep -qF 'tolerated (D-181)' "$WORK/allowance.$label"; then
    git -C "$root" tag --points-at HEAD --list 'v[0-9]*' >"$WORK/head.tags.$label"
    if ! grep -qxF -e "$tag" "$WORK/head.tags.$label"; then
      echo "$msg — the drift gate tolerated a HEAD tag, but the newest tag $tag is not on HEAD"
      return 1
    fi
    if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
      echo "::warning title=CHANGELOG.md not stamped::$msg"
    fi
    echo "WARN: $msg — tolerated on the tagged commit itself (D-181); land the stamp next"
    echo "OK: newest tag $tag is HEAD and unstamped — tolerated (D-181)"
    return 0
  fi
  grep -qxF 'verify-changelog: ok' "$WORK/allowance.$label" || {
    cat "$WORK/allowance.$label"
    echo "$msg — and the drift gate's output is neither a plain ok nor the D-181 allowance"
    return 1
  }
  echo "OK: the newest tag $tag renders no section (every commit in it is cliff-skipped) and CHANGELOG.md is exactly the released-only render — nothing to stamp"
  return 0
}
newest_section_check "$ROOT" root >"$WORK/newest.root" || { cat "$WORK/newest.root" >&2; fail "§1: see above"; }
cat "$WORK/newest.root"
echo "OK: [0.1.0] present, no Unreleased section, newest section consistent with the newest tag"

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
# D-180: ONE definition of the committed form. If `changelog-write` rendered by
# its own git-cliff line it could drift from what verify-changelog.sh compares
# against — the writer and the checker disagreeing is a gate that is red after
# every regeneration, or green over a file nobody can reproduce.
extract_block "$TASKFILE" changelog-write >"$WORK/def.changelog-write"
[[ -s "$WORK/def.changelog-write" ]] || fail "Taskfile.yml defines no changelog-write task"
grep -qE '^[[:space:]]+- bash hack/release/render-changelog\.sh CHANGELOG\.md"?$' "$WORK/def.changelog-write" \
  || fail "the changelog-write task does not write CHANGELOG.md through hack/release/render-changelog.sh — the committed form must have one definition (D-180)"
if grep -qE '^[[:space:]]+- .*(GIT_CLIFF_BIN|bin/git-cliff|git-cliff )' "$WORK/def.changelog-write"; then
  fail "the changelog-write task calls git-cliff directly — it must go through render-changelog.sh, or it can render something verify-changelog.sh does not compare against (D-180)"
fi
grep -qF 'bash hack/release/render-changelog.sh' "$ROOT/hack/release/verify-changelog.sh" \
  || fail "verify-changelog.sh does not render through hack/release/render-changelog.sh — the checker and the writer can disagree (D-180)"
# render_sets_switch <file> — does <file> RUN git-cliff with the switch set? Anchored
# on the whole invocation line, not on the token: render-changelog.sh's header
# comment also names `ASSENT_CHANGELOG_RELEASED_ONLY=1`, so a bare-token grep
# stayed green with the real assignment deleted (review finding F4). §2b proves
# the anchor by deleting exactly that line.
render_sets_switch() {
  grep -qE '^ASSENT_CHANGELOG_RELEASED_ONLY=1 "\$\{CLIFF\}" --config ' "$1"
}
render_sets_switch "$ROOT/hack/release/render-changelog.sh" \
  || fail "render-changelog.sh no longer RUNS git-cliff with ASSENT_CHANGELOG_RELEASED_ONLY=1 — it would commit the Unreleased section again (D-180)"
extract_block "$TASKFILE" changelog >"$WORK/def.changelog"
grep -qF -- '--unreleased' "$WORK/def.changelog" \
  || fail "the changelog preview task no longer renders --unreleased — the only place the unreleased entries are visible before a release (D-180)"
grep -qF 'env -u ASSENT_CHANGELOG_RELEASED_ONLY' "$WORK/def.changelog" \
  || fail "the changelog preview task does not clear ASSENT_CHANGELOG_RELEASED_ONLY — a stray export would make the preview silently empty (D-180)"
grep -q 'hack/docs/readme_smoke_test.sh' "$WORK/def.docs-gates" \
  || fail "the docs-gates task no longer runs hack/docs/readme_smoke_test.sh (D-124)"
grep -q 'hack/docs/truthlag_pins_test.sh' "$WORK/def.docs-gates" \
  || fail "the docs-gates task no longer runs hack/docs/truthlag_pins_test.sh (D-124)"
grep -q 'hack/lint/depguard_test.sh' "$WORK/def.lint-depguard-test" \
  || fail "the lint-depguard-test task no longer runs hack/lint/depguard_test.sh (D-123/AUD-S07)"
echo "OK: each wired task still invokes its script"

echo "== 2b. the wiring assertion itself can fail (mutation) =="
# F4: delete the ONE line that runs git-cliff with the switch; the header comment
# naming the token stays. The assertion must go red on that file.
mutant_render="$WORK/render-changelog.no-switch.sh"
grep -vE '^ASSENT_CHANGELOG_RELEASED_ONLY=1 ' "$ROOT/hack/release/render-changelog.sh" >"$mutant_render"
[[ "$(wc -l <"$mutant_render")" -eq $(($(wc -l <"$ROOT/hack/release/render-changelog.sh") - 1)) ]] \
  || fail "mutation did not land: exactly one line (the switched git-cliff invocation) should have been removed from render-changelog.sh"
grep -qF 'ASSENT_CHANGELOG_RELEASED_ONLY=1' "$mutant_render" \
  || fail "mutation control is weaker than intended: the mutant no longer mentions the token anywhere, so it does not reproduce the comment-only shape F4 found"
if render_sets_switch "$mutant_render"; then
  fail "render_sets_switch reports the switch set in a render-changelog.sh whose invocation line is deleted — the assertion is satisfied by the comment (F4)"
fi
sed 's/^ASSENT_CHANGELOG_RELEASED_ONLY=1 /ASSENT_CHANGELOG_RELEASED_ONLY=0 /' "$ROOT/hack/release/render-changelog.sh" >"$mutant_render"
if render_sets_switch "$mutant_render"; then
  fail "render_sets_switch reports the switch set in a render-changelog.sh that runs git-cliff with ASSENT_CHANGELOG_RELEASED_ONLY=0"
fi
echo "OK: deleting the switched invocation (comment left in place) or setting it to 0 turns the render-changelog.sh assertion red"
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

# render_full <git-cliff args…> — the WITH-UNRELEASED render every quality check
# below (§6 release body, §7 merge skip, §8 grouping/Other detector) inspects.
# D-180 made the committed CHANGELOG.md released-only, so the unreleased entries
# — the ones about to reach the next Release page — exist only in this render.
# `env -u` is load-bearing: a caller with ASSENT_CHANGELOG_RELEASED_ONLY=1
# exported would otherwise blind every detector to exactly those entries. §10d
# proves both halves.
render_full() {
  env -u ASSENT_CHANGELOG_RELEASED_ONLY "$CLIFF" "$@"
}

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
render_full --config "$ROOT/cliff.toml" "${cliff_argv[@]}" >"$WORK/release-body.md" 2>"$WORK/release-body.err" || {
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

# D-180: the Release body is rendered from the TAGS, never from the committed
# CHANGELOG.md, so the released-only switch must not change it. (§10c shows the
# stronger form: a freshly pushed tag's body is right even BEFORE its section is
# stamped into CHANGELOG.md.)
ASSENT_CHANGELOG_RELEASED_ONLY=1 "$CLIFF" --config "$ROOT/cliff.toml" "${cliff_argv[@]}" \
  >"$WORK/release-body.released-only.md" 2>/dev/null \
  || fail "git-cliff failed rendering the release body with ASSENT_CHANGELOG_RELEASED_ONLY=1"
cmp -s "$WORK/release-body.md" "$WORK/release-body.released-only.md" \
  || fail "the GitHub Release body changes with ASSENT_CHANGELOG_RELEASED_ONLY — release.yaml's '$cliff_args' must render the latest TAG's section whatever the switch says (D-180)"
echo "OK: release body is identical with and without the released-only switch"

# Polarity control: re-render WITH the bug. If the note survives `--strip
# header` too, then the grep above passes for some unrelated reason and this
# section is not testing what it claims.
render_full --config "$ROOT/cliff.toml" "${cliff_argv[@]}" --strip header \
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

render_full --config "$ROOT/cliff.toml" -o "$WORK/clean-full.md" 2>"$WORK/clean-full.err" || {
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
render_full --config "$mutant_cfg" -o "$WORK/mutant-full.md" 2>"$WORK/mutant-full.err" || {
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
# Every sort and comm below is LC_ALL=C (D-179): comm requires both inputs in
# the order IT compares by, and under en_US.UTF-8 on uutils coreutils `sort`
# collated by locale while `comm` checked bytes — "comm: file 1 is not in sorted
# order" on a clean main. hack/lint/locale_pin_test.sh enforces the pin.
grep -E '^- ' "$WORK/clean-full.md" | LC_ALL=C sort >"$WORK/clean.bullets"
grep -E '^- ' "$WORK/mutant-full.md" | LC_ALL=C sort >"$WORK/mutant.bullets"
[[ -s "$WORK/clean.bullets" ]] || fail "clean render has no bullets at all — the comparison below would be vacuous"
LC_ALL=C comm -13 "$WORK/clean.bullets" "$WORK/mutant.bullets" >"$WORK/only-in-mutant"
LC_ALL=C comm -23 "$WORK/clean.bullets" "$WORK/mutant.bullets" >"$WORK/only-in-clean"
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
git -C "$ROOT" log --merges --format=%s | LC_ALL=C sort -u >"$WORK/merge-subjects"
[[ -s "$WORK/merge-subjects" ]] \
  || fail "git log --merges lists no merge commits — the membership check below would pass vacuously"
sed -e 's/^- //' "$WORK/only-in-mutant" | LC_ALL=C sort -u >"$WORK/only-in-mutant.subjects"
LC_ALL=C comm -23 "$WORK/only-in-mutant.subjects" "$WORK/merge-subjects" >"$WORK/only-in-mutant.extra"
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

render_full --config "$ROOT/cliff.toml" --repository "$SANDBOX" -o "$WORK/sandbox.md" 2>"$WORK/sandbox.err" || {
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
render_full --config "$mutant_cfg" --repository "$SANDBOX" -o "$WORK/sandbox-mutant.md" 2>/dev/null \
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
# therefore exempted here, by SHA, in OTHER_EXEMPT_SHAS below.
#
# WHY THAT LIST IS SEPARATE FROM `commit_subject_gate.sh`'s (P2-1). It was briefly
# derived from it, and that had NO GREEN STATE after REDMAIN-N3 lands. The two
# lists answer two different questions that DECOUPLE at exactly that moment:
#   * `LEGACY_ALLOW_SHAS` (commit_subject_gate.sh) — "is this commit's SUBJECT
#     still a literal emoji?" `dfdae69`'s subject can never change (hard rule 2),
#     so that entry is PERMANENT.
#   * `OTHER_EXEMPT_SHAS` (here) — "is this commit's RENDERED ENTRY still
#     mis-filed under Other?" REDMAIN-N3 makes that false, so this entry is
#     TEMPORARY.
# Derived from one list, the retire message had to say "drop the SHA", and
# following it reddened the commit-subject gate on `dfdae69` instead. Separated,
# the N3 lane deletes ONE line — the entry below — and both gates are green.
#
# They are still linked, but only ONE WAY and only in the direction that cannot
# deadlock: OTHER_EXEMPT_SHAS must be a SUBSET of LEGACY_ALLOW_SHAS, read live
# through `--legacy-shas`. So this section can only ever excuse a line whose
# commit is already an acknowledged, unrewritable literal-emoji subject — and the
# empty set is a subset, which is precisely the post-N3 state.
#
# The exemption stays self-retiring: an exempt entry that no longer renders under
# Other reds this section, and the remedy it prints is now executable.

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

# The detector (REDMAIN-N2 / D-168). Any number of gitmoji-ish prefix tokens,
# each an ASCII shortcode or a non-ASCII-leading token, separated and followed by
# any run of spaces INCLUDING NONE. So a fileable type is seen behind every
# spelling the classifier can be defeated by:
#   `- :bug: fix(a): …`      `- 👷 ci(docs): …`     `- ci(docs): …`
#   `- 👷  ci(docs): …`      `- 👷ci(docs): …`      `- 🚀 :rocket: feat(x): …`
# The first version of this fix accepted exactly ONE non-ASCII token followed by
# exactly ONE space, and review found the last three shapes slipping past — the
# same species of shape-specific detector that REDMAIN-N2 was filed against, so
# the quantifiers are now the general ones. The prefix alternatives stay narrow
# on purpose: an ordinary ASCII word is neither a shortcode nor non-ASCII-leading,
# so prose like `- some notes about a fix(thing): …` is still not matched.
# `[^ -~]` is a BYTE class under LC_ALL=C — every byte outside printable ASCII,
# which is every lead byte of a UTF-8 emoji. It is spelled that way because
# `[:ascii:]` is a PCRE extension GNU grep does not implement, and because a
# locale-dependent class would make this gate's verdict depend on the runner's
# LANG. Every use of these two patterns therefore goes through `LC_ALL=C grep`.
OTHER_MAPPABLE_RE="^- ((:[a-z0-9_+-]+:|[^ -~][^ ]*) *)*($FILEABLE_TYPES)[(:]"
# The pre-D-168 pattern, kept ONLY as §8b's regression control. It is what
# fail-open looked like; nothing outside §8b may use it.
OTHER_MAPPABLE_RE_PREFIX_REQUIRED="^- :[a-z0-9_]+: ($FILEABLE_TYPES)[(:]"

group_lines "$WORK/clean-full.md" >"$WORK/clean.groups"
[[ -s "$WORK/clean.groups" ]] || fail "group extraction produced no lines — section 8's assertions would all be vacuous"
distinct_groups="$(cut -f1 "$WORK/clean.groups" | LC_ALL=C sort -u | wc -l | tr -d ' ')"
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
# The Other block must be non-empty: an empty one would make the detector below
# pass for the wrong reason. The EXEMPTION list, by contrast, is allowed to be
# empty — empty is the post-REDMAIN-N3 steady state, and with no exemptions in
# force every Other line is checked, which is strictly stronger.
[[ -s "$WORK/clean.other" ]] \
  || fail "no bullets extracted for the Other group — the detector assertion below would be vacuous (the awk range or the render is broken)"

# SHAs whose RENDERED entry is knowingly still mis-filed under Other. TEMPORARY —
# see the section header: this answers "is the entry still mis-filed?", NOT "is
# the subject still a literal emoji?", and REDMAIN-N3 makes the first false while
# the second stays true forever. When N3 lands, DELETE THE LINE BELOW and nothing
# else; commit_subject_gate.sh's LEGACY_ALLOW_SHAS is not touched.
#   dfdae69 — `- 👷 ci(docs): stop uploading the Pages artifact on pull requests`,
#             the REDMAIN-N2 entry. cliff.toml has no parser for a literal-emoji
#             prefix, so it renders under Other; adding one is REDMAIN-N3.
OTHER_EXEMPT_SHAS=(
  dfdae69143c3bd5b4819df106bf6fbbad18eb4fc
)

SUBJECT_GATE="$ROOT/hack/release/commit_subject_gate.sh"
[[ -f "$SUBJECT_GATE" ]] || fail "missing $SUBJECT_GATE — §8's subset invariant and §9 both read it (REDMAIN-N1/N2, D-168)"

: >"$WORK/other.exempt"
n_exempt=0
if ((${#OTHER_EXEMPT_SHAS[@]} > 0)); then
  # The one-way link that replaced the deadlocking one: every SHA excused HERE
  # must already be an acknowledged unrewritable literal-emoji commit THERE. The
  # empty set trivially satisfies this, which is why it cannot deadlock.
  bash "$SUBJECT_GATE" --legacy-shas >"$WORK/legacy.shas" 2>"$WORK/legacy.err" || {
    cat "$WORK/legacy.err" >&2
    fail "'commit_subject_gate.sh --legacy-shas' failed — §8 cannot check that its exemptions are a subset of the acknowledged published-history set"
  }
  [[ -s "$WORK/legacy.shas" ]] \
    || fail "'commit_subject_gate.sh --legacy-shas' printed nothing while §8 lists ${#OTHER_EXEMPT_SHAS[@]} exemption(s) — the subset check below would be vacuous"
  for sha in "${OTHER_EXEMPT_SHAS[@]}"; do
    grep -qxF -e "$sha" "$WORK/legacy.shas" \
      || fail "§8 exempts $sha from the Other check, but it is NOT in LEGACY_ALLOW_SHAS in hack/release/commit_subject_gate.sh — this section may only excuse a rendered entry whose commit is already an acknowledged, unrewritable literal-emoji subject"
    subject="$(git -C "$ROOT" log -1 --format=%s "$sha" 2>/dev/null || true)"
    [[ -n "$subject" ]] \
      || fail "§8 exempts $sha but no subject can be read for it in this repository — the exemption names a commit that is not here"
    grep -qxF -e "- $subject" "$WORK/clean.other" \
      || fail "the exempted entry '- $subject' ($sha) no longer renders under Other — cliff.toml files it correctly now, so REDMAIN-N3 has landed and this exemption is dead scaffolding. REMEDY: delete $sha from OTHER_EXEMPT_SHAS in hack/release/changelog_gate_test.sh §8, and NOTHING else. Do NOT touch LEGACY_ALLOW_SHAS in hack/release/commit_subject_gate.sh — that list is keyed on the commit SUBJECT, which re-filing does not change, and removing it there would red the commit-subject gate on this very commit (REQ-REDMAIN-N2-02)"
    printf '%s\n' "- $subject" >>"$WORK/other.exempt"
  done
  [[ -s "$WORK/other.exempt" ]] \
    || fail "the exemption file is empty after iterating ${#OTHER_EXEMPT_SHAS[@]} non-empty exemption(s) — the loop is broken"
  n_exempt="$(wc -l <"$WORK/other.exempt" | tr -d ' ')"
  grep -v -x -F -f "$WORK/other.exempt" "$WORK/clean.other" >"$WORK/clean.other.checked" || true
else
  cp "$WORK/clean.other" "$WORK/clean.other.checked"
fi
n_other="$(wc -l <"$WORK/clean.other" | tr -d ' ')"
n_checked="$(wc -l <"$WORK/clean.other.checked" | tr -d ' ')"
[[ "$n_checked" -eq $((n_other - n_exempt)) ]] \
  || fail "exemption subtraction removed $((n_other - n_checked)) line(s) for $n_exempt exemption(s) — the filter is not matching whole lines"
if ((n_exempt == 0)); then
  echo "OK: no published-history exemption in force — all $n_checked Other line(s) are checked"
else
  echo "OK: $n_exempt published-history exemption(s) still render under Other, $n_checked line(s) left to check"
fi

if LC_ALL=C grep -nE "$OTHER_MAPPABLE_RE" "$WORK/clean.other.checked" >"$WORK/clean.other.mappable"; then
  cat "$WORK/clean.other.mappable" >&2
  fail "the lines above declare a conventional type this repo files, yet render under Other — extend cliff.toml's type-keyed parsers (REL-14). A literal-emoji prefix, in any spacing, is no longer a way past this check (REDMAIN-N2 / D-168)"
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
render_full --config "$mutant_cfg2" -o "$WORK/mutant-groups.md" 2>"$WORK/mutant-groups.err" || {
  cat "$WORK/mutant-groups.err" >&2
  fail "git-cliff failed with the type-parser mutation — the mutation broke the TOML instead of removing the entries"
}
group_lines "$WORK/mutant-groups.md" >"$WORK/mutant.groups"
mutant_anchor_group="$(grep -F -e "$AMBULANCE" "$WORK/mutant.groups" | head -1 | cut -f1)"
[[ "$mutant_anchor_group" == "Other" ]] \
  || fail "removing the $removed REL-14 parser entries leaves the hotfix under '$mutant_anchor_group' — section 8 does not detect the regression it exists to catch"
echo "OK: removing the $removed REL-14 parser entries puts the hotfix back under Other"

# The parsers must only RE-FILE lines, never add or drop one.
cut -f2 "$WORK/clean.groups" | LC_ALL=C sort >"$WORK/clean.subjects"
cut -f2 "$WORK/mutant.groups" | LC_ALL=C sort >"$WORK/mutant.subjects"
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
# The last four accept-lines are the shapes review found slipping past the first
# version of this fix, which required exactly one non-ASCII token and exactly one
# space. Each would render into Other with a fileable type and go unseen.
cat >"$WORK/other.probe" <<'PROBE'
- 👷 ci(docs): literal-emoji prefix, fileable type — the REDMAIN-N2 defect
- :bug: fix(forge): ASCII shortcode prefix, fileable type
- ci(docs): no prefix at all, fileable type
- 👷  ci(docs): literal emoji then TWO spaces, fileable type
- 👷ci(docs): literal emoji with NO space, fileable type
- 🚀 :rocket: feat(x): literal emoji AND a shortcode, fileable type
- 👷 :zap: ⚡ perf(core): three prefix tokens of both kinds, fileable type
- :rewind: revert(kind): revert is deliberately not fileable — must NOT be flagged
- :test(release): malformed subject declaring no parseable type — must NOT be flagged
- Merge pull request #1 from org/branch — not a conventional subject at all
- 👷 revert(kind): a literal emoji does not make revert fileable — must NOT be flagged
- some release notes about a fix(thing): prose, not a prefix — must NOT be flagged
PROBE
[[ "$(wc -l <"$WORK/other.probe" | tr -d ' ')" -eq 12 ]] \
  || fail "the §8b probe did not land with its 12 lines — the heredoc is broken and every count below is meaningless"

LC_ALL=C grep -nE "$OTHER_MAPPABLE_RE" "$WORK/other.probe" >"$WORK/probe.hits" || true
[[ "$(wc -l <"$WORK/probe.hits" | tr -d ' ')" -eq 7 ]] \
  || { cat "$WORK/probe.hits" >&2; fail "the Other detector flags $(wc -l <"$WORK/probe.hits" | tr -d ' ') of the 12 probe lines, want exactly 7 (the seven fileable ones)"; }
for want in \
  'ci(docs): literal-emoji prefix' \
  'fix(forge): ASCII shortcode prefix' \
  'ci(docs): no prefix at all' \
  'literal emoji then TWO spaces' \
  'literal emoji with NO space' \
  'literal emoji AND a shortcode' \
  'three prefix tokens of both kinds'; do
  grep -qF -e "$want" "$WORK/probe.hits" \
    || fail "the Other detector does NOT flag the probe line containing '$want' — a mis-filed entry of that shape would render on the Release page unseen, which is the REDMAIN-N2 defect in a different spacing"
done
for reject in \
  'revert is deliberately not fileable' \
  'declaring no parseable type' \
  'not a conventional subject at all' \
  'does not make revert fileable' \
  'prose, not a prefix'; do
  if grep -qF -e "$reject" "$WORK/probe.hits"; then
    fail "the Other detector flags the probe line containing '$reject' — it is over-firing on entries that belong in Other, which would make this gate unfixable"
  fi
done
echo "OK: 7/12 probe lines flagged — shortcode, literal emoji (one space, two spaces, no space), emoji+shortcode, three mixed tokens and a bare type all seen; revert (with and without emoji), the malformed subject, a merge subject and prose all left alone"

LC_ALL=C grep -nE "$OTHER_MAPPABLE_RE_PREFIX_REQUIRED" "$WORK/other.probe" >"$WORK/probe.hits.old" || true
[[ "$(wc -l <"$WORK/probe.hits.old" | tr -d ' ')" -eq 1 ]] \
  || { cat "$WORK/probe.hits.old" >&2; fail "the pre-D-168 pattern flags $(wc -l <"$WORK/probe.hits.old" | tr -d ' ') probe line(s), want exactly 1 — the regression control is not reproducing the old behaviour, so the 'this was fail-open' claim below is unproved"; }
if grep -qF -e 'literal-emoji prefix' "$WORK/probe.hits.old"; then
  fail "the pre-D-168 pattern flags the literal-emoji line — then REDMAIN-N2 was not a real fail-open and OTHER_MAPPABLE_RE has been reverted to the prefix-required form"
fi
echo "OK: regression control — the pre-D-168 pattern sees 1 of the 7, and is blind to every literal-emoji spelling that caused REDMAIN-N2"

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
# sandbox; RED the moment a literal-emoji commit is added to that sandbox; the
# allowlist pinned to an exact expected content so it cannot grow unremarked; and
# RED on real history with the published-history exemption stripped.

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

echo "== 9d. the published-history exemption is load-bearing, pinned, and not decoration =="
# The exemption list is an ALLOWLIST, so its size is a security property, not a
# detail. Without this pin a lane could land a literal-emoji commit and append its
# SHA to LEGACY_ALLOW_SHAS in a later commit of the SAME pull request, and every
# self-check inside the gate would still pass: the new SHA resolves, it is an
# ancestor of HEAD, and it genuinely IS a detection. Pinning the exact content
# here — in a different file — makes growing the allowlist a deliberate two-file
# change a reviewer sees, the same mechanism CHECK_STAGES uses on `task check`'s
# stage list. Adding an entry means editing BOTH lists and recording why in
# docs/decisions/decisions.md.
LEGACY_EXPECTED=(
  dfdae69143c3bd5b4819df106bf6fbbad18eb4fc
)
bash "$SUBJECT_GATE" --legacy-shas >"$WORK/legacy.actual" 2>"$WORK/legacy.actual.err" || {
  cat "$WORK/legacy.actual.err" >&2
  fail "'commit_subject_gate.sh --legacy-shas' failed — the allowlist cannot be pinned"
}
# Non-empty on BOTH sides: a mistyped path or a silently empty mode must fail
# loudly rather than compare two empty files and report agreement. The expected
# side is guarded on ARRAY LENGTH, not on file size — `printf '%s\n' "${a[@]}"`
# on an empty array still writes one blank line, so a file-size guard here would
# be dead code that never fires (and the array expansion itself is unsafe under
# `set -u` on bash 3.2 when empty).
((${#LEGACY_EXPECTED[@]} > 0)) \
  || fail "LEGACY_EXPECTED is empty — this pin would accept any allowlist at all; if LEGACY_ALLOW_SHAS is genuinely empty now, delete this whole pin deliberately rather than emptying it"
printf '%s\n' "${LEGACY_EXPECTED[@]}" | LC_ALL=C sort >"$WORK/legacy.expected.sorted"
LC_ALL=C sort "$WORK/legacy.actual" >"$WORK/legacy.actual.sorted"
[[ -s "$WORK/legacy.actual.sorted" ]] \
  || fail "'--legacy-shas' printed nothing — either LEGACY_ALLOW_SHAS is empty (then delete this pin deliberately) or the mode is broken; either way the comparison below would be vacuous"
if ! diff -u "$WORK/legacy.expected.sorted" "$WORK/legacy.actual.sorted" >"$WORK/legacy.diff"; then
  cat "$WORK/legacy.diff" >&2
  fail "LEGACY_ALLOW_SHAS in hack/release/commit_subject_gate.sh does not match LEGACY_EXPECTED here (${#LEGACY_EXPECTED[@]} pinned, $(wc -l <"$WORK/legacy.actual.sorted" | tr -d ' ') actual). An exemption is a DECISION: adding one means editing both lists in the same change and recording the reason in docs/decisions/decisions.md — it must never be an unremarked append"
fi
echo "OK: allowlist pinned at exactly ${#LEGACY_EXPECTED[@]} exemption(s), content-identical across the two files"

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

# ------ 10. the drift gate's scope: released versions only (D-180) ----------
#
# Until D-180 the committed CHANGELOG.md carried an `## Unreleased` section and
# verify-changelog.sh diffed the WHOLE render against it, so EVERY
# changelog-relevant commit made main stale until a regeneration commit landed.
# Lanes absorbed that with a regenerate-then-amend dance; Dependabot cannot run
# `task changelog-write` on its own branch, so every bot merge reddened main's
# push run and needed a hand-made regen PR (#131, #132).
#
# D-180 commits released versions only. The gate must now be red for exactly
# three causes and GREEN for everything else. Every row runs the REAL scripts
# (verify-changelog.sh, render-changelog.sh — the one `task changelog-write`
# runs) inside a clone of this repository, so real tags and real history are
# rendered; the working-tree copies of the changelog files are overlaid first,
# so the rows grade the tree under test, not HEAD.
#
#   10a GREEN after ordinary commits — a feature, a fix and a Dependabot-shaped
#       bump land after HEAD and the gate stays green. The property D-180 exists
#       for; its mutant (the pre-D-180 render) goes RED on the same commits.
#   10b RED on a hand edit, and RED on a cliff.toml change that re-renders
#       released history.
#   10c a pushed tag without its stamp commit: GREEN only while the tag is HEAD
#       itself and its section is the only difference (D-181 — a verify re-run
#       on the tagged SHA must not lock the tag out of REQ-AUD-S03-01); RED on a
#       hand edit or a history re-render at that same tagged HEAD, RED once any
#       commit lands on the unstamped tag, RED for an older unstamped tag behind
#       a newer tagged HEAD. The Release body is right at that very moment (it
#       reads tags); the stamp commit adds the section and is green with no
#       amend; the next ordinary commit stays green.
#   10e the WHOLE of this script, run nested on a tagged-but-unstamped commit, is
#       GREEN (§1 warns, the §10 sandbox rewinds the tag) and RED with a commit
#       on top — because CI runs it on the tag SHA via release-exitgate.
#   10d the pre-release detectors (§8's `### Other` / fileable-type check) read
#       the WITH-unreleased render: a malformed unreleased subject is flagged
#       there, is invisible in the committed form (the mutation), and stays
#       flagged when the released-only switch leaks in from the environment.

echo "== 10. the drift gate's scope: released versions only (D-180) =="

DSB="$WORK/drift-sandbox"
dgit() {
  env -u GIT_DIR -u GIT_WORK_TREE -u GIT_INDEX_FILE -u GIT_OBJECT_DIRECTORY \
    -u GIT_ALTERNATE_OBJECT_DIRECTORIES -u GIT_COMMON_DIR \
    git -C "$DSB" \
    -c user.name='changelog gate' -c user.email='gate@example.invalid' \
    -c commit.gpgsign=false -c core.hooksPath=/dev/null \
    -c advice.detachedHead=false "$@"
}
head_sha="$(git -C "$ROOT" rev-parse HEAD)"
env -u GIT_DIR -u GIT_WORK_TREE -u GIT_INDEX_FILE -u GIT_OBJECT_DIRECTORY \
  -u GIT_ALTERNATE_OBJECT_DIRECTORIES -u GIT_COMMON_DIR \
  git clone -q --no-checkout "$ROOT" "$DSB" >/dev/null 2>&1 || fail "could not clone this repository into the drift sandbox"
[[ -d "$DSB/.git" ]] || fail "drift sandbox has no .git — the clone landed somewhere else"
dsb_expected="$(cd "$DSB" && pwd -P)"
dsb_actual="$(cd "$(dgit rev-parse --show-toplevel)" && pwd -P)"
[[ "$dsb_actual" == "$dsb_expected" ]] \
  || fail "drift sandbox git commands resolve to '$dsb_actual', not '$dsb_expected' — the environment is redirecting them at another repository"
dgit checkout -q -B sandbox "$head_sha" || fail "could not check out HEAD ($head_sha) in the drift sandbox"
# D-181: when the tree under test is itself a tagged-but-unstamped release
# commit, the overlay commit below would sit ON TOP of that tag — the "commit
# on an unstamped tag" state the gate must red — and the baseline would red for
# a reason that is not drift. Rewind the sandbox to the pre-tag state: drop the
# tags on HEAD whose section CHANGELOG.md does not have yet (a stamped tag is
# kept). §10e runs this whole script on such a commit and expects green.
while IFS= read -r t; do
  [[ -n "$t" ]] || continue
  grep -qE "^## \[${t#v}\] - " "$ROOT/CHANGELOG.md" && continue
  dgit tag -d "$t" >/dev/null || fail "could not drop the unstamped HEAD tag $t in the drift sandbox"
  echo "note: HEAD carries the unstamped tag $t — the drift sandbox drops it to start from the pre-tag state (D-181)"
done < <(dgit tag --points-at "$head_sha" --list 'v[0-9]*')
[[ -n "$(dgit tag --list 'v[0-9]*')" ]] || fail "the drift sandbox has no release tags — every row below would render no released section and prove nothing"

# The files that decide the committed form, as they are in the WORKING TREE.
DRIFT_FILES=(cliff.toml CHANGELOG.md hack/release/verify-changelog.sh hack/release/render-changelog.sh hack/install-git-cliff.sh)
for f in "${DRIFT_FILES[@]}"; do
  [[ -f "$ROOT/$f" ]] || fail "missing $f — the drift sandbox cannot be built from the tree under test"
  cp "$ROOT/$f" "$DSB/$f"
done
# `:memo: chore(release):` is a cliff-skipped subject, so the overlay commit does
# not itself render.
dgit add -A && dgit commit -q --allow-empty -m ':memo: chore(release): drift sandbox baseline (working-tree overlay)' \
  || fail "could not commit the working-tree overlay in the drift sandbox"
mkdir -p "$DSB/bin" && ln -sf "$CLIFF" "$DSB/bin/git-cliff"

# dverify <label> — run the sandbox's own verify-changelog.sh; output to $WORK/dv.<label>.
dverify() {
  GIT_CLIFF_BIN="$CLIFF" env -u ASSENT_CHANGELOG_RELEASED_ONLY \
    bash "$DSB/hack/release/verify-changelog.sh" >"$WORK/dv.$1" 2>&1
}
# expect_red <label> <line the diff must add or remove> <why>
expect_red() {
  if dverify "$1"; then
    cat "$WORK/dv.$1" >&2
    fail "drift gate GREEN on '$1' — $3"
  fi
  grep -q 'CHANGELOG.md drift' "$WORK/dv.$1" \
    || { cat "$WORK/dv.$1" >&2; fail "drift gate red on '$1' but not for drift — it failed for another reason"; }
  grep -qE "^[-+]$2" "$WORK/dv.$1" \
    || { cat "$WORK/dv.$1" >&2; fail "drift gate red on '$1' but the diff does not carry '$2' — red for the wrong reason"; }
}

dverify baseline || { cat "$WORK/dv.baseline" >&2; fail "drift gate RED on the unmodified sandbox — the committed CHANGELOG.md does not match the released-only render of this tree; run 'task changelog-write'"; }
echo "OK: 10. baseline — the working tree's CHANGELOG.md is the released-only render of real history + tags"

echo "== 10a. ordinary commits after HEAD leave the gate GREEN =="
ORDINARY_SUBJECTS=(
  ':sparkles: feat(sandbox): a changelog-relevant feature landed after HEAD'
  ':bug: fix(sandbox): a changelog-relevant fix landed after HEAD'
  'build(deps): bump example/action from 1.0.0 to 1.0.1'
)
for subj in "${ORDINARY_SUBJECTS[@]}"; do
  printf '%s\n' "$subj" >>"$DSB/sandbox-ordinary.txt"
  dgit add -A && dgit commit -q -m "$subj" || fail "could not commit '$subj' in the drift sandbox"
done
# Positive control: these commits ARE changelog-relevant — they render in the
# preview. Without it, "green" could mean "skipped by a parser".
render_full --config "$DSB/cliff.toml" --repository "$DSB" --unreleased -o "$WORK/dsb.preview.md" 2>/dev/null \
  || fail "git-cliff failed rendering the sandbox preview"
for subj in "${ORDINARY_SUBJECTS[@]}"; do
  grep -qxF -e "- $subj" "$WORK/dsb.preview.md" \
    || fail "positive control: '$subj' does not render in the unreleased preview — it is not changelog-relevant, so a green below would prove nothing"
done
dverify ordinary || { cat "$WORK/dv.ordinary" >&2; fail "drift gate RED after ${#ORDINARY_SUBJECTS[@]} ordinary commits — an ordinary commit (a lane's, or a bot's that cannot regenerate) makes CHANGELOG.md stale again, which is the churn D-180 removed"; }
echo "OK: ${#ORDINARY_SUBJECTS[@]} changelog-relevant commits (feature, fix, Dependabot bump) after HEAD — all in the preview, gate still GREEN, no regeneration commit"

# The mutant: the pre-D-180 committed form (render everything). Regenerate with
# it, commit (the old working rule), add ONE more ordinary commit — red. This is
# the red every Dependabot merge produced on main.
cp "$DSB/hack/release/render-changelog.sh" "$WORK/render.orig"
sed 's/ASSENT_CHANGELOG_RELEASED_ONLY=1 /ASSENT_CHANGELOG_RELEASED_ONLY=0 /' "$WORK/render.orig" >"$DSB/hack/release/render-changelog.sh"
cmp -s "$WORK/render.orig" "$DSB/hack/release/render-changelog.sh" \
  && fail "mutation did not land: render-changelog.sh still sets ASSENT_CHANGELOG_RELEASED_ONLY=1"
GIT_CLIFF_BIN="$CLIFF" bash "$DSB/hack/release/render-changelog.sh" "$DSB/CHANGELOG.md" 2>/dev/null
grep -qE '^## Unreleased$' "$DSB/CHANGELOG.md" \
  || fail "mutation did not land: the pre-D-180 render has no Unreleased section"
dgit add -A && dgit commit -q -m ':memo: chore(release): regenerate CHANGELOG.md (pre-D-180 working rule)'
dverify mutant-fresh || { cat "$WORK/dv.mutant-fresh" >&2; fail "the pre-D-180 mutant is red straight after its own regeneration — the mutant is not a faithful reproduction of the old rule"; }
printf 'x\n' >>"$DSB/sandbox-ordinary.txt"
dgit add -A && dgit commit -q -m ':sparkles: feat(sandbox): one more ordinary commit'
expect_red mutant-stale '- :sparkles: feat\(sandbox\): one more ordinary commit' "the pre-D-180 mutant should go stale on the next ordinary commit; if it does not, row 10a's green proves nothing about D-180"
echo "OK: mutant — with the pre-D-180 render the same kind of commit reds the gate (the churn), so 10a's green is D-180's doing"
# Undo the mutant: restore the script, drop the two mutant commits.
dgit reset -q --hard HEAD~2
cp "$WORK/render.orig" "$DSB/hack/release/render-changelog.sh"
dgit diff --quiet || fail "the drift sandbox is not clean after undoing the mutant"
dverify restored || { cat "$WORK/dv.restored" >&2; fail "drift gate not green after undoing the mutant — the rows below would start from a red state"; }

echo "== 10b. a hand edit, and a cliff.toml change that re-renders history, are RED =="
cp "$DSB/CHANGELOG.md" "$WORK/dsb.changelog.orig"
printf '\n- a hand-written line\n' >>"$DSB/CHANGELOG.md"
expect_red hand-edit '- a hand-written line' "a hand edit of CHANGELOG.md is exactly what the drift gate exists to catch"
cp "$WORK/dsb.changelog.orig" "$DSB/CHANGELOG.md"
echo "OK: a hand edit reds the gate"

cp "$DSB/cliff.toml" "$WORK/dsb.cliff.orig"
sed 's/group = "Fixes"/group = "Bug fixes"/g' "$WORK/dsb.cliff.orig" >"$DSB/cliff.toml"
cmp -s "$WORK/dsb.cliff.orig" "$DSB/cliff.toml" && fail "mutation did not land: no 'group = \"Fixes\"' in cliff.toml"
expect_red cliff-regroup '### Bug fixes' "renaming a group re-renders every released section; CHANGELOG.md must be regenerated with it"
cp "$WORK/dsb.cliff.orig" "$DSB/cliff.toml"
dverify restored-b || { cat "$WORK/dv.restored-b" >&2; fail "drift gate not green after restoring cliff.toml"; }
echo "OK: a cliff.toml change that re-renders released history reds the gate"

echo "== 10d. the pre-release detectors read the WITH-unreleased render =="
# Placed before the tag so the malformed subject is UNRELEASED — the state in
# which it must be caught, before it reaches a Release page.
BAD_SUBJECT='👷 ci(sandbox): a literal-emoji subject that must not reach a Release page'
printf 'bad\n' >>"$DSB/sandbox-ordinary.txt"
dgit add -A && dgit commit -q -m "$BAD_SUBJECT"
render_full --config "$DSB/cliff.toml" --repository "$DSB" -o "$WORK/dsb.full.md" 2>/dev/null \
  || fail "git-cliff failed rendering the sandbox with render_full"
group_lines "$WORK/dsb.full.md" | awk -F'\t' '$1 == "Other" { print $2 }' >"$WORK/dsb.full.other"
LC_ALL=C grep -nE "$OTHER_MAPPABLE_RE" "$WORK/dsb.full.other" >"$WORK/dsb.full.hits" || true
grep -qF -e "- $BAD_SUBJECT" "$WORK/dsb.full.hits" \
  || fail "§8's detector, fed the render §8 uses, does NOT flag an unreleased '$BAD_SUBJECT' — a malformed subject would reach the next Release page unseen"
echo "OK: the detector on render_full flags the unreleased malformed subject"
# The mutation: feed the detector the COMMITTED form instead.
GIT_CLIFF_BIN="$CLIFF" bash "$DSB/hack/release/render-changelog.sh" "$WORK/dsb.committed.md" 2>/dev/null
group_lines "$WORK/dsb.committed.md" | awk -F'\t' '$1 == "Other" { print $2 }' >"$WORK/dsb.committed.other"
if grep -qF -e "- $BAD_SUBJECT" "$WORK/dsb.committed.other"; then
  fail "the committed form already shows the unreleased malformed subject — then D-180's released-only render is not released-only"
fi
echo "OK: mutation — pointed at the committed form (what CHANGELOG.md holds now), the detector is blind to it: the input choice is load-bearing"
ASSENT_CHANGELOG_RELEASED_ONLY=1 render_full --config "$DSB/cliff.toml" --repository "$DSB" -o "$WORK/dsb.leak.md" 2>/dev/null \
  || fail "git-cliff failed rendering with the switch exported"
grep -qxF -e "- $BAD_SUBJECT" "$WORK/dsb.leak.md" \
  || fail "with ASSENT_CHANGELOG_RELEASED_ONLY=1 exported, render_full drops the unreleased entries — every detector in §7/§8 would go blind for anyone with that variable set"
echo "OK: an exported ASSENT_CHANGELOG_RELEASED_ONLY=1 does not blind render_full"
dverify malformed || { cat "$WORK/dv.malformed" >&2; fail "drift gate RED after an unreleased malformed commit — the commit-subject gate and §8 catch it; the drift gate must not"; }
echo "OK: and the drift gate stays green on it — catching it is the detectors' job, not the drift gate's"

echo "== 10c. an unstamped tag: tolerated only AS HEAD and only for its own section (D-181); the stamp commit adds the section =="
dgit tag v99.0.0
# Positive control: the plain released-only render of this state DOES differ
# from the committed file by the [99.0.0] section — without it, the green below
# could just mean "the tag renders nothing".
GIT_CLIFF_BIN="$CLIFF" bash "$DSB/hack/release/render-changelog.sh" "$WORK/dsb.tagged.md" 2>/dev/null
grep -qE '^## \[99\.0\.0\] - ' "$WORK/dsb.tagged.md" \
  || fail "positive control: the released-only render of the tagged sandbox has no [99.0.0] section — the rows below would prove nothing"
cmp -s "$DSB/CHANGELOG.md" "$WORK/dsb.tagged.md" \
  && fail "positive control: the committed CHANGELOG.md already equals the tagged render — the tag is not actually unstamped"
dverify tag-at-head || { cat "$WORK/dv.tag-at-head" >&2; fail "drift gate RED on the tagged-but-unstamped commit itself — every verify run that checks out the tagged SHA after the tag exists (a re-run, the weekly schedule) would red and lock the tag out of release.yaml's verify-green gate (REQ-AUD-S03-01, D-181)"; }
grep -qF 'tolerated (D-181)' "$WORK/dv.tag-at-head" \
  || { cat "$WORK/dv.tag-at-head" >&2; fail "drift gate green on the tagged HEAD but not through the D-181 allowance — it must say so, and name the stamp commit to land"; }
grep -qF "stamp the v99.0.0 section after tagging" "$WORK/dv.tag-at-head" \
  || { cat "$WORK/dv.tag-at-head" >&2; fail "the D-181 message does not name the stamp commit for v99.0.0"; }
GITHUB_ACTIONS=true dverify tag-at-head-ci || { cat "$WORK/dv.tag-at-head-ci" >&2; fail "drift gate RED on the tagged HEAD under GITHUB_ACTIONS"; }
grep -qE '^::warning title=CHANGELOG\.md not stamped::' "$WORK/dv.tag-at-head-ci" \
  || { cat "$WORK/dv.tag-at-head-ci" >&2; fail "under GITHUB_ACTIONS the D-181 allowance does not raise a ::warning:: annotation — a tolerated unstamped tag would pass CI silently"; }
echo "OK: the tagged-but-unstamped HEAD is GREEN through the D-181 allowance, naming the stamp to land (and warning in CI)"

# The allowance must not widen into "anything goes on a tagged HEAD".
printf '\n- a hand-written line\n' >>"$DSB/CHANGELOG.md"
expect_red tagged-hand-edit '- a hand-written line' "a hand edit on a tagged-but-unstamped HEAD is still drift — the D-181 allowance covers the tag's own missing section and nothing else"
cp "$WORK/dsb.changelog.orig" "$DSB/CHANGELOG.md"
sed 's/group = "Fixes"/group = "Bug fixes"/g' "$WORK/dsb.cliff.orig" >"$DSB/cliff.toml"
expect_red tagged-cliff-regroup '### Bug fixes' "a cliff.toml change that re-renders released history is still drift on a tagged HEAD"
cp "$WORK/dsb.cliff.orig" "$DSB/cliff.toml"
dgit diff --quiet || fail "the drift sandbox is not clean after the tagged-HEAD mutations"
echo "OK: on the same tagged HEAD a hand edit and a history re-render are still RED"

printf 't\n' >>"$DSB/sandbox-ordinary.txt"
dgit add -A && dgit commit -q -m ':sparkles: feat(sandbox): an ordinary commit on top of the unstamped tag'
expect_red tag-unstamped '## \[99\.0\.0\]' "once anything lands on an unstamped tag the gate must red, naming the missing section — otherwise the stamp can be forgotten for good"
echo "OK: one commit on top of the unstamped tag reds the gate, naming the missing [99.0.0] section"
dgit tag v99.0.1
expect_red older-tag-unstamped '## \[99\.0\.0\]' "the allowance covers the tags ON HEAD only; an older unstamped tag behind a newer tagged HEAD must stay red"
echo "OK: tagging that commit too does not excuse the older v99.0.0 — the allowance is HEAD's tags only"
dgit tag -d v99.0.1 >/dev/null
dgit reset -q --hard HEAD~1
[[ "$(dgit tag --points-at HEAD)" == "v99.0.0" ]] || fail "the drift sandbox HEAD is not back on v99.0.0 after undoing the rows above"
# Several tags on HEAD (a release and its pre-release on one commit): all are
# ignored together, and the stamp hint names the release, not the -rc.
dgit tag v99.0.0-rc.1
dverify tag-at-head-multi || { cat "$WORK/dv.tag-at-head-multi" >&2; fail "drift gate RED on a HEAD carrying v99.0.0 and v99.0.0-rc.1, both unstamped — every HEAD tag must be ignored together (D-181)"; }
grep -qF "stamp the v99.0.0 section after tagging" "$WORK/dv.tag-at-head-multi" \
  || { cat "$WORK/dv.tag-at-head-multi" >&2; fail "with v99.0.0 and v99.0.0-rc.1 on HEAD the stamp hint does not name v99.0.0"; }
dgit tag -d v99.0.0-rc.1 >/dev/null
echo "OK: a release and its pre-release tag on the same HEAD are tolerated together; the hint names the release"
# Escaping: an older unstamped tag whose name the HEAD tag would match as an
# UNESCAPED regex ('.' matches 'x'). Ignoring it would hide a missed stamp.
dgit tag -d v99.0.0 >/dev/null
dgit tag v99x0x0
printf 'e\n' >>"$DSB/sandbox-ordinary.txt"
dgit add -A && dgit commit -q -m ':sparkles: feat(sandbox): a commit between two tags'
dgit tag v99.0.0
expect_red escaped-regex '## \[99x0x0\]' "the HEAD tag v99.0.0 must be matched literally by --ignore-tags; unescaped it also ignores the older unstamped v99x0x0"
echo "OK: HEAD's tag is matched literally — an older unstamped v99x0x0 that 'v99.0.0' matches as a bare regex stays red"
dgit tag -d v99.0.0 v99x0x0 >/dev/null
dgit reset -q --hard HEAD~1
dgit tag v99.0.0
[[ "$(dgit tag --points-at HEAD)" == "v99.0.0" ]] || fail "the drift sandbox HEAD is not back on v99.0.0 after the escaping row"
dverify tag-at-head-again || { cat "$WORK/dv.tag-at-head-again" >&2; fail "drift gate not green back on the tagged HEAD — the rows below would start from a red state"; }
# The Release body at this moment — rendered with release.yaml's own args — is
# already right: it reads the tag, not CHANGELOG.md (which lacks the section).
render_full --config "$DSB/cliff.toml" --repository "$DSB" "${cliff_argv[@]}" >"$WORK/dsb.release-body.md" 2>/dev/null \
  || fail "git-cliff failed rendering the sandbox release body"
grep -qE '^## \[99\.0\.0\] - ' "$WORK/dsb.release-body.md" \
  || fail "the Release body for the fresh tag has no '## [99.0.0]' section — release.yaml's '$cliff_args' does not render the new tag"
grep -qxF -e "- ${ORDINARY_SUBJECTS[0]}" "$WORK/dsb.release-body.md" \
  || fail "the Release body for v99.0.0 does not carry the feature landed before the tag"
grep -qE '^## \[99\.0\.0\]' "$DSB/CHANGELOG.md" \
  && fail "the sandbox CHANGELOG.md already has [99.0.0] before the stamp — the row cannot show that the body comes from the tag"
echo "OK: release.yaml's '$cliff_args' renders the v99.0.0 body from the tag while CHANGELOG.md does not have the section yet"
# The stamp: exactly what `task changelog-write` runs, committed with the skipped subject.
GIT_CLIFF_BIN="$CLIFF" bash "$DSB/hack/release/render-changelog.sh" "$DSB/CHANGELOG.md" 2>/dev/null
grep -qE '^## \[99\.0\.0\] - ' "$DSB/CHANGELOG.md" || fail "the stamp did not add the [99.0.0] section"
grep -qxF -e "- ${ORDINARY_SUBJECTS[0]}" "$DSB/CHANGELOG.md" || fail "the stamped [99.0.0] section does not carry the feature landed before the tag"
grep -qE '^## Unreleased$' "$DSB/CHANGELOG.md" && fail "the stamp wrote an Unreleased section"
dgit add -A && dgit commit -q -m ':memo: chore(release): stamp the v99.0.0 section after tagging'
dverify stamped || { cat "$WORK/dv.stamped" >&2; fail "drift gate RED straight after the stamp commit — the stamp should need no amend"; }
render_full --config "$DSB/cliff.toml" --repository "$DSB" --unreleased -o "$WORK/dsb.after-stamp.md" 2>/dev/null
grep -qF 'stamp the v99.0.0 section' "$WORK/dsb.after-stamp.md" \
  && fail "the stamp commit renders in the next release's preview — its ':memo: chore(release):' subject must be cliff-skipped"
echo "OK: the stamp commit adds [99.0.0], is green with no amend, and (cliff-skipped) stays out of the next release's notes"
printf 'y\n' >>"$DSB/sandbox-ordinary.txt"
dgit add -A && dgit commit -q -m ':sparkles: feat(sandbox): the first ordinary commit after the release'
dverify after-release || { cat "$WORK/dv.after-release" >&2; fail "drift gate RED on the first ordinary commit after a stamped release"; }
echo "OK: the next ordinary commit after the release stays green"

# 10e — the whole of THIS script on a tagged-but-unstamped commit (D-181, review
# F1). CI runs this script on every non-PR verify run (release-exitgate →
# hack/audit/exitgate_test.sh → `task check`), and release.yaml's tag gate needs
# every verify run on the tag SHA green — so a check here that reds on the tagged
# commit (§1's newest-section assertion, the §10 baseline's overlay commit)
# locks the tag out just as the drift gate used to, and §10c alone, which runs
# only verify-changelog.sh, cannot see it. Nested once: the nested run skips 10e.
if [[ -z "${ASSENT_CHANGELOG_GATE_NESTED:-}" ]]; then
  echo "== 10e. this whole script is GREEN on a tagged-but-unstamped commit, RED once a commit lands on it =="
  TSB="$WORK/tagged-tree"
  tgit() {
    env -u GIT_DIR -u GIT_WORK_TREE -u GIT_INDEX_FILE -u GIT_OBJECT_DIRECTORY \
      -u GIT_ALTERNATE_OBJECT_DIRECTORIES -u GIT_COMMON_DIR \
      git -C "$TSB" \
      -c user.name='changelog gate' -c user.email='gate@example.invalid' \
      -c commit.gpgsign=false -c core.hooksPath=/dev/null \
      -c advice.detachedHead=false "$@"
  }
  env -u GIT_DIR -u GIT_WORK_TREE -u GIT_INDEX_FILE -u GIT_OBJECT_DIRECTORY \
    -u GIT_ALTERNATE_OBJECT_DIRECTORIES -u GIT_COMMON_DIR \
    git clone -q --no-checkout "$ROOT" "$TSB" >/dev/null 2>&1 || fail "could not clone this repository into the tagged-tree sandbox"
  tsb_expected="$(cd "$TSB" && pwd -P)"
  [[ "$(cd "$(tgit rev-parse --show-toplevel)" && pwd -P)" == "$tsb_expected" ]] \
    || fail "tagged-tree sandbox git commands resolve outside '$tsb_expected'"
  tgit checkout -q -B tagged "$head_sha" || fail "could not check out HEAD ($head_sha) in the tagged-tree sandbox"
  # Same rewind as the §10 sandbox, so a real unstamped tag on HEAD does not
  # sit under the overlay commit.
  while IFS= read -r t; do
    [[ -n "$t" ]] || continue
    grep -qE "^## \[${t#v}\] - " "$ROOT/CHANGELOG.md" && continue
    tgit tag -d "$t" >/dev/null
  done < <(tgit tag --points-at "$head_sha" --list 'v[0-9]*')
  # Overlay every tracked file as it is in the WORKING TREE, so the nested run
  # grades the tree under test.
  (cd "$ROOT" && git ls-files -z | tar -cf - --null -T -) | tar -xf - -C "$TSB" \
    || fail "could not overlay the working tree into the tagged-tree sandbox"
  # NOT a cliff-skipped subject (review N1): if every commit since the newest
  # real tag is skipped — as it is right after every stamp commit — a skipped
  # overlay would leave v98.0.0 an empty release, which git-cliff omits, and
  # the row would test nothing (or red for the wrong reason). The positive
  # control below pins that v98.0.0 renders.
  tgit add -A && tgit commit -q --allow-empty -m ':sparkles: feat(sandbox): tagged-tree sandbox overlay' \
    || fail "could not commit the overlay in the tagged-tree sandbox"
  tgit tag v98.0.0
  GIT_CLIFF_BIN="$CLIFF" bash "$TSB/hack/release/render-changelog.sh" "$WORK/tsb.tagged.md" 2>/dev/null \
    || fail "git-cliff failed rendering the tagged-tree sandbox"
  grep -qE '^## \[98\.0\.0\] - ' "$WORK/tsb.tagged.md" \
    || fail "positive control: v98.0.0 renders no section in the tagged-tree sandbox — it is not an unstamped release, so the nested run would prove nothing"
  cmp -s "$TSB/CHANGELOG.md" "$WORK/tsb.tagged.md" \
    && fail "positive control: the tagged-tree CHANGELOG.md already equals the tagged render — v98.0.0 is not unstamped"
  ASSENT_CHANGELOG_GATE_NESTED=1 GIT_CLIFF_BIN="$CLIFF" env -u ASSENT_CHANGELOG_RELEASED_ONLY \
    bash "$TSB/hack/release/changelog_gate_test.sh" >"$WORK/nested.tagged" 2>&1 \
    || { tail -40 "$WORK/nested.tagged" >&2; fail "this script is RED on a tagged-but-unstamped commit — in CI that is a red release-exitgate on the tag SHA, which locks the tag out of release.yaml (REQ-AUD-S03-01, D-181)"; }
  grep -qF 'newest tag v98.0.0 is HEAD and unstamped — tolerated (D-181)' "$WORK/nested.tagged" \
    || { tail -40 "$WORK/nested.tagged" >&2; fail "the nested run is green but §1 did not take the D-181 path — the sandbox is not the tagged-unstamped state this row claims"; }
  grep -qF 'drops it to start from the pre-tag state (D-181)' "$WORK/nested.tagged" \
    || { tail -40 "$WORK/nested.tagged" >&2; fail "the nested run is green but the §10 sandbox did not rewind the unstamped HEAD tag — the row did not exercise the baseline fix"; }
  echo "OK: the whole changelog gate is GREEN on a tagged-but-unstamped commit (§1 warns, §10 rewinds) — a CI run on the tag SHA cannot lock the release"
  printf 'n\n' >>"$TSB/sandbox-nested.txt"
  tgit add -A && tgit commit -q -m ':sparkles: feat(sandbox): an ordinary commit on top of the unstamped tag'
  if ASSENT_CHANGELOG_GATE_NESTED=1 GIT_CLIFF_BIN="$CLIFF" env -u ASSENT_CHANGELOG_RELEASED_ONLY \
    bash "$TSB/hack/release/changelog_gate_test.sh" >"$WORK/nested.ontop" 2>&1; then
    fail "this script is GREEN with a commit on top of an unstamped tag — the stamp could then be forgotten for good"
  fi
  grep -qF 'the post-tag stamp commit is missing' "$WORK/nested.ontop" \
    || { tail -20 "$WORK/nested.ontop" >&2; fail "the nested run is red with a commit on top of the unstamped tag, but not for the missing stamp"; }
  echo "OK: and RED once a commit lands on the unstamped tag, naming the missing stamp"
  # The two states after the release, graded by §1's function directly (cheap).
  tgit reset -q --hard HEAD~1
  GIT_CLIFF_BIN="$CLIFF" bash "$TSB/hack/release/render-changelog.sh" "$TSB/CHANGELOG.md" 2>/dev/null
  tgit add -A && tgit commit -q -m ':memo: chore(release): stamp the v98.0.0 section after tagging'
  GIT_CLIFF_BIN="$CLIFF" newest_section_check "$TSB" stamped >"$WORK/tsb.stamped" \
    || { cat "$WORK/tsb.stamped" >&2; fail "§1 is RED on the stamp commit — main would go red after every release (review N1)"; }
  grep -qF 'OK: newest section is the newest tag (v98.0.0)' "$WORK/tsb.stamped" \
    || { cat "$WORK/tsb.stamped" >&2; fail "§1 green on the stamp commit but not because the stamped section is the newest"; }
  echo "OK: the stamp commit is green in §1, the newest section being the stamped v98.0.0"
  printf 'w\n' >>"$TSB/sandbox-nested.txt"
  tgit add -A && tgit commit -q -m ':memo: chore(release): a tooling-only change'
  tgit tag v98.0.1
  GIT_CLIFF_BIN="$CLIFF" bash "$TSB/hack/release/render-changelog.sh" "$WORK/tsb.empty.md" 2>/dev/null
  grep -qE '^## \[98\.0\.1\]' "$WORK/tsb.empty.md" \
    && fail "positive control: v98.0.1 (cliff-skipped commits only) renders a section — the row below would not be the empty-release case"
  GIT_CLIFF_BIN="$CLIFF" newest_section_check "$TSB" empty-release >"$WORK/tsb.empty" \
    || { cat "$WORK/tsb.empty" >&2; fail "§1 is RED on a release whose commits are all cliff-skipped — it never gets a section, so this would red the tag SHA and main forever (review N2)"; }
  grep -qF 'renders no section' "$WORK/tsb.empty" \
    || { cat "$WORK/tsb.empty" >&2; fail "§1 green on the empty release but not through the renders-no-section path"; }
  echo "OK: a release of cliff-skipped commits only renders no section and §1 accepts it — nothing to stamp"
fi

echo "PASS: changelog drift gate regenerated, wired into task check + verify.yaml, and proven at both polarities (REQ-AUD-S02-01/02); release body carries the compatibility notes and no merge subject (D-136); every fileable subject reaches its real group (REL-14 / D-137) behind any prefix shape, a literal-emoji commit subject is rejected by a gate rather than by a human (REDMAIN-N1/N2 / D-168); and CHANGELOG.md holds released versions only, so the drift gate reds on a hand edit, a history-re-rendering cliff.toml change or an unstamped tag with anything on top of it, and never on an ordinary commit (D-180) nor on the tagged commit itself before its stamp (D-181)"
