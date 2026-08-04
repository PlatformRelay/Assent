package forge_test

import (
	"reflect"
	"testing"

	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/forge/fake"
)

// TestResolveFake — REQ-E4-S01-02/03: Resolve returns evidence or typed gap; deterministic.
func TestResolveFake(t *testing.T) {
	t.Parallel()

	const (
		proj     = "platform/orders-service"
		mr       = "482"
		subject  = "topic-registry:orders.events.v1"
		srcSHA   = "abc123deadbeef"
		tgtSHA   = "def456cafebabe"
		mergeDig = "sha256:merge-result-digest-fixed"
		author   = "alice"
		bot      = "assent-bot"
	)

	eligibleReq := forge.ResolveRequest{
		Project:           proj,
		MR:                mr,
		Subject:           subject,
		SourceSha:         srcSHA,
		TargetSha:         tgtSHA,
		MergeResultDigest: mergeDig,
		MRAuthor:          author,
	}

	t.Run("eligible approval returns explicit evidence fields", func(t *testing.T) {
		t.Parallel()
		f := fake.New(bot, srcSHA, tgtSHA, mergeDig)
		f.MRAuthor = author
		f.Capabilities = forge.CapabilityFlags{Tier: forge.TierPremium, HasApprovalRulesAPI: true}
		f.ResolveMode = fake.ResolveEligible

		var first forge.ResolveResult
		for run := 0; run < 2; run++ {
			got, err := f.Resolve(eligibleReq)
			if err != nil {
				t.Fatalf("run %d: Resolve: %v", run, err)
			}
			if !got.HasEvidence() || got.HasGap() {
				t.Fatalf("run %d: want evidence-only result, got %+v", run, got)
			}
			if run == 0 {
				first = got
			} else if !reflect.DeepEqual(first, got) {
				t.Fatalf("run %d: non-deterministic resolve:\nwant %+v\ngot  %+v", run, first, got)
			}
		}

		ev := first.Evidence
		if ev.VerifyingCapability != "approval-rules-api" {
			t.Errorf("VerifyingCapability = %q, want approval-rules-api", ev.VerifyingCapability)
		}
		if ev.ApprovalsRequired != 1 {
			t.Errorf("ApprovalsRequired = %d, want 1", ev.ApprovalsRequired)
		}
		if len(ev.Eligibility) == 0 || len(ev.ApprovedBy) == 0 {
			t.Errorf("Eligibility/ApprovedBy must be populated, got eligibility=%v approvedBy=%+v",
				ev.Eligibility, ev.ApprovedBy)
		}
		if ev.Pins.SourceSha != srcSHA {
			t.Errorf("Pins.SourceSha = %q, want %q", ev.Pins.SourceSha, srcSHA)
		}
		if ev.Expired {
			t.Error("eligible fixture evidence must not be expired")
		}
		for _, id := range ev.Eligibility {
			if id == author {
				t.Errorf("eligible set must not include MR author %q", author)
			}
		}
		for _, a := range ev.ApprovedBy {
			if a.Username == author || a.ID == author {
				t.Errorf("approvedBy must exclude MR author, got %+v", a)
			}
		}
	})

	t.Run("tier gap returns typed CapabilityGap never silent evidence", func(t *testing.T) {
		t.Parallel()
		f := fake.New(bot, srcSHA, tgtSHA, mergeDig)
		f.MRAuthor = author
		f.Capabilities = forge.CapabilityFlags{Tier: forge.TierFree, HasApprovalRulesAPI: false}
		f.ResolveMode = fake.ResolveGapFreeTier

		got, err := f.Resolve(eligibleReq)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got.HasEvidence() || !got.HasGap() {
			t.Fatalf("want gap-only result, got evidence=%v gap=%v", got.Evidence, got.Gap)
		}
		if got.Gap.Reason != forge.GapFreeTierRequireReview {
			t.Errorf("Gap.Reason = %q, want %q", got.Gap.Reason, forge.GapFreeTierRequireReview)
		}
		if got.Gap.Subject != subject {
			t.Errorf("Gap.Subject = %q, want %q", got.Gap.Subject, subject)
		}
	})

	var _ forge.Resolver = (*fake.Forge)(nil)
}
