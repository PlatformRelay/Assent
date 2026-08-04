package render

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/PlatformRelay/assent/internal/core/decision"
	"github.com/PlatformRelay/assent/schemas"
)

// Fixture is a typed PresentationModel loaded for render tests and goldens.
// Bytes are decoded strictly against the frozen schema; no rendered output yet.
type Fixture struct {
	Presentation decision.PresentationModel
}

// LoadPresentationModel validates raw JSON against PresentationModelSchema and
// decodes it into decision.PresentationModel. Pure — no filesystem I/O.
func LoadPresentationModel(raw []byte) (Fixture, error) {
	doc, err := jsonNumberDoc(raw)
	if err != nil {
		return Fixture{}, fmt.Errorf("presentation-model: %w", err)
	}
	if err := schemas.PresentationModelSchema.Validate(doc); err != nil {
		return Fixture{}, fmt.Errorf("presentation-model: %w", err)
	}
	var pm decision.PresentationModel
	if err := json.Unmarshal(raw, &pm); err != nil {
		return Fixture{}, fmt.Errorf("presentation-model decode: %w", err)
	}
	return Fixture{Presentation: pm}, nil
}

func jsonNumberDoc(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var doc any
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	return doc, nil
}
