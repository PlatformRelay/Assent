#!/usr/bin/env bash
# check-migration-invariants.sh — P3-E3-S04 migration-specific CI guard.
#
# Distinct from P3-E1-S06 schema validation: checks invariants a schema cannot
# express (marker text, quarantine shape). Runs *before* schema validation in
# .github/workflows/schemas.yml so a migration regression fails by name.
#
# Fails when:
#   (a) any file under examples/policies/** or examples/archetypes/** contains
#       a case-insensitive "DRAFT" marker;
#   (b) any file under examples/policies/rego/** is missing the S03 quarantine
#       marker (`locked: D-012`) on or before its first non-comment line;
#   (c) any file under examples/** outside examples/policies/rego/** contains a
#       `rego:` predicate leaf (scoped to examples/ so ADR/docs that document
#       the syntax are not false positives — see agent-context INBOX).
#
# Exit codes: 0 = clean, 1 = invariant broken, 2 = usage/environment error.
set -u

repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  echo "error: must run inside the git repository" >&2
  exit 2
}
cd "$repo_root" || exit 2

hits=0

report() {
  printf 'FAIL [%s] %s\n' "$1" "$2" >&2
  hits=1
}

# --- (a) DRAFT markers in migrated example trees --------------------------------
draft_hits=$(grep -rni --include='*' 'DRAFT' examples/policies examples/archetypes 2>/dev/null || true)
if [ -n "$draft_hits" ]; then
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    report "DRAFT-marker" "$line"
  done <<EOF
$draft_hits
EOF
fi

# --- (b) quarantine marker on every rego example --------------------------------
rego_root="examples/policies/rego"
if [ -d "$rego_root" ]; then
  while IFS= read -r f; do
    [ -f "$f" ] || continue
    # Marker must appear on or before the first non-comment, non-blank line.
    saw_marker=0
    while IFS= read -r line || [ -n "$line" ]; do
      case "$line" in
        *'locked: D-012'*) saw_marker=1 ;;
      esac
      # Non-blank, non-comment → stop scanning; marker must already be seen.
      trimmed=${line#"${line%%[![:space:]]*}"}
      case "$trimmed" in
        ''|\#*) continue ;;
        *) break ;;
      esac
    done <"$f"
    if [ "$saw_marker" -ne 1 ]; then
      report "missing-rego-quarantine-marker" "$f (need locked: D-012 on/before first non-comment line)"
    fi
  done <<EOF
$(find "$rego_root" -type f ! -name '.*' 2>/dev/null || true)
EOF
fi

# --- (c) no rego: predicate leaf outside the quarantined tree -------------------
# Scoped to examples/** excluding examples/policies/rego/** (migration corpus).
rego_leaf_hits=$(
  find examples -type f ! -path 'examples/policies/rego/*' -print0 2>/dev/null \
    | xargs -0 grep -n 'rego:' 2>/dev/null || true
)
if [ -n "$rego_leaf_hits" ]; then
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    report "rego-leaf-outside-quarantine" "$line"
  done <<EOF
$rego_leaf_hits
EOF
fi

if [ "$hits" -ne 0 ]; then
  echo "migration-invariants check FAILED — fix DRAFT markers, rego quarantine, or stray rego: leaves (P3-E3-S04)" >&2
  exit 1
fi
echo "migration-invariants check passed"
