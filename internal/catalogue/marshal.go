package catalogue

import (
	"bytes"
	"encoding/json"
)

// Marshal renders the catalogue as canonical, indented JSON with a trailing
// newline — the byte-identical-double-run surface (REQ-E3-S07-05) and exactly
// what `assent catalogue` writes to stdout for the docs pipeline. HTML escaping
// is disabled so URLs (docs.url) and CEL-derived text render literally.
func (c Catalogue) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(c); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
