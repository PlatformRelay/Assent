package render

import "github.com/PlatformRelay/assent/internal/core/policy"

const (
	// VerbosityMinimal omits evaluation details in rendered output (ADR-0016 §1).
	VerbosityMinimal = "minimal"
	// VerbosityStandard is the default renderer detail level (D-089).
	VerbosityStandard = "standard"
	// VerbosityFull shows all evaluation detail blocks.
	VerbosityFull = "full"

	// DefaultLocale is the shipped locale catalog id (D-089, ADR-0016 §5).
	DefaultLocale = "en"
	// DefaultCollapseThreshold hides detail bodies beyond N same-code findings (D-089).
	DefaultCollapseThreshold = 5
)

// DefaultOptions returns D-089 presentation defaults when config omits the block.
func DefaultOptions() Options {
	return Options{
		Verbosity:         VerbosityStandard,
		Emoji:             true,
		CollapseThreshold: DefaultCollapseThreshold,
		Locale:            DefaultLocale,
	}
}

// OptionsForEnvironment resolves render.Options for envName from cfg's optional
// presentation block: global knobs first, then the first matching environment
// override (D-089).
func OptionsForEnvironment(cfg *policy.Config, envName string) Options {
	if cfg == nil || cfg.Presentation == nil {
		return DefaultOptions()
	}
	opts := mergePresentation(DefaultOptions(), cfg.Presentation, nil)
	for i := range cfg.Presentation.Environments {
		ov := &cfg.Presentation.Environments[i]
		if ov.Name == envName {
			opts = mergePresentation(opts, cfg.Presentation, ov)
			break
		}
	}
	return opts
}

func mergePresentation(base Options, global *policy.Presentation, override *policy.PresentationEnvOverride) Options {
	out := base
	if global != nil && override == nil {
		if global.Verbosity != "" {
			out.Verbosity = global.Verbosity
		}
		if global.Emoji != nil {
			out.Emoji = *global.Emoji
		}
		if global.CollapseThreshold != nil {
			out.CollapseThreshold = *global.CollapseThreshold
		}
		if global.Locale != "" {
			out.Locale = global.Locale
		}
		return out
	}
	if override != nil {
		if override.Verbosity != "" {
			out.Verbosity = override.Verbosity
		}
		if override.Emoji != nil {
			out.Emoji = *override.Emoji
		}
		if override.CollapseThreshold != nil {
			out.CollapseThreshold = *override.CollapseThreshold
		}
		if override.Locale != "" {
			out.Locale = override.Locale
		}
	}
	return out
}
