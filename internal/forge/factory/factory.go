// Package factory is the NEUTRAL adapter constructor (E10-S02 / REQ-E10X-02-01).
//
// `cmd/assent` must not import a concrete adapter package (depguard, ADR-0021
// §1) but must still construct real adapters. The constructor therefore lives
// here: this package is the ONLY importer of both adapters, `cmd/assent` imports
// this package and nothing adapter-named crosses the boundary it owns.
//
// The constructors take NO project argument (REQ-E10X-01-01): the port is
// stateless and its MR-relative accessors carry the composite
// `(project, mr, path)` handle explicitly, so no RunPort implementation binds a
// project in constructor state.
package factory

import (
	"fmt"
	"time"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/github"
	"github.com/PlatformRelay/assent/internal/forge/gitlab"
)

// GitLab constructs the GitLab adapter as a forge.RunPort.
func GitLab(endpoint, token, botAuthor string, opts ...Option) forge.RunPort {
	var o Options
	for _, opt := range opts {
		opt(&o)
	}
	var adapter []gitlab.Option
	if o.DisableSleep {
		// Tests disable the retry sleeper; the adapter's own option vocabulary
		// never crosses this boundary in either direction.
		adapter = append(adapter, gitlab.WithSleeper(func(time.Duration) {}))
	}
	return gitlab.New(endpoint, token, botAuthor, adapter...)
}

// Option is a forge-neutral client option. Concrete adapter option types stay
// adapter-internal: cmd/assent names factory.Option only.
type Option func(*Options)

// Options is the neutral construction knob set.
type Options struct {
	// DisableSleep turns off the adapter's retry backoff sleeping (tests).
	DisableSleep bool
}

// NoSleep is the option that disables retry backoff sleeping.
func NoSleep(o *Options) { o.DisableSleep = true }

// Kind names the supported forges for forge selection (E10-S13). Values are
// stable CLI-facing vocabulary.
type Kind string

const (
	// KindGitLab selects the GitLab adapter.
	KindGitLab Kind = "gitlab"
	// KindGitHub selects the GitHub adapter once E10-S06 lands; until then
	// New returns an error naming it unimplemented — fail closed, never
	// default-to-GitLab.
	KindGitHub Kind = "github"
)

// New constructs a RunPort for the named forge. An unknown forge is an error —
// never a GitLab fallback (E10-S13's fail-closed direction, enforced here).
func New(kind Kind, endpoint, token, botAuthor string) (forge.RunPort, error) {
	switch kind {
	case KindGitLab:
		return GitLab(endpoint, token, botAuthor), nil
	case KindGitHub:
		return github.New(endpoint, token, botAuthor), nil
	default:
		return nil, fmt.Errorf("factory: unknown forge %q (expected %q or %q)", kind, KindGitLab, KindGitHub)
	}
}
