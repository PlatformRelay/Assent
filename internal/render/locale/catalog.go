// Package locale ships fixed renderer chrome strings keyed by stable ids (ADR-0016 §5).
package locale

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

// DefaultLocale is the only shipped catalog id (D-089, ADR-0016 §5).
const DefaultLocale = "en"

// CodeUnknownLocale is emitted when presentation.locale is not shipped; lookup
// fails closed to DefaultLocale.
const CodeUnknownLocale = "unknown-locale"

// Warning is a non-fatal presentation diagnostic (E8-S03: unknown locale).
type Warning struct {
	Code    string
	Message string
}

// Catalog holds resolved chrome strings for one effective locale.
type Catalog struct {
	locale  string
	strings map[string]string
}

// Lookup returns the chrome string for id and whether it exists.
func (c Catalog) Lookup(id string) (string, bool) {
	s, ok := c.strings[id]
	return s, ok
}

// Locale returns the effective catalog locale (after fallback).
func (c Catalog) Locale() string {
	return c.locale
}

//go:embed en.yaml
var enYAML []byte

var enCatalog = mustLoadCatalog(DefaultLocale, enYAML)

func mustLoadCatalog(locale string, raw []byte) Catalog {
	var m map[string]string
	if err := yaml.Unmarshal(raw, &m); err != nil {
		panic(fmt.Sprintf("locale: parse %s catalog: %v", locale, err))
	}
	return Catalog{locale: locale, strings: m}
}

// ForLocale resolves the chrome catalog for locale. Unknown locales fail closed
// to DefaultLocale and emit a Warning (never panic).
func ForLocale(locale string) (Catalog, []Warning) {
	if locale == "" || locale == DefaultLocale {
		return enCatalog, nil
	}
	return enCatalog, []Warning{{
		Code: CodeUnknownLocale,
		Message: fmt.Sprintf(
			"presentation locale %q is not shipped; using %q catalog",
			locale, DefaultLocale,
		),
	}}
}
