package fake

import (
	"fmt"

	"github.com/PlatformRelay/assent/internal/forge"
)

// runport.go gives the fake the MR-relative governed-subject accessors and the
// identity the port exposes (E10-S02, REQ-E10-S02-03: the fake implements
// forge.RunPort directly, making the PORT — not the GitLab client — the
// conformance-tested thing).
//
// Content is keyed path → side → bytes, where a side is one of the two named
// sides ("base" / "head") or an explicit ref string for the ref-addressed
// FileAtRef loads. This models the ACCESSOR CONTRACT, not a forge: the port
// says what may be asked (base/head/refs), never how the adapter reaches it
// (that freedom is S00 Q1's — GitHub via the PR head, GitLab via the source
// project id or the MR-relative ref). A fork MR's head content is therefore
// seeded on the head side exactly like any other head — the conformance fork
// cases are about the ACCESSOR boundary (unchanged head ⇒ no lifecycle event),
// not about any forge's URL shapes.

// fileSideBase / fileSideHead are the side keys of the governed-subject fixture
// storage. Side-keyed rather than SHA-keyed because the port's accessors are
// MR-relative (project, mr, path) — the fake models the two SIDES of one merge
// request, which is the contract FileAtBase/FileAtHead promise.
const (
	fileSideBase = "base"
	fileSideHead = "head"
)

// SeedFileAtRef records content visible to FileAtRef at an explicit ref of an
// explicit project (the ref-addressed policy loads). path → ref → content.
func (f *Forge) SeedFileAtRef(path, ref string, content []byte) {
	if f.Files == nil {
		f.Files = map[string]map[string][]byte{}
	}
	if f.Files[path] == nil {
		f.Files[path] = map[string][]byte{}
	}
	f.Files[path][ref] = append([]byte(nil), content...)
}

// refused returns the refusal mapped to a path (the forbidden≠absent seam), or
// nil when the path is not refused.
func (f *Forge) refused(path string) error {
	if f.RefusedReads == nil {
		return nil
	}
	return f.RefusedReads[path]
}

// FileAtRef reads file content at an explicit ref of an explicit project. The
// project is deliberately unnamed (`_`): the fake's content store is keyed by
// path/ref only — the port carries the project, the fake's store does not need
// it, and naming the parameter would only promise a discrimination it does not
// model.
func (f *Forge) FileAtRef(_, path, ref string) ([]byte, error) {
	if err := f.refused(path); err != nil {
		return nil, err
	}
	if m, ok := f.Files[path]; ok {
		if raw, ok := m[ref]; ok {
			return raw, nil
		}
	}
	return nil, fmt.Errorf("fake: %w: file %q at ref %q", forge.ErrNotFound, path, ref)
}

// FileAtBase reads the governed subject's base-side content for the MR. The
// fixture is side-keyed ("base"), so a test controls the judged bytes directly;
// an absent side is forge.ErrNotFound (absence — the EFE-S03 presence signal),
// never a fabricated lifecycle event.
func (f *Forge) FileAtBase(_, mr, path string) ([]byte, error) {
	if err := f.refused(path); err != nil {
		return nil, err
	}
	if m, ok := f.Files[path]; ok {
		if raw, ok := m[fileSideBase]; ok {
			return raw, nil
		}
	}
	return nil, fmt.Errorf("fake: %w: file %q on base side of MR %s", forge.ErrNotFound, path, mr)
}

// FileAtHead reads the governed subject's content on the HEAD side of the MR.
// MR-relative: on the fake the head side is the fixture the test seeded, which
// is precisely the contract — the adapter owns the fork-head reach, the port
// only names the side. Unchanged governed files seed the SAME bytes on both
// sides; a fork whose head branch does not exist in the base repository is
// expressed by seeding the head side — never by a missing branch.
func (f *Forge) FileAtHead(_, mr, path string) ([]byte, error) {
	if err := f.refused(path); err != nil {
		return nil, err
	}
	if m, ok := f.Files[path]; ok {
		if raw, ok := m[fileSideHead]; ok {
			return raw, nil
		}
	}
	return nil, fmt.Errorf("fake: %w: file %q on head side of MR %s", forge.ErrNotFound, path, mr)
}

// Identity returns the configured bot identity as a USER identity.
func (f *Forge) Identity() (forge.Identity, error) {
	return forge.Identity{
		Kind:  forge.IdentityUser,
		Login: f.BotAuthor,
		ID:    f.BotAuthor,
	}, nil
}

var _ forge.RunPort = (*Forge)(nil)
