package factory_test

import (
	"reflect"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/factory"
)

// factory_test.go pins the constructor shape S00 §Forward obligations 2b's
// resolution requires (REQ-E10X-01-01): no RunPort implementation binds a
// project in constructor state, because the port's content accessors carry the
// composite (project, mr, path) handle explicitly.

// TestFactoryConstructorsTakeNoProjectArgument is the compile-time-adjacent
// assertion the delta spec calls for: the factory's constructor signatures carry
// endpoint/token/botAuthor only — a parameter that looked like a project would
// be the stateful binding the decision exists to prevent.
func TestFactoryConstructorsTakeNoProjectArgument(t *testing.T) {
	// Go reflection carries no parameter NAMES, so the pin is on the SHAPE: the
	// constructor takes three strings (endpoint, token, botAuthor) plus a
	// variadic option — a constructor that bound a project would carry a fourth
	// fixed parameter, and this is the shape that fails.
	typ := reflect.TypeOf(factory.GitLab)
	if got := typ.NumIn(); got != 4 {
		t.Fatalf("factory.GitLab takes %d parameters, want 4 (endpoint, token, botAuthor, variadic opts)", got)
	}
	for i := 0; i < 3; i++ {
		if got := typ.In(i); got.Kind() != reflect.String {
			t.Fatalf("factory.GitLab param %d is %v, want string", i, got)
		}
	}
	if !typ.IsVariadic() {
		t.Fatal("factory.GitLab must be variadic over its options")
	}
	if got := typ.Out(0); got != reflect.TypeOf((*forge.RunPort)(nil)).Elem() {
		t.Fatalf("factory.GitLab must return forge.RunPort, got %v", got)
	}
}

// TestFactoryNewRejectsUnknownForge is the fail-closed polarity of the neutral
// constructor: an unknown forge name is an ERROR, never a GitLab fallback
// (REQ-E10-S13-01's direction, pinned at the factory one story early).
func TestFactoryNewRejectsUnknownForge(t *testing.T) {
	if _, err := factory.New("gitea", "https://example.invalid", "tok", "bot"); err == nil {
		t.Fatal("an unknown forge must fail closed at the factory, not default to GitLab")
	}
}
