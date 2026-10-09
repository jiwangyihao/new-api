package common

import (
	"fmt"
)

// ResolveModelMapping follows exact aliases. baseName supplies the host's
// reasoning-suffix normalization without coupling JSON utilities to settings.
func ResolveModelMapping(name, mapping string, baseName func(string) string) (string, bool, error) {
	if mapping == "" || mapping == "{}" {
		return name, false, nil
	}
	var aliases map[string]string
	if err := UnmarshalJsonStr(mapping, &aliases); err != nil {
		return "", false, fmt.Errorf("unmarshal_model_mapping_failed: %w", err)
	}
	current := name
	seen := make(map[string]struct{}, len(aliases))
	for {
		seen[current] = struct{}{}
		next := aliases[current]
		if next == "" && baseName != nil {
			next = aliases[baseName(current)]
		}
		if next == "" || next == current {
			return current, current != name, nil
		}
		if _, exists := seen[next]; exists {
			return "", false, fmt.Errorf("model_mapping_contains_cycle")
		}
		current = next
	}
}
