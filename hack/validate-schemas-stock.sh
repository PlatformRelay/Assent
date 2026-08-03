#!/usr/bin/env bash
# validate-schemas-stock.sh — P3-P1-3 stock Draft 2020-12 validator (D-027 Opt A).
#
# Proves the frozen JSON Schemas and example contract fixtures are valid
# *outside the Go compiler*, using an off-the-shelf Draft 2020-12 validator
# (ajv-cli). Closes the P1-3 residual: before this, only Go
# (santhosh-tekuri/jsonschema via `go test ./schemas/...`) proved the schemas,
# so a structural / `$ref`-bundling bug that Go tolerates but the wider
# ecosystem rejects could ship silently.
#
# Two independent halves (kept separate on purpose):
#   1. SCHEMA VALIDITY — every schemas/**/*.schema.json compiles as a valid
#      Draft 2020-12 schema, with all cross-file absolute `$ref`s
#      (https://assent.dev/schemas/...) resolving *offline* (ajv registers
#      every schema by its `$id` via -r; it never fetches remotely — an
#      unresolved ref is a compile error, which is exactly what we want).
#   2. FIXTURE CONFORMANCE — every apiVersion/kind-bearing fixture under
#      examples/contracts/** validates against its matching schema, dispatched
#      by (apiVersion, kind) exactly as schemas/fixtures_validate_test.go does.
#      Fixtures whose apiVersion is not the frozen contract group (e.g. the
#      *-placeholder named-consumer signals doc) are SKIPPED, matching the Go
#      harness — no frozen schema is force-applied to a field a later epic owns.
#
# NON-VACUITY — the script also asserts a permanent NEGATIVE fixture
# (hack/testdata/stock-validator-negative/bad-presentation-model.json) is
# REJECTED. That fixture is well-formed except for an illegal `effect` value
# living inside a finding that PresentationModel only reaches through its
# cross-file `$ref` into decision-record.schema.json — so its rejection proves
# BOTH that the validator is non-vacuous AND that cross-file `$ref` resolution
# actually happened.
#
# The vendor keyword `x-uniqueKeys` is deliberately unknown to stock tools;
# uniqueness stays Go-enforced (D-027). ajv is run with --strict=false so the
# unknown keyword is ignored rather than erroring; this script does not and
# must not try to enforce uniqueness.
#
# Exit codes: 0 = all green, 1 = a validation/self-test failed,
#             2 = usage/environment error (e.g. ajv not installed).
#
# Requires bash >= 4.4 (uses `mapfile -d ''`) and python3 (kind extraction).
# macOS ships bash 3.2 — run under a modern bash (e.g. `brew install bash`);
# CI's ubuntu-latest bash satisfies this.
set -u
set -o pipefail

repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  echo "error: must run inside the git repository" >&2
  exit 2
}
cd "$repo_root" || exit 2

if ! command -v ajv >/dev/null 2>&1; then
  cat >&2 <<'MSG'
error: ajv (ajv-cli) not found on PATH.
  Install with:  npm install -g ajv-cli ajv-formats
  ajv-cli is the off-the-shelf Draft 2020-12 validator this check uses; it
  registers every schema by its $id so cross-file $refs resolve offline.
MSG
  exit 2
fi

if ! command -v python3 >/dev/null 2>&1; then
  echo "error: python3 not found on PATH — required for (apiVersion, kind) extraction." >&2
  echo "  Without it every fixture would skip and the check would pass vacuously." >&2
  exit 2
fi

# Common ajv flags:
#   --spec=draft2020   frozen schemas are Draft 2020-12
#   --strict=false     ignore the vendor `x-uniqueKeys` keyword (Go-enforced)
#   -c ajv-formats     honour "format": "date-time" etc. (Go enforces formats)
AJV_FLAGS=(--spec=draft2020 --strict=false -c ajv-formats)

# sed program used to indent captured validator stderr under its FAIL line.
indent_sed='s/^/    /'

schema_root="schemas"
contracts_root="examples/contracts"
neg_root="hack/testdata/stock-validator-negative"
contract_group="assent.dev/v1alpha1"

fail=0
corpus_validated=0
negatives_rejected=0
report_fail() {
  local kind="$1" detail="$2"
  printf 'FAIL [%s] %s\n' "$kind" "$detail" >&2
  fail=1
  return
}

# All schema files (sorted for determinism).
mapfile -t all_schemas < <(find "$schema_root" -name '*.schema.json' -type f | sort)
if [[ "${#all_schemas[@]}" -eq 0 ]]; then
  report_fail "no-schemas" "found no $schema_root/**/*.schema.json"
  echo "stock-validator check FAILED (P3-P1-3)" >&2
  exit 1
fi

# ref_args_excluding <target> — emit NUL-delimited `-r <file>` pairs for every
# schema except <target>, so <target> can be the -s schema without a
# duplicate-$id clash (ajv rejects a schema whose $id is already registered).
ref_args_excluding() {
  local target="$1" f
  for f in "${all_schemas[@]}"; do
    [[ "$f" = "$target" ]] && continue
    printf '%s\0%s\0' -r "$f"
  done
  return
}

# --- half 1: every schema is a valid Draft 2020-12 schema, refs resolve -------
echo "== schema validity (Draft 2020-12 compile + offline \$ref resolution) =="
for schema in "${all_schemas[@]}"; do
  mapfile -d '' -t refs < <(ref_args_excluding "$schema")
  if ajv compile "${AJV_FLAGS[@]}" -s "$schema" "${refs[@]}" >/dev/null 2>compile.err; then
    printf 'ok  %s\n' "$schema"
  else
    report_fail "schema-invalid" "$schema"
    sed "$indent_sed" compile.err >&2
  fi
  rm -f compile.err
done

# --- kind -> schema dispatch table, derived from the schemas themselves -------
# Each contract schema declares properties.kind.const; build (kind -> file) so
# the table stays in lock-step with the corpus (mirrors contractSchemasByKind
# in schemas/fixtures_validate_test.go without hand-duplicating it).
declare -A kind_to_schema
for schema in "${all_schemas[@]}"; do
  read -r k av < <(python3 - "$schema" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
p = d.get("properties", {})
k = p.get("kind", {}).get("const")
av = p.get("apiVersion", {}).get("const")
print(k if k is not None else "", av if av is not None else "")
PY
  )
  [[ -n "$k" ]] && [[ "$av" = "$contract_group" ]] && kind_to_schema["$k"]="$schema"
done

# validate_fixture <fixture> <expect: pass|reject>
# Returns 0 if the observed result matches the expectation.
validate_fixture() {
  local fixture="$1" expect="$2"
  local api_version kind
  read -r api_version kind < <(python3 - "$fixture" <<'PY'
import json, sys
try:
    d = json.load(open(sys.argv[1]))
except Exception:
    print("", ""); sys.exit(0)
if not isinstance(d, dict):
    print("", ""); sys.exit(0)
print(d.get("apiVersion", ""), d.get("kind", ""))
PY
  )
  # Skip docs that are not in the frozen contract group or carry no kind —
  # mirrors the Go harness (the *-placeholder signals doc lands here).
  if [[ "$api_version" != "$contract_group" ]] || [[ -z "$kind" ]]; then
    printf 'skip %s (apiVersion=%s kind=%s)\n' "$fixture" "${api_version:-<none>}" "${kind:-<none>}"
    return 0
  fi
  local schema="${kind_to_schema[$kind]:-}"
  if [[ -z "$schema" ]]; then
    report_fail "unknown-kind" "$fixture: kind '$kind' has no schemas/**/v1alpha1 owner"
    return 1
  fi
  mapfile -d '' -t refs < <(ref_args_excluding "$schema")
  local observed
  if ajv validate "${AJV_FLAGS[@]}" -s "$schema" "${refs[@]}" -d "$fixture" >/dev/null 2>val.err; then
    observed=pass
  else
    observed=reject
  fi
  if [[ "$observed" = "$expect" ]]; then
    printf 'ok   %s [%s] -> %s (expected %s)\n' "$fixture" "$kind" "$observed" "$expect"
    case "$expect" in
      pass) corpus_validated=$((corpus_validated + 1)) ;;
      reject) negatives_rejected=$((negatives_rejected + 1)) ;;
      *) ;;
    esac
    rm -f val.err
    return 0
  fi
  report_fail "fixture-$observed-expected-$expect" "$fixture [$kind] against $schema"
  sed "$indent_sed" val.err >&2
  rm -f val.err
  return 1
}

# --- half 2a: NEGATIVE self-test (non-vacuity + proves refs resolved) ---------
echo "== negative self-test (must be REJECTED) =="
if [[ -d "$neg_root" ]]; then
  while IFS= read -r fixture; do
    validate_fixture "$fixture" reject
  done < <(find "$neg_root" -type f \( -name '*.json' -o -name '*.yaml' -o -name '*.yml' \) | sort)
else
  report_fail "missing-negative" "$neg_root not found — the non-vacuity self-test is mandatory"
fi

# --- half 2b: real corpus fixtures (must all PASS or be skipped) --------------
echo "== contract fixtures (must PASS) =="
if [[ -d "$contracts_root" ]]; then
  while IFS= read -r fixture; do
    validate_fixture "$fixture" pass
  done < <(find "$contracts_root" -type f \( -name '*.json' -o -name '*.yaml' -o -name '*.yml' \) | sort)
else
  echo "note: $contracts_root not present — no contract fixtures to validate"
fi

# Non-vacuity backstop (F1): a check whose whole job is catching defects must
# prove it actually validated something. If python3 were missing or a corpus /
# negative dir were empty, every fixture would have skipped to a vacuous exit 0
# — assert at least one real pass and one real rejection actually happened.
if [[ "$negatives_rejected" -lt 1 ]] || [[ "$corpus_validated" -lt 1 ]]; then
  report_fail "vacuous" "expected >=1 negative rejected (got $negatives_rejected) and >=1 corpus fixture validated (got $corpus_validated) — the check ran vacuously"
fi

if [[ "$fail" -ne 0 ]]; then
  echo "stock-validator check FAILED — see FAIL lines above (P3-P1-3)" >&2
  exit 1
fi
echo "stock-validator check passed (P3-P1-3)"
