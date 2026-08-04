package render

// Options holds resolved presentation knobs (E8-S02 populates from config).
type Options struct {
	Verbosity         string
	Emoji             bool
	CollapseThreshold int
	Locale            string
}

// RuleMeta holds pack-rule metadata used during message rendering (E8-S07).
type RuleMeta struct {
	Message string
	Docs    RuleDocs
	Debug   []string
}

// RuleDocs holds docs.* metadata from a pack rule.
type RuleDocs struct {
	Summary string
	URL     string
}

// Context is the ephemeral render-time bundle (D-096): resolved Options, CEL
// activation, and pack-rule metadata. It is never serialized and is not a frozen
// contract beside PresentationModel.
type Context struct {
	Options       Options
	Activation    any
	Rules         map[string]RuleMeta
	RiskThreshold int // binding approve threshold for summary score line (E8-S13)
}
