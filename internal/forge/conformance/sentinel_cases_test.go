package conformance

import (
	"errors"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
)

// sentinel_cases_test.go holds the two S00 Q4 sentinel rows that are
// ADAPTER-FACTORY-shaped by nature (catalog rows ratelimit-403-is-transport-
// error and metadata-only-token-is-not-absence):
//
//	ratelimit-403-is-transport-error   — a rate-limited 403 is a transport
//	                                     error, never a sentinel, never absence
//	metadata-only-token-is-not-absence — a token that reads metadata but not
//	                                     contents never renders absence
//
// They are DIRECT entry tests rather than Cases() rows: the fake backend has
// no transport (no headers, no status codes, no content-scope probe) to
// violate, so these shapes can only be driven against the GitLab and GitHub
// harnesses — the catalog rows carry that disposition, the same one the
// transport cases carry.

// TestConformanceRateLimit403NotAbsent is S00 Q4's rate-limit row (ADR-0021
// item 6): a 403 carrying Retry-After or X-RateLimit-Remaining: 0 is a
// TRANSPORT error — retried/errored by the port's transport policy — never
// forge.ErrNotFound (absence) and never forge.ErrUnauthorized.
func TestConformanceRateLimit403NotAbsent(t *testing.T) {
	t.Run("gitlab", func(t *testing.T) {
		h := transportHarness(t)
		h.rateLimited403 = true
		c := h.client(t)
		for name, read := range map[string]func() ([]byte, error){
			"FileAtBase": func() ([]byte, error) { return c.FileAtBase("42", "7", "topics/orders.yaml") },
			"FileAtHead": func() ([]byte, error) { return c.FileAtHead("42", "7", "topics/orders.yaml") },
		} {
			_, err := read()
			if err == nil {
				t.Fatalf("%s: a rate-limited 403 must not succeed", name)
			}
			if errors.Is(err, forge.ErrNotFound) {
				t.Fatalf("%s: a rate-limited 403 rendered as ABSENT — the absent-means-trusted collapse (S00 Q4)", name)
			}
			if errors.Is(err, forge.ErrUnauthorized) {
				t.Fatalf("%s: a rate-limited 403 rendered as a permission failure — it is a transport error (S00 Q4)", name)
			}
		}
	})

	t.Run("github", func(t *testing.T) {
		h := githubTransportHarness(t)
		h.contentsStatus = 403 // refused at the content-scope probe AND the specific path
		h.rateLimitedContent = true
		c := h.client(t)
		for name, read := range map[string]func() ([]byte, error){
			"FileAtBase": func() ([]byte, error) { return c.FileAtBase("platform/orders-service", "482", h.governedPath) },
			"FileAtHead": func() ([]byte, error) { return c.FileAtHead("platform/orders-service", "482", h.governedPath) },
			"FileAtRef":  func() ([]byte, error) { return c.FileAtRef("platform/orders-service", h.governedPath, pinTarget) },
		} {
			_, err := read()
			if err == nil {
				t.Fatalf("%s: a rate-limited 403 must not succeed", name)
			}
			if errors.Is(err, forge.ErrNotFound) {
				t.Fatalf("%s: a rate-limited 403 rendered as ABSENT — the absent-means-trusted collapse (S00 Q4)", name)
			}
			if errors.Is(err, forge.ErrUnauthorized) {
				t.Fatalf("%s: a rate-limited 403 rendered as a permission failure — it is a transport error (S00 Q4)", name)
			}
		}
	})
}

// TestConformanceMetadataOnlyTokenNotAbsent is S00 Q4's metadata-only token
// row: a credential that reads the repo/MR object but is forbidden on every
// CONTENT read must render forge.ErrUnauthorized on every governed read —
// never absence, never a successful empty read.
func TestConformanceMetadataOnlyTokenNotAbsent(t *testing.T) {
	t.Run("gitlab", func(t *testing.T) {
		h := transportHarness(t)
		h.governedPath = "topics/orders.yaml"
		h.refusedPath = h.governedPath // the content read is refused; the MR/branch reads stay 200
		c := h.client(t)

		for name, read := range map[string]func() ([]byte, error){
			"FileAtBase": func() ([]byte, error) { return c.FileAtBase("42", "7", h.governedPath) },
			"FileAtHead": func() ([]byte, error) { return c.FileAtHead("42", "7", h.governedPath) },
			"FileAtRef":  func() ([]byte, error) { return c.FileAtRef("42", h.governedPath, pinTarget) },
		} {
			_, err := read()
			if err == nil {
				t.Fatalf("%s: a metadata-only token must not read governed content", name)
			}
			if errors.Is(err, forge.ErrNotFound) {
				t.Fatalf("%s: a forbidden read rendered as ABSENT — the absent-means-trusted collapse (S00 Q4)", name)
			}
			if !errors.Is(err, forge.ErrUnauthorized) {
				t.Fatalf("%s: the refusal must carry forge.ErrUnauthorized, got %v", name, err)
			}
		}
	})

	t.Run("github", func(t *testing.T) {
		// The fine-grained-PAT shape (S00 Q4): the repo GET stays 200 (the
		// token CAN read the repo object), and EVERY content read — the
		// content-scope probe and the specific path alike — is refused. The
		// probe is the same permission the governed read needs, so the read
		// fails closed with forge.ErrUnauthorized, never absence.
		h := githubTransportHarness(t)
		h.contentsStatus = 403
		c := h.client(t)
		for name, read := range map[string]func() ([]byte, error){
			"FileAtBase": func() ([]byte, error) { return c.FileAtBase("platform/orders-service", "482", h.governedPath) },
			"FileAtHead": func() ([]byte, error) { return c.FileAtHead("platform/orders-service", "482", h.governedPath) },
			"FileAtRef":  func() ([]byte, error) { return c.FileAtRef("platform/orders-service", h.governedPath, pinTarget) },
		} {
			_, err := read()
			switch {
			case err == nil:
				t.Fatalf("%s: a metadata-only token must not read governed content", name)
			case errors.Is(err, forge.ErrNotFound):
				t.Fatalf("%s: a forbidden read under a metadata-only token rendered as ABSENT — the absent-means-trusted collapse (S00 Q4)", name)
			case !errors.Is(err, forge.ErrUnauthorized):
				t.Fatalf("%s: the refused read must carry forge.ErrUnauthorized, got %v", name, err)
			}
		}
	})
}
