package schemas

// Blank import: enables the //go:embed directive below to embed schema bytes.
import _ "embed"

//go:embed approval/v1alpha1/approval-evidence.schema.json
var approvalEvidenceSchemaJSON []byte

const approvalEvidenceSchemaID = "https://assent.dev/schemas/approval/v1alpha1/approval-evidence.schema.json"

// ApprovalEvidenceSchema validates approval-evidence.schema.json instances.
// Compiled with DecisionRecord in the same compiler so the cross-file pins
// $ref resolves (roast P1-B — one pins shape only).
var ApprovalEvidenceSchema = mustCompileCrossReferenced(map[string][]byte{
	decisionRecordSchemaID:   decisionRecordSchemaJSON,
	approvalEvidenceSchemaID: approvalEvidenceSchemaJSON,
})[approvalEvidenceSchemaID]

// ValidateApprovalEvidence checks raw JSON against approval-evidence.schema.json.
func ValidateApprovalEvidence(raw []byte) error {
	return validateJSON(ApprovalEvidenceSchema, string(raw))
}
