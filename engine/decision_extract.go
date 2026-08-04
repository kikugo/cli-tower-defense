package engine

import (
	"encoding/json"
	"regexp"
)

// extractDecisionJSON finds the first balanced, valid JSON object in a raw
// model response and unmarshals it. It is the single shared implementation
// used by both parseTowerResponse and parseEnemyResponse -- they used to
// each carry their own `regexp.MustCompile(`\{.*\}`)`, which is how they
// came to share the same bug: Go's regexp `.` does not match `\n` without
// the `(?s)` flag, so any pretty-printed or multi-line-fenced JSON (the most
// natural output shape for a model that "thinks" before answering) fell
// through to the fallback path on every call. See ARENA-AUDIT.md / the Bug A
// writeup this fix is tracked against.
//
// The obvious one-line fix, `(?s)\{.*\}`, trades that bug for a worse one:
// greedy across newlines, it spans from the first `{` in the whole response
// to the very last `}`, so prose containing any brace, or a response with
// two JSON objects in it, captures everything in between as one "object"
// that fails to parse (or worse, parses as garbage). Instead this scans for
// the first `{`, walks forward tracking brace depth -- ignoring braces that
// appear inside string literals, and respecting `\` escapes so a literal
// `\"` inside a string does not end it early -- and cuts the candidate at
// the matching close brace. If that candidate is not valid JSON (e.g. the
// response contains prose with a stray, non-JSON `{...}` before the real
// object), the scan resumes from the next `{` rather than giving up, so the
// real object further down the response still gets found.
//
// A last-resort greedy regex match over the whole response is tried only if
// the balanced scan finds no valid object at all, to preserve behaviour on
// any response shape the scan does not anticipate.
func extractDecisionJSON(response string) (map[string]interface{}, bool) {
	for i := 0; i < len(response); i++ {
		if response[i] != '{' {
			continue
		}
		candidate, ok := scanBalancedJSONObject(response[i:])
		if !ok {
			continue
		}
		var decision map[string]interface{}
		if err := json.Unmarshal([]byte(candidate), &decision); err == nil {
			return decision, true
		}
		// Not valid JSON -- e.g. a stray "{word}" in prose ahead of the real
		// object. Keep scanning from the next byte for another candidate.
	}

	// Fallback for response shapes the balanced scan did not find a valid
	// object in at all.
	re := regexp.MustCompile(`(?s)\{.*\}`)
	if match := re.FindString(response); match != "" {
		var decision map[string]interface{}
		if err := json.Unmarshal([]byte(match), &decision); err == nil {
			return decision, true
		}
	}
	return nil, false
}

// scanBalancedJSONObject expects s[0] == '{' and returns the substring from
// that opening brace to its matching closing brace, tracking nesting depth
// and skipping over brace characters that occur inside JSON string
// literals. It does not validate the result is well-formed JSON -- callers
// are expected to attempt json.Unmarshal and treat a failure as "try the
// next candidate".
func scanBalancedJSONObject(s string) (string, bool) {
	depth := 0
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[:i+1], true
			}
		}
	}
	return "", false
}
