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

// Config is the host-owned provider declaration (D-065): projections,
// capabilities, and typed outputs. Loaded beside frozen policy.Provider —
// not via a silent config.schema.json widen.
type Config struct {
	Name         string                 `json:"name"`
	Requests     ConfigRequests         `json:"requests"`
	Capabilities []string               `json:"capabilities,omitempty"`
	Outputs      map[string]Declaration `json:"outputs"`
}

// ConfigRequests is the "requests" block: which value slices the provider may
// see and whether it asks for full old/new content.
type ConfigRequests struct {
	Values      ConfigValues `json:"values"`
	FullContent bool         `json:"fullContent"`
}

// ConfigValues is the declared set of JSON-Pointer value projections.
type ConfigValues struct {
	Pointers []string `json:"pointers"`
}

// ValueChange is one touched JSON-Pointer in the change under judgment.
type ValueChange struct {
	Old any
	New any
}

// LoadProviderConfig parses and validates a host-owned provider declaration.
// fullContent without trusted-full-content is refused before any query exists.
// Each output's maxAge is validated against provider-contract.md ceilings
// (omit → error; exceed → reject, never clamp).
func LoadProviderConfig(raw []byte) (Config, error) {
	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("provider config: %w", err)
	}
	if cfg.Name == "" {
		return Config{}, fmt.Errorf("provider config: name is required")
	}
	if cfg.Requests.FullContent && !slices.Contains(cfg.Capabilities, CapabilityFullContent) {
		return Config{}, fmt.Errorf(
			"provider %q requests full old/new content but lacks the %q capability — refused",
			cfg.Name, CapabilityFullContent)
	}
	if len(cfg.Outputs) == 0 {
		return Config{}, fmt.Errorf("provider %q: outputs are required", cfg.Name)
	}
	for name, decl := range cfg.Outputs {
		if err := ValidateDeclarationMaxAge(decl); err != nil {
			return Config{}, fmt.Errorf("provider %q output %q: %w", cfg.Name, name, err)
		}
	}
	return cfg, nil
}

// BuildQuery assembles the minimized FactQuery: projections are the
// intersection of declared pointers and what the change actually touched —
// undeclared content never enters the request (REQ-E5-S02-01).
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
