package metrics

import "fmt"

// PredefinedBuilder builds one predefined rule definition from input params.
type PredefinedBuilder func(params map[string]any) (RuleConfig, error)

var predefinedBuilders = map[string]PredefinedBuilder{}

// RegisterPredefinedBuilder registers one predefined rule template builder.
func RegisterPredefinedBuilder(provider, name string, builder PredefinedBuilder) {
	if provider == "" || name == "" || builder == nil {
		return
	}

	predefinedBuilders[fmt.Sprintf("%s/%s", provider, name)] = builder
}

// BuildPredefinedRule builds a predefined rule definition from registry.
func BuildPredefinedRule(provider, name string, params map[string]any) (RuleConfig, error) {
	builder, exists := predefinedBuilders[fmt.Sprintf("%s/%s", provider, name)]
	if !exists {
		return RuleConfig{}, fmt.Errorf("predefined rule %q is not registered for provider %q", name, provider)
	}

	rule, errBuild := builder(params)
	if errBuild != nil {
		return RuleConfig{}, fmt.Errorf("could not build predefined rule %q: %w", name, errBuild)
	}

	return rule, nil
}
