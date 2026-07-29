package change

import (
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// parseHCL is the HCL/tfvars producer for the canonical value tree (E1-S04). It projects a tfvars
// document into the SAME *vnode tree the YAML and JSON producers build, so the format-neutral
// walker diffs tfvars exactly as the other formats and yields the same ChangeSet shape.
//
// Scope — tfvars' LITERAL-ONLY subset (ADR-0003's explicit HCL caveat): full HCL with expressions
// is represented but expression EVALUATION is out of scope for v1. Concretely, only literal
// scalars (strings, numbers, bools, null) and object-constructor nesting are decidable. A
// non-literal expression — an interpolation (`"${var.x}"`), a bare variable/traversal (`var.x`), a
// function call, an operator — is OPAQUE with a reason naming the construct: never silently
// evaluated (which could hide the real value) and never silently dropped (fail-safe, GUIDELINES §2).
// A tuple/list is a vSequence LEAF (opaque when walked) — list walking is E1-S05 territory.
//
// Injective, matching the YAML/JSON producers: a string renders JSON-quoted; a number renders as
// its RAW source literal (sliced from the input bytes, never a reformatted cty value, so `2048`
// stays "2048" and distinct literals never collapse); comparison keys are tag-qualified so a
// scalar KIND change is a detected change. Malformed HCL is opaque. PURE: no clock/env/net/random;
// positions come from the HCL parser's source ranges, so the path double-runs byte-identical.
func parseHCL(data []byte) (*vnode, string) {
	file, diags := hclsyntax.ParseConfig(data, "in.tfvars", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, "does not parse as HCL: " + diags.Error()
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, "HCL body is not a native-syntax body — not a field-diffable shape"
	}
	// A tfvars file must have no blocks (only `name = value` attributes). A block (e.g. a `resource`
	// block from a .tf file) is not a tfvars shape; fail closed rather than silently ignore it.
	if len(body.Blocks) != 0 {
		return nil, fmt.Sprintf("HCL blocks are not tfvars attributes (found %d block(s)) — not decidable", len(body.Blocks))
	}

	fields := make(map[string]*vnode)
	for name, attr := range body.Attributes {
		v, reason := hclExprToNode(attr.Expr, data)
		if reason != "" {
			return nil, reason
		}
		fields[name] = v
	}
	// The document root is the set of top-level attributes: a mapping (matching YAML/JSON roots).
	// Position at the file start (attributes have their own positions).
	return &vnode{kind: vMapping, fields: fields, pos: &Position{Line: 1, Column: 1}}, ""
}

// hclExprToNode projects one HCL expression into a *vnode, decidable only for literals and object
// constructors. `data` is the original source, used to slice raw numeric literals for injective
// rendering.
func hclExprToNode(expr hclsyntax.Expression, data []byte) (*vnode, string) {
	switch e := expr.(type) {
	case *hclsyntax.ObjectConsExpr:
		fields := make(map[string]*vnode)
		for _, item := range e.Items {
			key, ok := hclKeyName(item.KeyExpr)
			if !ok {
				return nil, "non-literal HCL object key — not decidable (E1-S04 is literal-only)"
			}
			if _, dup := fields[key]; dup {
				return nil, fmt.Sprintf("duplicate HCL object key %q — not decidable", key)
			}
			v, reason := hclExprToNode(item.ValueExpr, data)
			if reason != "" {
				return nil, reason
			}
			fields[key] = v
		}
		return &vnode{kind: vMapping, fields: fields, pos: rangePos(expr)}, ""

	case *hclsyntax.TupleConsExpr:
		// A list/tuple is a vSequence leaf: list walking is E1-S05, opaque when reached.
		return &vnode{kind: vSequence, pos: rangePos(expr)}, ""

	case *hclsyntax.LiteralValueExpr:
		return hclScalar(e.Val, expr, data)

	case *hclsyntax.TemplateExpr:
		// A quoted string with no interpolation is a pure string literal (one literal part). Any
		// interpolation makes IsStringLiteral() false -> opaque (never partially evaluate).
		if !e.IsStringLiteral() {
			return nil, fmt.Sprintf("non-literal HCL template/interpolation at %s — not decidable (E1-S04 is literal-only)", rangeStr(expr))
		}
		val, d := e.Value(nil) // pure literal: no EvalContext needed, cannot reference variables
		if d.HasErrors() {
			return nil, "HCL string literal did not evaluate: " + d.Error()
		}
		return hclScalar(val, expr, data)

	default:
		// ScopeTraversalExpr (var.x), FunctionCallExpr, BinaryOpExpr, TemplateWrapExpr ("${x}"),
		// conditional, index, etc.: every non-literal construct is opaque, naming its type so the
		// reason is actionable — never silently evaluated or dropped.
		return nil, fmt.Sprintf("non-literal HCL expression %T at %s — not decidable (E1-S04 is literal-only)", expr, rangeStr(expr))
	}
}

// hclScalar renders a literal cty scalar into a scalar vnode with the injective render/cmpKey
// discipline shared across formats. A number uses its RAW source literal (not a reformatted cty
// value) so distinct literals never collapse. A non-scalar cty value (should not occur for a
// LiteralValueExpr in tfvars) is opaque.
func hclScalar(val cty.Value, expr hclsyntax.Expression, data []byte) (*vnode, string) {
	if val.IsNull() {
		return &vnode{kind: vScalar, render: "null", cmpKey: "z\x00null", pos: rangePos(expr)}, ""
	}
	switch val.Type() {
	case cty.String:
		render := jsonQuote(val.AsString())
		return &vnode{kind: vScalar, render: render, cmpKey: "s\x00" + render, pos: rangePos(expr)}, ""
	case cty.Number:
		lit := rawLiteral(expr, data)
		if lit == "" {
			// Defensive/unreachable: a parsed LiteralValueExpr always has an in-bounds range, so
			// rawLiteral cannot be empty here. Fail CLOSED (opaque) rather than fall back to a
			// reformatted cty value, which would be a lossy render and risk a numeric collapse.
			return nil, fmt.Sprintf("HCL numeric literal at %s has no source text — not decidable", rangeStr(expr))
		}
		return &vnode{kind: vScalar, render: lit, cmpKey: "n\x00" + lit, pos: rangePos(expr)}, ""
	case cty.Bool:
		lit := "false"
		if val.True() {
			lit = "true"
		}
		return &vnode{kind: vScalar, render: lit, cmpKey: "b\x00" + lit, pos: rangePos(expr)}, ""
	default:
		return nil, fmt.Sprintf("unsupported HCL literal type %s — not decidable", val.Type().FriendlyName())
	}
}

// hclKeyName extracts a literal object key name. tfvars object keys are bare identifiers
// (orders-api) or quoted strings ("orders-api"); anything else (a computed key) is not literal.
func hclKeyName(keyExpr hclsyntax.Expression) (string, bool) {
	// An object-cons key is wrapped; unwrap to inspect the real key expression.
	if wrap, ok := keyExpr.(*hclsyntax.ObjectConsKeyExpr); ok {
		keyExpr = wrap.Wrapped
	}
	// Bare identifier key (orders-api) -> a single-step traversal keyword.
	if kw := hcl.ExprAsKeyword(keyExpr); kw != "" {
		return kw, true
	}
	// Quoted string key ("orders-api") -> a pure string literal.
	if v, d := keyExpr.Value(nil); !d.HasErrors() && v.Type() == cty.String && !v.IsNull() {
		return v.AsString(), true
	}
	return "", false
}

// rawLiteral returns the exact source text of an expression's range (used for numeric literals so
// the render is the raw literal, injective). Returns "" if the range is out of bounds.
func rawLiteral(expr hclsyntax.Expression, data []byte) string {
	r := expr.Range()
	if r.Start.Byte < 0 || r.End.Byte > len(data) || r.Start.Byte > r.End.Byte {
		return ""
	}
	return string(data[r.Start.Byte:r.End.Byte])
}

// rangePos is the 1-indexed source position of an expression's start.
func rangePos(expr hclsyntax.Expression) *Position {
	r := expr.Range()
	return &Position{Line: r.Start.Line, Column: r.Start.Column}
}

// rangeStr renders an expression's start position for opaque reasons.
func rangeStr(expr hclsyntax.Expression) string {
	r := expr.Range()
	return fmt.Sprintf("line %d, column %d", r.Start.Line, r.Start.Column)
}
