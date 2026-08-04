package schemadrift

import "testing"

func TestSplitNonEmptyLines(t *testing.T) {
	got := splitNonEmptyLines("schemas/a\n\n schemas/b \n")
	if len(got) != 2 || got[0] != "schemas/a" || got[1] != "schemas/b" {
		t.Fatalf("got %#v", got)
	}
}

func TestAsObjectNil(t *testing.T) {
	obj, err := asObject(nil, "properties")
	if err != nil {
		t.Fatalf("asObject(nil): %v", err)
	}
	if len(obj) != 0 {
		t.Fatalf("expected empty object, got %#v", obj)
	}
}
