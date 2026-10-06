# Operating safely

The trust-model side of running assent: what actually gates approve and merge, how to
keep a run advisory, and the checkout-tree caveats that decide when a verdict is
trustworthy. These five sections moved here from the [CLI reference](cli.md) so that page
stays a per-command, per-flag lookup — word-for-word, with heading levels and the
retargeted links as the only adjustments. The [walkthrough](walkthrough.md) walks a first
adoption end to end.

## What gates approve and merge

**Not `--arm`.** The flag is a leftover from the walking skeleton, where it did gate the
writes. Since the forge-probe wiring landed it has had exactly one effect: it echoes into
the run summary's `arm=<bool>` token. Passing it, omitting it, and passing `--arm=false`
all produce the same forge writes. Read `arm=false` in a summary as *the operator did not
pass the flag* — never as *nothing was written*; the same line's trailing clause
(`→ 3 forge operation(s) written`) is the one that says what happened.

The real gate is the **forge-probed arming precondition**, computed from the snapshot's
capability flags and **default-deny** — it is met only when all three hold:

| Precondition | Forge dossier | Refused when |
| --- | --- | --- |
| CI configuration is external/protected, not author-editable in-repo | C17 | `insecure-topology` |
| the project's *all discussions resolved* merge gate is enabled | C3 / [ADR-0009](../adr/0009-execution-modes.md) | `discussions-gate-missing` |
| the tier exposes an enforced approval-rules API (not Free) | C6/C7 | `tier-capability-gap` |

Run [`assent doctor`](cli.md#assent-doctor) to see which of the three this environment meets.
When any is unmet the run degrades to advisory: nothing is approved or merged, the summary
reads `advisory-only (arming precondition unmet, no approve/merge)`, and the exit code is
still `0`. When all three are met, an `APPROVE` **approves and merges** — with or without
`--arm`.

Further guards refuse the writes even with the precondition met: a self-modifying
`.assent/**` merge request (BLOCK, zero writes — not even a thread); a fork/untrusted MR
context (advisory-only, [ADR-0015](../adr/0015-trust-boundaries-merge-integrity.md) §8); a
controlling authorization fact past its `maxAge` at arming time
([ADR-0017](../adr/0017-contract-model-obligations.md) §4); and the pre-write SHA guard, which re-reads
the forge's current heads and refuses on drift. Each is a clean `0` with no merge.

## How to keep assent advisory

**There is no dry-run mode today.** `assent run` has no `--dry-run` flag — passing one exits
`2` with `flag provided but not defined: -dry-run` before the forge is contacted.

**The only reliable lever is leaving one of the three arming preconditions above unmet.**
Then every APPROVE degrades to `advisory-only (arming precondition unmet, no approve/merge)`
and no approve or merge is written.

**"Advisory" means no approve and no merge — not no writes.** The run still posts its summary
comment to the merge request, and on REVIEW or BLOCK one resolvable thread as well. The only
path that writes nothing at all is a self-modifying `.assent/**` merge request, which fails
closed for a separate reason.

**Do not use a rollout phase as a safety switch.** A pack's `spec.phase`
([ADR-0018](../adr/0018-policy-lifecycle-phase-profile-comparison.md)) is a *rollout* control,
not a kill switch, and using it as one can have the exact opposite effect. `observe` and `off`
exclude the capped rules from the decision **structurally** — which removes the very findings
that were withholding approval. Measured on an enforcing rule that produces a BLOCK:

| binding | `spec.phase` ceiling | decision |
| --- | --- | --- |
| `require: [signal]` | `enforce` | BLOCK |
| `require: [signal]` | `observe` | REVIEW |
| `require: [signal]` | `off` | REVIEW |
| *(no `require:`)* | any | **refused** — fails closed before any forge write |

The saving grace in the top half is the binding's `require:` list: an uncovered required
obligation is what degrades the run to REVIEW, because only an `enforce`-phase rule can mark
one covered. `require:` is **optional** in the RulesetBinding schema, but a binding that omits
it declares no required obligations, so the obligation layer is vacuous — the shape the schema
description once called "vacuously covered". That is **no longer a silent APPROVE**: `assent
run` refuses to arm on an empty `require:` (a hard error, exit `1`, zero forge writes), and
`assent lint` reports it as the `binding-require-empty` hard error. The bottom row is therefore
a broken rollout, not a safe one — the refusal is fail-closed, but it is not a working advisory
mode. A binding that has not declared a `require:` yet is exactly the first-pack-rollout case,
which is when someone reaches for `observe`: declare the obligations before rolling the pack
out.

`spec.phase` is also **inert unless you pass `--pack`**: without the flag the ceiling is
`enforce` and the manifest is never read. Editing the manifest alone changes nothing, so an
operator who edits it and reruns the [`assent run` invocation](cli.md#assent-run) — which passes no `--pack` — stays fully
enforcing.

## Symlinks in the checkout tree

**`--checkout` cannot judge a repository that contains a symlink — any symlink, anywhere
under `base/` or `head/`.** The restriction is not limited to symlinks the merge request adds
or touches. A link that predates the branch, lives in a directory unrelated to the governed
subject and to `.assent/**`, and that nobody has modified still stops the run: a plain
`LICENSE -> LICENSES/Apache-2.0.txt` present on the **base** side is enough.

The failure is loud and fail-closed — never a wrong verdict. `assent run` exits `1`, writes
**nothing** to the forge (no thread, no approval, no merge), and prints an error on stderr
naming the offending path, the checkout side it was found on, and the reason:

```text
assent run: enumerate changed-file set: list changed files: refusing "LICENSE" in checkout tree /tmp/co/base: reached through a symlink at "LICENSE" — the checkout is the content under judgment, so symlinks are refused, never followed
```

This is a trust boundary, not an oversight. With `--checkout` the local tree is the sole
authority (D-077) and `head/` is the merge-request head — contributor-authored content under
judgment — so a symlink is refused rather than followed. Following one would let a link at a
governed path substitute an off-tree file for the document being judged, or drop a path from
the changed-file set and hide an `.assent/**` edit from the self-edit guard. See
[ADR-0008](../adr/0008-change-classification-routing-scope.md) Amendment 2.

What to do about it:

- **Run without `--checkout`.** The forge snapshot then enumerates the changed-file set and
  none of the above applies — symlinks in the repository become irrelevant. This is the
  supported way to evaluate a repository that legitimately contains one; the trade-off is the
  snapshot-completeness behaviour described below. Note that the symlink refusal hardens what
  the checkout may *contain*; it does not establish which *commit* the checkout is — see
  *Known limitation: the checkout is not bound to the evaluated commit* below.
- **Or provision a symlink-free checkout** for `base/` and `head/`.

The two side directories `base/` and `head/` may themselves be symlinks — they are
operator-provisioned, and only what lies beneath them is contributor content.

Non-regular files (FIFOs, sockets, devices) under either side are refused the same way, for
the same reason. Loosening the symlink refusal will mean folding it into the opaque /
fail-safe **REVIEW** path, so such a repository gets a decision someone must look at — never
by following the link. There is no release commitment for that today.

## Known limitation: the checkout is not bound to the evaluated commit

**assent judges the tree you hand it, and nothing verifies that tree is the commit the forge
will merge.** With `--checkout` the local tree is the sole authority for the bytes under
judgment and for the changed-file set (D-077); the SHAs that pin the approval and the
compare-and-swap merge come from the forge's view of the merge request. The two are never
compared — `assent run` has no step that hashes the checkout or matches it against
`pins.sourceSha`. If they disagree, assent decides on one tree and the forge merges another.

This is a property of **how the checkout is constructed**, not a fault that fires on every
run. Two operator obligations make it a non-issue, and assent performs neither of them for
you:

- **Construct `head/` from the merge-request head commit** — the SHA the pipeline was
  triggered for — rather than from a branch tip resolved at clone time.
- **Cancel superseded pipelines on a new push.** A push landing between the clone and the
  decision leaves assent judging the older tree while the forge merges the newer head.
  This is a project setting on your forge; assent never reads it and never reports on it,
  so do not treat a green `assent doctor` as evidence that it is set.

Runs without `--checkout` are not exposed to this: the forge snapshot is then both the
enumerator and the thing the pins describe.

## Checkout-less runs and enumeration completeness

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
