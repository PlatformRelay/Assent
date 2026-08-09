// Package builtin holds in-tree fact providers (ADR-0004 tier 1).
//
// E5-S07 owns builtin/repo-file only — forge-groups (E5-S06) lands in sibling
// files this package must not edit in the S07 lane.
package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
	"time"

	"github.com/PlatformRelay/assent/internal/provider"
	"gopkg.in/yaml.v3"
)

// TypeRepoFile is the Config.providers[].type string for this builtin.
const TypeRepoFile = "builtin/repo-file"

// OpenRepoRoot opens dir as a SYMLINK-SAFE root for RepoFileOpts.FS and for the
// resource-owner registry read (D-129).
//
// `os.DirFS` is not a security boundary: it resolves symlinks through the host
// filesystem, so a merge request that ships `topics/prod/quota.yaml -> /etc/x`
// (or a directory symlink `topics/evil -> /outside`) would turn an arbitrary host
// file into a decision fact. `(*os.Root).FS()` refuses any traversal that leaves
// the root, at the syscall level.
//
// The caller MUST close the returned io.Closer when the FS is no longer read; the
// root holds an open directory handle.
func OpenRepoRoot(dir string) (fs.FS, io.Closer, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("open checkout root %s: %w", dir, err)
	}
	return root.FS(), root, nil
}

// RepoFileOpts configures most-specific-first resolution over a checkout or
// fixture filesystem (REQ-E5-S07-01). Absent files yield unavailable — never
// resolved with a nil/empty value pretending presence (REQ-E5-S07-02).
type RepoFileOpts struct {
	// FS is the checkout / fixture root. Required.
	//
	// CONTRACT (D-129): FS MUST be a symlink-safe root — `(*os.Root).FS()` via
	// OpenRepoRoot, or an in-memory fixture FS. A bare `os.DirFS` is NOT a
	// security boundary (Go documents this explicitly) and its `fs.Stat`
	// follows links out of the tree, so an MR that ships
	// `topics/prod/quota.yaml -> /etc/anything` would otherwise feed the
	// decision engine a fact from outside the checkout.
	//
	// This builtin additionally refuses any candidate whose path traverses a
	// symlink (see findMostSpecific) — defence in depth, and the only layer
	// that can protect the declared Roots clip, which a root FS cannot see.
	// That refusal is NOT a substitute for injecting a safe root: it can only
	// observe what the injected FS chooses to report.
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

	matched, status := findMostSpecific(opts.FS, anchor, fileName, roots)
	switch status {
	case candidateSymlink:
		// Containment refusal, not malformed input: same fail direction as
		// "anchor outside declared roots" (unavailable). The operator's config is
		// well-formed; the repo content is trying to leave the declared roots.
		resp.Facts = synthesizeAll(q, provider.StateUnavailable,
			"builtin/repo-file: refusing candidate reached through a symlink: "+matched, asOf, opts.Declarations)
		return resp
	case candidateFile:
	default:
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

// candidateStatus is the verdict on one walk-up candidate path.
type candidateStatus int

const (
	// candidateMiss: absent, a directory, or otherwise not a usable regular file.
	candidateMiss candidateStatus = iota
	// candidateFile: a real regular file, reachable without traversing a symlink.
	candidateFile
	// candidateSymlink: the path traverses a symlink → containment refusal (D-129).
	candidateSymlink
)

// findMostSpecific walks from the anchor directory up to the FS root, returning
// the first existing regular file named fileName (most-specific-first).
//
// D-129: a candidate reached through a symlink STOPS the walk with
// candidateSymlink — it is never skipped over. Skipping would silently fall back
// to a less-specific legitimate file and hide an MR trying to read outside the
// declared roots; the fact would then look like an ordinary resolution.
func findMostSpecific(fsys fs.FS, anchor, fileName string, roots []string) (string, candidateStatus) {
	for _, cand := range candidates(anchor, fileName) {
		if len(roots) > 0 && !underAnyRoot(cand, roots) {
			continue
		}
		switch classifyCandidate(fsys, cand) {
		case candidateSymlink:
			return cand, candidateSymlink
		case candidateFile:
			return cand, candidateFile
		case candidateMiss:
		}
	}
	return "", candidateMiss
}

// classifyCandidate decides whether name is a usable regular file, refusing any
// path that traverses a symlink at ANY component (D-129).
//
// Why every component and not just the leaf: a directory symlink
// (`topics/evil -> /outside`) leaves the leaf looking like a plain regular file
// to both fs.Stat and fs.Lstat on a bare os.DirFS — that is reproduction form #1.
//
// Why refuse in-root symlinks too: a link that never leaves the FS root can still
// leave the declared Roots clip (`topics/prod/quota.yaml -> ../../secrets/…`). A
// symlink-safe root cannot defend that — it does not know about Roots — so the
// only safe rule this layer can state is "no symlinks on the candidate path".
// One rule, no per-link reachability reasoning. No repo fixture uses symlinks.
//
// fs.Lstat falls back to fs.Stat for a filesystem that does not implement
// fs.ReadLinkFS; such a filesystem cannot report links at all, which is exactly
// why RepoFileOpts.FS must be a symlink-safe root.
func classifyCandidate(fsys fs.FS, name string) candidateStatus {
	for _, prefix := range pathPrefixes(name) {
		st, err := fs.Lstat(fsys, prefix)
		if err != nil {
			// Absent, or refused by a symlink-safe root (an escaping traversal).
			// Either way: not a usable candidate.
			return candidateMiss
		}
		if st.Mode()&fs.ModeSymlink != 0 {
			return candidateSymlink
		}
	}
	if !isRegular(fsys, name) {
		return candidateMiss
	}
	return candidateFile
}

// pathPrefixes lists every path component prefix of a slash path, outermost
// first: "topics/prod/quota.yaml" → topics, topics/prod, topics/prod/quota.yaml.
func pathPrefixes(name string) []string {
	if name == "." || name == "" {
		return nil
	}
	parts := strings.Split(name, "/")
	out := make([]string, 0, len(parts))
	acc := ""
	for _, part := range parts {
		if acc == "" {
			acc = part
		} else {
			acc += "/" + part
		}
		out = append(out, acc)
	}
	return out
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
