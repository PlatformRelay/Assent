# Installing assent

Assent ships as a single static Go binary. Prefer a checksum-verified install for
release artifacts; use `go install` when developing from source.

Only the **release archives and the Homebrew bottle carry a stamped version** —
goreleaser injects it at link time (`-s -w -X main.version={{.Version}}` in
`.goreleaser.yaml`). A `go install` build has no such injection; see the caveat below
before you rely on `assent version` for provenance.

## go install

Requires Go 1.25+ (see `go.mod`).

```bash
go install github.com/PlatformRelay/assent/cmd/assent@latest
```

Pin a tag when you need a reproducible toolchain:

```bash
go install github.com/PlatformRelay/assent/cmd/assent@v0.1.0
```

Confirm:

```bash
assent version
```

!!! warning "`go install` binaries report `0.0.0-dev`"

    `go install` does not pass the release ldflags, so the version stays at its
    compile-time default whatever ref you build:

    ```console
    $ go install github.com/PlatformRelay/assent/cmd/assent@v0.1.0
    $ assent version
    assent 0.0.0-dev
    ```

    That is cosmetic for local policy authoring (`assent lint` / `assent test`). A
    `DecisionRecord` from such a binary is still identifiable — `pins.toolDigest` is a
    sha256 over the binary's Go build info (D-120), so different builds differ regardless
    of the version string — but `pins.toolVersion` reads `0.0.0-dev` and cannot be mapped
    back to a released tag. Use the [release archive](#github-release-url-pattern) or
    [Homebrew](#homebrew) route when the version string itself has to be true.

## curl / local install script (checksum-verified)

[`hack/install.sh`](https://github.com/PlatformRelay/assent/blob/main/hack/install.sh) verifies the archive SHA256 against a
goreleaser `checksums.txt` **before** extract (D-110 — fail-closed on mismatch).
Cosign verification runs when a sibling `.sigstore.json` bundle is present; snapshot
builds without signatures skip cosign. Pass `--require-signature` to fail closed when
bundles are absent (post-S06 signed releases).

### Local snapshot (`dist/`)

After `task release-snapshot`:

```bash
./hack/install.sh \
  --archive dist/assent_*_$(uname -s | tr '[:upper:]' '[:lower:]')_*.tar.gz \
  --checksums dist/checksums.txt \
  --dest ~/.local/bin
```

Pick the archive that matches your OS/arch if the glob expands to more than one file.

### GitHub release URL pattern

Tagged releases publish under this pattern (`v0.1.0` onwards):

```bash
VERSION=0.1.0
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
BASE="https://github.com/PlatformRelay/assent/releases/download/v${VERSION}"

curl -fsSL -o "/tmp/assent_${VERSION}_${OS}_${ARCH}.tar.gz" \
  "${BASE}/assent_${VERSION}_${OS}_${ARCH}.tar.gz"
curl -fsSL -o /tmp/checksums.txt "${BASE}/checksums.txt"

./hack/install.sh \
  --archive "/tmp/assent_${VERSION}_${OS}_${ARCH}.tar.gz" \
  --checksums /tmp/checksums.txt \
  --dest ~/.local/bin
```

Default `--dest` is `/usr/local/bin` when writable, otherwise `~/.local/bin`.

## Homebrew

Assent is packaged via the
[`PlatformRelay/homebrew-tap`](https://github.com/PlatformRelay/homebrew-tap)
(E9-S07b, D-107). Formula updates ship on tagged releases when
`HOMEBREW_TAP_GITHUB_TOKEN` is set (maintainer runbook:
[`hack/release/README.md`](https://github.com/PlatformRelay/assent/blob/main/hack/release/README.md#homebrew-tap-e9-s07b-d-107)).

Third-party taps require an explicit trust step on current Homebrew:

```bash
brew tap PlatformRelay/tap
brew trust PlatformRelay/tap   # needed if brew refuses an untrusted tap
brew install assent
assent version
```

**Review template:** [`hack/release/homebrew/assent.rb.template`](https://github.com/PlatformRelay/assent/blob/main/hack/release/homebrew/assent.rb.template)
(in-repo Formula shape; release checksums are authoritative).
