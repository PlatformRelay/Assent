package aggregate

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/PlatformRelay/assent/schemas"
)

// EvaluationInput is the Go form of the frozen evaluation-input.schema.json —
// the engine's decision input (E2-S02). Numeric old/new/fact values are decoded
// as json.Number so a later numeric compare stays injective (no float64
// collapse), mirroring internal/change's numeric discipline.
type EvaluationInput struct {
	ChangeSet ChangeSet                  `json:"changeSet"`
	Facts     map[string]map[string]Fact `json:"facts"`
	MR        MR                         `json:"mr"`
	Require   []string                   `json:"require"`
}

// ChangeSet holds the enumerated changes the decision is evaluated over.
type ChangeSet struct {
	Changes []EvalChange `json:"changes"`
}

// EvalChange is one governed change: its subject/file/path/kind plus the typed
// pre/post values (any: json.Number, string, bool, map, slice, or nil).
type EvalChange struct {
	Subject string `json:"subject"`
	File    string `json:"file"`
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Old     any    `json:"old"`
	New     any    `json:"new"`
	// Entry is the reconstructed whole-entry object for this change's EntryRef;
	// nil = not reconstructed (fall back to New). In-memory enrichment only, NOT
	// part of the frozen wire contract — hence json:"-" (LoadEvaluationInput's
	// strict decode never reads it; the frozen schema is untouched).
	Entry any `json:"-"`
	// OldEntry is the pre-image entry object; nil = fall back to Old. In-memory only.
	OldEntry any `json:"-"`
}

// Fact is a resolved provider fact (typed states; value absent unless resolved).
type Fact struct {
	State      string `json:"state"`
	Sensitive  bool   `json:"sensitive"`
	ObservedAt string `json:"observedAt"`
	ExpiresAt  string `json:"expiresAt,omitempty"`
	Value      any    `json:"value,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

// MR is the merge-request metadata bound to the `mr` predicate-scope field.
type MR struct {
	Author       string   `json:"author"`
	SourceBranch string   `json:"sourceBranch"`
	TargetBranch string   `json:"targetBranch"`
	Labels       []string `json:"labels,omitempty"`
}

// LoadEvaluationInput validates raw JSON against the frozen EvaluationInput
// schema, then decodes it with json.Number semantics so numeric values stay
// exact. It is pure (no clock/env/network/random).
func LoadEvaluationInput(raw []byte) (*EvaluationInput, error) {
	doc, err := jsonNumberDoc(raw)
	if err != nil {
		return nil, fmt.Errorf("evaluation-input: %w", err)
	}
	if err := schemas.EvaluationInputSchema.Validate(doc); err != nil {
		return nil, fmt.Errorf("evaluation-input: %w", err)
	}
	var in EvaluationInput
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&in); err != nil {
		return nil, fmt.Errorf("evaluation-input decode: %w", err)
	}
	return &in, nil
}

// jsonNumberDoc decodes raw into the any-tree jsonschema validates, with numbers
// as json.Number (the shape the frozen schemas expect).
func jsonNumberDoc(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return doc, nil
}
