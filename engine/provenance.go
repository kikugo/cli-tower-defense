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
	// SourceParserFallbackTruncated is stamped when the Gemini API reports
	// finishReason "MAX_TOKENS" for a response: the model's output was cut
	// off by the completion token budget before it finished, most often
	// because a thinking model spent its budget on hidden reasoning before
	// it ever got to write the JSON decision (see defaultCompletionTokens in
	// provider_runtime.go). Without this tag, a truncated response that
	// failed to parse landed on the generic SourceParserUnparseable, which
	// is indistinguishable from a model simply writing malformed JSON on its
	// own -- exactly the kind of silent ambiguity provenance tracking exists
	// to eliminate. The Gemini provider applies this via overrideDecisionSource
	// on MAX_TOKENS, replacing whatever fallback tag the parser already
	// applied, because "the budget cut this response off" is a strictly more
	// useful diagnosis than "this did not parse" once it is known to be true.
	// It deliberately does not follow the first-writer-wins convention used
	// everywhere else in this file -- but it also never overwrites a decision
	// that parsed cleanly (SourceModel): see overrideDecisionSource. A model
	// can finish its JSON object and then keep generating trailing prose
	// until it hits the token cap, so MAX_TOKENS alone does not imply the
	// decision was truncated; only an unparsed/fallback decision is relabelled.
	// A response that genuinely was cut off mid-object is still not
	// model-authored (the model never got to finish), so ModelAuthored counts
	// it against the authored share exactly like the other substitution
	// sources: it is anything other than SourceModel.
	SourceParserFallbackTruncated DecisionSource = "parser_fallback_truncated"
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

// overrideDecisionSource replaces an existing fallback tag with a more
// specific one -- e.g. an already-applied SourceParserUnparseable becoming
// SourceParserFallbackTruncated once the caller learns the real cause
// (finishReason MAX_TOKENS). This is the one deliberate exception to the
// first-writer-wins convention markDecisionSource enforces: it exists for
// callers who learn a substitution's true cause only after the parser has
// already run and stamped a generic tag.
//
// It will never downgrade a genuine model decision: if decision is
// currently untagged or already stamped SourceModel, this is a no-op. That
// guarantee lives here, in the one place all callers share, rather than
// being each call site's responsibility -- a response can finish a
// complete, valid JSON object and then keep generating trailing prose until
// it hits a token cap, so a signal like "finishReason was MAX_TOKENS" is
// not on its own evidence that the recorded decision was truncated. Only a
// decision that is already tagged with some OTHER fallback source (proof
// the parser itself considered this response a substitution) is eligible
// to be relabelled.
func overrideDecisionSource(decision map[string]interface{}, src DecisionSource) {
	if decision == nil {
		return
	}
	if peekDecisionSource(decision) == SourceModel {
		return
	}
	decision[decisionSourceKey] = string(src)
}

// peekDecisionSource reads a decision map's current provenance stamp
// without removing it (unlike takeDecisionSource, which is destructive),
// defaulting to SourceModel when no tag is present.
func peekDecisionSource(decision map[string]interface{}) DecisionSource {
	if decision == nil {
		return SourceModel
	}
	raw, ok := decision[decisionSourceKey]
	if !ok {
		return SourceModel
	}
	if s, ok := raw.(string); ok && s != "" {
		return DecisionSource(s)
	}
	return SourceModel
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
