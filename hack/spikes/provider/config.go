package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"time"
)

// CapabilityFullContent is the explicit trusted capability an operator must
// grant before a provider may receive full old/new content (ADR-0017 §6).
const CapabilityFullContent = "trusted-full-content"

// Config is the operator-authored declaration of one provider: what it
// outputs and what slices of the change it may see.
type Config struct {
	Name         string         `json:"name"`
	Requests     ConfigRequests `json:"requests"`
	Capabilities []string       `json:"capabilities"`
}

// ConfigRequests is the "requests" block of a provider declaration: which value
// slices it wants and whether it asks for full old/new content.
type ConfigRequests struct {
	Values      ConfigValues `json:"values"`
	FullContent bool         `json:"fullContent"`
}

// ConfigValues is the declared set of JSON-Pointer value projections a provider
// requests.
type ConfigValues struct {
	Pointers []string `json:"pointers"`
}

// LoadProviderConfig parses and validates a provider declaration. A provider
// requesting full old/new content without the explicit trusted capability is
// refused here — before any query is ever built.
func LoadProviderConfig(raw []byte) (Config, error) {
	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("provider config: %w", err)
	}
	if cfg.Requests.FullContent && !slices.Contains(cfg.Capabilities, CapabilityFullContent) {
		return Config{}, fmt.Errorf(
			"provider %q requests full old/new content but lacks the %q capability — refused",
			cfg.Name, CapabilityFullContent)
	}
	return cfg, nil
}

// ValueChange is one touched JSON-Pointer in the change under judgment.
type ValueChange struct {
	Old any
	New any
}

// BuildQuery assembles the minimized FactQuery: the projections are the
// intersection of what the provider declared and what the change actually
// touched — undeclared content never enters the request.
func BuildQuery(cfg Config, queryID string, asOf time.Time, subject Subject, outputs []string, change map[string]ValueChange) FactQuery {
	q := FactQuery{
		APIVersion: APIVersion,
		Kind:       KindFactQuery,
		QueryID:    queryID,
		AsOf:       asOf,
		Subject:    subject,
		Outputs:    outputs,
	}
	for _, ptr := range cfg.Requests.Values.Pointers {
		vc, touched := change[ptr]
		if !touched {
			continue
		}
		q.Projections.Values = append(q.Projections.Values, ValueProjection{
			Pointer: ptr,
			Old:     vc.Old,
			New:     vc.New,
		})
	}
	return q
}
