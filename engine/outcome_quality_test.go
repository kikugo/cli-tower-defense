package engine

import "testing"

func TestClassifyActionOutcome(t *testing.T) {
	cases := map[string]string{
		"applied_primary":        "primary",
		"applied_fallback":       "fallback",
		"applied_auto_wave":      "auto_corrected",
		"rejected:out_of_bounds": "rejected",
		"weird":                  "unknown",
		// A substituted decision must never classify as primary, even when
		// the substitution was itself applied successfully -- the
		// "substituted:" prefix is checked first.
		"substituted:parser_fallback_unparseable:applied_primary": "substituted",
		"substituted:provider_fallback:rejected:out_of_bounds":    "substituted",
	}
	for in, want := range cases {
		got := classifyActionOutcome(in)
		if got != want {
			t.Fatalf("classify(%q) = %q, want %q", in, got, want)
		}
	}
}
