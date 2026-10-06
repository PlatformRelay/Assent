package fake

import (
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

// runport_test.go exercises the fake's forge.RunPort surface directly
// (E10-S02, REQ-E10-S02-03): the MR-relative accessors, the identity, and the
// GetMR fork trap. The conformance suite drives the same code through the
// Factory; these unit tests cover the branches a case body does not reach.

func TestRunPortFileAccessorsAreSideKeyed(t *testing.T) {
	f := fake()
	f.SeedFileAtRef("policy.yaml", "tgtSHA", []byte("policy bytes"))
	f.SeedFileAtRef("governed.yaml", fileSideBase, []byte("base bytes"))
	f.SeedFileAtRef("governed.yaml", fileSideHead, []byte("head bytes"))

	raw, err := f.FileAtRef("42", "policy.yaml", "tgtSHA")
	if err != nil || string(raw) != "policy bytes" {
		t.Fatalf("FileAtRef: %v (%q)", err, raw)
	}
	base, err := f.FileAtBase("42", "7", "governed.yaml")
	if err != nil || string(base) != "base bytes" {
		t.Fatalf("FileAtBase: %v (%q)", err, base)
	}
	head, err := f.FileAtHead("42", "7", "governed.yaml")
	if err != nil || string(head) != "head bytes" {
		t.Fatalf("FileAtHead: %v (%q)", err, head)
	}

	// Unseeded side = absence, never fabricated content; and the refusal knob
	// outranks whatever content exists (forbidden ≠ absent, S00 Q4).
	if _, err := f.FileAtHead("42", "7", "absent.yaml"); err == nil || !strings.Contains(err.Error(), "not found (404)") {
		t.Fatalf("an unseeded head side must be forge.ErrNotFound, got %v", err)
	}
	f.RefusedReads = map[string]error{"governed.yaml": forge.ErrUnauthorized}
	for _, read := range []func() ([]byte, error){
		func() ([]byte, error) { return f.FileAtRef("42", "governed.yaml", "tgtSHA") },
		func() ([]byte, error) { return f.FileAtBase("42", "mr", "governed.yaml") },
		func() ([]byte, error) { return f.FileAtHead("42", "mr", "governed.yaml") },
	} {
		_, err := read()
		if err == nil || !strings.Contains(err.Error(), "unauthorized (401/403)") {
			t.Fatalf("a refused read must carry forge.ErrUnauthorized, got %v", err)
		}
	}
}

func TestRunPortGetMRForkTrap(t *testing.T) {
	f := fake()
	f.MRFork = true
	if _, err := f.GetMR("42", "7"); err == nil {
		t.Fatal("a fork fixture without a source project id must fail closed (absent-means-trusted trap)")
	}
	f.MRSourceProjectID = "99"
	info, err := f.GetMR("42", "7")
	if err != nil || !info.ForkMR || info.SourceProjectID != "99" {
		t.Fatalf("GetMR: info=%+v err=%v", info, err)
	}
}

func TestRunPortIdentityIsTheFilterIdentity(t *testing.T) {
	f := fake()
	ident, err := f.Identity()
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	if ident.Kind != forge.IdentityUser || ident.Login != f.BotAuthor {
		t.Fatalf("Identity = %+v, want the configured bot identity as a user", ident)
	}
}

func fake() *Forge {
	f := New("assent-bot", "src", "tgt", "sha256:digest")
	return f
}
