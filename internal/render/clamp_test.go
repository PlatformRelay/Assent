package render

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestClamp500 (REQ-E8-S05-02) enforces D-091: 500 runes with a stable ellipsis suffix.
func TestClamp500(t *testing.T) {
	t.Parallel()
	exact := strings.Repeat("a", DefaultClampRunes)
	if got := Clamp(exact, DefaultClampRunes); got != exact {
		t.Fatalf("exact limit changed:\n got %q\nwant %q", got, exact)
	}

	over := strings.Repeat("b", DefaultClampRunes+1)
	got := Clamp(over, DefaultClampRunes)
	if runeLen(got) != DefaultClampRunes {
		t.Fatalf("Clamp500 rune len = %d, want %d", runeLen(got), DefaultClampRunes)
	}
	if !strings.HasSuffix(got, ClampEllipsis) {
		t.Fatalf("want %q suffix, got %q", ClampEllipsis, got)
	}
	wantPrefix := strings.Repeat("b", DefaultClampRunes-runeLen(ClampEllipsis))
	if !strings.HasPrefix(got, wantPrefix) {
		t.Fatalf("prefix mismatch:\n got %q\nwant prefix %q", got, wantPrefix)
	}

	// Unicode runes, not bytes.
	emoji := strings.Repeat("🚀", DefaultClampRunes+5)
	gotEmoji := Clamp(emoji, DefaultClampRunes)
	if runeLen(gotEmoji) != DefaultClampRunes {
		t.Fatalf("emoji clamp rune len = %d, want %d", runeLen(gotEmoji), DefaultClampRunes)
	}
	if !strings.HasSuffix(gotEmoji, ClampEllipsis) {
		t.Fatalf("emoji clamp missing ellipsis: %q", gotEmoji)
	}
}

func TestClampShortUnchanged(t *testing.T) {
	t.Parallel()
	input := "short value"
	if got := Clamp(input, DefaultClampRunes); got != input {
		t.Fatalf("short value changed: %q", got)
	}
}

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}
