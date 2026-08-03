package schemas

// Blank import: enables the //go:embed directives in this package to embed schema bytes.
import _ "embed"

// The provider envelope schemas are promoted unchanged from Spike C
// (hack/spikes/provider/*.schema.json, P2-E3) — same $id, same bytes
// (REQ-P3-E1-S03-01). They live in this package (rather than
// schemas/compiler.go, which S02 owns) so this lane's promotion stays
// isolated to its own files.

//go:embed provider/v1alpha1/request.schema.json
var providerRequestSchemaJSON []byte

//go:embed provider/v1alpha1/response.schema.json
var providerResponseSchemaJSON []byte

var (
	// ProviderRequestSchema validates schemas/provider/v1alpha1/request.schema.json instances (FactQuery).
	ProviderRequestSchema = mustCompile("request.schema.json", providerRequestSchemaJSON)
	// ProviderResponseSchema validates schemas/provider/v1alpha1/response.schema.json instances (FactResponse).
	ProviderResponseSchema = mustCompile("response.schema.json", providerResponseSchemaJSON)
)

// ValidateProviderRequest checks raw JSON against the promoted
// request.schema.json (FactQuery). Exported so hack/spikes/provider's own
// ValidateRequest can delegate here post-promotion (REQ-P3-E1-S03-01).
func ValidateProviderRequest(raw []byte) error {
	return validateJSON(ProviderRequestSchema, string(raw))
}

// ValidateProviderResponse checks raw JSON against the promoted
// response.schema.json (FactResponse).
func ValidateProviderResponse(raw []byte) error {
	return validateJSON(ProviderResponseSchema, string(raw))
}
