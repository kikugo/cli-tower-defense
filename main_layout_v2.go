package main

// This file is the layout half of the terminal UI redesign: it turns a
// terminal size into exact pane rectangles for the six-mode design
// (testdata/mockups/*.txt).
//
// It was written ALONGSIDE a four-mode computeLayout in main_layout.go, which
// the shipped view used until the Phase 4 cutover. That function, its layout
// and layoutMode types, and the hjoin/vstack composition it fed are now
// deleted -- both the live view and the replay inspector run on this one. The
// separate paneRectV2/layoutModeV2/layoutV2 types, originally so the two
// layouts could evolve independently, are simply the layout types now.
//
// --- The specification (taken verbatim from the design) ------------------
//
//	mode     width      columns                                    rows
//	wide     >=145      left 84 . rule 1 . right w-85 (right>=60)   header 4 . body h-5 . keys 1
//	mid      100-144    board 84 . legend gutter 16 . slack         header 3 . board 16 . feed h-20 . keys 1
//	narrow   84-99      framed board 84, feed full width            header 2 . board 16 . feed h-19 . keys 1
//	minimum  80-83      borderless map, all 80 columns, no panning  header 2 . label 1 . map 14 . feed h-18 . keys 1
//	compact  60-79      map panned to w columns                     header 2 . label 1 . map 14 . feed >=3 . keys 1
//	notice   <60 or h<15  unchanged from today                      two-line notice
//
// Wide mode's body (h-5 rows) splits into two side-by-side columns:
//   - left (84 wide): board 16, cards 16, timeline rest
//   - right (w-85 wide): feed rest, gap 1, legend 8 -- but the legend is
//     pinned to the bottom only while there are at least 20 rows left for
//     the feed above it (i.e. body-9 >= 20, so body >= 29); below that the
//     legend disappears entirely (reverts to a transient overlay handled
//     elsewhere) and feed takes the whole column.
//
// --- Height allocation strategy -------------------------------------------
//
// header and keys are small (<=4 and 1 rows respectively) and h is never
// smaller than 15 in any non-notice mode, so both are always paid in full,
// unconditionally, before any content pane is clamped. The remaining "body" budget
// is then allocated top-down through the fixed-size content panes (label,
// map/board, cards) via payRows' clamp-to-remaining, and the LAST pane
// listed in each column ("feed" almost everywhere, "timeline" in wide
// mode's left column) absorbs whatever is left over -- which can be less
// than the design's descriptive floor (e.g. mid mode's "feed floor 4") at
// the extreme low end of valid height (h=15), or even zero. That is a
// deliberate choice: this phase's only hard requirements are that panes
// never go negative or off-frame and that they exactly tile the frame (see
// main_layout_v2_test.go), and clamping-with-a-floor-of-zero is the only
// way to guarantee that at every h>=15. Rendering, once it exists, is free
// to treat a below-floor pane specially; that's not this phase's problem.
//
// One discrepancy worth flagging explicitly: the design's additional-rules
// section says "only the feed absorbs slack", but the wide-mode column
// table itself lists BOTH "timeline rest" (left column) and "feed rest"
// (right column) -- i.e. two independent panes both defined as "whatever's
// left in this column". Verified against testdata/mockups/160x50.txt (see
// main_layout_v2_test.go's TestComputeLayoutV2MockupsAgree), the mockup's
// actual row positions match the "timeline is also a rest-pane" reading,
// not the "only feed grows" reading -- growing h with w held in wide's band
// would grow both timeline and feed. This file follows the column table
// (and the mockup it must agree with) rather than the prose summary; see
// TestComputeLayoutV2FeedIsSlackAbsorber for how that's tested honestly.

// paneRectV2 is a pane's exact position and size in terminal cells: rows
// [y, y+h) and columns [x, x+w). Unlike the old rect type (w,h only -- the
// old design never needed absolute position because every pane was either
// full-width-stacked or a simple two-column hjoin), the new design's wide
// mode has panes nested inside two side-by-side columns that are themselves
// split into stacked panes, so an exact-tiling test needs real coordinates,
// not just sizes.
type paneRectV2 struct {
	x, y, w, h int
}

// area is w*h, clamped to 0 for any degenerate (unused-in-this-mode) rect
// -- a zero-value paneRectV2{} has area 0 and contributes nothing to a
// tiling check, which is what lets layoutV2.rects() below return every
// field unconditionally regardless of mode.
func (r paneRectV2) area() int {
	if r.w <= 0 || r.h <= 0 {
		return 0
	}
	return r.w * r.h
}

// layoutModeV2 names the arrangement chosen for a given terminal size.
// Ordered least-to-most-spacious by width band, so mode comparisons read as a
// plain non-decreasing check on the mode value as w grows (for fixed h).
type layoutModeV2 int

const (
	modeNotice layoutModeV2 = iota
	modeCompact
	modeMinimum
	modeNarrow
	modeMid
	modeWide
)

// layoutV2 is the pure output of computeLayoutV2: the arrangement plus
// every relevant pane's exact rect. Fields not used by the selected mode
// are left at their zero value (paneRectV2{0,0,0,0}), which is safe for the
// tiling check (area 0) and harmless for callers that only read the fields
// documented for their mode.
type layoutV2 struct {
	mode layoutModeV2
	w, h int

	// notice mode only: the entire frame.
	notice paneRectV2

	// common chrome, every non-notice mode.
	header paneRectV2
	keys   paneRectV2

	// minimum & compact only: borderless map plus its title label (there's
	// no border to embed the "SPAWN >> ... CORE" title in, so it gets its
	// own row).
	label   paneRectV2
	mapPane paneRectV2

	// narrow, mid & wide only: the framed board pane (boardMaxW=84 wide:
	// the 80-column map plus a 2-column border and 1 column of padding
	// each side, per boardMaxW's doc comment in main_layout.go).
	board paneRectV2

	// mid only: the 16-wide legend gutter beside the board, and any slack
	// (blank filler) to its right when w exceeds board+legend width.
	// narrow and minimum also use "slack" for their own blank filler
	// (narrow: right of the board; minimum: right of the 80-col map) --
	// only one of those is ever populated per call, since only one mode
	// runs per call.
	legend paneRectV2
	slack  paneRectV2

	// wide only: the two-column body layout.
	rule     paneRectV2 // 1-col vertical divider between the columns
	cards    paneRectV2 // left column, below the board
	timeline paneRectV2 // left column, below cards (rest of the column)
	gap      paneRectV2 // right column, 1 blank row above the legend --
	// present only when the legend is (see the disappearing-legend rule)

	// every non-notice mode.
	feed paneRectV2
}

// rects returns every pane this layout carries, across every mode, for
// exact-tiling and no-negative/off-frame checks. Unused fields for the
// current mode are zero-value and contribute area 0, so this doesn't need
// to branch on l.mode.
func (l layoutV2) rects() []paneRectV2 {
	return []paneRectV2{
		l.notice,
		l.header, l.keys,
		l.label, l.mapPane,
		l.board,
		l.legend, l.slack,
		l.rule, l.cards, l.timeline, l.gap,
		l.feed,
	}
}

// payRows returns min(target, *remaining) (floored at 0) and deducts that
// amount from *remaining. This is the top-down clamped allocator used
// throughout computeLayoutV2: every fixed-target pane is paid via payRows
// in listed order, so a too-small remaining budget shrinks (never negates)
// later panes, and the final pane in a column simply takes what's left
// (*remaining itself, already guaranteed >= 0 by construction).
func payRows(target int, remaining *int) int {
	got := target
	if got > *remaining {
		got = *remaining
	}
	if got < 0 {
		got = 0
	}
	*remaining -= got
	return got
}

// computeLayoutV2 is the pure function that turns a terminal size into the
// new design's arrangement and pane rects. It never reads global/package
// state; every width/height returned is derived from w/h.
//
// Mode is selected purely from w (>=145 wide, 100-144 mid, 84-99 narrow,
// 80-83 minimum, 60-79 compact), except that w<60 or h<15 always reports
// modeNotice regardless of width -- matching the design table exactly, and
// treating an undersized terminal as a special case rather than a width band.
func computeLayoutV2(w, h int) layoutV2 {
	if w < 60 || h < 15 {
		return layoutV2{mode: modeNotice, w: w, h: h, notice: paneRectV2{x: 0, y: 0, w: w, h: h}}
	}

	var mode layoutModeV2
	switch {
	case w >= 145:
		mode = modeWide
	case w >= 100:
		mode = modeMid
	case w >= 84:
		mode = modeNarrow
	case w >= 80:
		mode = modeMinimum
	default: // 60-79
		mode = modeCompact
	}

	l := layoutV2{mode: mode, w: w, h: h}

	switch mode {
	case modeCompact, modeMinimum:
		const headerH, keysH = 2, 1
		l.header = paneRectV2{x: 0, y: 0, w: w, h: headerH}
		y := headerH
		remaining := h - headerH - keysH

		labelH := payRows(1, &remaining)
		l.label = paneRectV2{x: 0, y: y, w: w, h: labelH}
		y += labelH

		mapW := w
		if mode == modeMinimum {
			mapW = 80 // "all 80 columns, no panning"
		}
		mapH := payRows(boardMaxH-2, &remaining) // 14 fixed map rows (boardMaxH minus its 2-row border)
		l.mapPane = paneRectV2{x: 0, y: y, w: mapW, h: mapH}
		if mapW < w {
			l.slack = paneRectV2{x: mapW, y: y, w: w - mapW, h: mapH}
		}
		y += mapH

		feedH := remaining
		l.feed = paneRectV2{x: 0, y: y, w: w, h: feedH}
		y += feedH

		l.keys = paneRectV2{x: 0, y: y, w: w, h: keysH}

	case modeNarrow:
		const headerH, keysH = 2, 1
		l.header = paneRectV2{x: 0, y: 0, w: w, h: headerH}
		y := headerH
		remaining := h - headerH - keysH

		boardW := boardMaxW // 84, fixed regardless of w within this band
		boardH := payRows(boardMaxH, &remaining)
		l.board = paneRectV2{x: 0, y: y, w: boardW, h: boardH}
		if boardW < w {
			l.slack = paneRectV2{x: boardW, y: y, w: w - boardW, h: boardH}
		}
		y += boardH

		feedH := remaining
		l.feed = paneRectV2{x: 0, y: y, w: w, h: feedH}
		y += feedH

		l.keys = paneRectV2{x: 0, y: y, w: w, h: keysH}

	case modeMid:
		const headerH, keysH = 3, 1
		l.header = paneRectV2{x: 0, y: 0, w: w, h: headerH}
		y := headerH
		remaining := h - headerH - keysH

		boardW, legendW := boardMaxW, 16
		boardH := payRows(boardMaxH, &remaining)
		l.board = paneRectV2{x: 0, y: y, w: boardW, h: boardH}
		l.legend = paneRectV2{x: boardW, y: y, w: legendW, h: boardH}
		if slackW := w - boardW - legendW; slackW > 0 {
			l.slack = paneRectV2{x: boardW + legendW, y: y, w: slackW, h: boardH}
		}
		y += boardH

		feedH := remaining
		l.feed = paneRectV2{x: 0, y: y, w: w, h: feedH}
		y += feedH

		l.keys = paneRectV2{x: 0, y: y, w: w, h: keysH}

	case modeWide:
		const headerH, keysH = 4, 1
		l.header = paneRectV2{x: 0, y: 0, w: w, h: headerH}
		y := headerH
		body := h - headerH - keysH // always >= 10 since h >= 15 here

		const leftW, ruleW = boardMaxW, 1
		rightX := leftW + ruleW
		rightW := w - leftW - ruleW // >= 60 guaranteed: mode is wide only at w >= 145

		// Left column: board, cards, timeline -- top-down clamped, timeline
		// (the last-listed pane) absorbs whatever remains.
		leftRemaining := body
		boardH := payRows(boardMaxH, &leftRemaining)
		cardsH := payRows(16, &leftRemaining)
		timelineH := leftRemaining
		l.board = paneRectV2{x: 0, y: y, w: leftW, h: boardH}
		l.cards = paneRectV2{x: 0, y: y + boardH, w: leftW, h: cardsH}
		l.timeline = paneRectV2{x: 0, y: y + boardH + cardsH, w: leftW, h: timelineH}

		l.rule = paneRectV2{x: leftW, y: y, w: ruleW, h: body}

		// Right column: the legend is pinned to the bottom while there are
		// at least 20 rows left for the feed above it (body-9 >= 20); below
		// that threshold it disappears and feed takes the whole column.
		const gapH, legendH = 1, 8
		if body-gapH-legendH >= 20 {
			feedH := body - gapH - legendH
			l.feed = paneRectV2{x: rightX, y: y, w: rightW, h: feedH}
			l.gap = paneRectV2{x: rightX, y: y + feedH, w: rightW, h: gapH}
			l.legend = paneRectV2{x: rightX, y: y + feedH + gapH, w: rightW, h: legendH}
		} else {
			l.feed = paneRectV2{x: rightX, y: y, w: rightW, h: body}
			// gap and legend stay zero-value: absent.
		}

		y += body
		l.keys = paneRectV2{x: 0, y: y, w: w, h: keysH}
	}

	return l
}
