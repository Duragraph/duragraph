package llm

import "strings"

// Registry provides automatic provider detection by model name prefix.
var defaultRegistry = map[string]func() Provider{}

// RegisterProvider registers a factory for a model prefix.
func RegisterProvider(prefix string, factory func() Provider) {
	defaultRegistry[prefix] = factory
}

// ProviderForModel returns a provider matching the model name.
// It matches by longest prefix first (e.g., "claude-" before "cl").
func ProviderForModel(model string) (Provider, bool) {
	var best string
	var factory func() Provider
	for prefix, f := range defaultRegistry {
		if strings.HasPrefix(model, prefix) && len(prefix) > len(best) {
			best = prefix
			factory = f
		}
	}
	if factory == nil {
		return nil, false
	}
	return factory(), true
}
