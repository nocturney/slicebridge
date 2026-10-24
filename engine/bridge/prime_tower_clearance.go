package bridge

import "strings"

// primeTowerProfileEnabled consults the resolved, in-memory target profile,
// never the original source printer's tower placement or a fabricated default.
func primeTowerProfileEnabled(settings map[string]any) bool {
	switch raw := settings["enable_prime_tower"].(type) {
	case string:
		return raw == "1" || strings.EqualFold(raw, "true")
	case bool:
		return raw
	case int:
		return raw == 1
	case float64:
		return raw == 1
	default:
		return false
	}
}
