#!/usr/bin/env bash
# check-sanitization.sh — open-source hygiene gate (D-002, REQ-P1-E1-S01-02).
#
# Scans the working tree for content that must never be committed to a public repo:
#   1. terms from a workspace-local denylist file (path in $ASSENT_SANITIZE_DENYLIST,
#      optional, NEVER committed) — matched case-insensitively as fixed strings;
#   2. built-in generic patterns: internal-looking hostnames (*.corp, *.internal,
#      *.intranet, *.lan) and employee-ID shapes (one uppercase letter + six digits).
# Obvious base64-encoded YAML values are decoded and matched too, so an encoded
# denylisted term is still caught.
#
# Exit codes: 0 = clean, 1 = hit found, 2 = usage/environment error.
set -u

repo_root=$(git rev-parse --show-toplevel 2>/dev/null) || {
  echo "error: must run inside the git repository" >&2
  exit 2
}
cd "$repo_root" || exit 2

self="hack/check-sanitization.sh"

# Built-in patterns (portable ERE — no \b, works with both BSD and GNU grep).
host_pat='[A-Za-z0-9][A-Za-z0-9_-]*\.(corp|internal|intranet|lan)($|[^A-Za-z0-9])'
empid_pat='(^|[^A-Za-z0-9])[A-Z][0-9]{6}($|[^0-9])'

hits=0

report() { # $1=source $2=kind $3=match-line
  local source="$1" kind="$2" match_line="$3"
  printf 'HIT [%s] %s: %s\n' "$kind" "$source" "$match_line" >&2
  hits=1
  return
}

decode_b64() { # stdin -> decoded stdout; tolerate both BSD (-D) and GNU (-d)
  base64 -d 2>/dev/null || base64 -D 2>/dev/null || true
}

# Denylist: optional, workspace-local, one term per line (# comments allowed).
denylist_terms=""
if [[ "${ASSENT_SANITIZE_DENYLIST:-}" != "" ]]; then
  if [[ ! -f "$ASSENT_SANITIZE_DENYLIST" ]]; then
    echo "error: ASSENT_SANITIZE_DENYLIST points to a missing file: $ASSENT_SANITIZE_DENYLIST" >&2
    exit 2
  fi
  denylist_terms=$(grep -v -e '^[[:space:]]*#' -e '^[[:space:]]*$' "$ASSENT_SANITIZE_DENYLIST" || true)
fi

scan_text() { # $1=source-label ; content on stdin
  local src="$1" content line
  content=$(cat)
  [[ -n "$content" ]] || return 0

  line=$(printf '%s\n' "$content" | grep -E -n "$host_pat" | head -5)
  [[ -n "$line" ]] && report "$src" "internal-hostname" "$line"
  line=$(printf '%s\n' "$content" | grep -E -n "$empid_pat" | head -5)
  [[ -n "$line" ]] && report "$src" "employee-id" "$line"

  if [[ -n "$denylist_terms" ]]; then
    while IFS= read -r term; do
      [[ -n "$term" ]] || continue
      line=$(printf '%s\n' "$content" | grep -F -i -n -- "$term" | head -5)
      [[ -n "$line" ]] && report "$src" "denylist" "$line"
    done <<EOF
$denylist_terms
EOF
  fi
  return 0
}

# Files under version control plus new not-yet-tracked files (pre-commit use case).
files=$(git ls-files --cached --others --exclude-standard | grep -v -F -x "$self" || true)

while IFS= read -r f; do
  [[ -f "$f" ]] || continue
  scan_text "$f" <"$f"

  # Adversarial case: base64-encoded YAML values still get matched after decoding.
  case "$f" in
    *.yaml|*.yml)
      tokens=$(grep -E -o '[A-Za-z0-9+/]{16,}={0,2}' "$f" || true)
      while IFS= read -r tok; do
        [[ -n "$tok" ]] || continue
        # No pipeline into scan_text: it must run in this shell so hits propagate.
        decoded=$(printf '%s' "$tok" | decode_b64 | LC_ALL=C tr -c '[:print:]\n' ' ')
        [[ -n "$decoded" ]] || continue
        scan_text "$f (base64-decoded value)" <<EOD
$decoded
EOD
      done <<EOF
$tokens
EOF
      ;;
    *) ;;
  esac
done <<EOF
$files
EOF

if [[ "$hits" -ne 0 ]]; then
  echo "sanitization check FAILED — remove the flagged content (D-002)" >&2
  exit 1
fi
echo "sanitization check passed"
