// Package builtin holds in-tree fact providers (ADR-0004 tier 1).
//
// E5-S07 owns builtin/repo-file only — forge-groups (E5-S06) lands in sibling
// files this package must not edit in the S07 lane.
package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/PlatformRelay/assent/internal/provider"
	"gopkg.in/yaml.v3"
)

// TypeRepoFile is the Config.providers[].type string for this builtin.
const TypeRepoFile = "builtin/repo-file"

// RepoFileOpts configures most-specific-first resolution over a checkout or
// fixture filesystem (REQ-E5-S07-01). Absent files yield unavailable — never
// resolved with a nil/empty value pretending presence (REQ-E5-S07-02).
type RepoFileOpts struct {
	// FS is the checkout / fixture root. Required.
	FS fs.FS
	// File is the basename sought while walking up from Anchor (e.g. "quota.yaml").
	File string
	// Anchor is the change path (file or dir) that starts the walk-up.
	Anchor string
	// Roots optionally clips candidates to declared prefixes (relative to FS).
	// Empty means the whole FS root is eligible. An anchor outside every root
	// yields unavailable (no silent fallback above the clip).
	Roots []string
	// Declarations are echoed on each fact and used to compute expiresAt.
	Declarations map[string]provider.Declaration
}

// CallRepoFile answers a FactQuery as a host CallFunc body: schema-valid
// FactResponse bytes. Walks Anchor→root for File (most-specific first), parses
// YAML/JSON, and maps each requested output to a top-level key.
//
// Fail-closed:
//   - no matching file → every output unavailable (not resolved-empty)
//   - file present but key missing / null → invalid (value dropped)
//   - bad opts / unsafe path / undecodable body → invalid
func CallRepoFile(_ context.Context, opts RepoFileOpts, q provider.FactQuery) ([]byte, error) {
	resp := answerRepoFile(opts, q)
	raw, err := json.Marshal(resp)
	if err != nil {
		return nil, fmt.Errorf("builtin/repo-file: marshal response: %w", err)
	}
	return raw, nil
}

func answerRepoFile(opts RepoFileOpts, q provider.FactQuery) provider.FactResponse {
	resp := provider.FactResponse{
		APIVersion: provider.APIVersion,
		Kind:       provider.KindFactResponse,
		QueryID:    q.QueryID,
	}
	asOf := q.AsOf.UTC()

	if opts.FS == nil || strings.TrimSpace(opts.File) == "" {
		resp.Facts = synthesizeAll(q, provider.StateInvalid, "builtin/repo-file: FS and File are required", asOf, opts.Declarations)
		return resp
	}
	fileName := path.Base(strings.TrimSpace(opts.File))
	if fileName == "." || fileName == "/" || fileName == ".." || strings.Contains(fileName, "/") {
		resp.Facts = synthesizeAll(q, provider.StateInvalid, "builtin/repo-file: File must be a basename", asOf, opts.Declarations)
		return resp
	}
	anchor, err := cleanRel(opts.Anchor)
	if err != nil {
		resp.Facts = synthesizeAll(q, provider.StateInvalid, "builtin/repo-file: "+err.Error(), asOf, opts.Declarations)
		return resp
	}
	roots, err := cleanRoots(opts.Roots)
	if err != nil {
		resp.Facts = synthesizeAll(q, provider.StateInvalid, "builtin/repo-file: "+err.Error(), asOf, opts.Declarations)
		return resp
	}
	if len(roots) > 0 && !underAnyRoot(anchor, roots) && anchor != "." {
		// Anchor itself must live under a declared root (or be the root).
		resp.Facts = synthesizeAll(q, provider.StateUnavailable, "builtin/repo-file: anchor outside declared roots", asOf, opts.Declarations)
		return resp
	}

	matched, ok := findMostSpecific(opts.FS, anchor, fileName, roots)
	if !ok {
		resp.Facts = synthesizeAll(q, provider.StateUnavailable, "builtin/repo-file: no matching file", asOf, opts.Declarations)
		return resp
	}

	doc, err := readMapping(opts.FS, matched)
	if err != nil {
		resp.Facts = synthesizeAll(q, provider.StateInvalid, "builtin/repo-file: "+err.Error(), asOf, opts.Declarations)
		return resp
	}

	facts := make([]provider.Fact, 0, len(q.Outputs))
	for _, name := range q.Outputs {
		decl := opts.Declarations[name]
		val, present := doc[name]
		if !present || val == nil {
			facts = append(facts, nonResolved(q, name, decl, provider.StateInvalid,
				"builtin/repo-file: key absent in "+matched, asOf))
			continue
		}
		exp, err := expiresAt(asOf, decl)
		if err != nil {
			facts = append(facts, nonResolved(q, name, decl, provider.StateInvalid,
				"builtin/repo-file: "+err.Error(), asOf))
			continue
		}
		facts = append(facts, provider.Fact{
			Name:        name,
			Declaration: decl,
			State:       provider.StateResolved,
			Subject:     q.Subject,
			ObservedAt:  asOf,
			ExpiresAt:   &exp,
			Value:       val,
		})
	}
	resp.Facts = facts
	return resp
}

// findMostSpecific walks from the anchor directory up to the FS root, returning
// the first existing regular file named fileName (most-specific-first).
func findMostSpecific(fsys fs.FS, anchor, fileName string, roots []string) (string, bool) {
	for _, cand := range candidates(anchor, fileName) {
		if len(roots) > 0 && !underAnyRoot(cand, roots) {
			continue
		}
		if isRegular(fsys, cand) {
			return cand, true
		}
	}
	return "", false
}

// candidates lists walk-up paths most-specific first.
// anchor "topics/prod/orders.yaml" + file "quota.yaml" →
//
//	topics/prod/quota.yaml, topics/quota.yaml, quota.yaml
func candidates(anchor, fileName string) []string {
	dir := anchor
	if dir != "." && !strings.HasSuffix(dir, "/") {
		// Anchor is a change file path — start in its directory.
		dir = path.Dir(dir)
	}
	dir = path.Clean(dir)
	if dir == "/" {
		dir = "."
	}

	var out []string
	for {
		if dir == "." {
			out = append(out, fileName)
			break
		}
		out = append(out, path.Join(dir, fileName))
		parent := path.Dir(dir)
		if parent == dir {
			out = append(out, fileName)
			break
		}
		dir = parent
	}
	return out
}

func isRegular(fsys fs.FS, name string) bool {
	st, err := fs.Stat(fsys, name)
	if err != nil {
		return false
	}
	return st.Mode().IsRegular()
}

func readMapping(fsys fs.FS, name string) (map[string]any, error) {
	raw, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decode %s: %w", name, err)
	}
	if doc == nil {
		// Empty / null YAML document — treat as present-but-empty mapping so
		// missing keys become invalid, not a silent resolved-empty fact.
		return map[string]any{}, nil
	}
	// Normalize YAML numbers / sequences into JSON-friendly values so the
	// FactResponse round-trips cleanly through the host classifier.
	normalized, err := jsonNormalize(doc)
	if err != nil {
		return nil, fmt.Errorf("normalize %s: %w", name, err)
	}
	return normalized, nil
}

func jsonNormalize(in map[string]any) (map[string]any, error) {
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func expiresAt(asOf time.Time, decl provider.Declaration) (time.Time, error) {
	if decl.MaxAge == "" {
		return time.Time{}, fmt.Errorf("declaration maxAge is required")
	}
	d, err := time.ParseDuration(decl.MaxAge)
	if err != nil {
		return time.Time{}, fmt.Errorf("declaration maxAge %q: %w", decl.MaxAge, err)
	}
	if d <= 0 {
		return time.Time{}, fmt.Errorf("declaration maxAge %q must be positive", decl.MaxAge)
	}
	return asOf.Add(d), nil
}

func synthesizeAll(q provider.FactQuery, state, reason string, asOf time.Time, decls map[string]provider.Declaration) []provider.Fact {
	out := make([]provider.Fact, 0, len(q.Outputs))
	for _, name := range q.Outputs {
		out = append(out, nonResolved(q, name, decls[name], state, reason, asOf))
	}
	return out
}

func nonResolved(q provider.FactQuery, name string, decl provider.Declaration, state, reason string, asOf time.Time) provider.Fact {
	return provider.Fact{
		Name:        name,
		Declaration: decl,
		State:       state,
		Subject:     q.Subject,
		ObservedAt:  asOf,
		Reason:      reason,
	}
}

func cleanRel(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return ".", nil
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if path.IsAbs(p) {
		return "", fmt.Errorf("anchor must be relative, got %q", p)
	}
	clean := path.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("anchor escapes filesystem root: %q", p)
	}
	if clean == "/" {
		return ".", nil
	}
	return strings.TrimPrefix(clean, "./"), nil
}

func cleanRoots(roots []string) ([]string, error) {
	if len(roots) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		c, err := cleanRel(r)
		if err != nil {
			return nil, fmt.Errorf("root %q: %w", r, err)
		}
		if c == "." {
			// "." means whole FS — equivalent to no clip.
			continue
		}
		out = append(out, c)
	}
	return out, nil
}

func underAnyRoot(p string, roots []string) bool {
	if p == "." {
		return false
	}
	for _, root := range roots {
		if p == root || strings.HasPrefix(p, root+"/") {
			return true
		}
	}
	return false
}
