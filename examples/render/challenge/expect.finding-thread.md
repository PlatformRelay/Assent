**⚠️ Retention shrinks from 12 to 8 — data loss possible. Sure?**

Resolve this thread to confirm. `topic-registry:orders.events.v1` · rule `partitions-must-not-shrink`

<details>
<summary>Why this check exists &amp; how to fix</summary>

Shrinking retention deletes data irreversibly once segments roll.
📖 [Full documentation](https://example.com/policies/retention)

</details>

<details>
<summary>Evaluation details</summary>

- quota max 24
- matched change: `/partitions` modify `12` -> `8`
- facts used: `quota.max_partitions`=24
- score contribution: +10 · rule `partitions-must-not-shrink` · code `partition-count-shrunk`

</details>
