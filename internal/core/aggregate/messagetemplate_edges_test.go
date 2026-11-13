package aggregate_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/PlatformRelay/assent/internal/core/aggregate"
)

func TestReplaceMessageSlotsEdges(t *testing.T) {
	boom := errors.New("boom")
	calls := 0
	replacer := func(expr string) (string, error) {
		calls++
		if expr == "bad" {
			return "", boom
		}
		return strings.ToUpper(expr), nil
	}
	tests := []struct {
		name    string
		tmpl    string
		want    string
		wantErr error
		calls   int
	}{
		{name: "no slots passthrough", tmpl: "plain text", want: "plain text"},
		{name: "substitutes each slot", tmpl: "{{ a }}-{{b}}", want: "A-B", calls: 2},
		{name: "first error aborts and later slots are not evaluated", tmpl: "{{ bad }} {{ later }}", wantErr: boom, calls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls = 0
			got, err := aggregate.ReplaceMessageSlots(tt.tmpl, replacer)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if calls != tt.calls {
				t.Fatalf("replacer called %d times, want %d", calls, tt.calls)
			}
		})
	}
}

func TestFactRefFromExprEdges(t *testing.T) {
	tests := []struct {
		expr         string
		provider, nm string
		ok           bool
	}{
		{"facts.git.sha", "git", "sha", true},
		{"facts.git", "", "", false},
		{"old", "", "", false},
	}
	for _, tt := range tests {
		p, n, ok := aggregate.FactRefFromExpr(tt.expr)
		if p != tt.provider || n != tt.nm || ok != tt.ok {
			t.Errorf("FactRefFromExpr(%q) = %q,%q,%v", tt.expr, p, n, ok)
		}
	}
}

func TestSensitiveFactAtShapes(t *testing.T) {
	sensitive := map[string]any{"facts": map[string]any{"p": map[string]any{"n": map[string]any{"sensitive": true}}}}
	tests := []struct {
		name string
		act  any
		want bool
	}{
		{"sensitive envelope", sensitive, true},
		{"non-map activation", "nope", false},
		{"facts not a map", map[string]any{"facts": 1}, false},
		{"provider missing", map[string]any{"facts": map[string]any{}}, false},
		{"name not an envelope", map[string]any{"facts": map[string]any{"p": map[string]any{"n": 3}}}, false},
		{"flag absent", map[string]any{"facts": map[string]any{"p": map[string]any{"n": map[string]any{}}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := aggregate.SensitiveFactAt(tt.act, "p", "n"); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompileMessageTemplateLocations(t *testing.T) {
	tests := []struct {
		name    string
		tmpl    string
		wantErr string // empty = must compile
	}{
		{name: "no slot", tmpl: "static"},
		{name: "valid slots compile", tmpl: "{{ old }} -> {{ new }}"},
		{name: "unknown ident on line 2", tmpl: "line one\nsecond {{ nope }}", wantErr: "message template:2:"},
		{name: "syntax error is located", tmpl: "x {{ old + }}", wantErr: "message template:1:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := aggregate.CompileMessageTemplate(tt.tmpl)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
