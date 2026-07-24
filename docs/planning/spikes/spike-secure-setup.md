# Spike — Secure-setup adoption (OQ-24 / P2-E4)

**Problem** (ADR-0017 §9 / roast P1-8): the "under one hour" north star is unproven, and the
*secure* topology (ADR-0015 §4 protected pipeline, least-privilege token, forge merge gates)
is exactly what adopters will skip if setup is slow.

**Scope**: choose the ONE supported v1 GitLab tier + topology; draft the walkthrough and
`doctor` checklist; time a clean-room run on a real repo. **Non-goals**: `doctor`
implementation (Phase 4/5); GitHub topology (locked, D-012).

Evidence base: [forge-dossier-gitlab.md](../forge-dossier-gitlab.md) (P1-E3), ADR-0015 §4/§5,
ADR-0017 §1/§3/§9, ADR-0009 amendment (forge merge-gate settings).

---

## Supported topology

**ONE supported v1 auto-merge path:** **GitLab Premium** (GitLab.com Premium or Self-Managed
Premium / Ultimate — Ultimate is a strict superset) with the combination below.

| Dimension | Choice | Why (dossier citation) |
| --- | --- | --- |
| **Tier** | **Premium** (cheapest that can satisfy the full set) | Free lacks enforced approval rules (`require-review` gap, C6), merged-results pipelines (C13), and merge trains (C14). On GitLab.com Free there is also no project access token (C9). |
| **Pipeline-trust model** | **CI/CD configuration file in another project** — Settings → CI/CD → "CI/CD configuration file" = `ci/assent.yml@<group>/<ci-config-project>` (or equivalent path) | Available F/P/U (C17a); MR authors cannot edit the assent job definition in the product repo. Prefer over deprecated compliance pipelines. Pipeline execution policies (Ultimate, C17b) are optional hardening, not the v1 required path. |
| **Token type** | **Project access token** on the *product* project (creates bot user `project_{id}_bot_*`) | GitLab.com: Premium+ (C9). Self-Managed: any license, but this topology already requires Premium for C6/C13/C14. |
| **Token scopes / role** | Role **Developer** (or Maintainer if the project requires Maintainer to merge); scopes **`api`** (narrower scopes that cannot approve/merge/comment are insufficient). Prefer expire ≤90d; rotate via doctor/runbook. | Least privilege that can still post discussions, approve with `sha=`, and arm auto-merge / merge-train add (C1, C4, C11, C14). Never instance-wide / group Owner PATs. |
| **Merge gates (project)** | (1) `only_allow_merge_if_all_discussions_are_resolved: true`; (2) **Pipelines must succeed**; (3) **merged results pipelines** on; (4) **merge trains** on (deferred auto-merge / merge-result pinning). | C3 + ADR-0009 amendment; C13+C14 for ADR-0017 §1. Without trains, deferred auto-merge is refused (capability gap). |
| **Approval settings** | Project approval rules: ≥1 required approval; `merge_requests_author_approval: false`; `reset_approvals_on_push: true`; recommend `merge_requests_disable_committers_approval: true`. | C6/C8/C19 — forge-proven eligible approval for `require-review` (ADR-0017 §3). |
| **`.assent/**` residual gate** | Approval rule (or CODEOWNERS → `rule_type: code_owner`) requiring a **human** eligible approver on `.assent/**` — an identity the assent bot cannot satisfy. | ADR-0015 §5 defense-in-depth: bot approval must not silently cover policy self-modification. |

### What this topology enables

- Protected assent job definition (ADR-0015 §4 / §8).
- `require-review` satisfiable via approval-rules evidence chain (dossier §4).
- Deferred auto-merge with merge-result re-validation via merge trains (dossier §5).
- All-threads-resolved gate so `challenge` acknowledgement actually blocks merge (C3).

### What remains advisory-only on Free

Free may still run assent in **advisory** mode (report / dry-run, no write token, no
auto-merge arming) using an external CI config file. That is **not** the supported v1
auto-merge topology and must not be marketed as "secure setup complete."

---

## Explicitly unsupported topologies

These are refused by `assent doctor` before any auto-merge arming (ADR-0015 §8). Naming
them is load-bearing for adopters and for the roast P1-8 adoption claim.

| Topology | Why insecure / insufficient |
| --- | --- |
| **Author-editable `.gitlab-ci.yml` (or included author-controlled file) that defines the assent job, paired with a privileged token** | **Unsupported and insecure** (ADR-0015 §4, dossier §6). The MR author can edit/remove the gate and exfiltrate the token. Doctor **refuses to arm**. |
| **GitLab Free as the auto-merge tier** | Capability gaps: no enforced approval rules → `require-review` never satisfiable (C6); no merged-results / merge trains → merge-result pinning unverifiable (C13/C14); GitLab.com Free has no project access tokens (C9). Advisory-only only. |
| **Compliance pipelines as the target v1 path** | Deprecated in favour of pipeline execution policies (dossier C17 / §6). Do not document as the walkthrough target; migrate if already in use. |
| **Instance-wide / group Owner personal access token** | Violates least-privilege (ADR-0015 §6 / §8). Prefer project access token scoped to one product project. |
| **"MR-approved pipeline" without an external/protected job definition** | Approving an MR does not make an author-editable job trusted. Without C17a/C17b, the topology remains unsupported. |
| **GitHub Actions topology** | Out of scope for this spike (D-012 locked); GitHub mapping lives in the GitHub dossier and later epics. |
| **Ultimate-only pipeline execution policies as the *required* path** | Supported as optional hardening on Ultimate, but **not** the single v1 supported path — Premium + external CI config already satisfies the trust boundary at lower cost. |

---

## Doctor checklist

Every precondition `assent doctor` (Phase 4/5) must report. **Consequence**: `refuse-to-arm`
blocks auto-merge arming; `warn` is informational (adoption/hardening) but does not by itself
block arming when the refuse-to-arm set is green.

| # | Precondition | Forge setting / evidence | API call that verifies it | Failure consequence |
| --- | --- | --- | --- | --- |
| D1 | Tier can enforce approvals | Licensed Premium or Ultimate (or Self-Managed EE with approval rules) | Probe `GET /projects/:id/approval_rules` (or project approvals config) — `403`/missing endpoints ⇒ Free/CE gap | **refuse-to-arm** (C6) |
| D2 | All-discussions-resolved merge gate | `only_allow_merge_if_all_discussions_are_resolved: true` | `GET /projects/:id` → that field | **refuse-to-arm** (C3 / ADR-0009) |
| D3 | Pipelines must succeed | Project merge setting requiring successful pipeline | `GET /projects/:id` (`only_allow_merge_if_pipeline_succeeds` / equivalent) | **refuse-to-arm** (auto-merge prerequisites) |
| D4 | Merged results pipelines | Enabled for the project | `GET /projects/:id` merged-results / CI setting fields (dossier C13) | **refuse-to-arm** for *deferred* auto-merge; immediate same-run merge may still be allowed only if adapter re-checks `merge_ref` (prefer refuse until trains+merged-results proven) |
| D5 | Merge trains (merge-result pinning) | Merge trains enabled | Project CI/CD settings via API / `GET` merge-train feature flags as exposed; smoke `GET /projects/:id/merge_trains/...` capability | **refuse-to-arm** deferred auto-merge (C14 / ADR-0017 §1) |
| D6 | Protected pipeline source | CI config points outside the product repo (`ci/*.yml@other/project`) **or** Ultimate pipeline execution policy injects assent | `GET /projects/:id` → `ci_config_path` matches `@` foreign project (or policy project linkage on Ultimate) | **refuse-to-arm** if config is in-repo / author-editable (ADR-0015 §4) |
| D7 | Assent job not suppressible by MR `workflow:rules` alone | External config uses a layout that always runs the assent job (document include contract in E4 adapter); Ultimate `override_project_ci` is strongest | Inspect resolved CI config / policy; heuristic: `ci_config_path` foreign + documented template checksum/pin | **refuse-to-arm** if only in-repo `.gitlab-ci.yml` defines assent; **warn** if foreign include still allows author `workflow:rules` to skip (residual dossier §6 caveat) |
| D8 | Token is project bot, least privilege | Project access token bot (`project_*_bot_*`), not instance Owner PAT | `GET /user` → bot username shape + `GET /projects/:id/members` role ≤ Maintainer | **refuse-to-arm** if identity is human Owner/Admin with broad scopes; **warn** if Maintainer when Developer would suffice |
| D9 | Token can write findings + approve + arm | Scopes include API write for discussions / approvals / merge | Live probes: create+delete a throwaway discussion on a draft MR *or* dry capability checks documented by adapter | **refuse-to-arm** if cannot discuss/approve/merge |
| D10 | Author cannot self-approve into `require-review` | `merge_requests_author_approval: false` | `GET /projects/:id/approvals` | **refuse-to-arm** for archetypes needing `require-review`; adapter still excludes `author.id` client-side |
| D11 | Approvals reset on new push | `reset_approvals_on_push: true` | `GET /projects/:id/approvals` | **refuse-to-arm** (stale approval must not survive new commits, C19) |
| D12 | Residual gate on `.assent/**` | Approval rule or CODEOWNERS covering `.assent/**` with human eligible approvers | `GET .../approval_rules` / CODEOWNERS parse; ensure bot not sole eligible approver | **warn** if missing (ADR-0015 §5 recommended); **refuse-to-arm** only if product policy later makes it mandatory |
| D13 | Merge-train enforcement (optional) | Enforce for all users / Owner override | Settings surface / API when available (C15) | **warn** if humans can "Merge immediately" around the train (human ≠ assent write path) |
| D14 | External status checks | Ultimate-only optional defense | `only_allow_merge_if_all_status_checks_passed` | **warn** if absent (C18 — not a v1 gap) |
| D15 | `duplicate_prevention` serialization | One-publisher-per-MR: CI job has per-MR-IID `resource_group` (e.g. `assent-mr-$CI_MERGE_REQUEST_IID`), or `serve` has a configured keyed-lock backend (ADR-0019 §3) | Inspect resolved CI job for `resource_group` matching the MR-IID key pattern; or probe `serve` lock-backend config. When unverifiable → emit `duplicate_prevention: unserialized-best-effort` | **warn** when unverifiable / misconfigured — never a silent `single-writer-serialized` claim (safe default: `unserialized-best-effort`) |

Typed doctor report fields (draft for Phase 4/5): `precondition_id`, `status:
ok|warn|fail`, `evidence` (endpoint + relevant JSON fields), `consequence:
warn|refuse-to-arm`, `remediation` (link into walkthrough step).

---

## Setup walkthrough (draft — clean-room runner)

Stopwatch from step 1. Personal/demo self-service repo qualifies (D-012). Mocked assent
decision output is acceptable pre-implementation — **setup** is what is timed.

### Prerequisites (before the clock)

- GitLab.com **Premium** trial or paid project (or Self-Managed Premium+).
- Permission to create a second project in the same group (CI config project).
- Ability to create a project access token on the product project.

### Timed steps

1. **Create / pick the product repo** — empty or sample self-service config repo; default
   branch protected.
2. **Create the CI-config project** — e.g. `group/assent-ci-config`; restrict Developers
   from pushing to its default branch (Maintainers only).
3. **Author the protected assent job** in the CI-config project, e.g. `ci/assent.yml`:
   - stage + job that will eventually run `assent run` (for the spike: a no-op / echo that
     prints a mocked decision JSON is enough);
   - do **not** put the privileged token in the CI-config repo's YAML — inject via masked
     CI/CD variable on the *product* project (or elsewhere outside MR control).
4. **Point the product project at the external config** — Settings → CI/CD → CI/CD
   configuration file → `ci/assent.yml@group/assent-ci-config`.
5. **Project merge settings** — enable: all threads must be resolved; pipelines must
   succeed; merged results pipelines; merge trains.
6. **Approval configuration** — require ≥1 approval; disallow author approval; keep reset
   approvals on push; add rule/CODEOWNERS for `.assent/**` → human reviewers.
7. **Create project access token** — Developer (or Maintainer if required), scope `api`,
   short expiry; store as masked/protected CI variable `ASSENT_TOKEN` (protected branches
   only).
8. **Copy sample policy** — minimal `.assent/` tree on the **default branch** (target-ref
   trust, ADR-0015 §1); open an MR that does *not* edit `.assent/**`.
9. **First dry-run / pipeline** — open MR → confirm pipeline runs from the *external* config
   (job name visible; editing product `.gitlab-ci.yml` must not remove assent); capture
   mocked decision artifact or job log.
10. **Sanity of gates** — unresolved discussion blocks merge; resolve → merge checks clear
    (or train entry succeeds). Record whether doctor-equivalent manual checks (table above)
    would pass.

### Done criteria for the timed run

- Steps 1–10 completed by someone who did **not** author this walkthrough.
- Per-step wall-clock recorded (minutes).
- Total **< 60 minutes** ⇒ north-star holds for this topology; otherwise note which step
  blew the budget and whether wording or automation must change.

---

## Timing log

| Step | Description | Minutes | Notes |
| --- | --- | --- | --- |
| 1 | Product repo | — | *pending operator clean-room* |
| 2 | CI-config project | — | |
| 3 | Protected assent job YAML | — | |
| 4 | External `ci_config_path` | — | |
| 5 | Merge settings | — | |
| 6 | Approval rules + `.assent/**` | — | |
| 7 | Project access token + CI var | — | |
| 8 | Sample `.assent/` on default branch | — | |
| 9 | First pipeline / dry-run | — | |
| 10 | Gate sanity | — | |
| | **Total** | — | |

Paste operator timings into this table; commit an amendment (or follow-up lane) once filled.

---

## North-star verdict

**PENDING operator clean-room run.**

The supported topology above is chosen and documented; the **<1h** claim is **not** yet
confirmed. Protocol:

1. Operator (or recruited tester who did not write this doc) follows [Setup walkthrough](#setup-walkthrough-draft--clean-room-runner) on a real Premium project with a stopwatch.
2. Record per-step minutes in [Timing log](#timing-log).
3. **Done** = steps 1–10 complete + total filled; mocked decision output is OK.
4. Paste timings here (edit this file) and set this section to either:
   - **HOLDS** — total &lt; 60m for Premium + external CI config; keep vision wording; or
   - **AMEND** — total ≥ 60m; file walkthrough fixes and/or change north-star wording via
     `docs/decisions/decisions.md` (ADR-0017 §9 / roast P1-8).

Until that run returns, marketing and Phase-4 copy must treat "under one hour" as
**aspirational pending spike evidence**, not proven.

---

## References

- [forge-dossier-gitlab.md](../forge-dossier-gitlab.md) — tier matrix, C3/C6/C9/C13/C14/C17, §6 topologies
- [ADR-0015](../../adr/0015-trust-boundaries-merge-integrity.md) §4/§5/§8
- [ADR-0017](../../adr/0017-contract-model-obligations.md) §1/§3/§9
- [ADR-0009](../../adr/0009-execution-modes.md) amendment (forge-native resolution gate)
- OQ-24 in [open-questions.md](../open-questions.md)
