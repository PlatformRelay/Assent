// Package provider hosts the typed provider protocol (ADR-0017 §6).
//
// P3-E2-S04 freezes major-version negotiation: host major H accepts a provider
// announcing major P iff P == H. Any mismatch (including missing/unparseable
// majors) is a capability gap — the provider is refused and every fact it would
// have supplied is unavailable; those facts never auto-merge.
package provider

import (
	"strconv"
	"strings"
	"unicode"
)

// HostMajor is the protocol major this host is built for. It matches the frozen
// FactResponse/FactQuery apiVersion "provider.assent.dev/v1alpha1" (major 1).
const HostMajor = 1

// FactStateUnavailable is forced onto every requested output when negotiation refuses a provider.
// Alias of StateUnavailable — kept for the P3-E2 negotiation surface.
const FactStateUnavailable = StateUnavailable

// Outcome is the resolved (H, P) negotiation cell — exactly one of accept or capability gap.
type Outcome string

// Negotiation outcome labels (exactly one per (H, P) cell).
const (
	OutcomeAccept        Outcome = "accept"
	OutcomeCapabilityGap Outcome = "capability_gap"
)

// Negotiation is the result of comparing provider major P against host major H.
type Negotiation struct {
	Outcome Outcome
	// ProviderRefused is true on capability gap — the host must not process the
	// provider's response body under the host schema.
	ProviderRefused bool
	// AutoMergeEligible is false on capability gap: unavailable facts must never
	// satisfy a controlling obligation that would arm auto-merge.
	AutoMergeEligible bool
	// FactStates maps each requested output to its forced state on gap; empty on accept.
	FactStates map[string]string
	// ProviderMajor is the parsed P, or nil when missing/unparseable.
	ProviderMajor *int
	HostMajor     int
}

// ParseMajor extracts the protocol major from a provider apiVersion
// ("provider.assent.dev/v1alpha1" → 1). Missing or unparseable → ok=false.
func ParseMajor(apiVersion string) (major int, ok bool) {
	apiVersion = strings.TrimSpace(apiVersion)
	if apiVersion == "" {
		return 0, false
	}
	group, version, found := strings.Cut(apiVersion, "/")
	if !found || group == "" || version == "" {
		return 0, false
	}
	if !strings.HasPrefix(version, "v") {
		return 0, false
	}
	digits := version[1:]
	end := 0
	for end < len(digits) && unicode.IsDigit(rune(digits[end])) {
		end++
	}
	if end == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(digits[:end])
	if err != nil {
		return 0, false
	}
	return n, true
}

// Negotiate resolves the (H, P) cell for a provider response's apiVersion.
// requestedOutputs are the fact names the host asked for; on capability gap each
// becomes unavailable. Empty/unparseable apiVersion is treated as mismatch
// (fail closed) — omitting the field must not be rewarded with acceptance.
func Negotiate(hostMajor int, apiVersion string, requestedOutputs []string) Negotiation {
	n := Negotiation{
		HostMajor: hostMajor,
	}
	p, ok := ParseMajor(apiVersion)
	if !ok {
		return capabilityGap(n, requestedOutputs, nil)
	}
	n.ProviderMajor = &p
	if p == hostMajor {
		n.Outcome = OutcomeAccept
		n.ProviderRefused = false
		n.AutoMergeEligible = true
		return n
	}
	return capabilityGap(n, requestedOutputs, &p)
}

func capabilityGap(n Negotiation, outputs []string, p *int) Negotiation {
	n.Outcome = OutcomeCapabilityGap
	n.ProviderRefused = true
	n.AutoMergeEligible = false
	n.ProviderMajor = p
	n.FactStates = make(map[string]string, len(outputs))
	for _, name := range outputs {
		n.FactStates[name] = FactStateUnavailable
	}
	return n
}
