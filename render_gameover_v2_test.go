package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

// sampleGameOverData is testdata/mockups/gameover-100x30.txt's own match
// state, so the demo test below can be read against that fixture.
func sampleGameOverData() GameOverData {
	return GameOverData{
		WinnerName: "o3", WinnerRole: "DEFENDER",
		EndedBy: "TICK CAP", EndedDetail: "400 ticks reached",
		Wave: 24, MaxWave: 30,
		Lives: 3, MaxLives: 10,
		DefName: "o3", AttName: "gpt-4o-mini",
		DefScore: 1240, AttScore: 980,
		DefAuthored: AuthoredShare{Known: true, HasData: true, Share: 0.84},
		AttAuthored: AuthoredShare{Known: true, HasData: true, Share: 0.91},
		DefSaves:    SavesStat{Known: true, Authored: 61, Total: 199},
		AttSaves:    SavesStat{Known: true, Authored: 109, Total: 199},
		Assist: TrustState{
			AssistKnown: true, AssistsEnabled: true, AssistCount: 9,
			ProvenanceKnown: true,
		},
		RejectedDef: 38, RejectedDefReason: "unaffordable",
		Cost:    "pricing unset -- unknown",
		Verdict: "TICK CAP, ENGINE-ASSISTED",
	}
}

// TestRenderGameOverCardV2Shape checks the card's own contract: a fixed
// width, a rounded border on all four corners, and one row per body line
// plus the two border rows.
func TestRenderGameOverCardV2Shape(t *testing.T) {
	card := RenderGameOverCardV2(sampleGameOverData())

	if len(card) != len(gameOverBodyLines(sampleGameOverData()))+2 {
		t.Fatalf("card has %d rows", len(card))
	}
	if err := checkFits(strings.Join(card, "\n"), gameOverCardW, len(card)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(card[0], "╭") || !strings.HasSuffix(card[0], "╮") {
		t.Fatalf("top border: %q", card[0])
	}
	last := card[len(card)-1]
	if !strings.HasPrefix(last, "╰") || !strings.HasSuffix(last, "╯") {
		t.Fatalf("bottom border: %q", last)
	}
	for i, row := range card[1 : len(card)-1] {
		if !strings.HasPrefix(row, "│") || !strings.HasSuffix(row, "│") {
			t.Fatalf("body row %d is not framed: %q", i, row)
		}
	}
}

// TestRenderGameOverCardV2NeverInventsACost is the one field this project
// has got wrong before: an unset cost must stay a stated unknown, and the
// caller's own text must pass through verbatim rather than being reformatted
// into a number.
func TestRenderGameOverCardV2NeverInventsACost(t *testing.T) {
	d := sampleGameOverData()
	d.Cost = ""
	out := strings.Join(RenderGameOverCardV2(d), "\n")
	if !strings.Contains(out, "pricing unset -- unknown") {
		t.Fatalf("empty cost did not render as an unknown:\n%s", out)
	}

	d.Cost = "$0.41 measured"
	out = strings.Join(RenderGameOverCardV2(d), "\n")
	if !strings.Contains(out, "$0.41 measured") {
		t.Fatalf("caller-supplied cost was not passed through:\n%s", out)
	}
}

// TestRenderGameOverCardV2Draw checks the no-winner path renders a draw
// rather than an empty WINNER row or a role-less name.
func TestRenderGameOverCardV2Draw(t *testing.T) {
	d := sampleGameOverData()
	d.WinnerRole = ""
	out := strings.Join(RenderGameOverCardV2(d), "\n")
	if !strings.Contains(out, "no winner") || !strings.Contains(out, "DRAW") {
		t.Fatalf("draw did not render as a draw:\n%s", out)
	}
}

// TestRenderGameOverCardV2UnknownsRenderAsWords: an unmeasured share or
// saves ratio on the final card is exactly where a false zero would be most
// damaging, since this is the panel a reader screenshots.
func TestRenderGameOverCardV2UnknownsRenderAsWords(t *testing.T) {
	d := sampleGameOverData()
	d.DefAuthored = AuthoredShare{}
	d.AttAuthored = AuthoredShare{}
	d.DefSaves = SavesStat{}
	d.AttSaves = SavesStat{}
	d.Assist = TrustState{}

	out := strings.Join(RenderGameOverCardV2(d), "\n")
	if strings.Count(out, "unknown") < 3 {
		t.Fatalf("unmeasured figures did not all render as unknown:\n%s", out)
	}
	if strings.Contains(out, "DEF 0%") || strings.Contains(out, "ATT 0%") {
		t.Fatalf("unmeasured share rendered as 0%%:\n%s", out)
	}
	if !strings.Contains(out, "ENGINE ASSIST UNKNOWN") {
		t.Fatalf("untracked assists did not render as unknown:\n%s", out)
	}
}

func TestCommaInt(t *testing.T) {
	cases := map[int]string{
		0: "0", 7: "7", 999: "999", 1000: "1,000", 1240: "1,240",
		980: "980", 1234567: "1,234,567", -1240: "-1,240",
	}
	for in, want := range cases {
		if got := commaInt(in); got != want {
			t.Fatalf("commaInt(%d) = %q, want %q", in, got, want)
		}
	}
}

// --- the compositor --------------------------------------------------------

// TestDropCellsV2DropsExactlyNColumns is dropCellsV2's basic contract on
// plain text, including the boundaries (drop nothing, drop everything, drop
// more than there is).
func TestDropCellsV2DropsExactlyNColumns(t *testing.T) {
	const s = "abcdefghij"
	cases := map[int]string{0: s, 1: "bcdefghij", 5: "fghij", 10: "", 20: ""}
	for n, want := range cases {
		if got := dropCellsV2(s, n); got != want {
			t.Fatalf("dropCellsV2(%q, %d) = %q, want %q", s, n, got, want)
		}
	}
}

// reverseX is a reverse-video "X" written as a literal escape sequence
// rather than via lipgloss.NewStyle().Reverse(true).Render("X").
//
// That distinction is the whole reason this constant exists. lipgloss picks
// a colour profile from the OUTPUT DEVICE, and a test binary has no TTY, so
// Render() returns the bare string with no escapes at all. A test that
// built its fixture that way would be asserting ANSI-safety against a
// string containing no ANSI -- it would pass no matter how badly
// dropCellsV2 mangled escape sequences. The bytes below are what
// render_board_v2.go's styleGridRowV2 actually emits when the program runs
// on a real terminal, which is the input that has to survive the cut.
const reverseX = "\x1b[7mX\x1b[0m"

// TestDropCellsV2PreservesStyling is the test the top-of-file comment
// promises: cutting through a styled string must keep the styling that was
// opened before the cut, so the surviving text is still rendered the way it
// was, and must never emit a partial escape sequence.
func TestDropCellsV2PreservesStyling(t *testing.T) {
	// "abc" + reverse("X") + "defg" -- the exact shape render_board_v2.go
	// produces for a board row carrying a breach marker.
	styled := "abc" + reverseX + "defg"
	if lipgloss.Width(styled) != 8 {
		t.Fatalf("fixture is %d columns wide, want 8 -- the escapes are being counted", lipgloss.Width(styled))
	}

	// Cutting after the styled glyph keeps the plain tail.
	got := dropCellsV2(styled, 5)
	if lipgloss.Width(got) != 3 {
		t.Fatalf("dropCellsV2(styled, 5) has width %d, want 3: %q", lipgloss.Width(got), got)
	}
	if !strings.HasSuffix(got, "efg") {
		t.Fatalf("dropCellsV2(styled, 5) lost the tail: %q", got)
	}

	// Cutting BEFORE the styled glyph must keep the glyph styled: the cut
	// falls inside "abc", so both of the marker's sequences are still to
	// the right of it and must survive intact.
	got = dropCellsV2(styled, 2)
	if lipgloss.Width(got) != 6 {
		t.Fatalf("dropCellsV2(styled, 2) has width %d, want 6: %q", lipgloss.Width(got), got)
	}
	if !strings.Contains(got, reverseX) {
		t.Fatalf("cut before a styled glyph lost its styling: %q", got)
	}

	// Cutting THROUGH the styled glyph must re-emit the sequences that were
	// opened in the dropped region, so the tail is not left with a dangling
	// style -- the case that would corrupt the rest of the terminal line.
	got = dropCellsV2(styled, 4)
	if lipgloss.Width(got) != 4 {
		t.Fatalf("dropCellsV2(styled, 4) has width %d, want 4: %q", lipgloss.Width(got), got)
	}
	if !strings.Contains(got, "\x1b[0m") {
		t.Fatalf("cut through a styled glyph dropped its reset: %q", got)
	}
}

// TestOverlayCenteredV2Contract checks the composite's shape: the row count
// and every row's display width are unchanged, whatever the card's size.
func TestOverlayCenteredV2Contract(t *testing.T) {
	base := make([]string, 30)
	for i := range base {
		base[i] = strings.Repeat(".", 100)
	}
	card := RenderGameOverCardV2(sampleGameOverData())

	for _, width := range []int{100, 84, 60, 46, 30} {
		out := OverlayCenteredV2(base, card, width)
		if len(out) != len(base) {
			t.Fatalf("width=%d: got %d rows, want %d", width, len(out), len(base))
		}
		if err := checkFits(strings.Join(out, "\n"), width, len(base)); err != nil {
			t.Fatalf("width=%d: %v", width, err)
		}
	}
}

// TestOverlayCenteredV2KeepsBoardBesideTheCard is the property that
// justifies dropCellsV2 existing at all: content to the RIGHT of the card
// must survive the composite. An implementation that truncated the row at
// the card's right edge would pass every width check above and still be
// wrong.
func TestOverlayCenteredV2KeepsBoardBesideTheCard(t *testing.T) {
	base := make([]string, 20)
	for i := range base {
		base[i] = strings.Repeat("<", 40) + strings.Repeat(">", 60)
	}
	card := []string{"╭────╮", "│ hi │", "╰────╯"}

	out := OverlayCenteredV2(base, card, 100)

	// Find the card's text row rather than assuming which base row it lands
	// on: the card is centred, so its middle row is at
	// (len(base)-len(card))/2 + 1, not at len(base)/2.
	row := ""
	for _, r := range out {
		if strings.Contains(r, "hi") {
			row = r
			break
		}
	}
	if row == "" {
		t.Fatalf("card was not composited onto any row:\n%s", strings.Join(out, "\n"))
	}
	if !strings.Contains(row, "<") {
		t.Fatalf("board content left of the card was lost: %q", row)
	}
	if !strings.Contains(row, ">") {
		t.Fatalf("board content right of the card was lost: %q", row)
	}
}

// TestOverlayCenteredV2PreservesStyledGlyphsBesideTheCard is the same
// property with the one ANSI sequence this codebase actually emits: a
// reverse-video breach marker sitting to the right of the card must still
// be reverse-video afterwards.
func TestOverlayCenteredV2PreservesStyledGlyphsBesideTheCard(t *testing.T) {
	base := []string{
		strings.Repeat(".", 60) + reverseX + strings.Repeat(".", 39),
		strings.Repeat(".", 100),
		strings.Repeat(".", 100),
	}
	card := []string{"╭──╮", "│▓▓│", "╰──╯"}

	out := OverlayCenteredV2(base, card, 100)
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, reverseX) {
		t.Fatalf("the reverse-video breach marker did not survive the composite:\n%q", joined)
	}
	// checkFits/frameDisplayWidth (mockup_fit_test.go) were built for the
	// plain-text mockup fixtures and count every rune, escapes included, so
	// they cannot measure a row carrying ANSI. Rows with escapes are
	// measured with lipgloss.Width instead -- the same split
	// board_viewport_test.go already uses for this project's other
	// ANSI-bearing rendered rows.
	for i, row := range out {
		if w := lipgloss.Width(row); w != 100 {
			t.Fatalf("row %d is %d printable columns wide, want 100: %q", i, w, row)
		}
	}
}

// TestOverlayCenteredV2DegenerateInputs covers the empty and oversized
// cases: an empty card leaves the base alone (padded), and a card taller
// than the frame is clipped rather than growing it.
func TestOverlayCenteredV2DegenerateInputs(t *testing.T) {
	base := []string{"aaa", "bbb", "ccc"}

	out := OverlayCenteredV2(base, nil, 5)
	if len(out) != 3 || lipgloss.Width(out[0]) != 5 {
		t.Fatalf("empty card: %q", out)
	}

	tall := make([]string, 20)
	for i := range tall {
		tall[i] = "####"
	}
	out = OverlayCenteredV2(base, tall, 10)
	if len(out) != 3 {
		t.Fatalf("oversized card grew the frame to %d rows", len(out))
	}
	if err := checkFits(strings.Join(out, "\n"), 10, 3); err != nil {
		t.Fatal(err)
	}
}

// TestRenderGameOverCardV2Demo prints the card, and the card composited
// over a placeholder board, for visual review against
// testdata/mockups/gameover-100x30.txt lines 6-18.
func TestRenderGameOverCardV2Demo(t *testing.T) {
	card := RenderGameOverCardV2(sampleGameOverData())
	t.Logf("game-over card:\n%s", strings.Join(card, "\n"))

	base := make([]string, 16)
	for i := range base {
		base[i] = strings.Repeat("·", boardMaxW)
	}
	t.Logf("composited over a %d-column board:\n%s",
		boardMaxW, strings.Join(OverlayCenteredV2(base, card, boardMaxW), "\n"))
}
