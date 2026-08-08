package schemas

import (
	"encoding/json"
	"testing"
)

// REQ-AUD-S16-01 / D-121: ReplayBundleSchemaID is exported as the HASH DOMAIN for
// compare.ReplayBundleDigest, so it must stay the schema's own `$id` rather than a
// hand-copied literal that can silently drift. If it drifts, every pinned
// replayBundleDigest in the corpus silently changes meaning.
func TestReplayBundleSchemaIDMatchesSchemaFile(t *testing.T) {
	var doc struct {
		ID string `json:"$id"`
	}
	if err := json.Unmarshal(replayBundleSchemaJSON, &doc); err != nil {
		t.Fatalf("decode replay-bundle.schema.json: %v", err)
	}
	if doc.ID == "" {
		t.Fatal("replay-bundle.schema.json has no $id")
	}
	if ReplayBundleSchemaID != doc.ID {
		t.Fatalf("ReplayBundleSchemaID = %q, want the schema's own $id %q (D-121 hash domain)",
			ReplayBundleSchemaID, doc.ID)
	}
}
