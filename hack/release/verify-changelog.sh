#!/usr/bin/env bash
# REQ-E9-S03-03: fail closed when CHANGELOG.md drifts from cliff.toml output.
#
# D-180: compares against the COMMITTED form rendered by render-changelog.sh —
# released versions only, no `## Unreleased` section. An ordinary commit
# therefore never makes this red. What does:
#   * a hand edit of CHANGELOG.md;
#   * a cliff.toml change that re-renders released history (header, grouping,
#     template);
#   * a release tag whose section has not been stamped yet, once anything else
#     has landed on top of it — the post-tag `:memo: chore(release): stamp the
#     vX.Y.Z section after tagging` commit (`task changelog-write`) adds it.
#
# D-181: the ONE tolerated drift is "HEAD is itself a release tag and the only
# difference is that tag's missing section". That is the state of the tagged
# commit between `git push origin vX.Y.Z` and the stamp commit, and it is the
# same bytes that were green before the tag existed. Without the allowance, any
# verify run that checks out the tagged SHA after the tag exists (a re-run, the
# weekly schedule on an unchanged main) reds, and release.yaml's verify-green
# tag gate (REQ-AUD-S03-01) then locks that tag out of its release job for good.
# The allowance is proved, not assumed: CHANGELOG.md must equal the render with
# exactly the HEAD tags ignored, so a hand edit or a cliff.toml re-render on a
# tagged HEAD is still red, and so is an unstamped tag with any commit after it.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT}"

if [[ ! -f CHANGELOG.md ]]; then
  echo "verify-changelog: CHANGELOG.md missing — run 'task changelog-write' and commit" >&2
  exit 1
fi

scratch="$(mktemp)"
pretag="$(mktemp)"
trap 'rm -f "${scratch}" "${pretag}"' EXIT

bash hack/release/render-changelog.sh "${scratch}"

if cmp -s CHANGELOG.md "${scratch}"; then
  echo "verify-changelog: ok"
  exit 0
fi

# D-181 — release tags on HEAD itself (same glob as cliff.toml's tag_pattern and
# changelog_gate_test.sh §1). Tags further back are never tolerated.
head_tags=""
ignore_re=""
last_tag=""
while IFS= read -r t; do
  [[ -n "${t}" ]] || continue
  head_tags+="${head_tags:+ }${t}"
  # Escape the regex metacharacters a refname may carry (git forbids ^ ~ : ? * [ \).
  e="${t}"
  for c in . + '(' ')' '{' '}' '|' '$'; do
    e="${e//"${c}"/\\${c}}"
  done
  ignore_re+="${ignore_re:+|}${e}"
  last_tag="${t}"
done < <(git tag --points-at HEAD --list 'v[0-9]*')
if [[ -n "${head_tags}" ]]; then
  bash hack/release/render-changelog.sh "${pretag}" --ignore-tags "^(${ignore_re})\$"
  if cmp -s CHANGELOG.md "${pretag}"; then
    msg="HEAD is tagged ${head_tags} and CHANGELOG.md lacks only that section — tolerated (D-181); land the stamp commit next: 'task changelog-write' and commit it as ':memo: chore(release): stamp the ${last_tag} section after tagging'"
    if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
      echo "::warning title=CHANGELOG.md not stamped::${msg}"
    fi
    echo "verify-changelog: ok — ${msg}"
    exit 0
  fi
fi

diff -u CHANGELOG.md "${scratch}" || true
echo "verify-changelog: CHANGELOG.md drift — it holds released versions only (D-180), so this means a hand edit, a cliff.toml change that re-renders released history, or a release tag not yet stamped with commits already on top of it: run 'task changelog-write' and commit it as ':memo: chore(release): …'" >&2
exit 1
