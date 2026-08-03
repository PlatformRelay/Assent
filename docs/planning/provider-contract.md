# Provider contract — the frozen `maxAge` defaults (OQ-17 residual)

The provider request/response envelope (`FactQuery`/`FactResponse`) is frozen at
[`schemas/provider/v1alpha1/`](https://github.com/PlatformRelay/assent/tree/main/schemas/provider/v1alpha1), promoted byte-for-byte from
Spike C (`hack/spikes/provider/`, P2-E3). This doc promotes Spike C's proposed per-fact-type
`maxAge` table from [`docs/planning/spikes/spike-c-provider.md`](spikes/spike-c-provider.md)
(historical evidence, unchanged) to **normative** status — the table the provider host and a
future `assent lint` implementation both read.

`maxAge` is an arming precondition (ADR-0017 §4), not advisory: a provider declaration may only
**shorten** its type's default below, never lengthen it past the global cap.

| Fact type (declaration) | Example | Default `maxAge` | Rationale |
| --- | --- | --- | --- |
| `principal` / membership & eligibility sets | group membership, approval eligibility | **1h** | authorization-bearing; revocation must propagate before a deferred merge fires |
| `boolean` authorization gates | "author is owner" | **1h** | same blast radius as principal facts |
| `string`/`integer` registry lookups | cost center exists, service registered | **24h** | slow-changing reference data; global cap applies |
| any fact declared `sensitive: true` | secret-adjacent metadata | **15m** | short-lived by policy; also subject to redaction (ADR-0012) |
| global cap (ADR-0015 §3) | — | **24h** | no declaration may exceed this regardless of type |

## Consequences frozen for P2-E5 (host implementation)

1. A fact whose `expiresAt` lands before a deferred auto-merge's arming horizon must **block
   arming** (ADR-0017 §4) rather than merely expire silently later.
2. Defaults belong in the **host**, keyed by declaration `type` + `sensitive`, so a lazy
   provider declaration inherits a safe bound instead of no bound at all — a declaration that
   omits `maxAge` is a load-time error, not a silent "no limit."
3. A provider declaration's `maxAge` is validated against this table at config load: `>` the
   type's default (or the global 24h cap, whichever is stricter) is rejected, not clamped.

## Note on schema-level `reason` enforcement (promotion-time observation)

The promoted `response.schema.json`'s `if/then` enforces that `state: resolved` requires
`value` + `expiresAt`, and any other state **forbids** `value` (`schemas/provider_test.go`'s
`TestProviderResponseStates`). It does **not** structurally require a `reason` string on
non-resolved states — `reason` remains schema-optional, matching the byte-for-byte promoted
Spike C shape (REQ-P3-E1-S03-01 forbids changing the promoted bytes). Requiring `reason` at the
schema level, if desired, is a follow-up schema change for the operator to decide, not a
promotion-time edit; logged to the INBOX.
