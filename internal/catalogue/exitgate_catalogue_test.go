package catalogue_test

// exitgate_catalogue_test.go is the catalogue half of the E3-S08 exit gate
// (REQ-E3-S08-03): the generated rule catalogue is produced COMPLETELY and
// DETERMINISTICALLY over the loaded, lane-C-conformant archetype packs
// (examples/packs/{service-catalog,infra-vars} — topic-registry is pinned/excluded
// per D-052, it does not load). It loads every `.assent/**` policy document through
// the E2 strict loader (the same authority `assent catalogue` uses), Builds the
// catalogue, and asserts every authored rule surfaces with the D-017 B10 field set,
// canonically sorted and byte-identical across runs.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/catalogue"
	"github.com/PlatformRelay/assent/internal/core/policy"
)

// archetypePacksDir is the conformant starter-pack corpus, relative to this
// package (internal/catalogue).
const archetypePacksDir = "../../examples/packs"

// conformantPacks are the lane-C-conformant packs the catalogue generates over —
// the exact set that loads (service-catalog + infra-vars); topic-registry is
// pinned as a known blocker (D-052) and is deliberately absent.
var conformantPacks = []string{"service-catalog", "infra-vars"}

// TestCatalogueGeneratesFromArchetypeCorpus is the REQ-E3-S08-03 gate.
func TestCatalogueGeneratesFromArchetypeCorpus(t *testing.T) {
	in := loadArchetypeCatalogueInput(t)

	// Completeness is pinned to the AUTHORED-rule count derived from the loaded
	// packs (never a hardcoded guess): every authored rule must surface exactly
	// once, so a silently dropped rule fails the gate (REQ-E3-S08-03). Also count
	// per underlying pack for a tighter, per-pack completeness assertion.
	authored := 0
	authoredByPack := map[string]int{}
	for _, pk := range in.Packs {
		for _, mp := range pk.Policies {
			authored += len(mp.Spec.Rules)
			authoredByPack[pk.Name] += len(mp.Spec.Rules)
		}
	}

	cat := catalogue.Build(in)
	if len(cat.Rules) != authored {
		t.Fatalf("catalogue is incomplete: %d rules catalogued, %d authored across the loaded packs", len(cat.Rules), authored)
	}

	// Completeness: every conformant pack contributes EXACTLY its authored rules,
	// and every entry carries the load-bearing D-017 B10 identity fields.
	byPack := map[string]int{}
	var lastID string
	for _, e := range cat.Rules {
		if e.ID == "" || e.Rule == "" || e.Pack == "" {
			t.Errorf("catalogue entry missing identity: %+v", e)
		}
		if e.DocsURL != catalogue.DocsBase+"/"+e.ID {
			t.Errorf("entry %q docsUrl %q is not minted from the stable ID", e.ID, e.DocsURL)
		}
		if e.Phase == "" || e.EffectivePhase == "" {
			t.Errorf("entry %q must surface an authored + effective phase, got %q/%q", e.ID, e.Phase, e.EffectivePhase)
		}
		// Additive-tolerant: canonically sorted by stable ID (never insertion order).
		if lastID != "" && e.ID < lastID {
			t.Errorf("catalogue not sorted by stable ID: %q after %q", e.ID, lastID)
		}
		lastID = e.ID
		byPack[e.Pack]++
	}
	// The pack key is the packs/<name>/ directory token (catalog / vars), not the
	// top-level starter-pack dir name — assert each underlying pack contributed
	// EXACTLY its authored rule count (a dropped rule in either pack fails here).
	for _, want := range []string{"catalog", "vars"} {
		if byPack[want] != authoredByPack[want] {
			t.Errorf("pack %q: %d rules catalogued, %d authored", want, byPack[want], authoredByPack[want])
		}
	}

	// Determinism: byte-identical across two independent Builds + Marshals.
	first, err := catalogue.Build(loadArchetypeCatalogueInput(t)).Marshal()
	if err != nil {
		t.Fatalf("marshal catalogue: %v", err)
	}
	second, err := catalogue.Build(loadArchetypeCatalogueInput(t)).Marshal()
	if err != nil {
		t.Fatalf("marshal catalogue (2): %v", err)
	}
	if string(first) != string(second) {
		t.Errorf("catalogue not byte-identical across runs:\n first=%s\n second=%s", first, second)
	}
}

// loadArchetypeCatalogueInput loads the conformant archetype packs via the E2
// strict loader into a catalogue.Input, grouping MergePolicy docs + pack manifests
// by their packs/<name>/ token (the join key binding.packs[] references) — the same
// assembly cmd/assent's loadCatalogueInput performs, replicated here so the pure
// package test needs no cmd import.
func loadArchetypeCatalogueInput(t *testing.T) catalogue.Input {
	t.Helper()
	packPolicies := map[string][]*policy.MergePolicy{}
	packManifests := map[string]*policy.Pack{}
	var bindings []*policy.RulesetBinding

	for _, starter := range conformantPacks {
		root := filepath.Join(archetypePacksDir, starter, ".assent")
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, werr error) error {
			if werr != nil {
				return werr
			}
			if d.IsDir() || (!strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml")) {
				return nil
			}
			raw, rerr := os.ReadFile(path) //nolint:gosec // in-repo example tree, read-only.
			if rerr != nil {
				return rerr
			}
			pack := packNameFromPath(path)
			switch documentKind(raw) {
			case "MergePolicy":
				mp, lerr := policy.LoadMergePolicy(raw)
				if lerr != nil {
					t.Fatalf("%s: strict loader rejected MergePolicy: %v", path, lerr)
				}
				packPolicies[pack] = append(packPolicies[pack], mp)
			case "Pack":
				pk, lerr := policy.LoadPack(raw)
				if lerr != nil {
					t.Fatalf("%s: strict loader rejected Pack: %v", path, lerr)
				}
				packManifests[pack] = pk
			case "RulesetBinding":
				rb, lerr := policy.LoadRulesetBinding(raw)
				if lerr != nil {
					t.Fatalf("%s: strict loader rejected RulesetBinding: %v", path, lerr)
				}
				bindings = append(bindings, rb)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %q: %v", root, err)
		}
	}

	var packs []catalogue.Pack
	for name, policies := range packPolicies {
		packs = append(packs, catalogue.Pack{Name: name, Policies: policies, Manifest: packManifests[name]})
	}
	return catalogue.Input{Packs: packs, Bindings: bindings}
}

// packNameFromPath returns the segment after the LAST "packs/" path segment — the
// `.assent/packs/<name>/` token, not the top-level examples/packs/ starter dir
// (both contain a "packs" segment when walking from the repo tree).
func packNameFromPath(path string) string {
	segs := strings.Split(filepath.ToSlash(path), "/")
	name := ""
	for i, s := range segs {
		if s == "packs" && i+1 < len(segs) {
			name = segs[i+1]
		}
	}
	return name
}

// documentKind decodes only the top-level kind discriminator.
func documentKind(raw []byte) string {
	// A minimal line scan avoids a YAML dependency in this helper: the frozen docs
	// all carry a top-level `kind:` key.
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "kind:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "kind:"))
		}
	}
	return ""
}
