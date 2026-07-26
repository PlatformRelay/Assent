# P4-E1 S10/S11 — live adoption evidence (D-012)

assent ran end-to-end against the **live** gitlab.com repository
`konrad.heimel/assent-lab` (project 84826619; the D-037-designated real-repo lab),
NOT a synthetic testcontainer fixture. Date: 2026-07-27.

Governed subject: `topics/orders.events.v1.yaml`; policy `.assent/policy.yaml`
(one binding, one obligation `partitions-not-decreased`, one CEL rule
`int(new) >= int(old)`, `onFailure: challenge`).

| MR | Change | Decision | Forge action | Evidence |
| --- | --- | --- | --- | --- |
| [!1](https://gitlab.com/konrad.heimel/assent-lab/-/merge_requests/1) | partitions 6→3 (violates) | REVIEW | one resolvable thread (ADR-0019 marker); reruns created ZERO duplicates | `mr1-review-decision-record.json` |
| [!2](https://gitlab.com/konrad.heimel/assent-lab/-/merge_requests/2) | partitions 6→12 (satisfies) | APPROVE | `--arm`: approve + SHA-pinned merge (merged `960c6dd`, pinned to evaluated source `9a816d4`) | `mr2-approve-merge-decision-record.json` |

Both DecisionRecords validate against `schemas/decision/v1alpha1/decision-record.schema.json`
and honestly carry `mergeResultDigest: null` + `capabilityGap` (gitlab plain-merge exposes
no merge-result digest; the CAS uses a synthetic source+target digest that never leaks into
the record). Unarmed APPROVE was verified to perform ZERO writes (advisory-only).
