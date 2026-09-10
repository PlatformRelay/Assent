#!/usr/bin/env bash
# render-changelog.sh <output-path> — render the COMMITTED form of CHANGELOG.md.
#
# D-180: the committed CHANGELOG.md carries RELEASED versions only — the header
# (with the compatibility notes), one `## [x.y.z]` section per tag, and the
# footer. The `## Unreleased` section is never committed: it is what
# `task changelog` previews. This script is the ONE definition of "the committed
# form": `task changelog-write` writes it to CHANGELOG.md and
# hack/release/verify-changelog.sh renders it to a scratch file and diffs, so
# the two can never disagree about what belongs in the file.
#
# Why this ends the churn: the committed file is a function of the TAGS and
# cliff.toml, not of HEAD, so an ordinary commit — a lane's, or a bot's that
# cannot run `task changelog-write` on its own branch — can no longer make it
# stale. It goes stale only on a hand edit, a cliff.toml change that re-renders
# released history, or a new tag whose section has not been stamped yet.
#
# Mechanism: cliff.toml's body template skips the unreleased release when
# ASSENT_CHANGELOG_RELEASED_ONLY=1. The variable's DEFAULT is "render
# everything", on purpose: every other git-cliff call (the preview, the release
# body, the pre-release quality detectors in changelog_gate_test.sh §7/§8) must
# see the unreleased entries, and a forgotten switch there fails CLOSED (an
# `## Unreleased` block in a committed file reds the drift gate) rather than
# silently blinding a detector.
set -euo pipefail

out="${1:?usage: render-changelog.sh <output-path>}"
case "$out" in
/*) ;;
*) out="$PWD/$out" ;;
esac

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "${ROOT}"

CLIFF="${GIT_CLIFF_BIN:-${ROOT}/bin/git-cliff}"
if [[ ! -x "${CLIFF}" ]]; then
  echo "render-changelog: installing git-cliff into bin/" >&2
  bash hack/install-git-cliff.sh v2.13.1 bin/git-cliff
  CLIFF="${ROOT}/bin/git-cliff"
fi

ASSENT_CHANGELOG_RELEASED_ONLY=1 "${CLIFF}" --config "${CLIFF_CONFIG:-cliff.toml}" -o "${out}"
