Round-3 final review of `fm/assent-github-support` (read-only). Nothing blocking is left; the two findings below are minor.

**What I ran**
- `go test -race -count=1 ./internal/forge/... ./cmd/assent/...`: all 7 packages pass, including `forge/github`, `forge/gitlab`, `conformance` and `cmd/assent`.
- `bash hack/release/action_pin_test.sh`: all 4 checks pass, ending "OK: action pin gate green".
- The checksum step of `action.yml`, run by hand against macOS's built-in `/sbin/sha256sum` (version "Darwin 1.0"). That is where finding 1 comes from.

**What I read**
- **`action.yml` (in full):**
  - **Asset name:** the runner OS/arch now map to `darwin`/`amd64`. I checked this against `.goreleaser.yaml`: the naming template `{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}` and the archive formats match. There is no `wrap_in_directory`, so `tar … assent` will find the binary at the archive root.
  - **Checksum line:** picked by exact asset name (awk on field 2), so the `.spdx.json` SBOM line can no longer be checked by mistake.
  - **Inputs:** all passed through `env:`. Nothing is interpolated inline any more.
- **`hack/release/action_pin_test.sh`:** it now asserts the name mapping, that the old `x86_64`/`macos` shapes are gone, and the exact-name checksum selection.
- **D-189 (`docs/decisions/decisions.md:195`):** records the GitLab-by-default compatibility choice and the plain-http override residual. It is explicitly an operator-owned decision, which is what I asked for.
- **Round-2 fixes, re-checked in the code rather than the comments:**
  - **First-write-wins pin (`internal/forge/github/snapshot.go:255-261`):** the pin is only written when the key differs. A re-read of the same MR, such as `CurrentHeads`, cannot move it.
  - **Read chain (`snapshot.go` `mrPinned`, lines 270-284):** the content reads go through the pin, and the compare-and-swap reads go fresh.
  - **`selectForge` (`cmd/assent/forge_select.go:28-71`):** it refuses in both directions. GitLab with a GitHub host is refused, and GitHub with any host other than the REST host is refused. The GitLab flag default becomes the GitHub default, and an empty forge with an empty endpoint fails closed.

**What I did not check**
- I did not run a real Actions job on Linux or macOS runners.
- I assumed the GitHub macOS runner image puts `/sbin/sha256sum` first on `PATH`; I did not confirm it.
- I did not re-read the round-2 CreateThread and changed-files fixes line by line. I relied on their tests passing under `-race`.

**Findings**

MINOR | `action.yml` checksum step (`awk … | sha256sum --check --strict --status -`) | On macOS, the built-in `/sbin/sha256sum` exits **0** when its input is empty. I ran this: an empty-input check gave `empty_rc=0`. So if `checksums.txt` has no line for the asset, the awk selects nothing and verification passes without checking anything on macOS runners. GNU `sha256sum` should fail on empty input, but I have not run it to confirm. In practice goreleaser always lists every archive, so this is very unlikely to trigger, but it is a fail-open in a step whose whole point is to fail closed. | Capture the line first and refuse if it is empty: `line=$(awk …); [[ -n "$line" ]] || { echo "no checksum for $asset" >&2; exit 1; }; printf '%s\n' "$line" \| sha256sum --check --strict --status -`. Add a check for this to the pin gate.

MINOR | `action.yml` input `subject` description and the `--subject "file:$ASSENT_SUBJECT"` line | The input is described as "entryRef (file:<path>)", but the step adds `file:` itself. A caller who follows the description gets `file:file:<path>`. I expect that fails loudly as a missing file rather than approving the wrong thing, but I did not test it. I found no `uses:` example in `docs/usage/` or `README.md` to show callers the right form. | Change the description to "repo-relative path of the governed file (the action adds `file:`)", or strip a leading `file:` before adding it.

VERDICT: APPROVE
