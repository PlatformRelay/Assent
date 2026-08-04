// Package render is the pure presentation layer for assent (P5-E8, ADR-0016).
//
// decision.PresentationModel is the sole frozen render contract — renderers consume
// that type (validated against schemas/decision/v1alpha1/presentation-model.schema.json)
// and must not introduce parallel wire structs. Ephemeral render.Context bundles
// resolved Options, CEL activation, and pack-rule metadata at render time (D-096);
// it is never serialized.
package render
