#!/usr/bin/env bash
# REQ-EX-S10-01..08 — the P5-EX epic exit gate.
#
# ONE invocation proves the epic's invariants so EX cannot be marked done with
# a missing format, a missing C-series case, stale docs, or schema drift:
#
#   (1) yaml/json/tfvars/tf are all present in the three shipped packs' class
#       match paths (REQ-EX-S10-01), independently derived here (not just
#       re-trusting S01's own extraction).
#   (2) REF-EX C1-C8 fixtures exist (REQ-EX-S10-02); C8's decision is REVIEW,
#       not APPROVE (REQ-EX-S10-03).
#   (3) the schema freeze is REF-RELATIVE against the released tag v0.1.0, not
#       a working-tree diff (REQ-EX-S10-04 / AUD-S18 / D-132).
#   (4) internal/core is untouched on this lane vs origin/main (REQ-EX-S10-07).
#   (5) task check still wires dogfood-examples, docs-gates and its own
#       dogfood-wiring-test guard (REQ-EX-S10-05).
#   (6) a format claimed in examples/README.md that dogfood does not walk
#       fails, by re-invoking S01's own gate (REQ-EX-S10-06).
#   (7) the provider fence still holds (REQ-EX-S07-05, re-verified here).
#   (8) D-002 sanitization is green (REQ-EX-S10-08).
#
# C8 AMENDMENT (read before editing the C-series checks below): REQ-EX-S07-04
# recorded that C8 (companion-file delete) canNOT be expressed as a
# directory-form case with its own expect.yaml — readSingleFilePair errors
# when head/<file> is absent, and the directory form has no way to say "this
# file is gone". C8 shipped as an INLINE `cases.yaml` entry (case name
# `companion-delete`) in examples/packs/infra-vars/.assent/tests/vars/, with
# its `expect.decision` nested under `cases: - name: ... expect: decision:`.
# REQ-EX-S10-03's "C8 expect.yaml" wording and the DoD's "C1-C8 fixture
# directories" both predate that amendment. This gate checks the fixture that
# actually exists (the amendment is the authority — S07 already shipped and
# reviewed it) rather than inventing a fake C8 directory to match stale prose.
#
# ANTI-VACUITY DISCIPLINE (hack/audit/exitgate_test.sh's standard, carried
# forward — this epic's reviewers found a real bug in 3 of its 4 prior
# lanes, and THIS is the gate whose entire job is catching vacuous gates):
#
#   * Every check is a FUNCTION over explicit input paths/args, never a
#     straight-line assertion over a global — so every one of them can be
#     mutation-tested by calling it again with a violating input.
#   * Every check run against the real tree (must be GREEN) also gets at
#     least one mutation run against a violating input (must be RED, and RED
#     FOR ITS OWN STATED REASON — expect_red pins a message fragment, so a
#     check going red for the wrong reason is caught too).
#   * Every ABSENCE/UNCHANGED assertion (schema freeze, internal/core) gets a
#     POSITIVE control that the comparison base really lists the expected
#     path, so a pathspec typo that silently matches nothing cannot report
#     "unchanged" vacuously.
#   * The schema-freeze and internal/core checks both take their BASE REF as
#     a parameter and validate its SHAPE before diffing — `HEAD` (or the
#     empty string) compared against itself is empty by construction, which
#     is exactly the disarm this gate exists to refuse (D-132). Mutation
#     tests below call the checks with `HEAD` and confirm they redden for
#     that specific, stated reason — not just "some check somewhere failed".
#   * No `grep -q` on the read end of a pipe under `set -o pipefail` (SIGPIPE
#     exits 141); matches go to a file. Portable ERE only (no `\t`/`\s`/`\b`,
#     no `grep -P`) — this repo's CI runs Linux/GNU grep, dev runs BSD/macOS.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

WORK="$(mktemp -d)"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT

fail() {
  echo "FAIL: $*" >&2
  exit 1
}

expect_green() { # <check-fn> <label> <args...>
  local fn="$1" label="$2"
  shift 2
  "$fn" "$@" || fail "$label — the real input violates $fn (see the findings above)"
  echo "OK: $label"
}

# A control that reds is not yet a control that WORKS: it could be red for an
# unrelated reason, which would mask the mutation going undetected. Every
# control therefore pins the finding text it must produce (same discipline as
# hack/audit/exitgate_test.sh's expect_red).
expect_red() { # <check-fn> <label> <required-stderr-fragment> <args...>
  local fn="$1" label="$2" want="$3"
  shift 3
  local err="$WORK/expect_red.err"
  if "$fn" "$@" 2>"$err"; then
    fail "mutation control: $fn accepted an input that DOES carry the violation ($label) — this check cannot fail and is therefore not a gate"
  fi
  grep -Fq -- "$want" "$err" || {
    echo "  actual finding was:" >&2
    sed 's/^/    /' "$err" >&2
    fail "mutation control: $fn went red on '$label' but NOT for its stated reason — expected the finding to mention: $want"
  }
  echo "OK: mutation control — $fn goes red on: $label"
}

count_lines() { # <file> -> integer, blank-safe
  wc -l <"$1" | tr -d '[:space:]'
}

# ============================================================================
# constants
# ============================================================================

# (1)/(6) — the three shipped packs and the four formats their class match
# paths must cover between them (yaml, json, tfvars, tf).
PACKS=(topic-registry service-catalog infra-vars)
WANT_FORMATS=(yaml json tfvars tf)

# (2) REF-EX C1-C7: directory-form cases, each with an expect.yaml somewhere
# under it (some are single-case, some carry base/negative/facts-omitted
# sub-cases — see EX-S06/EX-S07 for the per-case shape).
C_IDS=(C1 C2 C3 C4 C5 C6 C7)
C_PATHS=(
  "examples/packs/topic-registry/.assent/tests/topics/list-no-shrink"
  "examples/packs/service-catalog/.assent/tests/catalog/privilege-tier"
  "examples/packs/topic-registry/.assent/tests/topics/wildcard-grant"
  "examples/packs/topic-registry/.assent/tests/topics/soft-delete"
  "examples/packs/topic-registry/.assent/tests/topics/quota-ceiling"
  "examples/packs/infra-vars/.assent/tests/vars/placement"
  "examples/packs/topic-registry/.assent/tests/topics/resource-ownership"
)
# C8: see the amendment note above — inline cases.yaml, not a directory.
C8_CASES_FILE="examples/packs/infra-vars/.assent/tests/vars/cases.yaml"
C8_CASE_NAME="companion-delete"

# (3) Immutable base ref for the schema freeze (AUD-S18 / D-132 pattern,
# mirrored from hack/audit/exitgate_test.sh's SCHEMA_BASE). Overridable only
# to move it FORWARD to a later released tag — never to HEAD/a branch/empty,
# which is exactly the disarm check_schema_freeze's shape guard refuses.
SCHEMA_BASE="${ASSENT_EX_SCHEMA_BASE:-v0.1.0}"
# Positive control: v0.1.0 lists 51 schema/**/*.json at the time this gate was
# written (git ls-tree -r --name-only v0.1.0 -- schemas | grep -c '\.json$').
# 40 leaves margin without being so low a pathspec regression could still
# clear it.
SCHEMA_JSON_MIN=40
SCHEMA_PERMITTED_FILE='schemas/decision/v1alpha1/decision-record.schema.json'

# (4) Base ref for the internal/core untouched check. Must be an origin/*
# remote-tracking ref — see check_core_untouched's shape guard.
CORE_BASE="${ASSENT_EX_CORE_BASE:-origin/main}"
# Positive control: origin/main lists 48 internal/core/**/*.go files at the
# time this gate was written; 20 leaves margin.
CORE_FILE_MIN=20

# ============================================================================
# (1) REQ-EX-S10-01 — yaml/json/tfvars/tf present in class match paths
# ============================================================================

# Independently derived from each pack's .assent/config.yaml classes: block —
# NOT a call into hack/docs/example_format_inventory_test.sh's pack_formats
# (that reuse is REQ-EX-S10-06's job below). A regression in S01's own
# extraction must not be invisible just because this gate trusts it blindly.
check_formats() { # <root>
  local root="$1" rc=0
  local found="$WORK/formats.found"
  : >"$found"
  local pack cfg
  for pack in "${PACKS[@]}"; do
    cfg="$root/examples/packs/$pack/.assent/config.yaml"
    if [[ ! -f "$cfg" ]]; then
      echo "  REQ-EX-S10-01: missing $cfg" >&2
      rc=1
      continue
    fi
    awk '
      $0 ~ /^classes:[[:space:]]*$/ { in_cls = 1; next }
      in_cls && /^[a-zA-Z]/ { in_cls = 0 }
      in_cls { print }
    ' "$cfg" | grep -oE '\*\.[A-Za-z0-9]+' | sed -e 's/^\*\.//' -e 's/^yml$/yaml/' >>"$found"
  done
  ((rc == 0)) || return "$rc"

  local uniq="$WORK/formats.uniq"
  LC_ALL=C sort -u "$found" >"$uniq"
  if [[ ! -s "$uniq" ]]; then
    echo "  REQ-EX-S10-01: extracted ZERO format extensions from any pack's classes: block — the extraction is broken, so the format-presence assertions below would be vacuous" >&2
    return 1
  fi

  local w
  for w in "${WANT_FORMATS[@]}"; do
    grep -Fxq -- "$w" "$uniq" || {
      echo "  REQ-EX-S10-01: format '$w' is not covered by any pack's class match.paths (found: $(tr '\n' ' ' <"$uniq"))" >&2
      rc=1
    }
  done
  return "$rc"
}

# ============================================================================
# (2) REQ-EX-S10-02/03 — C1-C8 fixtures present; C8 decision is REVIEW
# ============================================================================

check_c_series() { # <root>
  local root="$1" rc=0 i id path full n
  for i in "${!C_IDS[@]}"; do
    id="${C_IDS[$i]}"
    path="${C_PATHS[$i]}"
    full="$root/$path"
    if [[ ! -d "$full" ]]; then
      echo "  REQ-EX-S10-02: $id fixture directory missing: $path" >&2
      rc=1
      continue
    fi
    n="$(find "$full" -name expect.yaml | wc -l | tr -d '[:space:]')"
    if [[ "$n" -eq 0 ]]; then
      echo "  REQ-EX-S10-02: $id fixture directory $path has no expect.yaml anywhere under it — an empty/incomplete directory is not a case" >&2
      rc=1
    fi
  done
  return "$rc"
}

# Bonus corroboration of the epic Goal text ("C1-C8 ... decisions match the
# table (C3 BLOCK, C8 REVIEW, etc.)"): C3 (wildcard-grant) must have at least
# one measured BLOCK decision somewhere under it. Not a formal REQ-EX-S10-0n
# line item on its own (only C8's REVIEW is), but cheap and directly stated in
# the Goal, so it is asserted here rather than left unchecked.
check_c3_block() { # <root>
  local root="$1"
  local dir="$root/examples/packs/topic-registry/.assent/tests/topics/wildcard-grant"
  [[ -d "$dir" ]] || {
    echo "  C3: wildcard-grant directory missing" >&2
    return 1
  }
  local hits="$WORK/c3.hits"
  grep -RE '^decision:[[:space:]]*BLOCK[[:space:]]*$' "$dir" >"$hits" 2>/dev/null || true
  [[ -s "$hits" ]] || {
    echo "  C3: no expect.yaml under $dir declares 'decision: BLOCK' — the wildcard-grant-block case is not measured as blocking" >&2
    return 1
  }
  return 0
}

# Isolates one named case's block from a cases.yaml `cases:` list: from its
# `- name: <name>` line up to (not including) the next `- name:` line.
extract_case_block() { # <cases-yaml> <case-name>
  awk -v want="$2" '
    /^[[:space:]]*-[[:space:]]*name:[[:space:]]*/ {
      line = $0
      sub(/^[[:space:]]*-[[:space:]]*name:[[:space:]]*/, "", line)
      gsub(/[[:space:]]+$/, "", line)
      if (line == want) { grabbing = 1; print; next }
      grabbing = 0
      next
    }
    grabbing { print }
  ' "$1"
}

check_c8_decision() { # <cases-yaml-path> <case-name>
  local f="$1" name="$2"
  [[ -f "$f" ]] || {
    echo "  REQ-EX-S10-03: missing $f" >&2
    return 1
  }
  local block="$WORK/c8.block"
  extract_case_block "$f" "$name" >"$block"
  [[ -s "$block" ]] || {
    echo "  REQ-EX-S10-03: case '$name' not found in $f — the extraction is vacuous (renamed/deleted case, or cases.yaml reshaped)" >&2
    return 1
  }
  local dline
  dline="$(grep -E '^[[:space:]]*decision:[[:space:]]*[A-Za-z_]+[[:space:]]*$' "$block" | head -1 || true)"
  [[ -n "$dline" ]] || {
    echo "  REQ-EX-S10-03: no 'decision:' line found in case '$name''s expect block" >&2
    return 1
  }
  local decision
  decision="$(printf '%s\n' "$dline" | sed -E 's/^[[:space:]]*decision:[[:space:]]*//; s/[[:space:]]*$//')"
  if [[ "$decision" != "REVIEW" ]]; then
    echo "  REQ-EX-S10-03: case '$name' decision is '$decision', expected REVIEW (C8 must never silently APPROVE)" >&2
    return 1
  fi
  return 0
}

# ============================================================================
# (3) REQ-EX-S10-04 — schema freeze, REF-RELATIVE against v0.1.0
# ============================================================================

# Pure parser over `git diff --name-status <base> -- schemas` output, JSON
# only. Split out from check_schema_freeze so the "unauthorized schema edit"
# branch can be mutation-tested with synthetic status lines — no git
# operations, no risk to the real tree, full coverage of every shape.
check_schema_status() { # <name-status-file>
  local status="$1" rc=0
  [[ -f "$status" ]] || {
    echo "  REQ-EX-S10-04: missing status file $status" >&2
    return 1
  }
  local changed="$WORK/schema_status.changed"
  awk -F'\t' '$NF ~ /\.json$/ { print }' "$status" >"$changed" || true

  local addel="$WORK/schema_status.addel"
  awk -F'\t' '$1 !~ /^M/ { print }' "$changed" >"$addel" || true
  if [[ -s "$addel" ]]; then
    echo "  REQ-EX-S10-04: JSON schema file(s) added, deleted or renamed since $SCHEMA_BASE — the frozen v1alpha1 contract set changed:" >&2
    sed 's/^/    /' "$addel" >&2
    rc=1
  fi

  local modified="$WORK/schema_status.modified"
  awk -F'\t' '$1 ~ /^M/ { print $NF }' "$changed" >"$modified" || true
  local f
  while IFS= read -r f; do
    [[ -n "$f" ]] || continue
    if [[ "$f" != "$SCHEMA_PERMITTED_FILE" ]]; then
      echo "  REQ-EX-S10-04: frozen schema $f modified since $SCHEMA_BASE — EX must add, delete or modify no schema JSON beyond the AUD-S18 permitted description-string on $SCHEMA_PERMITTED_FILE" >&2
      rc=1
    fi
  done <"$modified"
  return "$rc"
}

# Ref-relative, not working-tree-relative (D-132): a COMMITTED schema change
# is the only kind that ships, and `git diff schemas/` is silent on those.
# `base` is validated by SHAPE (must be a vX.Y.Z release tag) before it is
# ever used in a diff — this is the guard that makes `HEAD`/`` /a branch
# non-vacuous to reject, per REQ-EX-S10-04's edge clause.
check_schema_freeze() { # <repo-dir> <base-ref> <min-json-at-base>
  local dir="$1" base="$2" minjson="$3" rc=0

  if [[ ! "$base" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "  REQ-EX-S10-04: schema-freeze base '$base' is not a release tag of the form vX.Y.Z — comparing against a non-tag ref (HEAD, a branch, empty) makes the diff compare the tree against itself and pass over any schema change (D-132)" >&2
    return 1
  fi
  git -C "$dir" rev-parse --verify --quiet "refs/tags/${base}" >/dev/null || {
    echo "  REQ-EX-S10-04: '$base' is not a tag in $dir — the baseline must be the released tag itself, not a same-named branch or a loose revision" >&2
    return 1
  }

  # Positive control: the baseline must actually list the schema corpus.
  # Without this a pathspec that stopped matching would compare an empty set
  # to an empty set and report the schemas frozen.
  local baselist="$WORK/schemas.base"
  git -C "$dir" ls-tree -r --name-only "$base" -- schemas 2>/dev/null | grep -E '\.json$' >"$baselist" || true
  local n_base
  n_base="$(count_lines "$baselist")"
  if ((n_base < minjson)); then
    echo "  REQ-EX-S10-04: base ref '$base' lists only $n_base JSON file(s) under schemas/ (expected at least $minjson) — the pathspec is not matching, so 'schemas frozen' would be vacuous" >&2
    return 1
  fi

  local status="$WORK/schemas.status"
  git -C "$dir" diff --name-status "$base" -- schemas >"$status" 2>/dev/null || {
    echo "  REQ-EX-S10-04: 'git diff --name-status $base -- schemas' failed in $dir" >&2
    return 1
  }
  check_schema_status "$status" || rc=1
  return "$rc"
}

# ============================================================================
# (4) REQ-EX-S10-07 — internal/core untouched vs origin/main (three-dot)
# ============================================================================

# Same base-shape discipline as check_schema_freeze: `base` must be an
# origin/* remote-tracking ref before it is used in a diff. A `HEAD` (or any
# non-remote) base makes `base...HEAD` compare a commit against itself, which
# is empty by construction and would pass regardless of what changed.
check_core_untouched() { # <repo-dir> <base-ref> <min-files-at-base>
  local dir="$1" base="$2" minfiles="$3"

  case "$base" in
  origin/*) ;;
  *)
    echo "  REQ-EX-S10-07: core-untouched base '$base' is not an origin/* remote-tracking ref — substituting HEAD (or any non-remote ref) makes 'base...HEAD' compare a commit against itself, which is empty regardless of what changed" >&2
    return 1
    ;;
  esac
  git -C "$dir" rev-parse --verify --quiet "$base" >/dev/null || {
    echo "  REQ-EX-S10-07: base ref '$base' does not resolve in $dir" >&2
    return 1
  }

  # Positive control on the pathspec.
  local baselist="$WORK/core.base"
  git -C "$dir" ls-tree -r --name-only "$base" -- internal/core 2>/dev/null >"$baselist" || true
  local n_base
  n_base="$(count_lines "$baselist")"
  if ((n_base < minfiles)); then
    echo "  REQ-EX-S10-07: base ref '$base' lists only $n_base file(s) under internal/core (expected at least $minfiles) — the pathspec is not matching, so 'internal/core untouched' would be vacuous" >&2
    return 1
  fi

  git -C "$dir" merge-base "$base" HEAD >/dev/null 2>&1 || {
    echo "  REQ-EX-S10-07: no merge-base between '$base' and HEAD in $dir" >&2
    return 1
  }

  local diff="$WORK/core.diff"
  git -C "$dir" diff --name-status "${base}...HEAD" -- internal/core >"$diff" 2>/dev/null || {
    echo "  REQ-EX-S10-07: 'git diff --name-status $base...HEAD -- internal/core' failed in $dir" >&2
    return 1
  }
  if [[ -s "$diff" ]]; then
    echo "  REQ-EX-S10-07: internal/core changed on this lane vs $base — EX is an examples/gate lane, not an engine lane:" >&2
    sed 's/^/    /' "$diff" >&2
    return 1
  fi
  return 0
}

# ============================================================================
# (5) REQ-EX-S10-05 — task check still wires dogfood-examples/docs-gates/
#     dogfood-wiring-test (composes S08's own wiring pin, does not reimplement it)
# ============================================================================

extract_check_block() { # <taskfile>
  awk '
    $0 == "  check:" { inblk = 1; next }
    inblk && /^  [A-Za-z0-9_.:-]+:[[:space:]]*$/ { inblk = 0 }
    inblk { print }
  ' "$1"
}

check_task_wiring() { # <taskfile>
  local tf="$1" rc=0
  [[ -f "$tf" ]] || {
    echo "  REQ-EX-S10-05: missing $tf" >&2
    return 1
  }
  local block="$WORK/check.block"
  extract_check_block "$tf" >"$block"
  [[ -s "$block" ]] || {
    echo "  REQ-EX-S10-05: Taskfile check: block extracted EMPTY — the extraction is broken, so every wiring assertion below would be vacuous" >&2
    return 1
  }
  grep -qE '^[[:space:]]+- task: build$' "$block" || {
    echo "  REQ-EX-S10-05: extracted check: block does not contain the known-present '- task: build' — extraction is wrong" >&2
    return 1
  }

  local want
  for want in dogfood-examples docs-gates dogfood-wiring-test; do
    grep -qE "^[[:space:]]+- task: $want\$" "$block" || {
      echo "  REQ-EX-S10-05: 'task check' does not run '$want' — the dogfood/docs-gates/inventory gate would be unwired" >&2
      rc=1
    }
  done
  return "$rc"
}

# ============================================================================
# (6) REQ-EX-S10-06 — a README-claimed format absent from dogfood must fail
#     (re-invokes S01's own gate; both polarities remain S01's, not reimplemented)
# ============================================================================

check_inventory_gate() { # <script>
  local script="$1"
  [[ -f "$script" ]] || {
    echo "  REQ-EX-S10-06: missing $script" >&2
    return 1
  }
  local out="$WORK/inventory.out"
  if ! bash "$script" >"$out" 2>&1; then
    echo "  REQ-EX-S10-06: $script is RED against the real tree:" >&2
    sed 's/^/    /' "$out" >&2
    return 1
  fi
  # Named-PASS discipline: `bash script` exiting 0 is not itself proof it ran
  # the real assertions (a truncated script also exits 0). Require the
  # script's own terminal banner AND its real-tree PASS line by name.
  grep -Fq 'OK: example format inventory (EX-S01)' "$out" || {
    echo "  REQ-EX-S10-06: $script did not print its own success banner — it may have exited early" >&2
    sed 's/^/    /' "$out" >&2
    return 1
  }
  grep -Fq 'PASS  REQ-EX-S01-01:' "$out" || {
    echo "  REQ-EX-S10-06: $script's real-tree REQ-EX-S01-01 check did not report PASS by name" >&2
    sed 's/^/    /' "$out" >&2
    return 1
  }
  return 0
}

# ============================================================================
# (7) provider fence (REQ-EX-S07-05, re-verified)
# ============================================================================

check_provider_fence() { # <root>
  local root="$1"
  local out="$WORK/fence.out"
  ( cd "$root" && go test ./cmd/assent/ -run TestAssentTestNeverCallsProviderHost -v ) >"$out" 2>&1
  local rc=$?
  local n_pass
  n_pass="$(grep -c -- '--- PASS: ' "$out" || true)"
  if [[ "$n_pass" -eq 0 ]]; then
    echo "  provider fence: the go-test transcript contains ZERO '--- PASS:' lines — either -run matched no test (go test exits 0 for that) or -v was dropped; grading on exit code alone would be vacuous" >&2
    return 1
  fi
  grep -Fq -- '--- PASS: TestAssentTestNeverCallsProviderHost ' "$out" || {
    echo "  provider fence: TestAssentTestNeverCallsProviderHost did not PASS by name — 'assent test' may now construct the live provider host" >&2
    sed 's/^/    /' "$out" >&2
    return 1
  }
  if [[ "$rc" -ne 0 ]]; then
    echo "  provider fence: go test exited $rc" >&2
    return 1
  fi
  return 0
}

# ============================================================================
# (8) REQ-EX-S10-08 — D-002 sanitization green
# ============================================================================

check_sanitization() { # <script>
  local script="$1"
  [[ -f "$script" ]] || {
    echo "  REQ-EX-S10-08: missing $script" >&2
    return 1
  }
  local out="$WORK/sanitize.out"
  if ! bash "$script" >"$out" 2>&1; then
    echo "  REQ-EX-S10-08: $script is RED — D-002 hygiene violation:" >&2
    sed 's/^/    /' "$out" >&2
    return 1
  fi
  grep -Fq 'sanitization check passed' "$out" || {
    echo "  REQ-EX-S10-08: $script exited 0 but did not print its own success banner — it may have exited before scanning anything" >&2
    return 1
  }
  return 0
}

# Positive control on the underlying scanner: proves the built-in patterns
# check-sanitization.sh uses can actually MATCH a planted token, without
# touching any real fixture. Extracted BY NAME from the script itself (not
# re-typed) so a pattern edit there is reflected here automatically.
check_sanitization_patterns() { # <script>
  local script="$1" rc=0
  [[ -f "$script" ]] || {
    echo "  REQ-EX-S10-08: missing $script" >&2
    return 1
  }
  local hostline empidline host_pat empid_pat
  hostline="$(grep -E '^host_pat=' "$script" | head -1 || true)"
  empidline="$(grep -E '^empid_pat=' "$script" | head -1 || true)"
  [[ -n "$hostline" && -n "$empidline" ]] || {
    echo "  REQ-EX-S10-08: could not extract host_pat/empid_pat from $script — the positive control cannot run" >&2
    return 1
  }
  host_pat="$(printf '%s\n' "$hostline" | sed -E "s/^host_pat='(.*)'\$/\\1/")"
  empid_pat="$(printf '%s\n' "$empidline" | sed -E "s/^empid_pat='(.*)'\$/\\1/")"

  # Assembled at RUNTIME, never as a contiguous literal in this file's own
  # source: hack/check-sanitization.sh scans every tracked file (including
  # this one), so a fully-formed planted token sitting directly in this
  # script's text would trip D-002 on itself. Splitting the domain suffix and
  # the digit run apart keeps the assembled string out of the static text
  # while still producing the real match at runtime.
  local corp_suffix="cor"
  corp_suffix="${corp_suffix}p"
  local host_token="contact ops@example.${corp_suffix} for onboarding"
  printf '%s\n' "$host_token" | grep -Eq -- "$host_pat" || {
    echo "  REQ-EX-S10-08: host_pat did not match a planted internal-hostname token (*.corp) — the scanner's own pattern would let one through" >&2
    rc=1
  }

  local empid_digits="12345"
  empid_digits="${empid_digits}6"
  local empid_token="assigned to A${empid_digits} for review"
  printf '%s\n' "$empid_token" | grep -Eq -- "$empid_pat" || {
    echo "  REQ-EX-S10-08: empid_pat did not match a planted employee-id-shaped token — the scanner's own pattern would let one through" >&2
    rc=1
  }
  return "$rc"
}

# ============================================================================
# sandbox builders (schema-freeze / internal-core mutation tests)
# ============================================================================

# A tiny standalone git repo, never the repo under test — same discipline as
# hack/release/changelog_gate_test.sh's sandbox: verify it is its OWN
# repository before trusting any git command run against it.
new_sandbox() { # <dir>
  local dir="$1"
  mkdir -p "$dir"
  git init -q "$dir"
  git -C "$dir" symbolic-ref HEAD refs/heads/main
  local resolved expected
  resolved="$(cd "$(git -C "$dir" rev-parse --show-toplevel)" && pwd -P)"
  expected="$(cd "$dir" && pwd -P)"
  [[ "$resolved" == "$expected" ]] || fail "sandbox at $dir resolves git commands to '$resolved' — not isolated from the real repository"
}

sandbox_commit() { # <dir> <message>
  git -C "$1" add -A
  git -C "$1" -c user.email=t@example.com -c user.name=t commit -q -m "$2"
}

# ---------------------------------------------------------------------------
echo "== EX exit gate: REQ-EX-S10-01..08 =="
echo

echo "-- (1) REQ-EX-S10-01: yaml/json/tfvars/tf present in class match paths --"
expect_green check_formats "REQ-EX-S10-01: real tree covers yaml/json/tfvars/tf" "$ROOT"

MUT_FMT_ROOT="$WORK/mutant-formats"
rm -rf "$MUT_FMT_ROOT"
# Synthetic, comment-free config.yaml content — NOT a copy of the real files:
# the real infra-vars config.yaml's KNOWN LIMITATION comment prose also
# contains the literal substring "*.tf" (documenting the class match, in
# English), which check_formats' extraction (matching S01's own pack_formats,
# deliberately NOT comment-stripped) would then still see even after the
# match: line itself was mutated — a confound that would make this mutation
# pass for the wrong reason. Fully synthetic input sidesteps that.
mkdir -p "$MUT_FMT_ROOT/examples/packs/topic-registry/.assent"
mkdir -p "$MUT_FMT_ROOT/examples/packs/service-catalog/.assent"
mkdir -p "$MUT_FMT_ROOT/examples/packs/infra-vars/.assent"
printf 'classes:\n  - name: kafka-topic\n    match: { paths: ["topics/**/*.yaml"] }\n' \
  >"$MUT_FMT_ROOT/examples/packs/topic-registry/.assent/config.yaml"
printf 'classes:\n  - name: catalog-service\n    match: { paths: ["catalog/**/*.json"] }\n' \
  >"$MUT_FMT_ROOT/examples/packs/service-catalog/.assent/config.yaml"
# The mutation: infra-vars loses its .tf class-match alternative. .tfvars
# remains, so only "tf" as a distinct token should vanish from the found set.
printf 'classes:\n  - name: infra-vars\n    match: { paths: ["envs/**/*.tfvars"] }\n' \
  >"$MUT_FMT_ROOT/examples/packs/infra-vars/.assent/config.yaml"
grep -q '\*\.tf"' "$MUT_FMT_ROOT/examples/packs/infra-vars/.assent/config.yaml" && fail "sanity: synthetic mutant config.yaml still contains a .tf class match"
expect_red check_formats "dropping the .tf class match alternative" "format 'tf' is not covered" "$MUT_FMT_ROOT"
echo

echo "-- (2a) REQ-EX-S10-02: C1-C8 fixtures present --"
expect_green check_c_series "REQ-EX-S10-02: C1-C7 directories present with expect.yaml" "$ROOT"
[[ -f "$ROOT/$C8_CASES_FILE" ]] || fail "REQ-EX-S10-02: C8 cases.yaml missing: $C8_CASES_FILE"
echo "OK: REQ-EX-S10-02: C8 inline case file present: $C8_CASES_FILE"

MUT_C_ROOT="$WORK/mutant-c"
i=0
while [[ "$i" -lt "${#C_IDS[@]}" ]]; do
  id="${C_IDS[$i]}"
  rm -rf "$MUT_C_ROOT"
  mkdir -p "$MUT_C_ROOT/examples/packs"
  for p in "${PACKS[@]}"; do
    cp -R "$ROOT/examples/packs/$p" "$MUT_C_ROOT/examples/packs/$p"
  done
  rm -rf "${MUT_C_ROOT:?}/${C_PATHS[$i]}"
  [[ -d "$ROOT/${C_PATHS[$i]}" ]] || fail "sanity: real $id path missing before mutation even started"
  [[ ! -d "$MUT_C_ROOT/${C_PATHS[$i]}" ]] || fail "mutation did not remove the $id directory from the scratch copy"
  expect_red check_c_series "deleting the $id fixture directory" "$id fixture directory missing" "$MUT_C_ROOT"
  i=$((i + 1))
done
rm -rf "$MUT_C_ROOT"
echo

echo "-- (2b) C3 bonus corroboration: wildcard-grant carries a measured BLOCK --"
expect_green check_c3_block "C3: wildcard-grant declares decision: BLOCK somewhere under it" "$ROOT"

MUT_C3="$WORK/mutant-c3"
rm -rf "$MUT_C3"
mkdir -p "$MUT_C3"
cp -R "$ROOT/examples/packs/topic-registry" "$MUT_C3/topic-registry"
find "$MUT_C3/topic-registry/.assent/tests/topics/wildcard-grant" -name expect.yaml -print0 |
  xargs -0 -I{} sed -i.bak 's/decision: BLOCK/decision: APPROVE/' {}
find "$MUT_C3/topic-registry/.assent/tests/topics/wildcard-grant" -name '*.bak' -delete
mkdir -p "$WORK/mutant-c3-root/examples/packs"
mv "$MUT_C3/topic-registry" "$WORK/mutant-c3-root/examples/packs/topic-registry"
grep -RE '^decision:[[:space:]]*BLOCK[[:space:]]*$' "$WORK/mutant-c3-root/examples/packs/topic-registry/.assent/tests/topics/wildcard-grant" >/dev/null 2>&1 &&
  fail "mutation did not flip every 'decision: BLOCK' line under wildcard-grant"
expect_red check_c3_block "flipping wildcard-grant's BLOCK decision to APPROVE" "no expect.yaml under" "$WORK/mutant-c3-root"
rm -rf "$MUT_C3" "$WORK/mutant-c3-root"
echo

echo "-- (2c) REQ-EX-S10-03: C8 decision is REVIEW --"
expect_green check_c8_decision "REQ-EX-S10-03: C8 companion-delete decision is REVIEW" "$ROOT/$C8_CASES_FILE" "$C8_CASE_NAME"

MUT_C8_APPROVE="$WORK/cases.approve.yaml"
sed 's/decision: REVIEW/decision: APPROVE/' "$ROOT/$C8_CASES_FILE" >"$MUT_C8_APPROVE"
grep -q 'decision: APPROVE' "$MUT_C8_APPROVE" || fail "mutation did not flip C8's decision to APPROVE"
expect_red check_c8_decision "flipping C8's decision to APPROVE" "expected REVIEW" "$MUT_C8_APPROVE" "$C8_CASE_NAME"

MUT_C8_RENAME="$WORK/cases.rename.yaml"
sed 's/name: companion-delete/name: companion-delete-renamed/' "$ROOT/$C8_CASES_FILE" >"$MUT_C8_RENAME"
expect_red check_c8_decision "renaming the companion-delete case" "not found in" "$MUT_C8_RENAME" "$C8_CASE_NAME"
echo

echo "-- (3) REQ-EX-S10-04: schema freeze vs v0.1.0 (ref-relative, not working-tree) --"
expect_green check_schema_freeze "REQ-EX-S10-04: real tree's schemas/**/*.json unchanged vs v0.1.0 (except the D-120 permitted line)" "$ROOT" "$SCHEMA_BASE" "$SCHEMA_JSON_MIN"

echo "   -- base-ref shape/resolution mutation controls (the D-132 trap) --"
expect_red check_schema_freeze "substituting HEAD for the release tag" "not a release tag" "$ROOT" "HEAD" "$SCHEMA_JSON_MIN"
expect_red check_schema_freeze "an empty base ref" "not a release tag" "$ROOT" "" "$SCHEMA_JSON_MIN"
expect_red check_schema_freeze "a branch name shaped like a tag but not one" "not a release tag" "$ROOT" "main" "$SCHEMA_JSON_MIN"
expect_red check_schema_freeze "a well-shaped but nonexistent tag" "is not a tag in" "$ROOT" "v9.9.9" "$SCHEMA_JSON_MIN"

echo "   -- branch-vs-tag distinguishing control (same name, not a tag) --"
SCHEMA_SB="$WORK/schema-sandbox"
new_sandbox "$SCHEMA_SB"
mkdir -p "$SCHEMA_SB/schemas/decision/v1alpha1" "$SCHEMA_SB/schemas/change/v1alpha1"
printf '{"a":1}\n' >"$SCHEMA_SB/schemas/decision/v1alpha1/decision-record.schema.json"
printf '{"b":1}\n' >"$SCHEMA_SB/schemas/change/v1alpha1/changeset.schema.json"
printf 'package schemas\n' >"$SCHEMA_SB/schemas/compiler.go"
sandbox_commit "$SCHEMA_SB" init
git -C "$SCHEMA_SB" tag v0.1.0
git -C "$SCHEMA_SB" branch v9.9.9
expect_red check_schema_freeze "a same-named branch, not a tag" "is not a tag in" "$SCHEMA_SB" "v9.9.9" 1

echo "   -- positive control on the baseline pathspec --"
expect_red check_schema_freeze "an unreasonably high minjson floor" "the pathspec is not matching" "$SCHEMA_SB" "v0.1.0" 999999

echo "   -- unauthorized schema edit (in a sandbox, real tree never touched) --"
expect_green check_schema_freeze "sandbox: clean tree vs its own v0.1.0" "$SCHEMA_SB" "v0.1.0" 2
printf '{"b":2}\n' >"$SCHEMA_SB/schemas/change/v1alpha1/changeset.schema.json"
expect_red check_schema_freeze "modifying a NON-permitted schema JSON" "frozen schema" "$SCHEMA_SB" "v0.1.0" 2
git -C "$SCHEMA_SB" checkout -q -- schemas/change/v1alpha1/changeset.schema.json

rm "$SCHEMA_SB/schemas/change/v1alpha1/changeset.schema.json"
expect_red check_schema_freeze "deleting a frozen schema JSON" "added, deleted or renamed" "$SCHEMA_SB" "v0.1.0" 2
git -C "$SCHEMA_SB" checkout -q -- schemas/change/v1alpha1/changeset.schema.json

printf '{"c":1}\n' >"$SCHEMA_SB/schemas/new-one.schema.json"
git -C "$SCHEMA_SB" add "$SCHEMA_SB/schemas/new-one.schema.json"
expect_red check_schema_freeze "adding a new schema JSON" "added, deleted or renamed" "$SCHEMA_SB" "v0.1.0" 2
git -C "$SCHEMA_SB" reset -q -- schemas/new-one.schema.json
rm -f "$SCHEMA_SB/schemas/new-one.schema.json"

printf '{"a":2}\n' >"$SCHEMA_SB/schemas/decision/v1alpha1/decision-record.schema.json"
expect_green check_schema_freeze "modifying the ONE permitted schema file" "$SCHEMA_SB" "v0.1.0" 2
git -C "$SCHEMA_SB" checkout -q -- schemas/decision/v1alpha1/decision-record.schema.json

echo "   -- check_schema_status pure-parser controls (no git at all) --"
SS="$WORK/status.synth"
printf 'M\t%s\n' "$SCHEMA_PERMITTED_FILE" >"$SS"
expect_green check_schema_status "synthetic: only the permitted file modified" "$SS"
printf 'A\tschemas/foo/v1/new.schema.json\n' >"$SS"
expect_red check_schema_status "synthetic: an added schema JSON" "added, deleted or renamed" "$SS"
printf 'D\tschemas/foo/v1/old.schema.json\n' >"$SS"
expect_red check_schema_status "synthetic: a deleted schema JSON" "added, deleted or renamed" "$SS"
printf 'M\tschemas/change/v1alpha1/changeset.schema.json\n' >"$SS"
expect_red check_schema_status "synthetic: an unauthorized modified schema JSON" "frozen schema" "$SS"
: >"$SS"
expect_green check_schema_status "synthetic: empty status (no schema changes)" "$SS"
rm -rf "$SCHEMA_SB"
echo

echo "-- (4) REQ-EX-S10-07: internal/core untouched vs $CORE_BASE (three-dot) --"
expect_green check_core_untouched "REQ-EX-S10-07: internal/core unchanged on this lane" "$ROOT" "$CORE_BASE" "$CORE_FILE_MIN"

echo "   -- base-ref shape mutation control (the HEAD-substitution trap) --"
expect_red check_core_untouched "substituting HEAD for origin/main" "not an origin/* remote-tracking ref" "$ROOT" "HEAD" "$CORE_FILE_MIN"
expect_red check_core_untouched "an empty base ref" "not an origin/* remote-tracking ref" "$ROOT" "" "$CORE_FILE_MIN"

echo "   -- positive control on the baseline pathspec --"
expect_red check_core_untouched "an unreasonably high minfiles floor" "the pathspec is not matching" "$ROOT" "$CORE_BASE" 999999

echo "   -- real internal/core violation, caught in a sandbox --"
CORE_SB="$WORK/core-sandbox"
new_sandbox "$CORE_SB"
mkdir -p "$CORE_SB/internal/core" "$CORE_SB/internal/change"
printf 'package core\n' >"$CORE_SB/internal/core/a.go"
printf 'package change\n' >"$CORE_SB/internal/change/b.go"
sandbox_commit "$CORE_SB" init
BASESHA="$(git -C "$CORE_SB" rev-parse HEAD)"
git -C "$CORE_SB" update-ref refs/remotes/origin/main "$BASESHA"
expect_green check_core_untouched "sandbox: HEAD == origin/main, nothing changed" "$CORE_SB" "origin/main" 1
printf 'package core\nvar X = 1\n' >"$CORE_SB/internal/core/a.go"
sandbox_commit "$CORE_SB" "touch internal/core"
expect_red check_core_untouched "sandbox: a lane commit touches internal/core" "internal/core changed on this lane" "$CORE_SB" "origin/main" 1
# Same sandbox, HEAD-substitution trap against a REAL divergence: base==HEAD
# makes the range empty even though origin/main and HEAD really do differ —
# proof the base-shape guard is load-bearing, not decorative.
expect_red check_core_untouched "sandbox: HEAD substituted despite a real internal/core divergence" "not an origin/* remote-tracking ref" "$CORE_SB" "HEAD" 1
rm -rf "$CORE_SB"
echo

echo "-- (5) REQ-EX-S10-05: task check wires dogfood-examples/docs-gates/dogfood-wiring-test --"
expect_green check_task_wiring "REQ-EX-S10-05: task check runs dogfood-examples, docs-gates, dogfood-wiring-test" "$ROOT/Taskfile.yml"

for dropped in dogfood-examples docs-gates dogfood-wiring-test; do
  MUT_TF="$WORK/Taskfile.no-$dropped.yml"
  grep -vE "^[[:space:]]+- task: $dropped\$" "$ROOT/Taskfile.yml" >"$MUT_TF"
  grep -qE "^[[:space:]]+- task: $dropped\$" "$MUT_TF" && fail "mutation did not remove '- task: $dropped' from the scratch Taskfile"
  expect_red check_task_wiring "dropping '- task: $dropped' from check:" "does not run '$dropped'" "$MUT_TF"
done
echo

echo "-- (6) REQ-EX-S10-06: README-format-vs-dogfood inventory (re-invokes S01) --"
expect_green check_inventory_gate "REQ-EX-S10-06: hack/docs/example_format_inventory_test.sh is green and proves it ran" "$ROOT/hack/docs/example_format_inventory_test.sh"
echo "   (both red polarities — extra claimed pack, omitted pack, unmapped format token —"
echo "    are S01's own mutation controls, embedded in that script and re-run every time"
echo "    it executes above; not reimplemented here.)"
echo

echo "-- (7) provider fence (REQ-EX-S07-05, re-verified) --"
expect_green check_provider_fence "go test ./cmd/assent/ -run TestAssentTestNeverCallsProviderHost passes by name" "$ROOT"
echo

echo "-- (8) REQ-EX-S10-08: D-002 sanitization green --"
expect_green check_sanitization "REQ-EX-S10-08: hack/check-sanitization.sh is green on the real tree" "$ROOT/hack/check-sanitization.sh"
expect_green check_sanitization_patterns "REQ-EX-S10-08: the scanner's own patterns match a planted employer-hostname / employee-id token" "$ROOT/hack/check-sanitization.sh"
echo

echo "PASS: P5-EX exit gate (REQ-EX-S10-01..08) — four formats present; C1-C8 fixtures present with C8 measured REVIEW; schema freeze ref-relative against $SCHEMA_BASE; internal/core untouched vs $CORE_BASE; task check wiring intact; README/dogfood inventory agree; provider fence holds; D-002 sanitization clean"
