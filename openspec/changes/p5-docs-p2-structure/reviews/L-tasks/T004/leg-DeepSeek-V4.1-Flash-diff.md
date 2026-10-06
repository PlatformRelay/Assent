## Verdict: CLEAN

## Findings
- [NOTE] The new page's intro overstates the fidelity claim — `docs/usage/operating-safely.md:1-7` says "the text is unchanged apart from links retargeted to their new homes", but the move also changed heading level (`###`→`##`) and rewrote one sentence (the "CI snippet above" deixis), both of which the spec Goal explicitly authorises. Failure: a reader takes the intro as "only link hrefs changed" and is mildly misled; no gate reads it. Fix: reword to "unchanged apart from the navigation-only transformations". Confidence: 85.
- [NOTE] A fourth prose pointer into the moved sections survives stale: `docs/decisions/evidence/p4-e1-s11-adoption/README.md:27` calls *What gates approve and merge* "the CLI reference's", and that section now lives on the new page. Failure: a reader following the frozen record looks in `cli.md` and does not find it. It is a frozen historical record (`not_in_nav`, header says left unedited on purpose), names the title without linking, and breaks no build; the evidence dispositions it. Confidence: 60 that touching it is even desirable.
- [NOTE] `docs/adr/0009-execution-modes.md:55` similarly still names `docs/usage/cli.md §How to keep assent advisory`; spec declares ADRs immutable and dispositions this — correct, not a defect. Named only to show it was checked.

## Could not check
- `bash hack/audit/exitgate_test.sh` in full mode (no `--text-only`): it shells out to `task check` and writes a coverage profile, which plan-mode read-only forbids. I ran `--text-only` (green, incl. all corpus mutation controls) and verified the corpus arithmetic by hand (11 listed, `docs/usage/quickstart.md` pre-existing absent → 10 present; `shrunk` removes 4 → 6 < MIN 8 → red). The evidence's "full-mode red is pre-existing baseline coverage, unreachable by a docs diff" is plausible but I did not reproduce it.
- `task docs-build`: writes `site/`. I instead grepped the already-built `site/usage/operating-safely/index.html` (all five `id=` anchors present), `site/usage/cli/index.html` and `site/usage/walkthrough/index.html` (all `../operating-safely/#…` hrefs present, `id="assent-run"` present).

## What I verified
- Fidelity: re-ran the evidence's normaliser myself (old `0c83e61:cli.md:98–251` with only the two Goal-named transformations vs the new page's five sections) — diff empty, lengths equal (154/154).
- Flag rows: `-arm` (`cli.md:91`), `-checkout` (`:89`), `-pack` (`:88`) each carry caveat + `operating-safely.md#…` link; no `below`/`above` tails remain in the table; `--config` (`:87`) byte-identical to the XREV-S03-06 pin, which passes.
- Retargets: doctor link (`cli.md:114`) → `operating-safely.md#what-gates-approve-and-merge`, "above" dropped; `walkthrough.md:212` → `#how-to-keep-assent-advisory`; both anchors exist in the built site.
- Nav (`mkdocs.yml:87`, after CLI reference) and corpus (`exitgate_test.sh:270`, after `install.md`) added in the same single commit `8fffed0`.
- Gates run green: `check-sanitization.sh` (exit 0), `truthlag_pins_test.sh` (all pins), `TestNoStaleProductClaims`, `TestCLIDocCoversSubcommands`, `TestCLIDocDriftIsDetected`.
- Fitness: no allow-list/exclusion/baseline widened; the one register change (`PHRASE_CORPUS` +1) strengthens the sweep, as the spec instructs.
