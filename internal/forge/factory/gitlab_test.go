package factory_test

import (
	"testing"

	"github.com/PlatformRelay/assent/internal/forge/factory"
)

// GitLab construction coverage (REQ-E10X-02-01): the neutral factory is the
// only adapter importer, so its constructor paths are tested here.

func TestGitLabConstructsRunPort(t *testing.T) {
	port := factory.GitLab("https://gitlab.example", "tok", "assent-bot", factory.NoSleep)
	if _, err := port.Identity(); err != nil {
		t.Fatalf("Identity: %v", err)
	}
}

func TestNewServesGitLabKind(t *testing.T) {
	port, err := factory.New(factory.KindGitLab, "https://gitlab.example", "tok", "bot")
	if err != nil {
		t.Fatalf("New(gitlab): %v", err)
	}
	if _, err := port.Identity(); err != nil {
		t.Fatalf("Identity: %v", err)
	}
}
