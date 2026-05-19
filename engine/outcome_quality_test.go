package engine

import "testing"

func TestClassifyActionOutcome(t *testing.T) {
	cases := map[string]string{
		"applied_primary":        "primary",
		"applied_fallback":       "fallback",
		"applied_auto_wave":      "auto_corrected",
		"rejected:out_of_bounds": "rejected",
		"weird":                  "unknown",
	}
	for in, want := range cases {
		got := classifyActionOutcome(in)
		if got != want {
			t.Fatalf("classify(%q) = %q, want %q", in, got, want)
		}
	}
}
