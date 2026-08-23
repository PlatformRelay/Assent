#!/usr/bin/env bash
# hack/lib/pr_reach.sh — shared "does this gate actually run on a pull request?"
# helper for the PR-visible text gates (D-157 / GATES3).
#
# WHY THIS FILE EXISTS. Three gates —
#   hack/lint/workflow_pins_test.sh
#   hack/audit/aud2_exitgate_test.sh
#   hack/examples/dogfood_wiring_test.sh
# — each independently answered "am I reachable on a PR?", and two of them
# answered it with `grep -qE '^[[:space:]]+pull_request:'` over the workflow's
# `on:` block. (The third, workflow_pins_test.sh, did not answer it at all: it
# asserted its step exists and is undisarmed, and never that the workflow
# carrying the step reaches a pull request. That is new coverage here, not a
# strengthening.) An independent reviewer built the mutant that defeats the
# grep and measured rc=0:
#
#     on:
#       pull_request:
#         paths: ['internal/**']
#
# The trigger is present, the grep is satisfied, and a PR that touches only
# Taskfile.yml or .github/workflows/** — the very files these gates protect —
# never runs them. `types: [closed]` is the same defect by another route.
#
# Fixing that in one gate would have left two copies of the weak check for the
# next gate author to copy. So the check lives HERE, once, and the next gate
# inherits the strong form. Same reasoning for the step-wiring check
# (assent_step_wired): hack/examples/dogfood_wiring_test.sh had grown an
# anchored `run:`-command match while hack/audit/aud2_exitgate_test.sh still
# used a fixed-string search for the script NAME, which a reviewer proved is
# satisfied by a mere comment (it still went red, but with the wrong finding —
# "WITH ARGUMENTS" for a step whose command had been commented out).
#
# CONTRACT: this file is SOURCED, never executed. Every function returns a
# distinct numeric code and prints NOTHING. The caller owns the wording,
# because each caller's mutation controls pin their own stderr fragments.
#
# PORTABILITY RULES, inherited from hack/lint/workflow_pins_test.sh's preamble
# because that gate runs first in CI, before any toolchain, and must work in a
# bare `debian:stable-slim`:
#   * bash 3.2 clean — no `declare -A`, no `mapfile`, no namerefs. Stock macOS
#     /bin/bash is 3.2; hack/audit/aud2_exitgate_test.sh refuses to run there,
#     but the other two callers do not, and a bash-4 construct here would
#     silently weaken them on a maintainer's machine.
#   * portable ERE only: no `\t`, `\s`, `\b`, no `grep -P`.
#   * no `git`, no `sed -i`, no `grep -q` on the read end of a pipe (SIGPIPE
#     under `set -o pipefail`); matches go to a file, the file is inspected.
#   * `set -e` safe: no bare `[[ ... ]] && return N` tails.
#
# WHAT THIS IS NOT. It is a line-oriented reader with a structural bias, not a
# YAML parser. It understands block mappings at 2/4/6-space indent, block
# sequences, and inline flow SEQUENCES. It does NOT understand anchors, aliases,
# merge keys, or flow MAPPINGS, and it refuses (fails closed, with a distinct
# code) rather than guessing when it meets one. Stated here rather than
# discovered later: an honest narrow gate beats an overreaching one.

# The three PR-visible text gates, single-sourced so each can cross-pin the
# others (D-157 residual 2: before this, each gate pinned only its OWN step, so
# a PR deleting one of the other two was invisible to the gate that ran).
ASSENT_PR_GATES=(
  "hack/lint/workflow_pins_test.sh"
  "hack/audit/aud2_exitgate_test.sh"
  "hack/examples/dogfood_wiring_test.sh"
)
ASSENT_PR_WORKFLOW=".github/workflows/verify.yaml"
ASSENT_PR_JOB="verify"

# assent_pr_gate_others <self-rel-path> — prints ASSENT_PR_GATES minus <self>,
# one per line. Returns 1 if <self> is not a member, which is the positive
# control every caller runs: a renamed gate must not silently drop out of the
# cross-pin set and leave the caller asserting over an unrelated pair.
assent_pr_gate_others() {
  local self="$1" g found=0
  for g in "${ASSENT_PR_GATES[@]}"; do
    if [[ "$g" == "$self" ]]; then
      found=1
    else
      printf '%s\n' "$g"
    fi
  done
  if ((found != 1)); then
    return 1
  fi
  return 0
}

# _assent_ere_escape <literal> — the literal as an ERE. `.` in `.sh` must not be
# a wildcard; a path is otherwise metachar-free, but the full set is escaped so
# a future gate name carrying a `+` or `(` cannot silently widen a match.
_assent_ere_escape() {
  printf '%s' "$1" | sed 's/[][\.*+?(){}|^$]/\\&/g'
}

# _assent_strip_comment — drop a ` #` comment tail from stdin, line by line.
# That is YAML's rule for a PLAIN scalar. It is NOT the rule inside a quoted or
# block scalar (hack/lint/workflow_pins_test.sh's N6 note), which is why it is
# applied ONLY to `on:`/`types:`/`branches:` values, where a quoted ` #` would
# be absurd. Its effect on the membership tests is asymmetric and both
# directions are safe: for `types:`/`branches:` (must CONTAIN a token) stripping
# removes a way to satisfy the check with prose; for `branches-ignore:` (must
# NOT contain `main`) a leftover comment can only cause a false RED.
_assent_strip_comment() {
  sed -e 's/[[:space:]]#.*$//' -e 's/^#.*$//'
}

# ---------------------------------------------------------------------------
# assent_pr_reach <workflow-file> <scratch-dir>
#
# 0  the workflow runs on EVERY pull request, unfiltered by content
# 2  no top-level `pull_request:` trigger (absent, or `on:` unextractable)
# 10 the `on:` block uses a flow MAPPING, an anchor/alias, or another shape this
#    reader cannot honestly evaluate — refused rather than guessed (fail closed)
# 11 the `pull_request:` trigger carries `paths:` or `paths-ignore:` — the gate
#    is disarmed for PRs that do not touch the listed paths
# 12 the `pull_request:` trigger carries a `types:` filter that omits one of
#    GitHub's defaults (opened, synchronize, reopened) — e.g. `types: [closed]`
# 13 the `pull_request:` trigger carries a branch filter that excludes `main`
#
# DELIBERATELY ACCEPTED (legitimate shapes this must not red on):
#   * `on: pull_request` and `on: [push, pull_request]` — the inline forms
#     cannot carry a filter at all, so they are strictly SAFER than the block
#     form and refusing them would red an honest refactor.
#   * `"on":` / `'on':` — YAML 1.1 makes bare `on` a boolean and some linters
#     ask for the quoted key.
#   * `types:` that is a SUPERSET of the defaults (e.g. + ready_for_review).
#   * `branches:` that lists the literal `main` alongside anything else.
#   * `push:`/`schedule:` triggers carrying whatever filters they like — only
#     the `pull_request:` trigger is graded, because only it decides PR reach.
# DELIBERATE NARROWING, stated: a `branches:` GLOB that covers main without the
# literal token (`branches: ['ma*']`) reds. A bash gate cannot honestly evaluate
# GitHub's glob semantics, and refusing is the fail-closed direction.
# ---------------------------------------------------------------------------
assent_pr_reach() {
  local wf="$1" work="$2"
  if [[ ! -f "$wf" ]]; then
    return 2
  fi

  # --- the `on:` key itself, and any INLINE value it carries -----------------
  local onhits="$work/pr_reach.on_line"
  grep -nE "^(on|\"on\"|'on'):" "$wf" >"$onhits" || true
  if [[ ! -s "$onhits" ]]; then
    return 2
  fi
  local onlineno onvalue
  onlineno="$(head -1 "$onhits" | cut -d: -f1)"
  onvalue="$(head -1 "$onhits" | cut -d: -f3-)"
  onvalue="$(printf '%s' "$onvalue" | _assent_strip_comment | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"

  if [[ -n "$onvalue" ]]; then
    case "$onvalue" in
      '{'*)
        # `on: {pull_request: {paths: [...]}}` — a flow mapping CAN hide a
        # filter, and this reader cannot see into it. Refuse.
        return 10
        ;;
      '['*)
        # A flow SEQUENCE of event names. It cannot carry paths/types/branches,
        # so membership is the whole question.
        local seq
        seq="$(printf '%s' "$onvalue" | tr -c 'A-Za-z0-9_' '\n' | grep -x 'pull_request' || true)"
        if [[ -n "$seq" ]]; then
          return 0
        fi
        return 2
        ;;
      '|'* | '>'* | '&'* | '*'*)
        # Block scalar, anchor or alias in the `on:` value — not evaluable.
        return 10
        ;;
      pull_request)
        return 0
        ;;
      *)
        # A single event name that is not pull_request (`on: push`).
        return 2
        ;;
    esac
  fi

  # --- block-mapping form: the body of `on:` ---------------------------------
  # Everything after the `on:` line until the next COLUMN-0 key. A column-0
  # comment does not end the block: ending on one would be the fail-OPEN
  # direction, because a `paths:` below it would go unseen.
  local onblk="$work/pr_reach.on_block"
  awk -v start="$onlineno" 'NR > start { if ($0 ~ /^[^[:space:]#]/) exit; print }' "$wf" >"$onblk"
  if [[ ! -s "$onblk" ]]; then
    return 2
  fi

  # An anchor, alias or merge key anywhere in the trigger block means the
  # effective value is not what these lines say. Refuse rather than grade a
  # fiction.
  local anchors="$work/pr_reach.anchors"
  grep -nE '^[[:space:]]+(<<:|[A-Za-z0-9_-]+:[[:space:]]*[&*])' "$onblk" >"$anchors" || true
  if [[ -s "$anchors" ]]; then
    return 10
  fi

  # The trigger must be a key of `on:` ITSELF — exactly 2-space indent. A
  # `pull_request:` nested deeper (a `workflow_call:` input named
  # `pull_request`, an `inputs:` key, a `paths:` entry) satisfied the old
  # `^[[:space:]]+pull_request:` grep while reaching no pull request at all.
  local prhits="$work/pr_reach.pr_line"
  grep -nE '^  pull_request:' "$onblk" >"$prhits" || true
  if [[ ! -s "$prhits" ]]; then
    return 2
  fi
  local prlineno prvalue
  prlineno="$(head -1 "$prhits" | cut -d: -f1)"
  prvalue="$(head -1 "$prhits" | cut -d: -f3-)"
  prvalue="$(printf '%s' "$prvalue" | _assent_strip_comment | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
  if [[ -n "$prvalue" ]]; then
    # `pull_request: {paths: [...]}` or `pull_request: &anchor`. Not evaluable.
    return 10
  fi

  # --- the trigger's own filters --------------------------------------------
  local prblk="$work/pr_reach.pr_block"
  awk -v start="$prlineno" 'NR > start { if ($0 ~ /^  [^[:space:]#]/) exit; print }' "$onblk" >"$prblk"

  # No sub-block at all (`pull_request:` with nothing under it) is the
  # unfiltered default, and is the shape verify.yaml has today.
  if [[ ! -s "$prblk" ]]; then
    return 0
  fi

  local pathhits="$work/pr_reach.paths"
  grep -nE '^    (paths|paths-ignore):' "$prblk" >"$pathhits" || true
  if [[ -s "$pathhits" ]]; then
    return 11
  fi

  local typehits="$work/pr_reach.types"
  grep -nE '^    types:' "$prblk" >"$typehits" || true
  if [[ -s "$typehits" ]]; then
    local toks="$work/pr_reach.types.tok" t
    _assent_key_tokens "$prblk" types >"$toks"
    for t in opened synchronize reopened; do
      if ! grep -qx "$t" "$toks"; then
        return 12
      fi
    done
  fi

  local brhits="$work/pr_reach.branches"
  grep -nE '^    branches:' "$prblk" >"$brhits" || true
  if [[ -s "$brhits" ]]; then
    local btoks="$work/pr_reach.branches.tok"
    _assent_key_tokens "$prblk" branches >"$btoks"
    if ! grep -qx 'main' "$btoks"; then
      return 13
    fi
  fi

  local bihits="$work/pr_reach.branches_ignore"
  grep -nE '^    branches-ignore:' "$prblk" >"$bihits" || true
  if [[ -s "$bihits" ]]; then
    local bitoks="$work/pr_reach.branches_ignore.tok"
    _assent_key_tokens "$prblk" branches-ignore >"$bitoks"
    if grep -qx 'main' "$bitoks"; then
      return 13
    fi
  fi

  return 0
}

# _assent_key_tokens <pull_request-block> <key> — every token of a 4-space
# `key:` whose value is an inline flow sequence, a block sequence, or a plain
# scalar. One token per line, quotes and punctuation removed.
_assent_key_tokens() {
  local blk="$1" key="$2"
  awk -v key="$key" '
    BEGIN { re = "^    " key ":" }
    $0 ~ re { ink = 1; v = $0; sub(re, "", v); print v; next }
    ink && /^    [^[:space:]#]/ { ink = 0 }
    ink { print }
  ' "$blk" \
    | _assent_strip_comment \
    | tr -c 'A-Za-z0-9_' '\n' \
    | grep -v '^$' || true
}

# ---------------------------------------------------------------------------
# assent_step_wired <workflow-file> <job> <script-rel-path> <scratch-dir>
#
# 0  the job runs the script as a real command, argument-free and undisarmed
# 3  the job block extracted empty, or has no `steps:` key
# 4  the job carries a JOB-LEVEL `if:` — release-exitgate's push-only guard is
#    exactly this, and it takes every step in the job with it
# 5  no `run:` COMMAND invokes the script: deleted, commented out, or surviving
#    only as a comment tail beside a live no-op. THIS is the code the fixed-
#    string check in hack/audit/aud2_exitgate_test.sh could not reach — it
#    reported "WITH ARGUMENTS" instead, because the step's comments still
#    named the script (D-157 residual 3, measured by the reviewer).
# 6  the step could not be isolated: not exactly one `run:` key, or a `uses:`
#    key inside it. Both mean the region is not one step — the shape you get by
#    deleting a step's `- name:` line, which merges it into its neighbour.
# 7  the script is invoked WITH ARGUMENTS — how a wired-looking gate is
#    hollowed out (`--text-only` skips aud2's behavioural runs entirely)
# 8  the step is present but DISARMED with `if:` or `continue-on-error:`
# 9  the job carries a JOB-LEVEL `needs:` — a dependency that skips on pull
#    requests skips this job too, so the gate becomes push-only by proxy while
#    every other assertion here stays green
#
# DELIBERATELY ACCEPTED: a block-scalar invocation (`run: |` then
# `bash <script>`) — but ONLY when that is the block's sole command; a bare
# invocation smuggled into some OTHER step's multi-command `run: |` body is
# rc=5, because what the rest of that shell script does is not readable from
# here; arbitrarily long comment blocks INSIDE the step (all three
# gate steps carry them, which is why a line cap was rejected in favour of the
# one-`run:`-key invariant); step-level `if:` on OTHER steps in the same job
# (verify.yaml's changelog drift gate is legitimately PR-excluded).
#
# NOT PARSED, stated: `strategy:`/matrix skips, `jobs.<id>.uses:` reusable
# workflows, and a `container:` whose entrypoint could no-op the run. A gate
# reading text cannot see those.
# ---------------------------------------------------------------------------
assent_step_wired() {
  local wf="$1" job="$2" script="$3" work="$4"
  if [[ ! -f "$wf" ]]; then
    return 3
  fi

  local jb="$work/step_wired.job"
  awk -v name="$job" '
    $0 == "  " name ":" { inj = 1; next }
    inj && /^[^[:space:]#]/ { inj = 0 }
    inj && /^  [A-Za-z0-9_.-]+:[[:space:]]*$/ { inj = 0 }
    inj { print }
  ' "$wf" >"$jb"
  if [[ ! -s "$jb" ]]; then
    return 3
  fi
  local stepskey="$work/step_wired.stepskey"
  grep -nE '^    steps:' "$jb" >"$stepskey" || true
  if [[ ! -s "$stepskey" ]]; then
    return 3
  fi

  # Job-level keys sit at 4 spaces; step-level ones at 8.
  local jobif="$work/step_wired.jobif"
  grep -nE '^    if:' "$jb" >"$jobif" || true
  if [[ -s "$jobif" ]]; then
    return 4
  fi
  local jobneeds="$work/step_wired.jobneeds"
  grep -nE '^    needs:' "$jb" >"$jobneeds" || true
  if [[ -s "$jobneeds" ]]; then
    return 9
  fi

  # The COMMAND, never a mention. All three gate steps carry comments that name
  # this script by path; a fixed-string search calls those comments "wired".
  # A `#` in front of either form below fails the anchor.
  #
  # Two forms, tried in that order, because they are NOT equally safe:
  #   plain  `run: bash <script>` — the step's whole command is this gate.
  #   block  a bare `bash <script>` line inside a `run: |` body. Accepted, but
  #          only under the extra conditions checked further down: the step's
  #          `run:` must be a block-scalar header and that line must be the only
  #          command in the body. WITHOUT that restriction this is a FAIL-OPEN,
  #          measured while building this file: append `bash <script>` to any
  #          existing multi-command `run: |` step and delete the real step, and
  #          the check returned 0 — while `echo skip && exit 0` on the line
  #          above would mean the gate never runs. A shell script is not a
  #          workflow step, and a gate cannot read one.
  local re
  re="$(_assent_ere_escape "$script")"
  local cmdhits="$work/step_wired.cmd" cmdform="plain"
  grep -nE "^[[:space:]]*run:[[:space:]]*bash[[:space:]]+${re}([[:space:]]|\$)" "$jb" >"$cmdhits" || true
  if [[ ! -s "$cmdhits" ]]; then
    cmdform="block"
    grep -nE "^[[:space:]]*bash[[:space:]]+${re}([[:space:]]|\$)" "$jb" >"$cmdhits" || true
  fi
  if [[ ! -s "$cmdhits" ]]; then
    return 5
  fi

  local lineno
  lineno="$(head -1 "$cmdhits" | cut -d: -f1)"
  local step="$work/step_wired.step"
  awk -v target="$lineno" '
    { lines[NR] = $0; if ($0 ~ /^      - /) starts[NR] = 1 }
    END {
      start = 0
      for (i = target; i >= 1; i--) if (i in starts) { start = i; break }
      if (start == 0) exit 1
      end = NR
      for (i = start + 1; i <= NR; i++) if (i in starts) { end = i - 1; break }
      for (i = start; i <= end; i++) print lines[i]
    }
  ' "$jb" >"$step" || true
  if [[ ! -s "$step" ]]; then
    return 6
  fi
  local marker="$work/step_wired.marker"
  grep -nE '^      - ' "$step" >"$marker" || true
  if [[ ! -s "$marker" ]]; then
    return 6
  fi

  # THE isolation invariant, and the reason the earlier 1..6 LINE cap was
  # dropped: a step has exactly one `run:` key and, if it runs a command, no
  # `uses:` key. Both are what actionlint itself flags. Deleting a step's
  # `- name:` merges it into its neighbour, which yields either two `run:` keys
  # or a stray `uses:` — a malformed workflow the older checks accepted (rc=0,
  # measured). A line cap cannot separate those cases here, because these steps
  # carry long comment blocks and the honest size and the merged size differ by
  # one line.
  local n_run n_uses
  n_run="$(grep -cE '^[[:space:]]*(-[[:space:]]+)?run:' "$step" || true)"
  n_uses="$(grep -cE '^[[:space:]]*(-[[:space:]]+)?uses:' "$step" || true)"
  if [[ "$n_run" != "1" ]] || [[ "$n_uses" != "0" ]]; then
    return 6
  fi

  # The block-scalar form's extra conditions. `run:` must be a block-scalar
  # HEADER (`|`, `|-`, `>`, `>-`, `|2` …) and the body must contain exactly one
  # command — this one. Anything else means the isolated step's script does more
  # than run this gate, and what the rest of that script does (`exit 0` two
  # lines up) is beyond what a text gate can honestly grade. Shell comment lines
  # are allowed through: they cannot execute, and forbidding them would red an
  # honest step that documents its own command.
  local runhdr="$work/step_wired.runline"
  grep -nE '^[[:space:]]*(-[[:space:]]+)?run:' "$step" >"$runhdr" || true
  if [[ ! -s "$runhdr" ]]; then
    return 6
  fi
  if [[ "$cmdform" == "block" ]]; then
    local runno
    runno="$(head -1 "$runhdr" | cut -d: -f1)"
    local hdrline
    hdrline="$(sed -n "${runno}p" "$step")"
    case "$hdrline" in
      *run:*) : ;;
      *) return 6 ;;
    esac
    local blockhdr="$work/step_wired.blockhdr"
    printf '%s\n' "$hdrline" | grep -nE 'run:[[:space:]]*[|>][-+0-9]*[[:space:]]*$' >"$blockhdr" || true
    if [[ ! -s "$blockhdr" ]]; then
      return 5
    fi
    local body="$work/step_wired.runbody"
    awk -v s="$runno" '
      NR == s { match($0, /^[ ]*/); ind = RLENGTH; next }
      NR > s {
        if ($0 ~ /^[[:space:]]*$/) next
        match($0, /^[ ]*/)
        if (RLENGTH <= ind) exit
        if ($0 ~ /^[[:space:]]*#/) next
        print
      }
    ' "$step" >"$body"
    local n_body
    n_body="$(grep -c . "$body" || true)"
    if [[ "$n_body" != "1" ]]; then
      return 5
    fi
    local bodycmd="$work/step_wired.bodycmd"
    grep -nE "^[[:space:]]*bash[[:space:]]+${re}([[:space:]]|\$)" "$body" >"$bodycmd" || true
    if [[ ! -s "$bodycmd" ]]; then
      return 5
    fi
  fi

  local argfree="$work/step_wired.argfree"
  grep -nE "^[[:space:]]*(run:[[:space:]]*)?bash[[:space:]]+${re}[[:space:]]*\$" "$step" >"$argfree" || true
  if [[ ! -s "$argfree" ]]; then
    return 7
  fi

  local disarmed="$work/step_wired.disarmed"
  grep -nE '^[[:space:]]*(if|continue-on-error):' "$step" >"$disarmed" || true
  if [[ -s "$disarmed" ]]; then
    return 8
  fi

  return 0
}
