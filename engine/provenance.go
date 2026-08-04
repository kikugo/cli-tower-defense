package engine

// DecisionSource records where a decision object actually came from: a
// genuine model response, or one of the engine's own substitutions. Before
// this file existed, every substitution the engine made on a model's behalf
// (an unparseable response, a provider timeout, an invalid action) was
// recorded byte-identical to a decision the model actually made, so no
// artifact the arena produced could tell a real measurement from a
// substituted one. See ARENA-AUDIT.md A1 / AUDIT-FOLLOWUP.md Task 1.
type DecisionSource string

const (
	// SourceModel is the zero value: a decision object that reached
	// applyDecision without any substitution being stamped on it. Only a
	// genuinely parsed model response takes this path -- takeDecisionSource
	// defaults to it, so nothing has to stamp the good case explicitly.
	SourceModel DecisionSource = "model"
	// SourceParserUnparseable is stamped whenever a parser could not read a
	// value the model was supposed to supply and substituted its own: the
	// terminal fallback in parseTowerResponse / parseEnemyResponse when the
	// response contained no recognisable JSON object, and the inline
	// per-field defaults (tower_type, enemy_type, position) applied when an
	// otherwise-parseable object omitted a required field.
	SourceParserUnparseable DecisionSource = "parser_fallback_unparseable"
	// SourceParserEmpty is stamped when a provider returned an empty
	// response body, before any parsing was attempted.
	SourceParserEmpty DecisionSource = "parser_fallback_empty"
	// SourceProviderFailure is stamped when the HTTP call itself failed --
	// timeout, non-2xx status, decode error -- before any parsing was
	// attempted, or when the turn worker recovered from a panic with no
	// decision at all.
	SourceProviderFailure DecisionSource = "provider_fallback"
	// SourceNormalizerDefault is stamped when normalizeDecision replaced an
	// unknown action, an invalid tower/enemy type, an invalid research tech
	// or attacker ability, or a missing/malformed position with its default.
	SourceNormalizerDefault DecisionSource = "normalizer_default"
	// SourceSkippedForcedSave is stamped when the engine never asked the
	// provider at all because the player's legal action set was exactly
	// {save} -- nothing else was affordable or legal, so there was nothing
	// to decide. This is deliberately not one of the substitution sources
	// above: those all describe the engine papering over a model that was
	// asked and failed to produce something usable, whereas this describes
	// a turn where asking would have been pure overhead (the provider could
	// only ever have echoed "save" back). Keeping it as its own source
	// rather than reusing SourceModel or a substitution source keeps the
	// count auditable in DecisionSources while letting ModelAuthored treat
	// it as neither model-authored nor substituted -- see ModelAuthored in
	// match_result.go, which excludes it from both its numerator and
	// denominator.
	SourceSkippedForcedSave DecisionSource = "skipped_forced_save"
)

// decisionSourceKey is a reserved decision-map key, the same smuggling idiom
// already used by tokenUsageKey: providers and parsers stash provenance on
// the decision map itself so it survives the trip from the async turn
// worker back to applyDecision without changing the DecisionProvider
// interface. It is stripped before a decision is applied.
const decisionSourceKey = "_decision_source"

// markDecisionSource stamps a decision map with its provenance. First
// writer wins: calling it a second time on a map that already carries a tag
// is a no-op, so an earlier and more serious substitution is never
// overwritten by a later, lesser one reacting to the same placeholder.
func markDecisionSource(decision map[string]interface{}, src DecisionSource) {
	if decision == nil {
		return
	}
	if _, exists := decision[decisionSourceKey]; exists {
		return
	}
	decision[decisionSourceKey] = string(src)
}

// takeDecisionSource removes and returns the provenance stamp from a
// decision map, defaulting to SourceModel when absent -- only a genuinely
// parsed model decision reaches this call unstamped.
func takeDecisionSource(decision map[string]interface{}) DecisionSource {
	if decision == nil {
		return SourceModel
	}
	raw, ok := decision[decisionSourceKey]
	if !ok {
		return SourceModel
	}
	delete(decision, decisionSourceKey)
	if s, ok := raw.(string); ok && s != "" {
		return DecisionSource(s)
	}
	return SourceModel
}
