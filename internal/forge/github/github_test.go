package github

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
)

const (
	// Fixture identities — synthetic owner/repo pairs, never a real org or
	// user (D-002 sanitization).
	baseRepo  = "octo-org/base-repo"
	forkRepo  = "octo-contrib/fork-repo"
	botUser   = "assent-bot"
	patToken  = "ghp_test-token"
	instToken = "ghs_install-token"

	// sameRepoPR is the PR object for a same-repo PR (not a fork).
	sameRepoPR = `{"number":7,"sha":"srcSHA","user":{"login":"octocat"},"labels":[{"name":"security-hold"}],` +
		`"base":{"ref":"main","sha":"tgtTIP","repo":{"full_name":"octo-org/base-repo"}},` +
		`"head":{"ref":"feature","sha":"srcSHA","repo":{"full_name":"octo-org/base-repo"}},"mergeable_state":"clean"}`

	// forkPR is a fork PR: head.repo differs from base.repo.
	forkPR = `{"number":7,"sha":"srcSHA","user":{"login":"octocat"},"labels":[],` +
		`"base":{"ref":"main","sha":"tgtTIP","repo":{"full_name":"octo-org/base-repo"}},` +
		`"head":{"ref":"feature","sha":"srcSHA","repo":{"full_name":"octo-contrib/fork-repo"}},"mergeable_state":"clean"}`

	// deletedForkPR is the null-head-repo shape — an ERROR, never ForkMR=false.
	deletedForkPR = `{"number":7,"sha":"srcSHA","user":{"login":"octocat"},"labels":[],` +
		`"base":{"ref":"main","sha":"tgtTIP","repo":{"full_name":"octo-org/base-repo"}},` +
		`"head":{"ref":"feature","sha":"srcSHA","repo":null},"mergeable_state":"clean"}`
)

// newServer builds an httptest.Server from a handler and a *Client pointed at
// it. No live network — every test drives this in-process fake GitHub.
func newServer(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	// Keep the SHIPPED retry budget (a 5xx here is still attempted
	// defaultMaxAttempts times) but spend no wall-clock on the backoff.
	c := New(srv.URL, patToken, botUser, WithSleeper(func(time.Duration) {}))
	return c, srv
}

// wantBearer asserts the credential travels only in the Authorization header
// and never leaks into the URL — the redaction contract.
func wantBearer(t *testing.T, r *http.Request) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer "+patToken {
		t.Fatalf("Authorization header = %q, want the Bearer PAT fixture", got)
	}
	if strings.Contains(r.URL.RawQuery, patToken) || strings.Contains(r.URL.Path, patToken) {
		t.Fatal("token leaked into the URL — redaction violated")
	}
}

// testAppKeyPEM mints a fresh RSA key PEM for the App-credential tests.
func testAppKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
}

// TestGetMR proves the PR metadata read pins: head SHA (pr.sha), base BRANCH
// TIP (base.sha, not the merge base), the PR's OWN head branch (head.ref, not
// base.ref), and labels (REQ-E10-S07-01).
func TestGetMR(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		wantBearer(t, r)
		if r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7" {
			_, _ = io.WriteString(w, sameRepoPR)
			return
		}
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
	})

	info, err := c.GetMR(baseRepo, "7")
	if err != nil {
		t.Fatalf("GetMR: %v", err)
	}
	if info.SourceSHA != "srcSHA" || info.TargetSHA != "tgtTIP" {
		t.Errorf("pins = %q/%q, want srcSHA/tgtTIP", info.SourceSHA, info.TargetSHA)
	}
	// SourceBranch is the PR's OWN head branch (head.ref), never base.ref and
	// never refs/pull/N/head (S00 Q1).
	if info.SourceBranch != "feature" || info.TargetBranch != "main" {
		t.Errorf("branches = %q/%q, want feature/main", info.SourceBranch, info.TargetBranch)
	}
	if len(info.Labels) != 1 || info.Labels[0] != "security-hold" {
		t.Errorf("Labels = %#v, want [security-hold]", info.Labels)
	}
	if info.ForkMR {
		t.Error("same-repo PR must not report ForkMR")
	}
}

// TestGetMRForkDetectionBothPolarities proves REQ-E10-S07-01's fork flag: a
// fork PR reports ForkMR=true, a same-repo PR false.
func TestGetMRForkDetectionBothPolarities(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/octo-org/base-repo/pulls/7" {
			_, _ = io.WriteString(w, forkPR)
			return
		}
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
	})

	info, err := c.GetMR(baseRepo, "7")
	if err != nil {
		t.Fatalf("GetMR (fork): %v", err)
	}
	if !info.ForkMR {
		t.Error("ForkMR = false, want true for a fork PR")
	}
	if info.SourceProjectID != forkRepo {
		t.Errorf("SourceProjectID = %q, want the fork's full name", info.SourceProjectID)
	}
	if info.SourceBranch != "feature" {
		t.Errorf("SourceBranch = %q, want the PR's own head branch %q", info.SourceBranch, "feature")
	}

	// Second server: same-repo polarity.
	c2, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/octo-org/base-repo/pulls/7" {
			_, _ = io.WriteString(w, sameRepoPR)
			return
		}
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
	})
	info2, err := c2.GetMR(baseRepo, "7")
	if err != nil {
		t.Fatalf("GetMR same-repo: %v", err)
	}
	if info2.ForkMR {
		t.Error("same-repo PR must not report ForkMR")
	}
}

// TestGetMRNullHeadRepoFailsClosed proves the absent-means-trusted trap
// (REQ-E10-S07-01): a null head.repo is an ERROR, never ForkMR=false.
func TestGetMRNullHeadRepoFailsClosed(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/octo-org/base-repo/pulls/7" {
			_, _ = io.WriteString(w, deletedForkPR)
			return
		}
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
	})

	_, err := c.GetMR(baseRepo, "7")
	if err == nil {
		t.Fatal("null head.repo must fail closed, got nil error")
	}
	if !strings.Contains(err.Error(), "fork identity unprovable") {
		t.Errorf("error must name the unprovable fork identity, got %v", err)
	}
}

// TestPATIdentityAndHeader proves the PAT identity shape (ADR-0021 item 7):
// user kind, the client's bot login, and the credential on the wire as the
// Bearer header only.
func TestPATIdentityAndHeader(t *testing.T) {
	c, _ := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		wantBearer(t, r)
		if r.URL.Path == "/repos/octo-org/base-repo/pulls/7" {
			_, _ = io.WriteString(w, sameRepoPR)
			return
		}
		http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
	})

	id, err := c.Identity()
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	if id.Kind != forge.IdentityUser || id.Login != botUser {
		t.Errorf("Identity = %+v, want user identity for %q", id, botUser)
	}
	if _, err := c.GetMR(baseRepo, "7"); err != nil {
		t.Fatalf("GetMR: %v", err)
	}
}

func TestAppInstalationTokenMintsAndCarriesJWT(t *testing.T) {
	mintCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/app/installations/6789/access_tokens":
			mintCalls++
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				t.Fatalf("installation exchange Authorization = %q, want Bearer <jwt>", auth)
			}
			jwt := strings.TrimPrefix(auth, "Bearer ")
			if parts := strings.Split(jwt, "."); len(parts) != 3 {
				t.Fatalf("installation JWT has %d segments, want 3 (header.claims.signature)", strings.Count(jwt, ".")+1)
			}
			_, _ = io.WriteString(w, `{"token":"ghs_install-token","expires_at":"2026-01-01T00:00:00Z"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/octo-org/base-repo/pulls/7":
			if got := r.Header.Get("Authorization"); got != "Bearer "+instToken {
				t.Fatalf("request carried %q, want the minted installation token (never the JWT)", got)
			}
			_, _ = io.WriteString(w, sameRepoPR)
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.Path, http.StatusInternalServerError)
		}
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL, "", botUser, WithApp(testAppKeyPEM(t), 12345, 6789), WithSleeper(func(time.Duration) {}))

	if _, err := c.GetMR(baseRepo, "7"); err != nil {
		t.Fatalf("GetMR with App credentials: %v", err)
	}
	if mintCalls != 1 {
		t.Fatalf("installation token mints = %d, want 1 (cached for the client)", mintCalls)
	}

	// Identity: an App installation is a BOT identity.
	id, err := c.Identity()
	if err != nil {
		t.Fatalf("Identity: %v", err)
	}
	if id.Kind != forge.IdentityApp || id.Login != botUser {
		t.Errorf("Identity = %+v, want app identity for %q", id, botUser)
	}
}

// TestAppCredentialFailureNamesNoSecret — a missing or rejected credential
// fails CLOSED with an error that names no secret material (REQ-E10-S06-02/03).
func TestAppCredentialFailureNamesNoSecret(t *testing.T) {
	keyPEM := testAppKeyPEM(t)

	t.Run("missing PAT", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("no request must be issued without a credential, got %s %s", r.Method, r.URL.Path)
		}))
		t.Cleanup(srv.Close)
		c := New(srv.URL, "", botUser)
		_, err := c.GetMR(baseRepo, "7")
		if err == nil {
			t.Fatal("missing credential must fail closed, got nil error")
		}
		if strings.Contains(err.Error(), patToken) {
			t.Errorf("error names the token: %v", err)
		}
	})

	t.Run("bad pem", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		}))
		t.Cleanup(srv.Close)
		c := New(srv.URL, "", botUser, WithApp("not a pem", 1, 2))
		_, err := c.GetMR(baseRepo, "7")
		if err == nil {
			t.Fatal("unparseable app key must fail closed")
		}
		if !strings.Contains(err.Error(), "PEM private key") {
			t.Fatalf("error must name the credential shape problem, got %v", err)
		}
		if strings.Contains(err.Error(), "PRIVATE KEY") {
			t.Fatalf("error must never echo key material, got %v", err)
		}
	})

	t.Run("rejected exchange", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}))
		t.Cleanup(srv.Close)
		c := New(srv.URL, "", botUser, WithApp(keyPEM, 12, 34))
		_, err := c.GetMR(baseRepo, "7")
		if err == nil {
			t.Fatal("rejected installation credential must fail closed")
		}
		if strings.Contains(err.Error(), "PRIVATE KEY") || strings.Contains(err.Error(), "ghs_") {
			t.Fatalf("error names secret material: %v", err)
		}
	})
}
