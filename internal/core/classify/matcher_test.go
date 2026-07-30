package classify

import (
	"sort"
	"testing"

	"github.com/PlatformRelay/assent/internal/change"
)

func paths(cs []change.Change) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.File+c.Path)
	}
	sort.Strings(out)
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// REQ-E1-S06-01 — the `files` domain selects only changes under the matched path glob.
func TestFilesDomainMatch(t *testing.T) {
	cs := change.ChangeSet{Changes: []change.Change{
		{File: "topics/orders.yml", Path: "/partitions", Kind: change.KindModify},
		{File: "catalog/services.json", Path: "/tier", Kind: change.KindModify},
	}}
	got := MatchFiles(cs, "topics/**")
	if len(got) != 1 || got[0].File != "topics/orders.yml" {
		t.Fatalf("files:topics/** must select only the topics change, got %+v", got)
	}
	// A nested path under topics/ also matches; an unrelated tree does not.
	if len(MatchFiles(cs, "catalog/**")) != 1 {
		t.Errorf("catalog/** should select the catalog change")
	}
	if len(MatchFiles(cs, "other/**")) != 0 {
		t.Errorf("a non-matching glob must select nothing")
	}
}

// REQ-E1-S06-02 — the `values.pointers` domain matches the FIELD pointer, not the file glob.
func TestValuesPointersDomainMatch(t *testing.T) {
	cs := change.ChangeSet{Changes: []change.Change{
		{File: "topics/orders.yml", Path: "/partitions", Kind: change.KindModify},
		{File: "topics/orders.yml", Path: "/owner", Kind: change.KindModify},
	}}
	got := MatchValuePointers(cs, "/partitions")
	if len(got) != 1 || got[0].Path != "/partitions" {
		t.Fatalf("values.pointers:/partitions must select only /partitions, got %+v", got)
	}
}

// REQ-E1-S06-03 — the `entryEvents` domain selects a collection-ENTRY event (EntryRef set) of the
// matched Kind, and does NOT select a plain (non-entry) change of a different kind. It matches
// collection-entry identity churn only — never whole-file add/delete/rename (ADR-0003 fileEvents,
// out of scope per the epic Non-goals).
func TestEntryEventsDomainMatch(t *testing.T) {
	cs := change.ChangeSet{Changes: []change.Change{
		{File: "catalog/services.json", Path: "/services/payments-gateway", Kind: change.KindDelete, EntryRef: "service:payments-gateway"},
		{File: "catalog/services.json", Path: "/services/orders-api/tier", Kind: change.KindModify, EntryRef: "service:orders-api"},
		{File: "topics/orders.yml", Path: "/partitions", Kind: change.KindModify}, // no EntryRef
	}}
	got := MatchEntryEvents(cs, change.KindDelete)
	if len(got) != 1 || got[0].EntryRef != "service:payments-gateway" {
		t.Fatalf("entryEvents:deleted must select only the collection-entry delete, got %+v", got)
	}
	// A plain (no-EntryRef) change is never an entry event, even of the matched kind.
	if n := len(MatchEntryEvents(cs, change.KindModify)); n != 1 {
		t.Errorf("entryEvents:modified must select only the ENTRY modify (with EntryRef), got %d", n)
	}
}

// REQ-E1-S06-04 — the `valueChanges` domain matches structurally on Kind, independent of path.
func TestValueChangesDomainMatch(t *testing.T) {
	cs := change.ChangeSet{Changes: []change.Change{
		{File: "f.yaml", Path: "/a", Kind: change.KindAdd},
		{File: "f.yaml", Path: "/b", Kind: change.KindDelete},
		{File: "f.yaml", Path: "/c", Kind: change.KindModify},
	}}
	got := MatchValueChanges(cs, change.KindModify)
	if len(got) != 1 || got[0].Path != "/c" {
		t.Fatalf("valueChanges:modify must select only the modify, got %+v", got)
	}
	// Multiple kinds select the union.
	if n := len(MatchValueChanges(cs, change.KindAdd, change.KindDelete)); n != 2 {
		t.Errorf("valueChanges:{add,delete} must select 2, got %d", n)
	}
	// A combining example: files AND valueChanges narrow together (intersection via composition).
	both := MatchValueChanges(change.ChangeSet{Changes: MatchFiles(cs, "f.yaml")}, change.KindDelete)
	if len(both) != 1 || both[0].Path != "/b" {
		t.Errorf("files:f.yaml AND valueChanges:delete must narrow to /b, got %+v", both)
	}
}

// REQ-E1-S06-05 (adversarial arm) — the additive matchers do NOT weaken the `.assent/**`
// self-vouch boundary: a change under `.assent/**` still classifies to assent-policy regardless of
// what any matcher would select over it. (The shipped goldens TestAssentPolicyBlockGolden /
// TestAssentPolicyDominatesMixedChangeSet / TestNonPolicyChangeIsUnclassified are re-run unmodified
// and stay green — this test adds the matcher-specific adversarial case.)
func TestMatchersDoNotWeakenPolicyDominance(t *testing.T) {
	cs := change.ChangeSet{Changes: []change.Change{
		{File: ".assent/packs/topic.yml", Path: "/rules/0/effect", Kind: change.KindModify},
		{File: "topics/orders.yml", Path: "/partitions", Kind: change.KindModify},
	}}
	// A matcher would happily select the .assent change...
	if len(MatchFiles(cs, ".assent/**")) != 1 {
		t.Fatalf("precondition: files:.assent/** selects the policy change")
	}
	// ...but Classify still dominates the whole set to assent-policy (never a vouchable class).
	if got := Classify(cs); got != ClassAssentPolicy {
		t.Errorf("Classify must still route a set containing a .assent/** change to %q, got %q", ClassAssentPolicy, got)
	}
	// And a vouch routing of that reserved class is still rejected at load.
	if err := ValidateRouting(ClassAssentPolicy, DispositionVouch); err == nil {
		t.Errorf("ValidateRouting must still reject vouch on the reserved assent-policy class")
	}
}

// REQ-E1-S06-06 — matching is order-independent over the input ChangeSet: shuffling the input
// yields the same selected set. (Purity is proven by TestCorePurity over internal/core/**.)
func TestMatcherDomainOrderIndependent(t *testing.T) {
	forward := change.ChangeSet{Changes: []change.Change{
		{File: "topics/a.yml", Path: "/x", Kind: change.KindModify},
		{File: "topics/b.yml", Path: "/y", Kind: change.KindDelete},
		{File: "catalog/c.json", Path: "/z", Kind: change.KindModify},
	}}
	reversed := change.ChangeSet{Changes: []change.Change{
		forward.Changes[2], forward.Changes[1], forward.Changes[0],
	}}
	if !eq(paths(MatchFiles(forward, "topics/**")), paths(MatchFiles(reversed, "topics/**"))) {
		t.Errorf("files domain is order-dependent")
	}
	if !eq(paths(MatchValueChanges(forward, change.KindModify)), paths(MatchValueChanges(reversed, change.KindModify))) {
		t.Errorf("valueChanges domain is order-dependent")
	}
}
