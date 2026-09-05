# ADR-0022: Container image distribution and OCI namespace layout

| | |
| --- | --- |
| **Status** | Proposed |
| **Date** | 2026-09-05 |
| **Deciders** | |
| **Context links** | ADR-0015 (trust boundaries), D-109 (cosign/SBOM/SLSA), Kollect ADR-0709 + DIST-AH-03 (the migration this avoids), `docs/usage/walkthrough.md` Step 5 |

## Context

assent is a CI gate. Its documented adoption path is "drop it into a repo's CI pipeline"
— and there is nothing to drop in. `docs/usage/walkthrough.md:223` says so in the
flagship GitLab example:

```yaml
assent:
  image: golang:1.25            # no ghcr.io/…/assent image is published yet — install in-job
  before_script:
    - go install github.com/PlatformRelay/assent/cmd/assent@v0.1.0
```

Every adopter therefore compiles a Go toolchain image's worth of dependencies on every
merge request, and the example is flagged **"one caveat below: no container image is
published"** in its own heading. Three releases exist (v0.1.0, v0.2.0, v0.3.0) and
`ghcr.io/platformrelay/assent` has never been pushed to — anonymous `tags/list` on both
`platformrelay/assent` and `platformrelay/charts/assent` returns 403 (nonexistent).

The in-job install also degrades the product's own evidence chain. The comment concedes
that `go install` stamps the binary `0.0.0-dev`, so **every `DecisionRecord` an adopter
emits from the documented pipeline carries `pins.toolVersion: 0.0.0-dev`**. For a tool
whose thesis is deterministic, auditable decisions, the recommended wiring produces
records that cannot name the tool that made them. (The example is also pinned to
`@v0.1.0` while `@latest` is v0.3.0 — stale, and worth fixing in the same change.)

What makes this decidable *now* rather than at first need: the sibling repo just spent
several sessions on DIST-AH-03 unpicking an OCI namespace that was never designed. The
findings are fresh and transferable, and they are all about choices made at the *first*
push. assent has not made its first push yet. This is the one moment where the layout is
free to choose.

### What Kollect paid for, that assent can have for nothing

**DR-FIND-07 — one OCI repository, two artifact kinds.** `ghcr.io/platformrelay/kollect`
holds the Helm chart at bare semver tags (`0.18.0`) and the controller image at
`v`-prefixed tags (`v0.18.0`). Both resolve to valid digests, so *every string-level
check passes* while consumers and scanners silently get the wrong artifact kind. It cost
a chart-path migration, six re-signed copies, and a permanently mixed legacy path that
can never be cleaned because the image digests are pinned in already-merged OLM bundles.

**The consequence that is easy to miss:** the collision was not fixable by configuration.
Artifact Hub's `ignore:` list filters *indexing*, strictly after `preparePackage()` has
already downloaded the artifact — so it can never suppress a load error. Two separate
sessions were spent tuning a regex that structurally could not work. The only lever was
what gets pushed to the path.

## Options

### D1 — where the image lives

| Option | Pros | Cons |
| --- | --- | --- |
| **A. `ghcr.io/platformrelay/assent`** (bare name = image) | Matches the org's settled convention — Kollect's image is at the bare `platformrelay/kollect`, its chart at `platformrelay/charts/kollect`. Shortest reference in adopter CI, which is the string most-copied into other people's pipelines. No migration for the artifact kinds assent actually has. | The bare name is "spent" on the image, so a future non-image artifact needs its own prefix (which is the point — see the reservation below). |
| B. `ghcr.io/platformrelay/images/assent` | Kind-explicit for every artifact, symmetric with `charts/`. | Contradicts the convention just established next door; Kollect's image cannot move to match (digests pinned in published OLM bundles), so the org would be permanently inconsistent in the opposite direction. Longer string in every adopter pipeline for no adopter-visible benefit. |
| C. Defer — keep compiling in-job | Zero work. | Leaves the documented adoption path emitting `toolVersion: 0.0.0-dev`, and leaves the namespace undesigned for whenever the first push does happen. |

### D2 — Artifact Hub registration

**Decided by the operator, against the recommendation originally drafted here.** That recommendation
(don't register — assent is not a Kubernetes artifact) is preserved in "Counterpoints considered"
rather than deleted, because the maintenance cost it predicted is real and should be visible to
whoever carries it. The decision is to register, so what matters now is registering it *correctly*,
and the container-image kind has requirements the Helm kind does not.

| Option | Pros | Cons |
| --- | --- | --- |
| **A. Register as an Artifact Hub `container image` repository** | One discoverable surface for the whole org rather than a listing that covers Kollect and silently omits assent. Verified Publisher carries over the same ownership proof already in use next door, so the mechanism is understood. Forces the image to carry proper OCI metadata (below), which is worth having regardless of who reads the listing. | Tag list is **manually curated and capped at 10**, so releases and the listing can drift apart. Adds a tracking surface that can email on failure. |
| B. Do not register | No new maintenance surface. | Leaves the org's public presence inconsistent; declines a decision the operator has made. |

**What the container-image kind actually requires** — verified against Artifact Hub's documentation,
because it differs from the Helm path in ways that would otherwise be found at registration time:

- **URL** `oci://ghcr.io/platformrelay/assent` — registry host mandatory, **no tag**.
- **Tags are configured by hand in the control panel, maximum 10 per repository**, each marked
  *immutable* (processed once) or *mutable* (re-processed periodically). This is the operationally
  significant one: unlike the Helm kind, Artifact Hub does **not** discover new versions by listing
  the registry. A new release does not appear until someone adds its tag.
- **Three labels are mandatory on every listed image tag**, and a tag missing any of them does not
  appear at all:
  - `io.artifacthub.package.readme-url`
  - `org.opencontainers.image.created` (RFC3339)
  - `org.opencontainers.image.description`
  Optional and worth setting: `org.opencontainers.image.title`, `org.opencontainers.image.vendor`,
  `io.artifacthub.package.license`, `io.artifacthub.package.maintainers`.
- **Ownership / Verified Publisher** uses the same mechanism as the Helm path: `artifacthub-repo.yml`
  pushed to the `artifacthub.io` tag with media type
  `application/vnd.cncf.artifacthub.repository-metadata.layer.v1.yaml`. Same `oras push` shape that
  Kollect uses, so it is a known quantity.

**The tag cap changes the release process, so it is decided here rather than discovered later.**
Publish a **mutable `latest`** plus **immutable `vX.Y.Z`** tags, and treat the ten slots as a rolling
window: `latest` is permanent, the nine most recent releases fill the rest, and the oldest is dropped
from the *listing* (never from the registry — the image stays pullable and the digest stays valid;
only its Artifact Hub row goes). `latest` being mutable is what keeps the page alive between manual
edits, so the listing degrades to "current" rather than to "stale" when nobody curates it.

## Decision

**D1: publish the multi-arch image at `ghcr.io/platformrelay/assent`, public, cosign-signed,
and hold `ghcr.io/platformrelay/charts/assent` in reserve for a chart that does not exist
yet.** One artifact kind per OCI repository, from the first push, permanently. The bare
name carries the image because that is what the org already does next door and because it
is the string that gets copied into adopters' pipelines.

**D2: register assent on Artifact Hub as a `container image` repository**, at
`oci://ghcr.io/platformrelay/assent`, with Verified Publisher via `artifacthub-repo.yml`
pushed to the `artifacthub.io` tag, and a curated tag window of mutable `latest` plus the
nine most recent immutable `vX.Y.Z` tags. Registration happens **after** the image is
public and carries the three mandatory labels — registering a repository whose tags cannot
be loaded is precisely how Kollect generated tracking-error mail for two sessions.

Should assent later ship a Helm chart, it goes to the reserved `charts/assent` path and is
registered as a **separate** Artifact Hub repository of kind `helm`. One repository entry
per artifact kind, mirroring the OCI layout — not a second kind bolted onto this entry.

## Consequences

**Easier.** Adopter CI becomes `image: ghcr.io/platformrelay/assent:v0.3.0` with no
`before_script` — and `pins.toolVersion` starts naming a real release, which is the
evidence property the product claims. The image digest is a stronger `pins.toolDigest`
than a locally compiled binary.

**Committed to.** Two things. First, never pushing a second artifact kind to
`platformrelay/assent` — a discipline, not a mechanism, since GHCR will happily accept a
chart there, which is why step 5 asserts it. Second, **a manual step in every release**:
adding the new tag to Artifact Hub's ten-slot curated list. The container-image kind does
not discover versions by listing the registry, so an unattended release publishes an image
that the listing never mentions. That is the standing cost of D2 and it is why `latest` is
mutable — the page stays current on its own even when the per-release step is missed.

**Reversible?** The path choice is effectively permanent once a digest is referenced by
an adopter pipeline or pinned in a `DecisionRecord`. That asymmetry is the argument for
deciding it before the first push rather than after — Kollect's image could not move for
exactly this reason.

### Four defects inherited from DIST-AH-03 — do not re-implement them

These were found by executing the Kollect migration, not by reading it. They apply to any
new signing/publishing code:

1. **`cosign copy` does not carry referrers.** It is deprecated in cosign 3.x, and copying
   a signed artifact with it moves the manifest and silently drops the signature — the
   digest assertion still passes. Use `oras copy -r`. Verify with `oras discover` on both
   ends, never by comparing digests alone. *(A digest match is not a signature check.)*
2. **cosign identity regexps are case-sensitive Go RE2, and GitHub's OIDC SAN preserves
   canonical repo casing** — `PlatformRelay/Assent`. assent already handles this correctly
   with the `[Aa]ssent` character class in `SECURITY.md`, `hack/install.sh` and
   `hack/release/verify-artifacts.sh`, graded by `hack/release/install_cosign_pin_test.sh`.
   **Kollect does not** — its published `docs/RELEASE.md` verify command uses lowercase
   `platformrelay/kollect` and has never verified any release for anyone. Any image-verify
   command added here must reuse assent's existing pinned pair, and the existing gate must
   be extended to cover the new invocation rather than a second convention starting up.
3. **GHCR returns base64(PAT), not a JWT**, from its token endpoint under PAT auth — so
   scope-probing by decoding `.access[].actions` cannot work there and dies as a bare
   non-zero exit under `set -e`/`pipefail`. Don't write that probe.
4. **A dry run that cannot authenticate to the destination reports DENIED, not "would
   succeed"** — distinguish "no credential" from "no permission" in any preflight, or the
   rehearsal fails for a reason unrelated to the thing being rehearsed.

## Counterpoints considered

**"Publishing an image widens the supply-chain surface of a security tool."** It does, and
ADR-0015 makes assent's trust boundaries load-bearing. But the status quo is not smaller:
today every adopter pulls `golang:1.25` and resolves the full module graph inside their own
CI, on every MR. A signed, SBOM-carrying, digest-pinnable distroless image with one static
binary is a *reduction* in what adopters execute — and it is the artifact form the trust
model can actually reason about.

**"Wait for demand — nobody has asked for an image."** The walkthrough asks for it, twice,
in the voice of the project. And the cost of waiting is not zero: it is paid in
`toolVersion: 0.0.0-dev` on every DecisionRecord produced by the documented pipeline, which
is a defect in the product's central claim rather than a missing convenience.

**"Artifact Hub adds a maintenance surface with no audience."** This was the recommendation
originally drafted here, and it was overruled — recorded because the cost it names is real
and lands on whoever maintains the listing. Artifact Hub is a Kubernetes-artifact discovery
surface; assent is a CI-job gate, so the audience overlap is genuinely unproven. Concretely
the cost is: ten manually curated tag slots that do not track releases automatically, three
mandatory labels whose absence silently hides a tag, and a tracker that emails on failure —
the same class of mail DIST-AH-03 spent two sessions eliminating next door. The decision to
register stands; the mitigations are in the plan (mutable `latest` so the page cannot go
stale unattended, labels asserted in CI so a tag cannot be silently unlisted, and
registration deliberately sequenced *after* the image is public and loadable). If the
listing later proves to draw no traffic, this is the paragraph to reread — delisting is
cheap and loses nothing, because the image itself is the artifact and the registry is its
home.

**"The bare name should be reserved for something more important later."** This is the
Kollect mistake in prospective form — `platformrelay/kollect` was left to accumulate
whatever needed publishing, and got two kinds. Naming the bare path for exactly one kind,
now, is what makes the reservation of `charts/assent` meaningful.

## Executable plan

Steps 1–5 are one lane; steps 6 and 8 are maintainer web-UI actions with no API; steps 7,
9 and 10 follow a release. **Nothing about Artifact Hub happens until the image is public and
loadable** — registering a repository whose tags cannot be loaded is exactly how Kollect
generated tracking-error mail.

1. **`Dockerfile`** — distroless static base, `COPY` the goreleaser-built binary (already
   `CGO_ENABLED=0`, `-trimpath`, linux/amd64+arm64). No compile stage: goreleaser has built
   the binary before the image step, and rebuilding in Docker would produce a *different*
   binary than the one that was signed and SBOM'd.
2. **`.goreleaser.yaml`** — add a `dockers_v2` (or `dockers` + `docker_manifests`) block
   producing a multi-arch manifest list for `ghcr.io/platformrelay/assent`, tags `v{{.Version}}`
   and `latest`. Reuse the existing `builds.id: assent` artifacts. **Set the three labels
   Artifact Hub requires** — a tag missing any of them is silently omitted from the listing
   rather than reported:
   - `io.artifacthub.package.readme-url` → the raw README URL at the release tag (pin to the
     tag, not to `main`, or an old image's page re-renders as the current docs)
   - `org.opencontainers.image.created` → `{{.Date}}` (RFC3339)
   - `org.opencontainers.image.description`
   Plus `org.opencontainers.image.title`, `.vendor`, `.source`, `.revision`,
   `io.artifacthub.package.license`, `io.artifacthub.package.maintainers`.
3. **`.github/workflows/release.yaml`** — add `packages: write` to the publish job's
   permissions, `docker/login-action` against ghcr.io with `GITHUB_TOKEN`, and after push
   `cosign sign --yes ghcr.io/platformrelay/assent@<digest>` (keyless, same OIDC identity as
   the existing blob signing). Sign the **digest**, never the tag.
4. **Verify docs** — extend `SECURITY.md` with the image-verify command using the *existing*
   pinned identity pair, and extend `hack/release/install_cosign_pin_test.sh` to grade the
   new invocation. Per defect 2 above, the gate is universal over invocations, so a new
   unpinned call must red the build; confirm that by mutation rather than assumption.
5. **Two release-workflow assertions**, both against the registry the workflow just pushed to:
   - **Collision** — `platformrelay/assent` holds only image manifests (every tag's media type
     is an image manifest or manifest list, no `application/vnd.cncf.helm.*`). This is the check
     whose absence cost Kollect a migration; ~10 lines.
   - **Labels** — the three mandatory Artifact Hub labels are present and non-empty on the
     pushed manifest. Assert it here, where it fails loudly, rather than discovering it as a
     tag that quietly never appears on the listing.
6. **Maintainer, web UI, once** — GHCR creates the package **private** on first push. Set it
   public at `https://github.com/orgs/PlatformRelay/packages/container/package/assent`
   → Package settings → Change visibility → Public. There is no API for this; an unpublicised
   image fails every adopter pull with a 403 that reads like a missing tag.
7. **Docs** — rewrite `docs/usage/walkthrough.md` Step 5 to `image: ghcr.io/platformrelay/assent:vX.Y.Z`
   with no `before_script`, delete the "no container image is published" caveat from the
   heading, and fix the stale `@v0.1.0` pin in the same pass. Update `docs/usage/install.md`
   with the container route and its verify command.

8. **Maintainer, web UI, once** — register the repository on Artifact Hub: kind **container
   image**, URL `oci://ghcr.io/platformrelay/assent` (no tag), owned by the same account that
   owns the `kollect` entry so Verified Publisher can match. Add the tags: **`latest` as
   *mutable*** and the current **`vX.Y.Z` as *immutable***. Then copy the `repository_id` the
   hub assigns — step 9 needs it.
9. **`artifacthub-repo.yml` + push** — add the file at the repo root carrying that
   `repositoryID` and an `owners:` entry whose email matches the Artifact Hub account
   (the hub compares the two addresses directly; it does not send a verification mail). Push it
   from the release workflow, same shape Kollect uses:

   ```
   oras push ghcr.io/platformrelay/assent:artifacthub.io \
     --config /dev/null:application/vnd.cncf.artifacthub.config.v1+yaml \
     artifacthub-repo.yml:application/vnd.cncf.artifacthub.repository-metadata.layer.v1.yaml
   ```

   Note this consumes one of the ten tag slots only if it is *listed* — do not add
   `artifacthub.io` to the curated tag list; it is metadata, not a version.
10. **Release checklist entry** — adding the new `vX.Y.Z` tag in the Artifact Hub control panel
    is a **manual step per release** and belongs in the release runbook, not in tribal memory.
    Ten slots: keep `latest` plus the nine most recent, dropping the oldest *from the listing
    only*. Nothing is ever deleted from the registry.

**Ordering.** 6 gates 7 (docs must not tell adopters to pull a private image) and 6 gates 8
(never register a repository whose tags cannot be loaded). Step 5's assertions are only
meaningful once something has been pushed. So: run one real release through 1–5 → make the
package public (6) → land the docs (7) → register on Artifact Hub (8) → push the metadata and
confirm Verified Publisher on the next tracking run (9) → record the per-release manual step (10).

**One caution carried from DIST-AH-03.** After registration, judge the listing by
`last_tracking_ts` advancing past the moment of change — not by the error list, which is a
frozen snapshot from the previous run and reads as a live verdict when it is not.
