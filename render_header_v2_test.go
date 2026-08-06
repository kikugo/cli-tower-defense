package main

import (
	"strings"
	"testing"
)

// --- plausible test fixtures ------------------------------------------

// plausibleHeaderData returns a MatchHeaderData with realistic, fully-known
// values -- the "everything is measured" case. Individual tests mutate a
// copy to exercise the unknown/none-yet branches.
func plausibleHeaderData() MatchHeaderData {
	return MatchHeaderData{
		Defender: PlayerHeaderData{
			ModelName: "o3",
			Resources: 55,
			Income:    5,
			Lives:     7,
			MaxLives:  10,
			Built:     "^3 !2 *1 +1",
			Saves:     SavesStat{Known: true, Authored: 17, Total: 59},
			Authored:  AuthoredShare{Known: true, HasData: true, Share: 0.84},
		},
		Attacker: PlayerHeaderData{
			ModelName: "gpt-4o-mini",
			Resources: 10,
			Income:    5,
			Live:      "o4 f2 t1 s1 h1",
			Sent:      130,
			Saves:     SavesStat{Known: true, Authored: 32, Total: 59},
			Authored:  AuthoredShare{Known: true, HasData: true, Share: 0.91},
		},
		Wave:     12,
		MaxWave:  30,
		Tick:     117,
		MaxTick:  400,
		TurnSide: "ATT",
		Speed:    1.0,
		RunState: "RUN",
		Breached: 3,
		Leak:     LeakStat{Leaked: 3, Window: 8},
	}
}

func plausibleTrustState() TrustState {
	return TrustState{
		AssistKnown:     true,
		AssistsEnabled:  true,
		AssistCount:     2,
		AssistDetail:    "queued 4 enemies, fired 1 ability",
		ProvenanceKnown: true,
	}
}

// noneYetHeaderData is the "match just started" state: every denominator is
// zero and every tally is empty -- trust-160.txt / trust-80.txt STATE 2.
func noneYetHeaderData() MatchHeaderData {
	return MatchHeaderData{
		Defender: PlayerHeaderData{
			ModelName: "o3",
			Resources: 50,
			Income:    5,
			Lives:     10,
			MaxLives:  10,
			Built:     "",
			Saves:     SavesStat{Known: true, Authored: 0, Total: 0},
			Authored:  AuthoredShare{Known: true, HasData: false},
		},
		Attacker: PlayerHeaderData{
			ModelName: "gpt-4o-mini",
			Resources: 50,
			Income:    5,
			Live:      "",
			Sent:      0,
			Saves:     SavesStat{Known: true, Authored: 0, Total: 0},
			Authored:  AuthoredShare{Known: true, HasData: false},
		},
		Wave:     1,
		MaxWave:  30,
		Tick:     1,
		MaxTick:  400,
		TurnSide: "ATT",
		Speed:    1.0,
		RunState: "RUN",
		Breached: 0,
		Leak:     LeakStat{Leaked: 0, Window: 0},
	}
}

func armedNotFiredTrust() TrustState {
	return TrustState{
		AssistKnown:     true,
		AssistsEnabled:  true,
		AssistCount:     0,
		AssistDetail:    "armed at tick 1, has not acted",
		ProvenanceKnown: true,
	}
}

func offTrust() TrustState {
	return TrustState{
		AssistKnown:     true,
		AssistsEnabled:  false,
		AssistDetail:    "every action this match was chosen by a model",
		ProvenanceKnown: true,
	}
}

func unknownTrust() TrustState {
	return TrustState{
		AssistKnown:     false,
		AssistDetail:    "this replay predates assist tracking",
		ProvenanceKnown: false,
	}
}

func firingTrust() TrustState {
	return TrustState{
		AssistKnown:     true,
		AssistsEnabled:  true,
		AssistCount:     9,
		AssistDetail:    "6 unrecorded, started 4 of 5 waves",
		ProvenanceKnown: true,
	}
}

// --- 1. every mode: exact fit across the full size sweep ----------------

// TestRenderHeaderV2FitsEveryMode sweeps every width 60-200 and height
// 15-60 (sweepWidthsV2/sweepHeightsV2, defined in main_layout_v2_test.go)
// and asserts the rendered header is exactly layout.header.h rows of
// exactly layout.header.w display columns via checkFits, for both the
// "everything measured" data and the "match just started" (none-yet) data
// -- the two ends of the content-length spectrum a real header can render.
func TestRenderHeaderV2FitsEveryMode(t *testing.T) {
	cases := []struct {
		name  string
		data  MatchHeaderData
		trust TrustState
	}{
		{"plausible", plausibleHeaderData(), plausibleTrustState()},
		{"none-yet", noneYetHeaderData(), armedNotFiredTrust()},
	}

	for _, tc := range cases {
		for _, w := range sweepWidthsV2() {
			for _, h := range sweepHeightsV2() {
				l := computeLayoutV2(w, h)
				if l.mode == modeNotice {
					continue // no header pane in notice mode
				}
				rows := RenderHeaderV2(l, tc.data, tc.trust)
				frame := strings.Join(rows, "\n")
				if err := checkFits(frame, l.header.w, l.header.h); err != nil {
					t.Fatalf("%s w=%d h=%d mode=%v: %v", tc.name, w, h, l.mode, err)
				}
			}
		}
	}
}

// --- 2. the five absence states ------------------------------------------

// TestTrustBandAssistStatesRenderWordsNeverZero asserts the four
// engine-assist states -- off, on-and-not-fired, on-and-firing, unknown --
// render the words the design specifies, and in particular that
// "ENGINE HELPED 0x" can never appear: an armed-but-idle assist state
// renders as "ENGINE ASSIST ON", not a zero count.
func TestTrustBandAssistStatesRenderWordsNeverZero(t *testing.T) {
	cases := []struct {
		name  string
		trust TrustState
		want  string
	}{
		{"off", offTrust(), "ENGINE ASSIST OFF"},
		{"on-not-fired", armedNotFiredTrust(), "ENGINE ASSIST ON"},
		{"on-firing", firingTrust(), "ENGINE HELPED 9x"},
		{"unknown", unknownTrust(), "ENGINE ASSIST UNKNOWN"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			band := RenderTrustBand(160, boardMaxW, tc.trust)
			if !strings.Contains(band, tc.want) {
				t.Errorf("RenderTrustBand(%s) = %q, want it to contain %q", tc.name, band, tc.want)
			}
			if strings.Contains(band, "HELPED 0x") {
				t.Errorf("RenderTrustBand(%s) = %q contains a zero assist count, which must never render", tc.name, band)
			}
		})
	}

	// A count of exactly zero must never surface as "HELPED 0x" no matter
	// how AssistCount is set, since assistLabel's switch only reaches the
	// "HELPED" branch when count != 0 -- assert that directly too, as a
	// belt-and-braces check independent of the on-not-fired fixture above.
	zero := TrustState{AssistKnown: true, AssistsEnabled: true, AssistCount: 0}
	if got := zero.assistLabel(); strings.Contains(got, "0x") {
		t.Errorf("assistLabel with AssistCount=0 = %q, must not mention a zero count", got)
	}
}

// TestHeaderNoneYetStatesNeverShowZero is the fifth absence state: before
// any decision has resolved, authored share and the leak window have no
// denominator, and must render "none yet" / "unknown", never "0%" or
// "0/0" or "leaked 0 of last 0". This checks both the raw formatting
// helpers and the full RenderHeaderV2 output (wide mode, which shows every
// field) for the none-yet fixture.
func TestHeaderNoneYetStatesNeverShowZero(t *testing.T) {
	if got := fmtAuthored(AuthoredShare{Known: true, HasData: false}); got != "none yet" {
		t.Errorf("fmtAuthored(known, no data) = %q, want %q", got, "none yet")
	}
	if got := fmtAuthored(AuthoredShare{Known: false}); got != "unknown" {
		t.Errorf("fmtAuthored(unknown) = %q, want %q", got, "unknown")
	}
	if got := fmtSaves(SavesStat{Known: true, Total: 0}); got != "none yet" {
		t.Errorf("fmtSaves(known, total 0) = %q, want %q", got, "none yet")
	}
	if got := fmtSaves(SavesStat{Known: false}); got != "unknown" {
		t.Errorf("fmtSaves(unknown) = %q, want %q", got, "unknown")
	}
	if got := fmtLeakLong(LeakStat{Window: 0}); !strings.Contains(got, "none yet") {
		t.Errorf("fmtLeakLong(window 0) = %q, want it to contain %q", got, "none yet")
	}
	if got := fmtLeakShort(LeakStat{Window: 0}); !strings.Contains(got, "none yet") {
		t.Errorf("fmtLeakShort(window 0) = %q, want it to contain %q", got, "none yet")
	}

	l := computeLayoutV2(160, 50) // wide mode: every field appears
	rows := RenderHeaderV2(l, noneYetHeaderData(), armedNotFiredTrust())
	frame := strings.Join(rows, "\n")

	for _, bad := range []string{"authored 0%", "0%  authored", "0/0", "leaked 0 of", "saves 0/0"} {
		if strings.Contains(frame, bad) {
			t.Errorf("none-yet header contains %q, want the absence word instead:\n%s", bad, frame)
		}
	}
	for _, want := range []string{"none yet", "nothing", "ENGINE ASSIST ON"} {
		if !strings.Contains(frame, want) {
			t.Errorf("none-yet header missing %q:\n%s", want, frame)
		}
	}
}

// --- 3. fixture agreement at 100x30 and 80x24 -----------------------------

// TestRenderHeaderV2FixtureAgreement100x30 checks the mid-mode header
// (100x30 selects modeMid: header.h == 3) against testdata/mockups/100x30.txt
// structurally, NOT byte-for-byte: the fixture's model names ("o3",
// "gpt-4o-mini") and numbers are hand-invented for the mockup, and this
// renderer's exact column spacing is its own design (see threeCol), not a
// transcription of the mockup's. What IS asserted, because both the
// fixture and this renderer are expressions of the same design brief:
//   - exactly 3 header rows (computeLayoutV2's mid-mode header height)
//   - each row is exactly 100 display columns (checkFits)
//   - row 1 carries DEFENDER before WAVE before ATTACKER, in that order
//   - row 2 carries the defender's lives/resources before the attacker's
//     breached/resources, with the match-state clause ([ATT]/speed/RUN)
//     in between
//   - row 3 carries "saves"/"authored" on both sides of a central
//     "model-authored" caption
//
// This is deliberately a field-ordering check, not a column-position
// check: the two renderers are free to (and do) choose different padding.
func TestRenderHeaderV2FixtureAgreement100x30(t *testing.T) {
	fixture := mustReadMockup(t, "100x30.txt")[:3]
	if !strings.Contains(fixture[0], "DEFENDER") || !strings.Contains(fixture[0], "WAVE") || !strings.Contains(fixture[0], "ATTACKER") {
		t.Fatalf("fixture assumption broken: 100x30.txt row 1 = %q, expected DEFENDER/WAVE/ATTACKER", fixture[0])
	}

	l := computeLayoutV2(100, 30)
	if l.mode != modeMid {
		t.Fatalf("computeLayoutV2(100,30).mode = %v, want modeMid", l.mode)
	}
	if l.header.h != 3 {
		t.Fatalf("mid-mode header height = %d, want 3 (matches 100x30.txt's own row budget)", l.header.h)
	}

	rows := RenderHeaderV2(l, plausibleHeaderData(), plausibleTrustState())
	if len(rows) != 3 {
		t.Fatalf("RenderHeaderV2 returned %d rows, want 3", len(rows))
	}
	if err := checkFits(strings.Join(rows, "\n"), 100, 3); err != nil {
		t.Fatalf("checkFits: %v", err)
	}

	assertOrder(t, rows[0], "DEFENDER", "WAVE", "ATTACKER")
	assertOrder(t, rows[1], "$55", "[ATT]", "breached")
	// "saves" appears once per side (defender 17/59, attacker 32/59) around
	// a central "model-authored" caption -- assertOrder can't tell two
	// occurrences of the same needle apart, so the two sides are
	// disambiguated by their distinct save counts here.
	assertOrder(t, rows[2], "saves 17/59", "model-authored", "saves 32/59")
}

// TestRenderHeaderV2FixtureAgreement80x24 is TestRenderHeaderV2FixtureAgreement100x30's
// counterpart for the collapsed two-line form (80x24 selects modeMinimum:
// header.h == 2). Same structural-not-literal caveat applies.
func TestRenderHeaderV2FixtureAgreement80x24(t *testing.T) {
	fixture := mustReadMockup(t, "80x24.txt")[:2]
	if !strings.Contains(fixture[0], "DEF") || !strings.Contains(fixture[1], "ATT") {
		t.Fatalf("fixture assumption broken: 80x24.txt rows 1-2 = %q / %q, expected DEF / ATT", fixture[0], fixture[1])
	}

	l := computeLayoutV2(80, 24)
	if l.mode != modeMinimum {
		t.Fatalf("computeLayoutV2(80,24).mode = %v, want modeMinimum", l.mode)
	}
	if l.header.h != 2 {
		t.Fatalf("minimum-mode header height = %d, want 2 (matches 80x24.txt's own row budget)", l.header.h)
	}

	rows := RenderHeaderV2(l, plausibleHeaderData(), plausibleTrustState())
	if len(rows) != 2 {
		t.Fatalf("RenderHeaderV2 returned %d rows, want 2", len(rows))
	}
	if err := checkFits(strings.Join(rows, "\n"), 80, 2); err != nil {
		t.Fatalf("checkFits: %v", err)
	}

	assertOrder(t, rows[0], "DEF", "saves", "authored")
	assertOrder(t, rows[1], "ATT", "breached", "saves", "authored")
}

// assertOrder asserts every needle appears in haystack, and that each
// needle's index is strictly after the previous one's -- i.e. the needles
// appear in the given order, left to right.
func assertOrder(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	pos := -1
	for _, needle := range needles {
		idx := strings.Index(haystack, needle)
		if idx < 0 {
			t.Errorf("row %q missing %q", haystack, needle)
			return
		}
		if idx <= pos {
			t.Errorf("row %q: %q at index %d is not after the previous field (index %d)", haystack, needle, idx, pos)
		}
		pos = idx
	}
}

// --- 4. long model names cannot break the grid ---------------------------

// TestRenderHeaderV2LongModelNameNeverBreaksGrid feeds a 60-character model
// name (far longer than any real model identifier) at a representative
// width in every mode and asserts the header still fits exactly -- the
// degrade path in threeCol/joinFields (truncate center, then right, then
// left, then padCells as a final hard truncation) must hold regardless of
// how long a name gets.
func TestRenderHeaderV2LongModelNameNeverBreaksGrid(t *testing.T) {
	longName := strings.Repeat("x", 60)
	data := plausibleHeaderData()
	data.Defender.ModelName = longName
	data.Attacker.ModelName = longName
	trust := plausibleTrustState()

	widths := []int{60, 70, 80, 84, 90, 100, 144, 145, 160, 200}
	for _, w := range widths {
		l := computeLayoutV2(w, 30)
		if l.mode == modeNotice {
			continue
		}
		rows := RenderHeaderV2(l, data, trust)
		if err := checkFits(strings.Join(rows, "\n"), l.header.w, l.header.h); err != nil {
			t.Errorf("w=%d mode=%v with 60-char model name: %v", w, l.mode, err)
		}
	}
}

// --- RenderTrustBand-specific coverage ------------------------------------

// TestRenderTrustBandSplitsAtBoardWidth asserts the split form places the
// "┬" divider at exactly splitCol, matching trust-160.txt's alignment with
// the vertical rule directly beneath it in wide mode (splitCol == boardMaxW
// == 84), and that both halves plus the divider add up to exactly width.
func TestRenderTrustBandSplitsAtBoardWidth(t *testing.T) {
	band := RenderTrustBand(160, boardMaxW, firingTrust())
	if err := checkFits(band, 160, 1); err != nil {
		t.Fatalf("checkFits: %v", err)
	}
	runes := []rune(band)
	if boardMaxW >= len(runes) || runes[boardMaxW] != '┬' {
		t.Fatalf("RenderTrustBand split at boardMaxW=%d: rune there is %q, want '┬'\nband=%q", boardMaxW, string(runes[boardMaxW]), band)
	}
	if !strings.Contains(band, "provenance measured") {
		t.Errorf("band = %q, want it to contain the provenance clause", band)
	}
}

// TestRenderTrustBandUnsplitFillsWidth asserts splitCol<=0 renders a single
// unsplit dash rule with no "┬" at all, still exactly width columns.
func TestRenderTrustBandUnsplitFillsWidth(t *testing.T) {
	band := RenderTrustBand(80, 0, offTrust())
	if err := checkFits(band, 80, 1); err != nil {
		t.Fatalf("checkFits: %v", err)
	}
	if strings.ContainsRune(band, '┬') {
		t.Errorf("unsplit band = %q, should not contain '┬'", band)
	}
	if !strings.Contains(band, "ENGINE ASSIST OFF") {
		t.Errorf("band = %q, want the assist label", band)
	}
}

// TestRenderTrustBandProvenanceUnknown asserts STATE 4's provenance clause
// ("provenance UNKNOWN", upper case) renders when ProvenanceKnown is false.
func TestRenderTrustBandProvenanceUnknown(t *testing.T) {
	band := RenderTrustBand(160, boardMaxW, unknownTrust())
	if !strings.Contains(band, "provenance UNKNOWN") {
		t.Errorf("band = %q, want it to contain %q", band, "provenance UNKNOWN")
	}
	if !strings.Contains(band, "ENGINE ASSIST UNKNOWN") {
		t.Errorf("band = %q, want it to contain %q", band, "ENGINE ASSIST UNKNOWN")
	}
}

// --- printable samples for manual review ----------------------------------

// TestRenderHeaderV2PrintSamples is not a correctness check (each mode is
// already covered above) -- it exists to print the rendered header at the
// three named reference sizes with plausible data, run with `go test -run
// TestRenderHeaderV2PrintSamples -v`, for a human to read.
func TestRenderHeaderV2PrintSamples(t *testing.T) {
	data := plausibleHeaderData()
	trust := plausibleTrustState()

	for _, size := range []struct{ w, h int }{{160, 50}, {100, 30}, {80, 24}} {
		l := computeLayoutV2(size.w, size.h)
		rows := RenderHeaderV2(l, data, trust)
		t.Logf("--- %dx%d (mode=%v, header %dx%d) ---\n%s", size.w, size.h, l.mode, l.header.w, l.header.h, strings.Join(rows, "\n"))
	}
}
