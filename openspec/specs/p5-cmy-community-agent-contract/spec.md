# P5-CMY — Community files and agent-contract hygiene

**Epic ID / REQ prefix:** `CMY` / `REQ-CMY-Snn-nn`. Cross-cutting hygiene epic. Keep small. Gate
wiring: see the [gate-wiring rule](../README.md) (D1).

**Problem.** Verified 2026-10-05: no `CONTRIBUTING.md`; `.github/ISSUE_TEMPLATE/` has two
Markdown templates and no `config.yml`; `CLAUDE.md` is a one-line `@AGENTS.md` import but there
is no `.github/copilot-instructions.md`; the verification discipline learned in review rounds is
not in `GUIDELINES.md`. Prior art: attune `CONTRIBUTING.md` and issue forms.

**Not in scope:** CODE_OF_CONDUCT/SECURITY (exist); PR labeler and size labels (cut: for a solo
maintainer they add a privileged workflow to protect for little reading value); Discussions; bots.

---

## CMY-S01 — Community files and agent pointers `[autonomous]`

**Depends on:** none. One PR.

- **CONTRIBUTING.md** states, linking to `AGENTS.md`/`GUIDELINES.md` rather than copying them,
  these **seven** topics: the `:gitmoji: type(scope): summary` ASCII-shortcode convention (no
  Unicode emoji); one logical change per commit; `task check` before pushing; TDD expectation;
  **rebase-merge only** (no squash); the determinism and default-deny invariants (link); the
  private vulnerability path (link to `SECURITY.md`). D-002: nothing employer/internal.
- **Issue forms:** `bug_report.yml` and `feature_request.yml` replace the `.md` templates
  (required: version, forge, policy snippet, expected/actual); `config.yml` sets
  `blank_issues_enabled: false` and routes vulnerabilities to the **private** advisory URL.
  **Adversarial** — no field asks for secrets/tokens; no public-issue security route.
- **Agent pointers:** `CLAUDE.md` keeps its `@AGENTS.md` import and
  `.github/copilot-instructions.md` is a one-line pointer to `AGENTS.md`. **Adversarial** — a
  pointer with rule text or more than ~3 non-blank lines is red (a copy is a fork).

Requirements:

- **REQ-CMY-S01-01** — CONTRIBUTING exists with the seven topics and links. Test:
  `hack/lint/community_files_test.sh` (create); Verify:
  `bash hack/lint/community_files_test.sh`; Level: doc
- **REQ-CMY-S01-02** *(adversarial)* — forms parse with required fields, old `.md` removed,
  security link private, no secret-asking field; pointers stay pointers. Test: same; Verify:
  `bash hack/lint/community_files_test.sh --self-test`; Level: L0
- **REQ-CMY-S01-03** — passes sanitization. Test: `hack/check-sanitization.sh`; Verify:
  `bash hack/check-sanitization.sh`; Level: doc

---

## CMY-S02 — Evidence and recurring-defect rules in GUIDELINES `[autonomous]`

Add four concise rules to `GUIDELINES.md` (Testing section) and one reference line from the
`specs:` rules in `openspec/config.yaml`:

1. **Evidence:** a verification row counts only if the named test visibly ran (`=== RUN` /
   `-v` output names it); a `-run` filter that matches nothing is "ok" with no evidence.
2. **Controls:** a defect/mutation control must be seen to **go red on a real defect** and a
   **no-op/equivalent change must leave it green**; a harness that reds on a no-op, or stays
   green on a real defect, is broken, and proves nothing about the test under it.
3. **Flake/race:** a pass claimed for a flaky or concurrent behaviour needs `-count=N` (N stated,
   `-race` where relevant); one green run is not evidence.
4. **Recurring defects:** a defect class seen twice gets its own automated sensor before the
   third fix.

- **Counterpoint:** more rules, more reading; four bullets, each already enforced informally.
  No token-matching test is added (a test that greps for heading words proves the words exist,
  not that the rule is followed).

Requirements:

- **REQ-CMY-S02-01** — the four rules are present in `GUIDELINES.md`. Test: `GUIDELINES.md`;
  Verify: `rg -c '^\*\*(Evidence|Controls|Flake/race|Recurring defects):' GUIDELINES.md` prints
  `4`; Level: doc

---

## Exit

S01 and S02 landed; `task check` green. The S01 guard follows the
[gate-wiring rule](../README.md): pinned `verify` steps for each mode and a `task check` stage with
`STAGE_BODY_PINS` entries.
