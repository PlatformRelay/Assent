**👀 Topic owner change team-payments -&gt; team-platform needs explicit approval from the current owner team.**

Resolve this thread to confirm. `topic-registry:payments.events.v1` · rule `topic-owner-must-approve`

<details>
<summary>Why this check exists &amp; how to fix</summary>

Ownership changes affect on-call routing and data-access boundaries.
📖 [Full documentation](https://example.com/policies/topic-ownership)

</details>

<details>
<summary>Evaluation details</summary>

- resolved owner=team-platform
- matched change: `/metadata/owner` modify `team-payments` -> `team-platform`
- facts used: `owner.team`=team-platform
- score contribution: +0 · rule `topic-owner-must-approve` · code `ownership-approval-missing`

</details>
