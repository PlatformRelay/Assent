# CLI demo tapes (VHS)

Reproducible terminal recordings for core **assent** commands. Checked-in `.tape` files are the
source of truth (D-106); GIF/WebM output is optional and not committed until an operator approves
asset weight.

## Prerequisites

| Tool | Purpose | Install |
| --- | --- | --- |
| [Task](https://taskfile.dev/) | Build the CLI | `go install github.com/go-task/task/v3/cmd/task@latest` |
| Go (see `go.mod`) | Compile `assent` | [go.dev](https://go.dev/dl/) |
| [VHS](https://github.com/charmbracelet/vhs) **v0.10.0** (pinned) | Render tapes → GIF | `go install github.com/charmbracelet/vhs@v0.10.0` |

Verify:

```bash
task --version
go version
vhs --version   # expect v0.10.0
```

## Tapes

| Tape | Command demonstrated |
| --- | --- |
| `assent-test.tape` | `./bin/assent test examples/packs/service-catalog` |
| `assent-render-finding.tape` | `./bin/assent render --finding examples/render/block` |
| `assent-lint.tape` | `./bin/assent lint examples/packs/service-catalog` |

Each tape runs `task build` first so `./bin/assent` exists.

## Render

From the **repository root** (after installing the pinned VHS):

```bash
vhs docs/assets/demos/assent-test.tape
vhs docs/assets/demos/assent-render-finding.tape
vhs docs/assets/demos/assent-lint.tape
```

GIFs are written beside the tapes (`docs/assets/demos/*.gif`) per each tape's `Output` directive.
Do not commit GIFs until the root README demo section is updated (E9-S09 guard).

Validate tape syntax without rendering:

```bash
vhs validate docs/assets/demos/*.tape
```

## Gate script

```bash
bash hack/release/demo_test.sh
```
