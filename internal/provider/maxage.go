package provider

import (
	"fmt"
	"time"
)

// Host maxAge ceilings from docs/planning/provider-contract.md.
// A declaration may only shorten below its ceiling; omit and exceed are
// load-time errors (never fill-in, never clamp).
const (
	MaxAgePrincipal = time.Hour
	MaxAgeBoolean   = time.Hour
	MaxAgeRegistry  = 24 * time.Hour // string / integer reference data
	MaxAgeSensitive = 15 * time.Minute
	MaxAgeGlobalCap = 24 * time.Hour
)

// MaxAgeCeiling returns the host validation ceiling for a declaration's
// type + sensitive flag (provider-contract.md table). Sensitive always
// forces the 15m bound regardless of type. Unknown / empty types return 0 —
// ValidateDeclarationMaxAge rejects them at load (never inherit the 24h
// global cap: a typo like "principals" would otherwise accept maxAge above
// the real type's 1h ceiling — fail-open vs the contract enum).
func MaxAgeCeiling(decl Declaration) time.Duration {
	if decl.Sensitive {
		// Sensitive still requires a known type so a typo cannot skip the
		// principal/boolean 1h table via sensitive:true + 24h inheritance.
		if !knownDeclarationType(decl.Type) {
			return 0
		}
		return MaxAgeSensitive
	}
	switch decl.Type {
	case "principal", "boolean":
		return MaxAgePrincipal
	case "string", "integer":
		return MaxAgeRegistry
	default:
		return 0
	}
}

func knownDeclarationType(typ string) bool {
	switch typ {
	case "boolean", "string", "integer", "principal":
		return true
	default:
		return false
	}
}

// ValidateDeclarationMaxAge enforces provider-contract.md at load time:
// omit → error; unknown type → error; unparseable → error; exceed type/sensitive/global
// ceiling → reject (never clamp).
func ValidateDeclarationMaxAge(decl Declaration) error {
	if decl.MaxAge == "" {
		return fmt.Errorf("maxAge is required (omit is a load-time error per provider-contract.md)")
	}
	if !knownDeclarationType(decl.Type) {
		return fmt.Errorf("declaration type %q is not in the provider-contract enum (boolean|string|integer|principal); rejected at load", decl.Type)
	}
	d, err := time.ParseDuration(decl.MaxAge)
	if err != nil {
		return fmt.Errorf("maxAge %q: %w", decl.MaxAge, err)
	}
	if d <= 0 {
		return fmt.Errorf("maxAge %q must be positive", decl.MaxAge)
	}
	ceiling := MaxAgeCeiling(decl)
	if ceiling <= 0 {
		return fmt.Errorf("declaration type %q has no host maxAge ceiling", decl.Type)
	}
	if ceiling > MaxAgeGlobalCap {
		ceiling = MaxAgeGlobalCap
	}
	if d > ceiling {
		return fmt.Errorf(
			"maxAge %q exceeds ceiling %s for type=%q sensitive=%v (rejected at load, never clamped)",
			decl.MaxAge, ceiling, decl.Type, decl.Sensitive)
	}
	return nil
}
