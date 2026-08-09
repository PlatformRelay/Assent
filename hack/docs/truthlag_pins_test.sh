#!/usr/bin/env bash
# REQ-AUD-S06-02 — grep pins for the truth-lag DOC-05/06/09/10/11 closed.
#
# These are cheap mechanisms that keep a corrected claim corrected. They cover the
# surfaces `cmd/assent`'s TestNoStaleProductClaims does NOT walk (repo-root markdown
# and examples/), plus four drift pairs that no build step checks:
#
#   DOC-05  README.md's relative links resolve on the filesystem. `mkdocs --strict`
#           cannot see them: README.md is outside docs_dir.
#   DOC-06  neither API-stability copy claims fileEvents is unimplemented, and the
#           published mirror stays byte-identical to the root file modulo the
#           docs-relative link prefixes.
#   DOC-09  the walkthrough carries no design-fiction banner, and every step carries a
#           Shipped/Planned banner.
#   DOC-10  the meta-plan Phase-5 epic table covers E1..E9 and does not bind a
#           deferred-tier concept (Rego, GitHub adapter) to one of those numbers.
#   DOC-11  the `go install` caveat names the version it actually prints.
#   DOC-02  every docs-site URL in the tree — INCLUDING .go files, since the released
#           binary prints one — carries mkdocs.yml's `site_url` prefix. The Pages path
#           is case-sensitive (the repo is `Assent`), so a lowercase spelling 404s.
#   plus    docs/adr/README.md's status column agrees with each ADR's own Status row.
#
# Every check prints PASS or FAIL and the script exits 1 if any failed, so a
# regression names the finding it reopens.
#
# WIRED (D-124/D-125): `task docs-gates` runs this and readme_smoke_test.sh, and
# `task check` runs `docs-gates`. A docs edit that reopens one of these findings reds
# the gate every developer runs before every commit.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

fails=0
pass() { echo "PASS  $1"; }
fail() { echo "FAIL  $1" >&2; fails=$((fails + 1)); }

# --- retired pre-release phrases -------------------------------------------------
# Scope: repo-root markdown + examples/**, i.e. exactly what TestNoStaleProductClaims
# leaves uncovered. README's maturity legend legitimately defines "Planned = designed
# seam, not yet implemented", so that phrase is pinned only on the contract files.
for f in README.md API_STABILITY.md examples/README.md; do
  if grep -qi 'pre-alpha' "$f"; then
    fail "DOC-08/examples: $f still carries a 'pre-alpha' claim"
  else
    pass "no pre-alpha claim in $f"
  fi
done

for f in API_STABILITY.md docs/api-stability.md; do
  if grep -qi 'not yet implemented' "$f"; then
    fail "DOC-06: $f still says something is 'not yet implemented' — fileEvents ships add/delete"
  else
    pass "DOC-06: no unimplemented claim in $f"
  fi
  if ! grep -q 'fileEvents' "$f"; then
    fail "DOC-06: $f no longer discusses fileEvents at all — the pin above went vacuous"
  else
    pass "DOC-06: $f still carries the fileEvents match-domain note"
  fi
done

# --- DOC-06: the published mirror tracks the root file ---------------------------
MIRROR_EXPECTED="$(mktemp)"
trap 'rm -f "$MIRROR_EXPECTED"' EXIT
sed -e 's|](docs/adr/|](adr/|g' -e 's|](docs/planning/|](planning/|g' API_STABILITY.md > "$MIRROR_EXPECTED"
if diff -q "$MIRROR_EXPECTED" docs/api-stability.md >/dev/null; then
  pass "DOC-06: docs/api-stability.md matches API_STABILITY.md (modulo docs-relative links)"
else
  fail "DOC-06: docs/api-stability.md has drifted from API_STABILITY.md:"
  diff "$MIRROR_EXPECTED" docs/api-stability.md >&2 || true
fi

# --- DOC-05: README relative links resolve ---------------------------------------
broken=0
checked=0
while read -r target; do
  [[ -z "$target" ]] && continue
  checked=$((checked + 1))
  # Strip any #anchor; only the file must exist.
  path="${target%%#*}"
  [[ -z "$path" ]] && continue
  if [[ ! -e "$path" ]]; then
    echo "      broken README link: $target" >&2
    broken=$((broken + 1))
  fi
done < <(grep -o '](\(docs\|examples\|hack\|internal\|cmd\|schemas\|openspec\|test\)/[^)]*)' README.md | sed -e 's/^](//' -e 's/)$//')

if [[ "$checked" -eq 0 ]]; then
  fail "DOC-05: README.md has no in-repo relative links to check — the pin would be vacuous"
elif [[ "$broken" -ne 0 ]]; then
  fail "DOC-05: $broken README.md link(s) do not resolve on the filesystem"
else
  pass "DOC-05: all $checked in-repo README.md links resolve"
fi

# --- DOC-09: walkthrough banners --------------------------------------------------
WT=docs/usage/walkthrough.md
if grep -qi 'design fiction\|Nothing below is implemented' "$WT"; then
  fail "DOC-09: $WT still opens with a design-fiction banner"
else
  pass "DOC-09: no design-fiction banner in $WT"
fi

# Per-step, not a count: each `## Step` heading must be FOLLOWED by its own
# Shipped/Planned banner before the next heading. A count comparison is not
# discriminating — extra banners elsewhere on the page would mask a step that lost one.
unbannered="$(awk '
  /^## / {
    if (step != "" && !seen) print step
    step = ""
    if ($0 ~ /^## Step /) { step = $0; seen = 0 }
    next
  }
  step != "" && /^> \*\*(Shipped|Planned)/ { seen = 1 }
  END { if (step != "" && !seen) print step }
' "$WT")"
steps="$(grep -c '^## Step ' "$WT")"
if [[ "$steps" -eq 0 ]]; then
  fail "DOC-09: $WT has no '## Step' sections — the banner pin would be vacuous"
elif [[ -n "$unbannered" ]]; then
  fail "DOC-09: walkthrough step(s) with no Shipped/Planned banner:"
  printf '      %s\n' "$unbannered" >&2
else
  pass "DOC-09: all $steps walkthrough steps carry a Shipped/Planned banner"
fi

# --- DOC-10: meta-plan epic numbering ---------------------------------------------
MP=docs/planning/meta-plan.md
# The Phase-5 epic table rows, E1..E9.
epic_rows="$(grep -c '^| E[1-9] |' "$MP")"
if [[ "$epic_rows" -ne 9 ]]; then
  fail "DOC-10: $MP Phase-5 table has $epic_rows E1..E9 rows, expected 9"
else
  pass "DOC-10: $MP lists all nine executed epics"
fi
# Deferred tiers must not be bound to an E1..E9 row (the old table had E2=Rego, E8=GitHub).
for concept in Rego GitHub serve; do
  if grep '^| E[1-9] |' "$MP" | grep -qi "$concept"; then
    fail "DOC-10: $MP binds deferred tier '$concept' to an E1..E9 row — README says E10-E13"
  else
    pass "DOC-10: deferred tier '$concept' is not an E1..E9 row in $MP"
  fi
done

# --- DOC-11: the go install version caveat ----------------------------------------
for f in README.md docs/usage/install.md; do
  if grep -q '0\.0\.0-dev' "$f" && grep -q 'go install' "$f"; then
    pass "DOC-11: $f states what \`go install\` actually reports"
  else
    fail "DOC-11: $f mentions \`go install\` without the 0.0.0-dev caveat"
  fi
done

# --- DOC-02: every docs-site URL agrees with mkdocs.yml's site_url ----------------
# GitHub *repo* URLs are case-insensitive; **Pages paths are not**. The repo is named
# `Assent`, so `platformrelay.github.io/assent/...` 404s — and one such URL is compiled
# into the released binary (`assent --help`), which is why this sweep covers .go files
# and not just markdown. `mkdocs build --strict` cannot catch it: it does not resolve
# external URLs, and the DOC-05 pin above matches only relative links.
#
# SCOPE LIMIT, deliberately: this compares the PREFIX against site_url. It proves the
# host and the case-sensitive repo segment are right; it does NOT prove the path
# resolves. A known instance of the gap: `internal/catalogue`'s DocsBase mints
# `<site_url>rules/<pack>/<rule>`, and no `rules/` space exists on the site (DOC-03) —
# correct prefix, still a 404. Liveness would need network I/O in `task check`.
SITE_URL="$(grep -m1 '^site_url:' mkdocs.yml | sed -e 's/^site_url: *//' -e 's/[[:space:]]*$//')"
if [[ -z "$SITE_URL" ]]; then
  fail "DOC-02: mkdocs.yml has no site_url — the docs-URL pin has no authority to compare against"
else
  # Strip the trailing slash so the prefix also matches `<site_url minus />rules`.
  SITE_PREFIX="${SITE_URL%/}"
  url_checked=0
  url_bad=0
  while IFS= read -r hit; do
    [[ -z "$hit" ]] && continue
    url_checked=$((url_checked + 1))
    case "$hit" in
      *"$SITE_PREFIX"*) ;;
      *) echo "      $hit" >&2; url_bad=$((url_bad + 1)) ;;
    esac
  done < <(git grep -n 'platformrelay\.github\.io' -- . ':(exclude)hack/docs/truthlag_pins_test.sh' 2>/dev/null)

  if [[ "$url_checked" -eq 0 ]]; then
    fail "DOC-02: no platformrelay.github.io URLs found anywhere — the site_url pin would be vacuous"
  elif [[ "$url_bad" -ne 0 ]]; then
    fail "DOC-02: $url_bad docs-site URL(s) do not carry mkdocs.yml's site_url prefix '$SITE_PREFIX' (Pages paths are case-sensitive)"
  else
    pass "DOC-02: all $url_checked docs-site URLs carry the site_url prefix '$SITE_PREFIX'"
  fi
fi

# --- ADR index status agrees with each ADR's own Status row ------------------------
adr_checked=0
for adr in docs/adr/0*.md; do
  base="$(basename "$adr")"
  own="$(grep -m1 '^| \*\*Status\*\* |' "$adr" | sed -e 's/^| \*\*Status\*\* | *//' -e 's/ *|$//' | awk '{print $1}')"
  # Appendices (e.g. 0013-appendix-syntax-gallery.md) carry no Status row of their own
  # and get no index row of their own; they are linked from their parent ADR's row.
  [[ -z "$own" ]] && continue
  # Only the row whose FIRST cell links this file — not a parent ADR row that mentions it.
  row="$(grep "^| \[[0-9]*\]($base)" docs/adr/README.md | head -1)"
  [[ -z "$row" ]] && { fail "ADR index: no row for $base"; continue; }
  idx="$(printf '%s' "$row" | awk -F'|' '{print $4}' | sed -e 's/^ *//' | awk '{print $1}')"
  adr_checked=$((adr_checked + 1))
  if [[ "$own" != "$idx" ]]; then
    fail "ADR index: $base says '$own' but docs/adr/README.md says '$idx'"
  fi
done
if [[ "$adr_checked" -eq 0 ]]; then
  fail "ADR index: no ADRs compared — the pin would be vacuous"
else
  pass "ADR index: $adr_checked ADR status rows agree with their files"
fi

if [[ "$fails" -ne 0 ]]; then
  echo "FAILED: $fails truth-lag pin(s) reopened" >&2
  exit 1
fi
echo "OK: all truth-lag pins green"
