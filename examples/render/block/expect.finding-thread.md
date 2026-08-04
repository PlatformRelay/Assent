**⚠️ Deleting catalog file examples/repos/service-catalog/catalog/prod/core-services.json is blocked — restore the file or split the removal into a dedicated change.**

Resolve this thread to confirm. `file:examples/repos/service-catalog/catalog/prod/core-services.json` · rule `no-catalog-file-deletion`

<details>
<summary>Why this check exists &amp; how to fix</summary>

Whole-file catalog deletes remove service entries without a reviewable diff.
📖 [Full documentation](https://example.com/policies/catalog-deletion)

</details>

<details>
<summary>Evaluation details</summary>

- file event kind=delete
- matched change: `examples/repos/service-catalog/catalog/prod/core-services.json` delete `∅` -> `∅`
- score contribution: +0 · rule `no-catalog-file-deletion` · code `catalog-file-deleted`

</details>
