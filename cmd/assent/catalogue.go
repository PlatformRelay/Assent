package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/PlatformRelay/assent/internal/catalogue"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// catalogue.go is the thin filesystem+command shell for `assent catalogue <dir>`
// (E3-S07). It discovers the repo's `.assent/**` tree, loads every document via
// the E2 STRICT loader into policy.* types, and hands them to the PURE
// internal/catalogue generator, which emits the deterministic, additive-tolerant
// rule catalogue (D-017 B10) as JSON on stdout for the docs pipeline.
//
// Surface (D-048): a DISTINCT subcommand, not `assent lint --catalogue` —
// catalogue generation is a docs-pipeline artifact, not a pass/fail gate, so it
// carries no error-diagnostic exit semantics. A clean generation over loadable
// packs exits 0; a malformed pack the strict loader rejects, or an undiscoverable
// tree, is a hard error (non-zero) — the catalogue is generated over conformant
// packs (lint owns tolerant diagnosis of malformed ones).
//
// The directory walk + loader calls are the ONLY I/O; the generator is pure.

// runCatalogue is the testable entry point for `assent catalogue`. args[0] is the
// repo directory (the tree containing `.assent/**`). It returns a process exit
// code: 0 on a clean generation, 2 on a usage/discovery/load error.
func runCatalogue(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 || args[0] == "" {
		_, _ = fmt.Fprintln(stderr, "assent catalogue: a directory argument is required (usage: assent catalogue <dir>)")
		return 2
	}
	dir := args[0]

	in, err := loadCatalogueInput(dir)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent catalogue:", err)
		return 2
	}

	out, err := catalogue.Build(in).Marshal()
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "assent catalogue:", err)
		return 2
	}
	_, _ = stdout.Write(out)
	return 0
}

// loadCatalogueInput walks <dir>/.assent, loads every policy document via the E2
// strict loader, groups MergePolicy docs and pack manifests by their pack-name
// (the `packs/<name>/` directory token — the join key binding.packs[] references),
// and returns the assembled catalogue.Input.
func loadCatalogueInput(dir string) (catalogue.Input, error) {
	root := filepath.Join(dir, ".assent")
	// #nosec G703 -- dir is an operator-supplied CLI path to their own repo, read-only.
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return catalogue.Input{}, fmt.Errorf("no .assent/** tree found under %q", dir)
	}

	packPolicies := map[string][]*policy.MergePolicy{}
	packManifests := map[string]*policy.Pack{}
	var bindings []*policy.RulesetBinding

	// #nosec G703 -- operator-supplied local path, read-only walk.
	walkErr := filepath.WalkDir(root, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() || !isYAML(path) {
			return nil
		}
		raw, rerr := os.ReadFile(path) // #nosec G304,G122 -- path from walking a fixed in-repo .assent tree, not user input; no symlink surface.
		if rerr != nil {
			return rerr
		}
		kind, kerr := catalogueDocKind(raw)
		if kerr != nil {
			return fmt.Errorf("%s: %w", relTo(dir, path), kerr)
		}
		pack := packNameFromPath(dir, path)
		switch kind {
		case "MergePolicy":
			mp, lerr := policy.LoadMergePolicy(raw)
			if lerr != nil {
				return fmt.Errorf("%s: %w", relTo(dir, path), lerr)
			}
			packPolicies[pack] = append(packPolicies[pack], mp)
		case "Pack":
			pk, lerr := policy.LoadPack(raw)
			if lerr != nil {
				return fmt.Errorf("%s: %w", relTo(dir, path), lerr)
			}
			packManifests[pack] = pk
		case "RulesetBinding":
			rb, lerr := policy.LoadRulesetBinding(raw)
			if lerr != nil {
				return fmt.Errorf("%s: %w", relTo(dir, path), lerr)
			}
			bindings = append(bindings, rb)
		default:
			// A non-rule policy doc (Config) or a non-policy YAML (e.g. a test
			// facts.yaml) — the catalogue's D-017 B10 field set derives from packs
			// + the binding graph only, so nothing else is loaded.
		}
		return nil
	})
	if walkErr != nil {
		return catalogue.Input{}, fmt.Errorf("walk .assent tree: %w", walkErr)
	}

	// Assemble packs in deterministic name order (the generator sorts entries by
	// stable ID regardless, but a stable pack order keeps the walk auditable).
	names := make([]string, 0, len(packPolicies))
	for name := range packPolicies {
		names = append(names, name)
	}
	sort.Strings(names)
	packs := make([]catalogue.Pack, 0, len(names))
	for _, name := range names {
		packs = append(packs, catalogue.Pack{
			Name:     name,
			Policies: packPolicies[name],
			Manifest: packManifests[name],
		})
	}
	return catalogue.Input{Packs: packs, Bindings: bindings}, nil
}

// catalogueDocKind decodes only the top-level `kind` discriminator to route a doc
// to its loader. A doc that is not a YAML mapping is a hard error.
func catalogueDocKind(raw []byte) (string, error) {
	var header struct {
		Kind string `yaml:"kind"`
	}
	if err := yaml.Unmarshal(raw, &header); err != nil {
		return "", err
	}
	return header.Kind, nil
}

// packNameFromPath derives a MergePolicy/Pack's pack name from its path: the
// segment following a `packs/` directory (e.g. `.assent/packs/topics/rules/x.yaml`
// → "topics"), which is exactly the token binding.packs[] references. A doc not
// under a `packs/` directory falls back to its parent directory name so it still
// catalogues under a stable, deterministic key.
func packNameFromPath(dir, path string) string {
	rel := filepath.ToSlash(relTo(dir, path))
	segs := strings.Split(rel, "/")
	for i, s := range segs {
		if s == "packs" && i+1 < len(segs) {
			return segs[i+1]
		}
	}
	// Fallback: the immediate parent directory (stable for flat layouts).
	if len(segs) >= 2 {
		return segs[len(segs)-2]
	}
	return ""
}

// relTo returns path made relative to dir (slash-separated), or path unchanged if
// it cannot be relativized.
func relTo(dir, path string) string {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
