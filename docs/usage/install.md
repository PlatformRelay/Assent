# Installing assent

Assent ships as a single static Go binary. Prefer a checksum-verified install for
release artifacts; use `go install` when developing from source.

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

Once tagged releases publish (E9-S05/S06):

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

Homebrew packaging is **configured but not yet available** (E9-S07b, D-107). Goreleaser
targets [`PlatformRelay/homebrew-tap`](https://github.com/PlatformRelay/homebrew-tap), which
**does not exist yet** — do not expect `brew install assent` to work until the operator creates
the tap and the first tagged release publishes a formula.

**Today:** use `go install` or `hack/install.sh` above.

**Review template:** [`hack/release/homebrew/assent.rb.template`](https://github.com/PlatformRelay/assent/blob/main/hack/release/homebrew/assent.rb.template)
shows the Formula goreleaser will commit on release (checksums are placeholders until a real tag).

**When the tap lands** (after `PlatformRelay/homebrew-tap` exists and a release runs with
`HOMEBREW_TAP_GITHUB_TOKEN`):

```bash
brew tap PlatformRelay/tap
brew install assent
assent version
```
