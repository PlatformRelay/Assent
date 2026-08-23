#!/usr/bin/env bash
# require-bash.sh — a gate script's bash version floor, asserted before it can run.
#
# WHY THIS EXISTS (D-154). Stock macOS /bin/bash is 3.2.57. It has no associative
# arrays and no `mapfile`, and the way it fails differs by construct:
#
#   declare -A x=([k]=v)  under `set -u`  ->  3.2 parses the literal as an INDEXED
#                                             array assignment and evaluates the
#                                             subscript `k` ARITHMETICALLY, hitting
#                                             "unbound variable". The shell
#                                             terminates the script — and the exit
#                                             status is 0. Every caller reads that
#                                             as a green gate. This is a FAIL-OPEN.
#   declare -A x=()                       ->  no subscript to evaluate, so it
#                                             degrades to `declare: -A: invalid
#                                             option` (status 2). That fails closed
#                                             only because the script happens to
#                                             run `set -e`.
#   mapfile / readarray / local -n        ->  "command not found". Fails closed.
#
# Only the first shape is silent, but "fails closed for an incidental reason" is
# not a guarantee: dropping `set -e`, or populating an empty literal, converts one
# into the other with no visible signal. So every hack/** script that uses a bash
# 4+ feature declares its own floor here instead of relying on how its particular
# construct happens to degrade. hack/lint/bash_version_guard_test.sh enforces that.
#
# The floor is a PARAMETER, never a blanket: `declare -A` needs 4.0, `local -n`
# needs 4.3, `mapfile -d ''` needs 4.4. A single repo-wide floor would either
# refuse shells that are perfectly capable of running a given gate, or admit
# shells that are not.
#
# THIS FILE IS BASH-3.2-SAFE BY CONSTRUCTION AND MUST STAY THAT WAY. It is sourced
# by scripts whose entire job under 3.2 is to REFUSE cleanly; a bash 4 feature in
# here would reinstate the defect inside the fix. It uses only POSIX-shell `[`
# tests and `${var%%…}` / `${var#…}` expansions, all of which 3.2 has.
#
# USAGE — at the very top of the gate, after `set …` and before anything else:
#
#   _assent_lib="$(cd "$(dirname "${BASH_SOURCE[0]}")/../lib" 2>/dev/null && pwd || true)"
#   [ -n "$_assent_lib" ] || _assent_lib="$(cd "$(git rev-parse --show-toplevel 2>/dev/null)/hack/lib" 2>/dev/null && pwd || true)"
#   [ -r "${_assent_lib}/require-bash.sh" ] || { echo "FAIL: …" >&2; exit 1; }
#   # shellcheck source=hack/lib/require-bash.sh
#   . "${_assent_lib}/require-bash.sh" || exit 1
#   require_bash 4.0 "declare -A (associative arrays)" || exit 1
#
# Two resolution steps, deliberately, and NO environment override: other gates
# execute mutated COPIES of these scripts out of a mktemp dir (see
# hack/release/install_cosign_pin_test.sh §5e-5g), where script-relative
# resolution cannot work and the enclosing git checkout is the only anchor left.
# An env var pointing at an arbitrary directory would be a disarm switch — a
# `require_bash() { return 0; }` on that path turns every guard off — which is the
# `--text-only` defect class this repo already pins against. An unresolvable lib
# means REFUSE; it never means proceed unguarded.
#
# The `|| exit 1` is required on the source and require_bash lines and is pinned
# by the lint gate. Neither is decoration:
#   - callers such as hack/docs/truthlag_pins_test.sh run `set -uo pipefail` with
#     NO `-e`, so a failed `.` would merely return non-zero and execution would
#     carry straight on into the very construct being guarded;
#   - if the file loaded but `require_bash` were undefined, the call would return
#     127 (command not found) and, again without `-e`, continue. `|| exit 1` turns
#     both of those into a closed failure.
#
# require_bash RETURNS non-zero rather than calling `exit`, on purpose: the caller
# owns its exit status, and returning keeps the function exercisable from a
# harness that has to survive the negative case.

# require_bash <major>[.<minor>] <feature description>
#   0 — this bash is new enough
#   1 — too old (a refusal naming the feature is printed to stderr), or bad args
require_bash() {
  local want="${1:-}" feature="${2:-}" self want_major want_minor
  self="${BASH_SOURCE[1]:-$0}"

  if [ -z "$want" ] || [ -z "$feature" ]; then
    echo "FAIL: require_bash needs <major>[.<minor>] and a feature description; got '${want}' '${feature}'." >&2
    return 1
  fi

  want_major="${want%%.*}"
  if [ "$want" = "$want_major" ]; then
    want_minor=0
  else
    want_minor="${want#*.}"
  fi
  case "${want_major}" in '' | *[!0-9]*)
    echo "FAIL: require_bash: '${want}' is not <major>[.<minor>]." >&2
    return 1
    ;;
  esac
  case "${want_minor}" in '' | *[!0-9]*)
    echo "FAIL: require_bash: '${want}' is not <major>[.<minor>]." >&2
    return 1
    ;;
  esac

  if [ "${BASH_VERSINFO[0]}" -gt "$want_major" ] ||
    { [ "${BASH_VERSINFO[0]}" -eq "$want_major" ] && [ "${BASH_VERSINFO[1]}" -ge "$want_minor" ]; }; then
    return 0
  fi

  echo "FAIL: ${self} requires bash >= ${want_major}.${want_minor} (${feature}); this is bash ${BASH_VERSION}." >&2
  echo "  Refusing to run rather than aborting mid-way and exiting 0 — a gate that certifies nothing must say so." >&2
  echo "  Stock macOS /bin/bash is 3.2: under 'set -u' a populated 'declare -A x=([k]=v)' dies with an" >&2
  echo "  unbound-variable error and an exit status of 0, which every caller reads as a PASS (D-154)." >&2
  echo "  On macOS: run it under a modern bash (e.g. \`brew install bash\`), which is what 'task check' and CI use." >&2
  return 1
}
