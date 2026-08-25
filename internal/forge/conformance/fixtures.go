package conformance

import (
	"github.com/PlatformRelay/assent/internal/core/decision"
	"github.com/PlatformRelay/assent/internal/forge"
	"github.com/PlatformRelay/assent/internal/render"
)

// fixtures.go holds the conformance suite's FIXTURE VOCABULARY — the markers,
// summaries and desired-states replayed from the P3-E5 publication-protocol
// contract fixtures. Moved out of `_test.go` by E10-S01 (REQ-E10-S01-01) for one
// reason only: Go cannot import a `_test.go` file, so while these lived there the
// only way to conformance-test a second adapter was to COPY them — which
// guarantees drift and is why D-084's `github-deferred` rows were unflippable.
//
// Nothing here is new and nothing is changed: each helper is the byte-identical
// body it had in `reconciliation_test.go`, fixture line references included, so a
// reviewer can diff rather than re-derive.

const decHex = "sha256:1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaaa"

// ---- rerun-idempotence.yaml (docs/contracts/p3-e5-publication-protocol/fixtures/rerun-idempotence.yaml) ----

const occChallenge = "sha256:c6957a516c95532386bed08f56441dfbb8d18efda24f5abdab1e48437aa3357d" // line 23
const occComment = "sha256:1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaa1111aaaa"   // line 25

func rerunChallengeMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  proj,
			MR:       mrIID,
			Rule:     "topic-safety/retention-shrink-challenge",
			Effect:   "challenge",
			EntryRef: "topic-registry:orders.events.v1",
		},
		Occurrence: occChallenge,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

func rerunCommentMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  proj,
			MR:       mrIID,
			Rule:     "ownership/entry-owner-required",
			Effect:   "comment",
			EntryRef: "topic-registry:orders.events.v1",
		},
		Occurrence: occComment,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

// ---- crash-then-rerun.yaml ----

const occCrashChallenge = "sha256:aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111" // line 23
const occCrashComment = "sha256:bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222"   // line 25

func crashChallengeMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  "platform/orders-service",
			MR:       "551",
			Rule:     "topic-safety/retention-shrink-challenge",
			Effect:   "challenge",
			EntryRef: "topic-registry:payments.events.v1",
		},
		Occurrence: occCrashChallenge,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

func crashCommentMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  "platform/orders-service",
			MR:       "551",
			Rule:     "ownership/entry-owner-required",
			Effect:   "comment",
			EntryRef: "topic-registry:payments.events.v1",
		},
		Occurrence: occCrashComment,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

// ---- duplicate-repair.yaml ----

const occDup = "sha256:dddd4444dddd4444dddd4444dddd4444dddd4444dddd4444dddd4444dddd4444" // line 38

func dupMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project:  "platform/orders-service",
			MR:       "612",
			Rule:     "topic-safety/retention-shrink-challenge",
			Effect:   "challenge",
			EntryRef: "topic-registry:orders.events.v1",
		},
		Occurrence: occDup,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "finding-thread", SchemaVersion: "v1alpha1"},
	}
}

func rerunSummaryMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project: proj,
			MR:      mrIID,
			Rule:    "assent/summary",
			Effect:  "comment",
		},
		Occurrence: decHex,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "summary-comment", SchemaVersion: "v1alpha1"},
	}
}

func crashSummaryMarker() forge.Marker {
	return forge.Marker{
		Slot: forge.Slot{
			Project: "platform/orders-service",
			MR:      "551",
			Rule:    "assent/summary",
			Effect:  "comment",
		},
		Occurrence: decHex,
		Decision:   decHex,
		Artifact:   forge.Artifact{Kind: "summary-comment", SchemaVersion: "v1alpha1"},
	}
}

func fixtureSummaryBody() string {
	pm := decision.PresentationModel{
		APIVersion: "assent.dev/v1alpha1",
		Kind:       "PresentationModel",
		Decision:   "REVIEW",
		Findings: []decision.Finding{{
			Rule:    "topic-safety/retention-shrink-challenge",
			Effect:  "challenge",
			Subject: "topic-registry:orders.events.v1",
			Code:    "retention-shrink",
			Points:  10,
		}},
	}
	body, err := render.RenderSummary(pm, render.Context{
		Options:       render.DefaultOptions(),
		RiskThreshold: 10,
	})
	if err != nil {
		panic(err)
	}
	return body
}

func rerunSummary() *forge.DesiredSummary {
	return &forge.DesiredSummary{
		Marker: rerunSummaryMarker(),
		Body:   fixtureSummaryBody(),
	}
}

func crashSummary() *forge.DesiredSummary {
	return &forge.DesiredSummary{
		Marker: crashSummaryMarker(),
		Body:   fixtureSummaryBody(),
	}
}

func desiredThreadFor(m forge.Marker, summary *forge.DesiredSummary) forge.DesiredReviewState {
	return forge.DesiredReviewState{
		Project: m.Slot.Project,
		MR:      m.Slot.MR,
		Thread:  &forge.DesiredThread{Marker: m, Body: "obligation not proven"},
		Summary: summary,
	}
}

func dupDesired() forge.DesiredReviewState {
	return desiredThreadFor(dupMarker(), nil)
}
