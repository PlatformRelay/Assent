package render

import "github.com/PlatformRelay/assent/internal/render/locale"

// Chrome resolves a fixed renderer string for opts.Locale. Unknown locales fail
// closed to the en catalog and return a non-fatal warning (E8-S03).
func Chrome(opts Options, id string) (string, []locale.Warning) {
	cat, warns := locale.ForLocale(opts.Locale)
	s, ok := cat.Lookup(id)
	if !ok {
		return "", warns
	}
	return s, warns
}

// CatalogFor resolves the full chrome catalog for opts.Locale.
func CatalogFor(opts Options) (locale.Catalog, []locale.Warning) {
	return locale.ForLocale(opts.Locale)
}
