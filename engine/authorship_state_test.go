package engine

import "testing"

// TestModelAuthoredStateSeparatesUntrackedFromEmpty is the whole reason
// ModelAuthoredState exists: ModelAuthored reports (0, false) for both a
// MatchResult that never recorded provenance and one that recorded it and has
// no decisions yet, and the UI has to render those as different words.
func TestModelAuthoredStateSeparatesUntrackedFromEmpty(t *testing.T) {
	untracked := MatchResult{ProvenanceVersion: 0}
	if _, state := untracked.ModelAuthoredState("p1"); state != AuthorshipUntracked {
		t.Fatalf("ProvenanceVersion 0: got state %v, want AuthorshipUntracked", state)
	}

	// Provenance IS being recorded; this player simply has no countable
	// decisions yet.
	empty := MatchResult{ProvenanceVersion: 3, DecisionSources: map[string]int{}}
	if _, state := empty.ModelAuthoredState("p1"); state != AuthorshipNoDecisions {
		t.Fatalf("tracked but empty: got state %v, want AuthorshipNoDecisions", state)
	}

	// Both must still look identical through the two-state accessor, which is
	// the collapse ModelAuthoredState exists to work around -- if this ever
	// stops holding, the two accessors have drifted.
	if _, ok := untracked.ModelAuthored("p1"); ok {
		t.Fatal("ModelAuthored reported ok for untracked provenance")
	}
	if _, ok := empty.ModelAuthored("p1"); ok {
		t.Fatal("ModelAuthored reported ok for tracked-but-empty provenance")
	}
}

// TestModelAuthoredStateMeasuredZeroIsNotAGap checks the other direction: a
// player whose every decision was substituted has a MEASURED share of 0, and
// must not be reported as having no data.
func TestModelAuthoredStateMeasuredZeroIsNotAGap(t *testing.T) {
	r := MatchResult{
		ProvenanceVersion: 3,
		DecisionSources: map[string]int{
			"p1:" + string(SourceParserUnparseable): 7,
		},
	}
	share, state := r.ModelAuthoredState("p1")
	if state != AuthorshipMeasured {
		t.Fatalf("got state %v, want AuthorshipMeasured", state)
	}
	if share != 0 {
		t.Fatalf("got share %v, want 0", share)
	}
}

// TestModelAuthoredStateAgreesWithModelAuthored checks the two accessors
// never disagree about the share itself, across every shape that matters:
// all-authored, all-substituted, mixed, and a player whose only turns were
// skipped forced saves (which must not count in either direction).
func TestModelAuthoredStateAgreesWithModelAuthored(t *testing.T) {
	cases := []struct {
		name      string
		sources   map[string]int
		wantShare float64
		wantState AuthorshipState
	}{
		{
			name:      "all authored",
			sources:   map[string]int{"p1:" + string(SourceModel): 10},
			wantShare: 1,
			wantState: AuthorshipMeasured,
		},
		{
			name: "half authored",
			sources: map[string]int{
				"p1:" + string(SourceModel):             5,
				"p1:" + string(SourceParserUnparseable): 5,
			},
			wantShare: 0.5,
			wantState: AuthorshipMeasured,
		},
		{
			// Only skipped forced saves: excluded from the denominator, so
			// this is indistinguishable from having no decisions at all --
			// which is exactly right, since the model was never asked.
			name:      "only skipped forced saves",
			sources:   map[string]int{"p1:" + string(SourceSkippedForcedSave): 12},
			wantShare: 0,
			wantState: AuthorshipNoDecisions,
		},
		{
			// Another player's decisions must not leak into this one's.
			name: "other player only",
			sources: map[string]int{
				"p2:" + string(SourceModel): 20,
			},
			wantShare: 0,
			wantState: AuthorshipNoDecisions,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := MatchResult{ProvenanceVersion: 3, DecisionSources: c.sources}

			share, state := r.ModelAuthoredState("p1")
			if state != c.wantState {
				t.Fatalf("state: got %v, want %v", state, c.wantState)
			}
			if share != c.wantShare {
				t.Fatalf("share: got %v, want %v", share, c.wantShare)
			}

			legacyShare, ok := r.ModelAuthored("p1")
			if ok != (c.wantState == AuthorshipMeasured) {
				t.Fatalf("ModelAuthored ok: got %v, want %v", ok, c.wantState == AuthorshipMeasured)
			}
			if legacyShare != share {
				t.Fatalf("accessors disagree on the share: %v vs %v", legacyShare, share)
			}
		})
	}
}
