package conformance

import (
	"errors"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

// addressing_portfailures_test.go is the PORT-CORRUPTION companion to
// canfail_test.go. The sabotage gate corrupts the FIXTURE and the OBSERVER; it
// leaves the PORT honest, which is why the case bodies' defensive branches
// (a port that errors, returns an empty identity, serves the wrong bytes, or
// maps a forbidden read to absence) are never executed there. These tests
// corrupt the PORT itself, so they prove two things at once: the addressing
// cases notice port-level misbehaviour, and every defensive branch they carry
// is reachable — not dead code written for a coverage line.

type brokenPort struct {
	forge.RunPort
	mode string
}

func (b brokenPort) Identity() (forge.Identity, error) {
	switch b.mode {
	case "identity-error":
		return forge.Identity{}, errors.New("brokenPort: identity probe failed")
	case "identity-empty":
		return forge.Identity{}, nil
	}
	return b.RunPort.Identity()
}

func (b brokenPort) ListBotThreads(project, mr string) ([]forge.Thread, error) {
	if b.mode == "list-error" {
		return nil, errors.New("brokenPort: listing failed")
	}
	threads, err := b.RunPort.ListBotThreads(project, mr)
	if err != nil {
		return nil, err
	}
	if b.mode == "no-own-threads" {
		return nil, nil
	}
	if b.mode == "wrong-author" && len(threads) == 1 {
		threads[0].Author = "second-bot"
	}
	return threads, nil
}

func (b brokenPort) FileAtBase(project, mr, path string) ([]byte, error) {
	if b.mode == "base-error" {
		return nil, errors.New("brokenPort: base read failed")
	}
	if b.mode == "base-mismatch" {
		return []byte("not the seeded content"), nil
	}
	return b.RunPort.FileAtBase(project, mr, path)
}

func (b brokenPort) FileAtHead(project, mr, path string) ([]byte, error) {
	switch b.mode {
	case "head-success-absent":
		return nil, nil // present-empty success where absence is the truth
	case "head-unauthorized-where-absent":
		return nil, forge.ErrUnauthorized // absence rendered as forbidden
	case "head-forbidden-renders-absent":
		return nil, nil // a refusal rendered as absent content
	case "head-notfound-where-forbidden":
		return nil, forge.ErrNotFound // the absent-means-trusted collapse
	case "head-error":
		return nil, errors.New("brokenPort: head read failed")
	}
	return b.RunPort.FileAtHead(project, mr, path)
}

func (b brokenPort) FileAtRef(project, path, ref string) ([]byte, error) {
	switch b.mode {
	case "ref-success-where-forbidden":
		return []byte("forbidden content served"), nil
	case "ref-notfound-where-forbidden":
		return nil, forge.ErrNotFound
	case "ref-unauthorized-where-absent":
		return nil, forge.ErrUnauthorized
	case "ref-error":
		return nil, errors.New("brokenPort: ref read failed")
	}
	return b.RunPort.FileAtRef(project, path, ref)
}

// brokenPortFactory wraps fakeFactory so the case under test drives a port with
// one corrupted read. In seed-error mode the fixture refuses every arrangement.
func brokenPortFactory(mode string) Factory {
	return func(t TB, cfg Config) Backend {
		b := fakeFactory(t, cfg)
		fixture := b.Fixture
		if mode == "seed-error" {
			fixture = failingSeed{real: fixture}
		}
		return Backend{Port: brokenPort{RunPort: b.Port, mode: mode}, Fixture: fixture, Observer: corruptNothing(b.Observer)}
	}
}

// corruptNothing returns a truthful Observer: the port-corruption tests pin the
// cases' PORT branches, so the observer must report the real backend's counts.
func corruptNothing(o Observer) Observer { return o }

// failingSeed is a fixture whose SEEDING fails (the harness analog of a forge
// that rejects the arrange step); the identity case's seed-error branches are
// reachable only through it.
type failingSeed struct{ real Fixture }

func (f failingSeed) SeedThread(string, string, forge.Marker, bool) error {
	return errors.New("failingSeed: seed refused")
}
func (f failingSeed) SeedNote(id string, author string, marker forge.Marker, body string) error {
	return f.real.SeedNote(id, author, marker, body)
}
func (f failingSeed) Pins() forge.DesiredMerge          { return f.real.Pins() }
func (f failingSeed) MoveTargetHead(sha string)         { f.real.MoveTargetHead(sha) }
func (f failingSeed) MoveSourceHead(sha string)         { f.real.MoveSourceHead(sha) }
func (f failingSeed) DriftSourceHeadAfterRead(s string) { f.real.DriftSourceHeadAfterRead(s) }
func (f failingSeed) SeedFile(_ string, side string, c []byte) {
	f.real.SeedFile(side, side, c)
}
func (f failingSeed) RefuseFileRead(path string) { f.real.RefuseFileRead(path) }

func TestAddressingCasesNoticeBrokenPorts(t *testing.T) {
	// Each row: the case id, the corruption, and a substring of the verdict the
	// corruption must produce. A case that PASSED here would have missed the
	// port defect it was written to catch.
	for _, tc := range []struct {
		id      string
		mode    string
		wantSub string
	}{
		{"fork-head-unchanged-file-no-lifecycle", "base-error", "FileAtBase"},
		{"fork-head-unchanged-file-no-lifecycle", "base-mismatch", "read identically"},
		{"fork-head-genuine-delete-detected", "base-error", "FileAtBase"},
		{"fork-head-genuine-delete-detected", "head-success-absent", "ABSENT"},
		{"forbidden-never-renders-as-absent", "head-success-absent", "must not succeed"},
		{"forbidden-never-renders-as-absent", "head-notfound-where-forbidden", "rendered as ABSENT"},
		{"forbidden-never-renders-as-absent", "head-error", "forge.ErrUnauthorized"},
		{"fork-head-unchanged-file-no-lifecycle", "head-error", "FileAtHead must reach"},
		{"fork-head-genuine-delete-detected", "head-error", "ABSENT"},
		{"absent-file-still-renders-as-absent", "ref-error", "FileAtRef"},
		{"forbidden-never-renders-as-absent", "ref-notfound-where-forbidden", "rendered as ABSENT"},
		{"absent-file-still-renders-as-absent", "head-success-absent", "forge.ErrNotFound"},
		{"absent-file-still-renders-as-absent", "head-unauthorized-where-absent", "must yield forge.ErrNotFound"},
		{"absent-file-still-renders-as-absent", "ref-unauthorized-where-absent", "must yield forge.ErrNotFound"},
		{"absent-file-still-renders-as-absent", "base-mismatch", "want the seeded content"},
		{"own-markers-recognised-identity", "seed-error", "seed own thread"},
		{"own-markers-recognised-identity", "identity-error", "Identity"},
		{"own-markers-recognised-identity", "list-error", "ListBotThreads"},
		{"own-markers-recognised-identity", "identity-empty", "must expose the authenticated identity"},
		{"own-markers-recognised-identity", "no-own-threads", "must be recognised as our own"},
		{"own-markers-recognised-identity", "wrong-author", "want the authenticated identity"},
	} {
		t.Run(tc.id+"/"+tc.mode, func(t *testing.T) {
			var found *Case
			for _, c := range Cases() {
				if c.ID == tc.id {
					found = &c
					break
				}
			}
			if found == nil {
				t.Fatalf("no case %q", tc.id)
			}
			failed, msg := runAndRecord(*found, brokenPortFactory(tc.mode))
			if !failed {
				t.Fatalf("case %q PASSED with a %s-corrupted port — its port-level assertions are dead", tc.id, tc.mode)
			}
			if !strings.Contains(msg, tc.wantSub) {
				t.Fatalf("case failed with the WRONG verdict: %s\nwant substring %q", msg, tc.wantSub)
			}
		})
	}
}

// own-markers case needs a port whose Identity matches a SEEDED thread author
// while ListBotThreads filters wrongly — covered by no-own-threads/wrong-author.
