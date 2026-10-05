# P5-CMY — Community files and agent-contract hygiene

**Epic ID / REQ prefix:** `CMY` / `REQ-CMY-Snn-nn`. Cross-cutting hygiene epic. Keep small.

**Problem.** Verified 2026-10-05: no `CONTRIBUTING.md` (README/AGENTS point nowhere for
outside contributors); `.github/ISSUE_TEMPLATE/` has two Markdown templates and no
`config.yml`/issue forms; no PR labelling; `CLAUDE.md` is a one-line `@AGENTS.md` import
(good) but there is no `.github/copilot-instructions.md`; and the verification discipline
learned in review rounds (a test row only counts if it visibly ran) is not written into
`GUIDELINES.md`. Prior art: attune `CONTRIBUTING.md`, issue forms, labeler config.

**Not in scope:** CODE_OF_CONDUCT/SECURITY (exist); a docs site restructure; GitHub
Discussions; bots that comment on PRs.

---

## CMY-S01 — CONTRIBUTING.md `[autonomous]`

**Depends on:** none.

- Given an outside contributor, when they open `CONTRIBUTING.md`, then it states: the
  `:gitmoji: type(scope): summary` ASCII-shortcode convention (no Unicode emoji), one
  logical change per commit, `task check` before pushing, TDD expectation, **rebase-merge
  only** (no squash), the determinism and default-deny invariants by link to `GUIDELINES.md`,
  and the private vulnerability path by link to `SECURITY.md`.
- Given D-002, then it names no employer/internal system; the sanitization script passes.
- **Edge** — it must link to, not copy, `AGENTS.md`/`GUIDELINES.md` (single source).

Requirements:

- **REQ-CMY-S01-01** — file exists with the six required topics. Test:
  `hack/lint/community_files_test.sh` (create); Verify:
  `bash hack/lint/community_files_test.sh`; Level: doc
- **REQ-CMY-S01-02** — passes `hack/check-sanitization.sh`. Test: same script; Verify:
  `bash hack/check-sanitization.sh`; Level: doc

---

## CMY-S02 — YAML issue forms + `config.yml` `[autonomous]`

**Depends on:** CMY-S01.

- Given `.github/ISSUE_TEMPLATE/`, when read, then `bug_report.yml` and `feature_request.yml`
  forms replace the `.md` templates (required fields: version, forge, policy snippet,
  expected/actual), and `config.yml` sets `blank_issues_enabled: false` and a contact link
  routing vulnerabilities to the private advisory URL, never a public issue.
- **Adversarial** — a form that asks for secrets/tokens, or a `config.yml` pointing
  security reports at public issues, reds the test.

Requirements:

- **REQ-CMY-S02-01** — forms parse as YAML with required fields; old `.md` removed. Test:
  `hack/lint/community_files_test.sh`; Verify: `bash hack/lint/community_files_test.sh --issue-forms`; Level: L0
- **REQ-CMY-S02-02** *(adversarial)* — security contact link is the private advisory URL;
  no field label matches token/secret/password. Test: same; Verify: `bash hack/lint/community_files_test.sh --self-test`; Level: L0

---

## CMY-S03 — PR labeler and size labels `[autonomous · optional]`

**Depends on:** CIH-S02 (zizmor lints the new `pull_request_target`/permissions usage).

**Justify or drop.** For a solo maintainer, labels add a workflow (`pull_request_target`,
write permissions) to protect for little reading value. **Recommendation: drop size labels;
keep a path labeler only if the operator wants triage filters; default = drop S03.**
If kept: `actions/labeler` + path map; `pull_request_target` only with no PR-head checkout
and `permissions: pull-requests: write` alone; SHA-pinned.

- **Adversarial** — given a labeler workflow that checks out PR head under
  `pull_request_target`, then red.

Requirements (only if kept):

- **REQ-CMY-S03-01** — labeler workflow pinned, minimal permissions, no head checkout.
  Test: `hack/lint/workflow_pins_test.sh`; Verify: `bash hack/lint/workflow_pins_test.sh`; Level: L0

---

## CMY-S04 — Agent-instruction parity `[autonomous]`

**Depends on:** none.

- Given `AGENTS.md` is the single source, when `CLAUDE.md` and
  `.github/copilot-instructions.md` are read, then each is a one-line pointer to
  `AGENTS.md` (CLAUDE.md keeps its `@AGENTS.md` import), no duplicated rules.
- **Adversarial** — given either pointer file grows beyond a pointer (more than ~3 non-blank
  lines or any rule text), when the test runs, then red (drift guard: a copy is a fork).

Requirements:

- **REQ-CMY-S04-01** — both pointers exist and reference `AGENTS.md`. Test:
  `hack/lint/agent_pointers_test.sh` (create); Verify:
  `bash hack/lint/agent_pointers_test.sh`; Level: L0
- **REQ-CMY-S04-02** *(adversarial)* — a pointer with duplicated rule text reds. Test: same;
  Verify: `bash hack/lint/agent_pointers_test.sh --self-test`; Level: L0

---

## CMY-S05 — Evidence rule + recurring-defects rule in GUIDELINES `[autonomous]`

**Depends on:** none.

Add to `GUIDELINES.md` (Testing section), concisely:

1. **Evidence:** a verification row counts only if the named test visibly ran (`=== RUN`
   / `-v` output names it) — a filtered `-run` that matches nothing is "ok" with no
   evidence.
2. **Controls:** a mutation/defect control needs a **no-op control** that must survive;
   a control that "kills" the no-op proves the harness, not the test.
3. **Flake/race:** a pass for a concurrency/flaky claim needs `-count=N` (N stated; `-race`
   where relevant); a single green run is not evidence.
4. **Recurring defects:** a defect class seen twice gets its own automated sensor (lint,
   test, or gate) before the third fix.

- Given the spec workflow template (`openspec/config.yaml` rules), when read, then rule (1)
  is referenced from the `specs:` rules (one line).
- **Counterpoint:** more rules, more reading; kept to four bullets and each already
  enforced informally in review rounds.

Requirements:

- **REQ-CMY-S05-01** — `GUIDELINES.md` contains the four rules, each findable by a stable
  heading token (`Evidence:`, `Controls:`, `Flake/race:`, `Recurring defects:`). Test:
  `hack/lint/guidelines_rules_test.sh` (create); Verify:
  `bash hack/lint/guidelines_rules_test.sh`; Level: doc
- **REQ-CMY-S05-02** *(adversarial)* — removing any token reds (self-test). Test: same;
  Verify: `bash hack/lint/guidelines_rules_test.sh --self-test`; Level: doc

---

## Exit

S01, S02, S04, S05 landed (S03 landed or recorded as dropped); `task check` green. The new
`hack/lint/*` scripts hook into an existing `check:` stage; none adds a stage, so
`CHECK_STAGES` is unchanged, but each hooked command gets its own `STAGE_BODY_PINS` entry (`hack/audit/exitgate_test.sh:207`) so dropping it from the stage reds the exit gate.
