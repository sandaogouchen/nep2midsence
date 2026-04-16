package fingerprint

import "github.com/sandaogouchen/nep2midsence/internal/types"

// Annotator annotates call chains with migration rules from the fingerprint library
type Annotator struct {
	mapping       map[string]*types.MigrationRule
	customMapping map[string]*types.MigrationRule
}

func NewAnnotator(customMappings map[string]*types.MigrationRule) *Annotator {
	return &Annotator{mapping: NepToMidsceneMapping, customMapping: customMappings}
}

// Annotate marks each nep API call in the chain with its migration rule
func (a *Annotator) Annotate(chains []*types.CallChain) []*types.AnnotatedCall {
	var annotated []*types.AnnotatedCall
	for _, chain := range chains {
		for i, step := range chain.Steps {
			if !step.IsNepAPI {
				continue
			}
			rule := a.findRule(step.Callee)
			if rule != nil {
				chain.Steps[i].MigrationRule = rule
			}
			annotated = append(annotated, &types.AnnotatedCall{
				Step: step,
				Rule: rule,
			})
		}
	}
	return annotated
}

func (a *Annotator) findRule(apiName string) *types.MigrationRule {
	// Check custom mappings first
	if a.customMapping != nil {
		if rule, ok := a.customMapping[apiName]; ok {
			return rule
		}
	}
	// Then check built-in mappings
	if rule, ok := a.mapping[apiName]; ok {
		return rule
	}
	// Try partial match (e.g., "nep.FindElement" matches "page.FindElement")
	for key, rule := range a.mapping {
		parts := splitAPIName(key)
		if len(parts) > 0 && containsSuffix(apiName, parts[len(parts)-1]) {
			return rule
		}
	}
	return nil
}

func splitAPIName(name string) []string {
	// Split "nep.FindElement" into ["nep", "FindElement"]
	result := []string{}
	current := ""
	for _, c := range name {
		if c == '.' {
			if current != "" {
				result = append(result, current)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

func containsSuffix(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}
