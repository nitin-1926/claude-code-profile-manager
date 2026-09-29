//go:build darwin

package rail

import (
	"math"
	"testing"
)

var allEdges = []Edge{EdgeRight, EdgeLeft, EdgeTop, EdgeBottom}

// A 16" MacBook Pro's logical frame and hardware notch — the machine this is
// developed on, and the notch the top edge joins to.
var (
	screen16 = Rect{X: 0, Y: 0, W: 1512, H: 982}
	notch16  = Rect{W: 185, H: 32}
)

// specs enumerates every combination worth checking: each edge, a range of
// profile counts, the top edge both with and without a hardware notch, and
// all of it with the percentages both shown and hidden.
func specs() []Spec {
	var out []Spec
	for _, hide := range []bool{false, true} {
		for _, e := range allEdges {
			for n := 1; n <= 6; n++ {
				out = append(out, Spec{Edge: e, Profiles: n, HidePercent: hide})
				if e == EdgeTop {
					out = append(out, Spec{Edge: e, Profiles: n, Hardware: notch16, HidePercent: hide})
				}
			}
		}
	}
	return out
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.01 }

// containsRect reports whether outer fully contains inner, with a hair of
// tolerance for the panel rounding.
func containsRect(outer, inner Rect) bool {
	const tol = 0.01
	return inner.X >= outer.X-tol && inner.Y >= outer.Y-tol &&
		inner.X+inner.W <= outer.X+outer.W+tol && inner.Y+inner.H <= outer.Y+outer.H+tol
}

func bounds(p Rect) Rect { return Rect{W: p.W, H: p.H} }

// TestPanelDoesNotDependOnExpansion is the premise of notch.go. PanelRect takes
// no expanded argument, so the compiler already enforces it; the test exists so
// adding one is a deliberate act with a failing test attached, not a quiet
// reintroduction of the resize that made the first attempt unusable.
func TestPanelDoesNotDependOnExpansion(t *testing.T) {
	for _, s := range specs() {
		p := PanelRect(screen16, s)
		if p != PanelRect(screen16, s) {
			t.Errorf("%+v: PanelRect is not deterministic", s)
		}
		if NotchRect(p, s, false) == NotchRect(p, s, true) {
			t.Errorf("%+v: folded and open shapes are identical", s)
		}
	}
}

// TestWakeIsContainedInExpanded is THE invariant. If the folded hit region can
// reach outside the open shape, a pointer in that sliver opens the notch and is
// immediately outside it — which schedules a fold, which re-arms the wake band:
// the stutter that parked the previous implementation.
func TestWakeIsContainedInExpanded(t *testing.T) {
	for _, s := range specs() {
		p := PanelRect(screen16, s)
		wake, open := WakeRect(p, s), NotchRect(p, s, true)
		if wake.W <= 0 || wake.H <= 0 {
			t.Errorf("%+v: empty wake region", s)
			continue
		}
		if !containsRect(open, wake) {
			t.Errorf("%+v: wake %+v escapes the open shape %+v", s, wake, open)
		}
	}
}

// TestExpandedContainsCollapsed is the assumption WakeRect's clamp rests on: if
// the pill could stick out of the open shape, the clamp would cut away part of
// a visible target. Joined to a hardware notch it holds side by side: each
// side opens longer than the pill it folds to.
func TestExpandedContainsCollapsed(t *testing.T) {
	for _, s := range specs() {
		p := PanelRect(screen16, s)
		if !containsRect(NotchRect(p, s, true), NotchRect(p, s, false)) {
			t.Errorf("%+v: folded %+v is not inside open %+v", s, NotchRect(p, s, false), NotchRect(p, s, true))
		}
	}
}

// TestFoldedAndOpenShareACentreLine keeps folding from sliding the notch along
// the bezel, away from the pointer that opened it.
func TestFoldedAndOpenShareACentreLine(t *testing.T) {
	for _, s := range specs() {
		// Joined with one ring the reading's side is shorter than the ring's,
		// so the open shape is pinned to the hole's walls rather than to a
		// centre line — TestJoinedNotchWidensTheHardwareNotch holds it there.
		if s.Flush() && s.n() == 1 {
			continue
		}
		p := PanelRect(screen16, s)
		c, o := NotchRect(p, s, false), NotchRect(p, s, true)
		cm, om := c.X+c.W/2, o.X+o.W/2
		if s.Edge.Vertical() {
			cm, om = c.Y+c.H/2, o.Y+o.H/2
		}
		if math.Abs(cm-om) > 0.5 {
			t.Errorf("%+v: centre lines differ by %.2f", s, cm-om)
		}
	}
}

// TestShapeIsFlushWithTheBezel catches the seam: a shape that stops short of
// the panel's outer face leaves a hairline of wallpaper, and the notch reads as
// a floating window.
func TestShapeIsFlushWithTheBezel(t *testing.T) {
	for _, s := range specs() {
		p := PanelRect(screen16, s)
		for _, open := range []bool{false, true} {
			r := NotchRect(p, s, open)
			var got, want float64
			switch s.Edge {
			case EdgeRight:
				got, want = r.X+r.W, p.W
			case EdgeLeft:
				got, want = r.X, 0
			case EdgeTop:
				got, want = r.Y+r.H, p.H
			default:
				got, want = r.Y, 0
			}
			if !near(got, want) {
				t.Errorf("%+v open=%v: shape face %.2f, bezel %.2f", s, open, got, want)
			}
		}
	}
}

// TestShapeParamsFollowTheReference pins the clamp order and the reference's
// numbers. The order matters: the corner claims its radius first out of half
// the depth and the shoulder takes what is left. The reverse — the obvious
// reading — gives the 10pt pill square corners.
func TestShapeParamsFollowTheReference(t *testing.T) {
	pill := Spec{Edge: EdgeRight, Profiles: 3}.Shape(false)
	if !near(pill.Corner, PillWidth/2) || !near(pill.Curl, PillWidth/2) {
		t.Errorf("folded pill: corner %.2f curl %.2f, want both %.2f (round ends, not square)",
			pill.Corner, pill.Curl, PillWidth/2)
	}

	open := Spec{Edge: EdgeRight, Profiles: 3}.Shape(true)
	if !near(open.Corner, CornerRadius) || !near(open.Curl, CurlRadius) {
		t.Errorf("open: corner %.2f curl %.2f, want %.2f and %.2f", open.Corner, open.Curl, CornerRadius, CurlRadius)
	}

	// Joined to a hardware notch: the far ends are the notch's own curve at the
	// joined scale — 32 / 97.06 = 0.3297, which lands the design depth on the
	// hole's — in BOTH states, so folding changes the length and nothing else.
	// 29.63 x 0.3297 = 9.77 and 38.74 x 0.3297 = 12.77.
	j := Spec{Edge: EdgeTop, Profiles: 3, Hardware: notch16}
	for _, open := range []bool{false, true} {
		sp := j.Shape(open)
		if !near(sp.Corner, 9.77) || !near(sp.Curl, 12.77) {
			t.Errorf("joined open=%v: corner %.2f curl %.2f, want 9.77 and 12.77", open, sp.Corner, sp.Curl)
		}
	}
}

// TestLayoutMatchesTheReference checks the arithmetic against numbers worked by
// hand from codenotch's NotchLayout, so a transcription slip in a constant or a
// formula shows up as a wrong number rather than as a notch that looks "off".
func TestLayoutMatchesTheReference(t *testing.T) {
	s := Spec{Edge: EdgeRight, Profiles: 3}
	// padTop + 3*(44 + 10.116 + 17) + 2*31.402 + padBottom
	body := PadTop + 3*(RingDiameter+RingLabelGap+PercentLineHeight) + 2*CellSpacing + PadBottom
	if !near(s.ExpandedLength(), body+2*CurlRadius) {
		t.Errorf("open length %.2f, want %.2f", s.ExpandedLength(), body+2*CurlRadius)
	}
	if !near(body, 321.13) {
		t.Errorf("body length %.2f, want 321.13 from the reference formula", body)
	}
	if !near(s.ExpandedDepth(), SideBodyDepth) || !near(SideBodyDepth, 69.95) {
		t.Errorf("open depth %.2f, want 69.95", s.ExpandedDepth())
	}

	// Ring centres: flare + padStart + half a ring, then one pitch apart.
	p := PanelRect(screen16, s)
	open := NotchRect(p, s, true)
	pitch := RingDiameter + RingLabelGap + PercentLineHeight + CellSpacing
	for i, c := range Cells(p, s) {
		// Vertical edges read from the TOP, so "along" is measured downward.
		along := (open.Y + open.H) - (c.Ring.Y + c.Ring.H/2)
		want := CurlRadius + PadTop + RingDiameter/2 + float64(i)*pitch
		if !near(along, want) {
			t.Errorf("ring %d centre %.2f from the shape start, want %.2f", i, along, want)
		}
		// Centred in the body's depth, with the reference's margin each side.
		margin := (SideBodyDepth - RingDiameter) / 2
		if !near(c.Ring.X-open.X, margin) || !near(c.Ring.W, RingDiameter) {
			t.Errorf("ring %d not centred in the body: x %.2f, want margin %.2f", i, c.Ring.X-open.X, margin)
		}
	}
}

// TestJoinedNotchWidensTheHardwareNotch pins codenotch 1.19's join: the notch
// is the Mac's own notch widened on both sides, at the hole's exact depth,
// each side tucked CutoutOverlap into the hole, and the rings clear of it.
// Reference numbers are worked by hand from the design constants.
func TestJoinedNotchWidensTheHardwareNotch(t *testing.T) {
	for n := 1; n <= 6; n++ {
		s := Spec{Edge: EdgeTop, Profiles: n, Hardware: notch16}
		p := PanelRect(screen16, s)
		holeX := (p.W - notch16.W) / 2
		open, folded := NotchRect(p, s, true), NotchRect(p, s, false)
		if !near(open.H, notch16.H) || !near(folded.H, notch16.H) {
			t.Errorf("n=%d: depth open %.2f folded %.2f, want the hole's %.0f in both", n, open.H, folded.H, notch16.H)
		}
		if !near(folded.X+folded.W/2, holeX+notch16.W/2) {
			t.Errorf("n=%d: folded shape is not centred on the hole", n)
		}
		if n > 1 && !near(open.X+open.W/2, holeX+notch16.W/2) {
			t.Errorf("n=%d: open pair is not symmetric about the hole", n)
		}
		if open.X >= holeX || open.X+open.W <= holeX+notch16.W {
			t.Errorf("n=%d: open %+v does not reach out of both sides of the hole at %.1f", n, open, holeX)
		}
		for i, c := range Cells(p, s) {
			if c.Ring.X < holeX+notch16.W {
				t.Errorf("n=%d: ring %d at %.1f is behind the hole, which ends at %.1f", n, i, c.Ring.X, holeX+notch16.W)
			}
			if c.Across != (n == 1) {
				t.Errorf("n=%d: ring %d across=%v", n, i, c.Across)
			}
		}
		if w := WakeRect(p, s); !containsRect(w, folded) || !containsRect(folded, w) {
			t.Errorf("n=%d: joined wake %+v should be exactly the folded shape %+v — a band around it opens the notch from window title bars", n, w, folded)
		}
	}

	// Three rings: scale 32 / 97.06 = 0.3297, so 14.51pt rings, and folded
	// the hole carries 78.97 x 0.3297 = 26.04pt out of each side: 237.07.
	s3 := Spec{Edge: EdgeTop, Profiles: 3, Hardware: notch16}
	if r := Cells(PanelRect(screen16, s3), s3)[0].Ring; !near(r.W, 14.51) {
		t.Errorf("three rings: ring %.2fpt, want 14.51", r.W)
	}
	if !near(s3.CollapsedLength(), 237.07) {
		t.Errorf("three rings folded: %.2f, want 237.07", s3.CollapsedLength())
	}

	// One ring: its percentage crosses the hole, so the depth holds the ring
	// alone (69.95 design, scale 0.4575, a 20.13pt ring), and the other side is
	// only as long as the reading: 12 + 0.4575 x (2 x 12.97 + 69.31 + 38.74)
	// = 73.30, where the ring's side is 88.15.
	s1 := Spec{Edge: EdgeTop, Profiles: 1, Hardware: notch16}
	p1 := PanelRect(screen16, s1)
	c := Cells(p1, s1)[0]
	if !near(c.Ring.W, 20.13) {
		t.Errorf("one ring: ring %.2fpt, want 20.13", c.Ring.W)
	}
	hole1 := (p1.W - notch16.W) / 2
	open1 := NotchRect(p1, s1, true)
	if other := hole1 + CutoutOverlap - open1.X; !near(other, 73.30) {
		t.Errorf("one ring: reading side %.2fpt long, want 73.30", other)
	}
	if carry := open1.X + open1.W - (hole1 + notch16.W - CutoutOverlap); !near(carry, 88.15) {
		t.Errorf("one ring: ring side %.2fpt long, want 88.15", carry)
	}
	if c.Label.X < open1.X || c.Label.X+c.Label.W > hole1 {
		t.Errorf("one ring: reading %+v is not on the far side of the hole inside the shape (from %.1f to %.1f)", c.Label, open1.X, hole1)
	}
}

// TestCellsReadInOrder: macOS y grows upward, so on a side edge ring 0 must be
// the HIGHEST — the one coordinate flip this package gets wrong most easily.
func TestCellsReadInOrder(t *testing.T) {
	for _, s := range specs() {
		cells := Cells(PanelRect(screen16, s), s)
		if len(cells) != s.Profiles {
			t.Fatalf("%+v: %d cells", s, len(cells))
		}
		for i := 1; i < len(cells); i++ {
			a, b := cells[i-1].Ring, cells[i].Ring
			if s.Edge.Vertical() && b.Y >= a.Y {
				t.Errorf("%+v: ring %d is not below ring %d", s, i, i-1)
			}
			if !s.Edge.Vertical() && b.X <= a.X {
				t.Errorf("%+v: ring %d is not right of ring %d", s, i, i-1)
			}
		}
	}
}

// TestSlotsTileTheBody — every point of the body belongs to exactly one
// profile. A gap between hit bands is a strip the pointer crosses on its way
// from one ring to the next where no card belongs, so the card blinks.
func TestSlotsTileTheBody(t *testing.T) {
	for _, s := range specs() {
		p := PanelRect(screen16, s)
		cells := Cells(p, s)
		for i, c := range cells {
			if !containsRect(c.Slot, c.Ring) {
				t.Errorf("%+v: ring %d is not inside its own hit band", s, i)
			}
			if !containsRect(NotchRect(p, s, true), c.Slot) {
				t.Errorf("%+v: band %d escapes the open shape", s, i)
			}
			if i == 0 {
				continue
			}
			a, b := cells[i-1].Slot, c.Slot
			var gap float64
			if s.Edge.Vertical() {
				gap = a.Y - (b.Y + b.H)
			} else {
				gap = b.X - (a.X + a.W)
			}
			if !near(gap, 0) {
				t.Errorf("%+v: %.2fpt between bands %d and %d", s, gap, i-1, i)
			}
		}
	}
}

func TestHitIndexAgreesWithTheRings(t *testing.T) {
	for _, s := range specs() {
		cells := Cells(PanelRect(screen16, s), s)
		for i, c := range cells {
			if got := HitIndex(cells, c.Ring.X+c.Ring.W/2, c.Ring.Y+c.Ring.H/2); got != i {
				t.Errorf("%+v: ring %d's centre hit-tests as %d", s, i, got)
			}
		}
		if got := HitIndex(cells, -50, -50); got != -1 {
			t.Errorf("%+v: a point outside the shape hit-tests as %d", s, got)
		}
	}
}

// TestCardsStayInsideThePanel is why the panel reserves slack at each end: the
// card is drawn in this one panel rather than a second NSPanel, so a card that
// fell outside would simply be clipped away.
func TestCardsStayInsideThePanel(t *testing.T) {
	for _, s := range specs() {
		p := PanelRect(screen16, s)
		for i, c := range Cells(p, s) {
			for name, r := range map[string]Rect{"card": c.Card, "body": c.Body, "tail": c.Tail} {
				if r.W <= 0 || r.H <= 0 || !containsRect(bounds(p), r) {
					t.Errorf("%+v ring %d: %s %+v escapes the %.0fx%.0f panel", s, i, name, r, p.W, p.H)
				}
			}
			if !containsRect(c.Card, c.Body) || !containsRect(c.Card, c.Tail) {
				t.Errorf("%+v ring %d: body or tail is outside the card's live region", s, i)
			}
		}
	}
}

// TestCardIsTheReferenceSizeOnScreen — the card does not rotate with the edge.
// It is 225.6pt wide and CardHeight tall on every edge.
func TestCardIsTheReferenceSizeOnScreen(t *testing.T) {
	for _, s := range specs() {
		for i, c := range Cells(PanelRect(screen16, s), s) {
			if !near(c.Body.W, CardWidth) || !near(c.Body.H, CardHeight) {
				t.Errorf("%+v ring %d: body %.1fx%.1f, want %.1fx%.1f", s, i, c.Body.W, c.Body.H, CardWidth, CardHeight)
			}
		}
	}
}

// TestCardTouchesTheShape closes the gap the pointer crosses between a ring and
// its card. A strip in neither live rect makes the panel click-through mid
// travel, and the card is dismissed on the way to it.
func TestCardTouchesTheShape(t *testing.T) {
	for _, s := range specs() {
		p := PanelRect(screen16, s)
		open := NotchRect(p, s, true)
		for i, c := range Cells(p, s) {
			var gap float64
			switch s.Edge {
			case EdgeRight:
				gap = open.X - (c.Card.X + c.Card.W)
			case EdgeLeft:
				gap = c.Card.X - (open.X + open.W)
			case EdgeTop:
				gap = open.Y - (c.Card.Y + c.Card.H)
			default:
				gap = c.Card.Y - (open.Y + open.H)
			}
			if !near(gap, 0) {
				t.Errorf("%+v ring %d: %.2fpt of dead space between the shape and its card", s, i, gap)
			}
		}
	}
}

// TestTailPointsAtItsRing — the tail must stay on its profile even when the
// card body is pushed in from an end of the panel.
func TestTailPointsAtItsRing(t *testing.T) {
	for _, s := range specs() {
		for i, c := range Cells(PanelRect(screen16, s), s) {
			ringMid, tailMid := c.Ring.Y+c.Ring.H/2, c.Tail.Y+c.Tail.H/2
			if !s.Edge.Vertical() {
				ringMid, tailMid = c.Ring.X+c.Ring.W/2, c.Tail.X+c.Tail.W/2
			}
			if !near(ringMid, tailMid) {
				t.Errorf("%+v ring %d: tail centred %.1fpt off its ring", s, i, tailMid-ringMid)
			}
		}
	}
}

// TestPanelStaysOnScreen guards the degenerate inputs.
func TestPanelStaysOnScreen(t *testing.T) {
	for _, e := range allEdges {
		for _, sc := range []Rect{screen16, {}, {W: 800, H: 600}} {
			for _, n := range []int{0, 1, 40} {
				p := PanelRect(sc, Spec{Edge: e, Profiles: n})
				bound := sc
				if bound.W <= 0 || bound.H <= 0 {
					bound = fallbackScreen
				}
				if p.W <= 0 || p.H <= 0 || p.W > bound.W+1 || p.H > bound.H+1 {
					t.Errorf("edge %s screen %+v n=%d: panel %+v", e, sc, n, p)
				}
			}
		}
	}
}

// TestFoldedLiveRegionDoesNotCoverThePanel is the click-through guarantee: the
// panel spans a large, mostly transparent strip of the screen edge, and a
// folded live region that covered it would swallow every click meant for the
// app underneath.
func TestFoldedLiveRegionDoesNotCoverThePanel(t *testing.T) {
	for _, s := range specs() {
		p := PanelRect(screen16, s)
		rects := LiveRects(p, s, false, -1)
		if len(rects) != 1 {
			t.Fatalf("%+v: folded should expose only the wake band, got %d rects", s, len(rects))
		}
		if a := rects[0].W * rects[0].H; a >= p.W*p.H*0.25 {
			t.Errorf("%+v: folded live region is %.0f%% of the panel", s, 100*a/(p.W*p.H))
		}
	}
}

// TestOpenLiveRegionsIncludeTheHoveredCard — without the card the panel goes
// click-through the instant the pointer leaves the ring, so the card can never
// be reached.
func TestOpenLiveRegionsIncludeTheHoveredCard(t *testing.T) {
	s := Spec{Edge: EdgeRight, Profiles: 3}
	p := PanelRect(screen16, s)
	if got := len(LiveRects(p, s, true, -1)); got != 1 {
		t.Errorf("nothing hovered: want only the shape, got %d rects", got)
	}
	rects := LiveRects(p, s, true, 1)
	card := Cells(p, s)[1].Card
	if len(rects) != 2 || !InAny(rects, card.X+card.W/2, card.Y+card.H/2) {
		t.Error("the hovered ring's card does not take the mouse")
	}
	if got := len(LiveRects(p, s, true, 9)); got != 1 {
		t.Errorf("out-of-range hover index produced %d rects", got)
	}
}

// TestHiddenPercentGivesTheRoomBack: with the percentages off, nothing may
// still be reserved for them. Numbers worked by hand from the design
// constants, like TestLayoutMatchesTheReference's.
func TestHiddenPercentGivesTheRoomBack(t *testing.T) {
	// Down a side edge the label line leaves each cell, and both ends pad a
	// ring, so they take the pads' mean: 2 x 22.49 + 3 x 44 + 2 x 31.40.
	side := Spec{Edge: EdgeRight, Profiles: 3, HidePercent: true}
	if got := side.ExpandedLength() - 2*CurlRadius; !near(got, 239.78) {
		t.Errorf("side, hidden: body length %.2f, want 239.78 (321.13 with labels)", got)
	}
	if !near(side.ExpandedDepth(), SideBodyDepth) {
		t.Errorf("side, hidden: depth %.2f, want the unchanged %.2f", side.ExpandedDepth(), SideBodyDepth)
	}

	// Across a horizontal edge the label line leaves the depth: the ring and
	// its margins, 69.95, not the 97.06 that holds a reading under it.
	bottom := Spec{Edge: EdgeBottom, Profiles: 3, HidePercent: true}
	if !near(bottom.ExpandedDepth(), 69.95) {
		t.Errorf("bottom, hidden: depth %.2f, want 69.95", bottom.ExpandedDepth())
	}

	// Joined to a hardware notch, that lighter depth is what scales onto the
	// hole: 32 / 69.95 = 0.4575, a 20.13pt ring where labels leave 14.51 —
	// the one-ring case's size, for every ring.
	for n := 1; n <= 3; n++ {
		j := Spec{Edge: EdgeTop, Profiles: n, Hardware: notch16, HidePercent: true}
		p := PanelRect(screen16, j)
		hole := (p.W - notch16.W) / 2
		open := NotchRect(p, j, true)
		// Nothing crosses the hole, so the pair balances about it even with
		// one ring, where a shown percentage makes the far side short.
		if !near(open.X+open.W/2, hole+notch16.W/2) {
			t.Errorf("joined n=%d, hidden: open pair is not symmetric about the hole", n)
		}
		for i, c := range Cells(p, j) {
			if !near(c.Ring.W, 20.13) {
				t.Errorf("joined n=%d, hidden: ring %d is %.2fpt, want 20.13", n, i, c.Ring.W)
			}
			if c.Across {
				t.Errorf("joined n=%d, hidden: ring %d reads across a hole with nothing to show", n, i)
			}
		}
	}

	// And no edge hands the renderer a label box to draw into.
	for _, s := range specs() {
		if !s.HidePercent {
			continue
		}
		for i, c := range Cells(PanelRect(screen16, s), s) {
			if c.Label != (Rect{}) {
				t.Errorf("%+v: ring %d has label box %+v with percentages hidden", s, i, c.Label)
			}
		}
	}
}
