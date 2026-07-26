# Naming candidates (OQ-1)

> **Decision 2026-07-21: assent** (#4) — see D-009. Table kept for the record.
> GitHub repo live (D-014). **Domain (D-028):** `assent.dev` is owned by a third party —
> apiVersion/`$id` group rename required; replacement TBD (see OQ-1).

Criteria: short CLI-friendly command, evokes judging/gating/merging, no strong collision in
the dev-tools space, available-ish on GitHub. Collision notes are quick checks, not clearance —
whatever shortlists must be re-verified (GitHub org, domain, pkg.go.dev, trademark smell test).

| # | Name | Angle | Quick collision note |
| --- | --- | --- | --- |
| 1 | **gavel** | the judge's hammer; `gavel run` | some small projects; no giant |
| 2 | **solon** | Athenian lawgiver; short, dignified | minor academic tools |
| 3 | **praetor** | Roman magistrate who ruled on cases | scattered small repos |
| 4 | **assent** | formal agreement → approval | generic word, low collision |
| 5 | **ratify** | formally approve a change | ⚠️ Ratify (CNCF artifact verification) |
| 6 | **quorum** | enough approvals to act | ⚠️ ConsenSys Quorum (blockchain) |
| 7 | **greenlight** | the go signal | many small projects; generic |
| 8 | **turnstile** | gate that admits valid tickets one at a time | ⚠️ Cloudflare Turnstile |
| 9 | **tollgate** | pass after paying the (policy) toll | low collision |
| 10 | **drawbridge** | lowered only for the trusted | some auth projects |
| 11 | **sluice** | gate that controls flow precisely | low collision, great metaphor |
| 12 | **lockkeeper** | canal lock: raises boats safely between levels | low collision |
| 13 | **rubberstamp** | self-aware: the stamp that actually checks | memorable, cheeky |
| 14 | **signoff** | the thing reviewers give | generic; git `Signed-off-by` association |
| 15 | **imprimatur** | official "let it be printed" | rare word, low collision, longish |
| 16 | **fiat** | an authoritative decree | ⚠️ carmaker; different domain though |
| 17 | **edict** | a published rule with force | low collision |
| 18 | **tribune** | Roman official who could veto | newspapers; low tech collision |
| 19 | **custos** | Latin "guardian" | low collision, unique |
| 20 | **mergegate** | boring-but-clear descriptive fallback | "-gate" scandal suffix reading |

Operator shortlist → verify → D-00x + rename module path (D-003).
