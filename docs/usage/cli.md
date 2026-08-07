# CLI reference

`assent` is a single static binary. Every command below is dispatched by the shipped
binary — this page is pinned to the binary's own help table by a test, so it cannot
drift from the tool.

Install it first (see [Install](install.md)), then check what you have:

```bash
assent version
```

## Help output

`assent --help` (also `-h`, `-help`, `help`) prints the command listing on **stdout**
and exits **0**. A bare `assent`, or an unknown command, prints the same listing on
**stderr** and exits **2** — the contract wrapper scripts rely on.

```text
assent — deterministic, policy-driven auto-merge for self-service repositories

Usage:
  assent <command> [arguments]

Commands:
  run
      Evaluate a merge request against its policy and reconcile the decision on the forge
      usage: GITLAB_TOKEN=<pat> assent run --project <id> --mr <iid> --subject file:<path> --bot-author <user> [flags]
  doctor
      Report whether this environment can arm auto-merge, and why not when it cannot
      usage: assent doctor
  lint
      Check a repository's .assent/** policy tree for hard errors
      usage: assent lint <dir>
  test
      Run a repository's .assent/tests/** adopter cases against the real engine
      usage: assent test [--update] [--coverage] <repo>
  compare
      Replay a comparison suite of baseline vs candidate policy and apply the promotion gates
      usage: assent compare <dir> | assent compare --suite <dir>
  catalogue
      Emit the generated rule catalogue for a policy tree as JSON on stdout
      usage: assent catalogue <dir>
  render
      Render a committed finding fixture as markdown for local preview
      usage: assent render --finding examples/render/<case> [--artifact finding-thread|summary] [--presentation-minimal|--presentation-full]
  eval-input
      Assemble the EvaluationInput and pinned SHAs from the CI environment
      usage: assent eval-input
  version
      Print the assent version
      usage: assent version
  help
      Print this help listing
      usage: assent help

assent run -h, assent compare -h and assent render -h list their flags.
Full command reference: https://platformrelay.github.io/assent/usage/cli/
```

## assent run

Evaluate a merge request against its policy and reconcile the decision on the forge:
read the MR, load the policy from the **target** ref, diff → classify → aggregate →
build and schema-validate the `DecisionRecord`, emit the record, then reconcile against
GitLab. The record is emitted **before** any forge write, so a run whose emit fails
aborts without touching the forge (D-122 — no record, no action).

```
GITLAB_TOKEN=<pat> assent run --project <id> --mr <iid> --subject file:<path> --bot-author <user> [flags]
```

The GitLab personal access token is read from the `GITLAB_TOKEN` environment variable
and is never a flag; without it the command exits `2` before contacting the forge.
`assent run -h` prints the same flag list.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-project` | — | GitLab numeric project id (required) |
| `-mr` | — | merge-request IID (required) |
| `-subject` | — | governed-subject entryRef (`file:<path>`) — the file diffed for evaluation (required) |
| `-bot-author` | — | bot username for the author-identity filter (required) |
| `-gitlab-endpoint` | `https://gitlab.com` | GitLab instance base URL |
| `-policy` | `.assent/merge-policy.yaml` | MergePolicy path, loaded from the target ref |
| `-binding` | `.assent/ruleset-binding.yaml` | RulesetBinding path, loaded from the target ref |
| `-config` | — | optional Config path; when set, provider posture is validated |
| `-pack` | — | optional Pack path; its `spec.phase` caps every rule's phase |
| `-checkout` | — | local checkout dir (`base/` + `head/` subtrees) used to enumerate the MR's full changed-file set; when unset, the forge snapshot is the sole enumerator (see below) |
| `-emit` | stdout | path to write the `DecisionRecord` JSON |
| `-arm` | off | sandbox arming override — approve and merge only when set **and** the decision is APPROVE |

Exit codes: `0` the run completed and produced a valid receipt (an advisory
REVIEW/BLOCK, or an APPROVE without `--arm`, is still a clean `0`); `1` a hard error
during orchestration; `2` a missing flag, a missing `GITLAB_TOKEN`, or `-h`.

### Checkout-less runs and enumeration completeness

Without `-checkout`, the forge snapshot's changed-file list is the only thing that can
see a `.assent/**` policy edit outside the governed subject — so an incomplete list
would silently starve the self-edit guard. Per [ADR-0020](../adr/0020-forge-snapshot-changed-file-completeness.md)
the adapter must therefore *prove* completeness (paginated `/diffs`, cross-checked
against the MR's `changes_count`, below a page ceiling). When it cannot, the run does
not guess and does not fail silently: the change set is marked opaque and the decision
degrades to **REVIEW** with finding code `changeset.undecidable`, carrying the gap
reason. A `DecisionRecord` is still emitted and a thread still posted; approve and
merge are impossible on that path. A `.assent/**` path that *is* visible in a partial
list still dominates to BLOCK.

With `-checkout` the local tree is the sole authority (D-077) and snapshot completeness
is not consulted.

## assent doctor

Report whether this environment can arm auto-merge, and why not when it cannot.

```
assent doctor
```

With `GITLAB_TOKEN` set (plus `CI_PROJECT_ID` and `CI_MERGE_REQUEST_IID`) it probes the
forge for verified capabilities. Without a token it falls back to an env-only
diagnostic and prints an explicit `INSECURE` banner, because env self-assertions are
spoofable by an author-editable CI job.

Exit codes: `0` arming precondition met; `1` not armed (each blocking reason is
listed); `2` the forge probe could not run.

## assent lint

Check a repository's `.assent/**` policy tree for hard errors. The directory argument
is the repository root — `assent` joins `.assent` itself.

```
assent lint <dir>
```

Exit codes: `0` no error diagnostics; `1` at least one error diagnostic; `2` usage or
discovery failure.

## assent test

Run a repository's `.assent/tests/**` adopter cases against the real engine: each case
diffs its `base/` ↔ `head/` trees with the production differ, stubs `facts.yaml` into
the resolved-fact envelope, evaluates the pack, and asserts the produced decision
equals `expect.yaml`.

```
assent test [--update] [--coverage] <repo>
```

| Flag | Meaning |
| --- | --- |
| `--update` | rewrite each failing case's `expect.yaml` from the produced actual. Refused when a `CI` environment variable is set — auto-accepting actuals in CI would ratify a regression. Run it locally and review the diff. |
| `--coverage` | read-only both-polarity completeness gate. Never writes goldens, and supersedes `--update` when both are passed. |

Exit codes: `0` every case matched (or, under `--update`, every failing case was
refreshed); `1` a mismatch, write or load error; `2` usage, discovery or CI-guard
refusal.

## assent compare

Replay a comparison suite of baseline vs candidate policy and apply the promotion
gates: load immutable replay bundles, evaluate both sides through the same engine,
classify the deltas, and gate the promotion.

```
assent compare <dir> | assent compare --suite <dir>
```

| Flag | Meaning |
| --- | --- |
| `--suite` | PolicyComparisonSuite directory, or a `suite.yaml`/`suite.json` path |
| `--baseline-profile` | baseline PolicyProfile name (overrides the suite default) |
| `--candidate-profile` | candidate PolicyProfile name (overrides the suite default) |
| `--record` | directory to write one `ComparisonRecord` JSON per case id |

Exit codes are the promotion-gate contract: `0` all gates pass; `1` a missed
destructive change; `2` a missed authorization/ownership change; `3` an unexpected
obligation removal; `4` auto-merge widening beyond the bound; `5` deltas that were not
explicitly accepted; `6` fail-closed — a load, schema, digest or classification error.

Two paths sit outside that contract and are easy to misread. `6` is also what an
unreadable or missing input directory returns, so a wrapper invoking
`assent compare "$DIR"` with `$DIR` unset reports fail-closed rather than a usage
error — check the stderr message before assuming a classification failure. And
`assent compare -h` prints the flag list and exits `0`, which here means "help was
printed", not "all gates pass".

## assent catalogue

Emit the generated rule catalogue for a policy tree as JSON on stdout, for the docs
pipeline. Catalogue generation is a docs artifact, not a gate, so it is a separate
command rather than a `lint` flag.

```
assent catalogue <dir>
```

Exit codes: `0` catalogue emitted; `2` usage, discovery or load failure.

## assent render

Render a committed finding fixture as markdown for local preview, without a live merge
request.

```
assent render --finding examples/render/<case> [--artifact finding-thread|summary] [--presentation-minimal|--presentation-full]
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `-finding` | — | render fixture directory under `examples/render/<case>` (required) |
| `-artifact` | `finding-thread` | artifact to render: `finding-thread` or `summary` |
| `-presentation-minimal` | off | omit evaluation details (verbosity `minimal`) |
| `-presentation-full` | off | show all evaluation detail blocks (verbosity `full`) |

Exit codes: `0` markdown emitted; `1` the fixture failed to load or validate; `2` a
usage error.

## assent eval-input

Assemble the EvaluationInput and pinned SHAs from the CI environment and print a
one-line summary. This is the CI-adapter smoke path: it proves the environment
boundary end-to-end without touching a policy.

```
assent eval-input
```

Exit codes: `0` assembled; `1` a required CI variable is missing or empty.

## assent version

Print the assent version.

```
assent version
```

Release archives and the Homebrew build stamp the real semver at link time. A binary
built with `go install` reports `0.0.0-dev` — see [Install](install.md).

## assent help

Print the command listing shown above on stdout and exit `0`.

```
assent help
```
