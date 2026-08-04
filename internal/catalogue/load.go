package catalogue

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/PlatformRelay/assent/internal/core/policy"
)

// LoadFromDir walks <dir>/.assent, loads every policy document via the E2 strict
// loader, groups MergePolicy docs and pack manifests by their pack-name (the
// `packs/<name>/` directory token — the join key binding.packs[] references), and
// returns the assembled Input. This is the shared catalogue loader consumed by
// `assent catalogue`, `assent test`, and `assent compare` (D-112 / PCS-S01).
func LoadFromDir(dir string) (Input, error) {
	root := filepath.Join(dir, ".assent")
	// #nosec G703 -- dir is an operator-supplied CLI path to their own repo, read-only.
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return Input{}, fmt.Errorf("no .assent/** tree found under %q", dir)
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
		kind, kerr := docKind(raw)
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
			// Non-rule policy docs and non-policy YAML are skipped — the catalogue
			// and profile activation derive from packs + the binding graph only.
		}
		return nil
	})
	if walkErr != nil {
		return Input{}, fmt.Errorf("walk .assent tree: %w", walkErr)
	}

	names := make([]string, 0, len(packPolicies))
	for name := range packPolicies {
		names = append(names, name)
	}
	sort.Strings(names)
	packs := make([]Pack, 0, len(names))
	for _, name := range names {
		packs = append(packs, Pack{
			Name:     name,
			Policies: packPolicies[name],
			Manifest: packManifests[name],
		})
	}
	return Input{Packs: packs, Bindings: bindings}, nil
}

// docKind decodes only the top-level `kind` discriminator to route a doc to its
// loader. A doc that is not a YAML mapping is a hard error.
func docKind(raw []byte) (string, error) {
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
// → "topics"), which is exactly the token binding.packs[] references.
func packNameFromPath(dir, path string) string {
	rel := filepath.ToSlash(relTo(dir, path))
	segs := strings.Split(rel, "/")
	for i, s := range segs {
		if s == "packs" && i+1 < len(segs) {
			return segs[i+1]
		}
	}
	if len(segs) >= 2 {
		return segs[len(segs)-2]
	}
	return ""
}

func relTo(dir, path string) string {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func isYAML(path string) bool {
	return strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml")
}
