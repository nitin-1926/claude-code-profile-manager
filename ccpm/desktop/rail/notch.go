//go:build darwin

package rail

// Notch geometry: a FIXED panel that never resizes, with the visible shape
// animating inside it.
//
// This replaces the peek/full two-frame model the parked rail used, and the
// reason is the whole point of this file.
//
// The old model animated the panel's own frame between a peek rect and a full
// rect. Revealing therefore moved the hit region out from under the pointer
// that had just triggered it, and AppKit answered the resize by re-evaluating
// the tracking area and sending exited/entered churn. Three rounds of fixes
// went into managing that feedback loop rather than removing it.
//
// Here the panel is sized ONCE, for the fully expanded shape plus the slack a
// hover card needs at either end, and never changes. Expanding morphs the
// shape inside that panel. Nothing the pointer is tested against moves when
// the notch opens, so the loop is gone by construction rather than damped.
//
// The invariant that makes it safe is asserted in the tests: the collapsed hit
// region is CONTAINED in the expanded shape. The pointer that opened the notch
// is therefore still inside it afterwards.
//
// Coordinates: origin bottom-left, points. "Panel-local" rects have their
// origin at the panel's bottom-left corner.

// The design scale, and every measurement below, come from the codenotch
// reference (MIT, github.com/vinzdg/codenotch). Its layout is specified in
// pixels of a design frame whose ring is 117px across, and that ring is 44pt
// on screen — so one design pixel is 44/117 of a point. The constants are kept
// in design pixels and converted, rather than restated as rounded points, so a
// difference from the reference is a visible edit to a number the reference
// also has, not a drift nobody can trace.
const designScale = 44.0 / 117.0

// Notch body and shape.
const (
	// SideBodyDepth is how far the open notch stands off a side edge.
	SideBodyDepth = 186 * designScale // 69.9pt
	// CurlRadius is the concave shoulder where the shape meets the bezel. It
	// is what makes the notch read as growing out of the screen edge rather
	// than as a panel parked against it.
	CurlRadius = 103 * designScale // 38.7pt
	// CornerRadius rounds the shape's inner corners.
	CornerRadius = 78.8 * designScale // 29.6pt
	// PadTop and PadBottom pad the ends of the ring stack. They are different
	// on purpose: PadTop is body-top to the first RING, PadBottom is the last
	// LABEL to body-foot, and they pad different things.
	PadTop    = 69.5 * designScale // 26.1pt
	PadBottom = 50.1 * designScale // 18.8pt
	// CellSpacing is label-bottom to the next ring's top.
	CellSpacing = 83.5 * designScale // 31.4pt
)

// Collapsed pill.
const (
	// PillWidth is the folded pill's depth off the bezel, PillHeight its
	// length along it.
	PillWidth  = 26 * designScale  // 9.8pt
	PillHeight = 210 * designScale // 79.0pt
	// PillHotZone widens the collapsed hit region on every side. Aiming at a
	// 10pt sliver is not a reasonable ask; this is what makes the notch open
	// on approach. Dropped entirely when joined to a hardware notch — see
	// WakeRect.
	PillHotZone = 90 * designScale // 33.8pt
)

// Joined to a display's hardware notch (codenotch 1.18-1.19).
//
// The notch is not a second black object ten points below or beside the
// camera housing — the eye reads that as one shape with a fault in it. It is
// the hardware's own notch, widened by the same amount on either side: two
// bars at the hole's exact depth, each running a little way INTO the hole and
// out of the far side of it. Nothing drawn behind the hole is on screen, so
// the pair and the hole are drawn as one bar spanning both, and folding draws
// each side back into its own wall of the hole.
const (
	// CutoutOverlap is how far each side tucks inside the hole. The hole's
	// bottom corners are rounded, and a bar that stopped dead on the wall would
	// leave a lit sliver in the crook of each. Screen points, not design ones:
	// it is measured against the hardware, which does not scale.
	CutoutOverlap = 12.0
)

// One profile's cell: a ring with its percentage label.
const (
	RingDiameter   = 44.0
	TrackStroke    = 15.5 * designScale // 5.8pt
	ProgressStroke = 8 * designScale    // 3.0pt
	GlyphSize      = 46 * designScale   // 17.3pt
	RingLabelGap   = 26.9 * designScale // 10.1pt
	// WeeklyStroke draws the seven-day window thinner than the five-hour arc:
	// the same kind of fact with a lesser claim on the eye. Two arcs of equal
	// weight in a 44pt circle read as one confused reading.
	WeeklyStroke = 5 * designScale  // 1.9pt
	WeeklyRadius = 28 * designScale // 10.5pt, in the band between glyph and track
	// PercentSize is the label's font size: a 27px cap height on SF Pro, whose
	// cap height is 0.714 of its em. PercentLineHeight is that font's line box.
	PercentSize       = 27 * designScale / 0.714 // 14.2pt
	PercentLineHeight = 17.0
	// PercentAcrossSize is the one ring's percentage when it sits across a
	// hardware notch from its ring, with the whole depth to itself: capitals
	// 40% of the ring's 117px.
	PercentAcrossSize = 47 * designScale / 0.714 // 24.8pt
	// ReadingAcrossWidth is the room that percentage is given along the bar.
	// ponytail: a fixed budget for "100%" (codenotch measures the live string),
	// so a short reading like "9%" leaves ~1.5em of black beyond it; measure
	// the text if that shows.
	ReadingAcrossWidth = 2.8 * PercentAcrossSize // 69.3pt
)

// The hover card.
const (
	CardWidth   = 600 * designScale  // 225.6pt
	CardCorner  = 49.5 * designScale // 18.6pt
	CardPadding = 32 * designScale   // 12.0pt
	TailLength  = 75 * designScale   // 28.2pt
	TailHeight  = 87 * designScale   // 32.7pt
	TailGap     = 28 * designScale   // 10.5pt, tail tip to the notch body
	BarHeight   = 10.5 * designScale // 3.9pt
	// CardHeight is the card body's height. Codenotch measures its tallest
	// card; ccpm's card has a fixed shape — a header and two usage windows —
	// so a fixed budget with headroom does the same job.
	CardHeight = 160.0
)

// The card body is always CardWidth wide and CardHeight tall ON SCREEN — it
// does not rotate with the edge. Down a side edge its width therefore runs
// away from the bezel; across a horizontal edge it runs along it. These two
// helpers are the only place that difference lives.
func (s Spec) cardAlong() float64 {
	if s.Edge.Vertical() {
		return CardHeight
	}
	return CardWidth
}

// cardDepth is the room reserved away from the bezel for the card body, its
// tail, and the gap the tail spans.
func (s Spec) cardDepth() float64 {
	if s.Edge.Vertical() {
		return TailGap + TailLength + CardWidth
	}
	return TailGap + TailLength + CardHeight
}

// BezelBleed is extra depth reserved against the bezel so a fractionally
// placed panel can never leave a hairline of wallpaper between the notch and
// the screen edge.
const BezelBleed = 2.0

// Spec is everything the geometry depends on.
type Spec struct {
	Edge     Edge
	Profiles int
	// Hardware is the display's physical notch (W, H), or zero. Only used on
	// the top edge, where the notch joins to it.
	Hardware Rect
	// HidePercent drops the percentage under every ring, and the room the
	// geometry reserves for it: a line reserved for a reading that is not
	// drawn would come straight off the rings. The zero value shows them, so
	// every Spec written before the setting existed still means what it did.
	HidePercent bool
}

// Flush reports whether the collapsed notch is joined to a hardware notch.
func (s Spec) Flush() bool {
	return s.Edge == EdgeTop && s.Hardware.W > 0 && s.Hardware.H > 0
}

func (s Spec) n() int { return max(1, s.Profiles) }

// cellExtent is a ring plus its label, measured down the stack on a side edge.
func cellExtent() float64 { return RingDiameter + RingLabelGap + PercentLineHeight }

// sideRingMargin is the clear band either side of a ring across a side edge.
func sideRingMargin() float64 { return (SideBodyDepth - RingDiameter) / 2 }

// labelsAlong reports whether each ring's percentage sits beneath it ALONG
// the stack, which is only down a side edge with percentages shown.
func (s Spec) labelsAlong() bool { return s.Edge.Vertical() && !s.HidePercent }

// cellAlong is what one cell claims ALONG the stack. Down a side edge that is
// the ring and the label beneath it; across a horizontal edge the label has
// moved into the depth, so the cell is the ring alone — as it is anywhere the
// percentage is hidden. Using the side-edge figure there left 27pt of nothing
// between every pair of rings.
func (s Spec) cellAlong() float64 {
	if s.labelsAlong() {
		return cellExtent()
	}
	return RingDiameter
}

// padStart and padEnd are PadTop/PadBottom down a side edge. Where both ends
// pad the same thing — a ring, across a horizontal edge or with no label under
// the last ring — they become one number, their mean, which keeps a single
// ring centred in its own bar.
func (s Spec) padStart() float64 {
	if s.labelsAlong() {
		return PadTop
	}
	return (PadTop + PadBottom) / 2
}

func (s Spec) padEnd() float64 {
	if s.labelsAlong() {
		return PadBottom
	}
	return (PadTop + PadBottom) / 2
}

// BodyDepth is the expanded body's depth in design measurements — on screen
// as it stands everywhere but a hardware notch, which scales it (see scale).
//
// Joined with one ring, the ring's percentage moves across the hole (see
// readsAcross), so the depth holds the ring alone: reserving a line for a
// reading that is not drawn there would come straight off the ring. The same
// holds when the percentage is hidden altogether.
func (s Spec) BodyDepth() float64 {
	if s.Edge.Vertical() {
		return SideBodyDepth
	}
	if s.readsAcross() || s.HidePercent {
		return 2*sideRingMargin() + RingDiameter
	}
	return 2*sideRingMargin() + cellExtent()
}

// readsAcross reports whether the one ring's percentage is drawn on the other
// side of the hardware notch. Joined, the notch widens the Mac's by the same on
// either side and only one side has rings to carry; with a single ring the
// other side takes its reading, level with it, and the ring keeps the whole
// depth. With more there is still only one other side, so each ring keeps its
// percentage under it. With the percentage hidden there is nothing to carry,
// so the two sides balance as they do with several rings.
func (s Spec) readsAcross() bool { return s.Flush() && s.n() == 1 && !s.HidePercent }

// scale is the size the notch is drawn at: 1 everywhere, except joined to a
// hardware notch, where it is the one scale that lands the body's design depth
// exactly on the hole's. One shape cannot be two thicknesses, and a shape
// whose depth is fixed while its rings, spacing and padding stay at design
// size is not smaller, it is distorted — so every measurement is drawn at this
// scale and every proportion stays the design's.
func (s Spec) scale() float64 {
	if !s.Flush() {
		return 1
	}
	return s.Hardware.H / s.BodyDepth()
}

func (s Spec) bodyLength() float64 {
	n := float64(s.n())
	return s.padStart() + n*s.cellAlong() + (n-1)*CellSpacing + s.padEnd()
}

// holeGap is the stretch of a hardware notch between the two sides' tucked-in
// ends. Nothing drawn there is on screen.
func (s Spec) holeGap() float64 { return max(0, s.Hardware.W-2*CutoutOverlap) }

// carryLength is the length of the side that carries the rings, from its end
// inside the hole to its far tip: a flare's allowance at BOTH ends as on every
// edge, plus the length buried in the hole, which nobody sees and so the rings
// must not be measured from. Folded it is the pill, plus the same buried run.
func (s Spec) carryLength(expanded bool) float64 {
	sc := s.scale()
	if !expanded {
		return sc*PillHeight + CutoutOverlap
	}
	return sc*(s.bodyLength()+2*CurlRadius) + CutoutOverlap
}

// otherLength is the other side of the hole: the same length, so the pair
// balances about the hole — except when it carries the one ring's percentage
// open, when it is only as long as that needs: out of the hole, the ring's
// margin, the number, the margin again, and its own flare. As long as the
// ring's side it left a short number with most of a side of black after it.
func (s Spec) otherLength(expanded bool) float64 {
	carry := s.carryLength(expanded)
	if !expanded || !s.readsAcross() {
		return carry
	}
	return min(carry, CutoutOverlap+s.scale()*(2*sideRingMargin()+ReadingAcrossWidth+CurlRadius))
}

// ExpandedLength and ExpandedDepth size the open shape, shoulders included.
// Joined, that is both sides and the hole between them.
func (s Spec) ExpandedLength() float64 {
	if s.Flush() {
		return s.otherLength(true) + s.holeGap() + s.carryLength(true)
	}
	return s.bodyLength() + 2*CurlRadius
}

func (s Spec) ExpandedDepth() float64 { return s.scale() * s.BodyDepth() }

// CollapsedLength and CollapsedDepth size the folded shape: the pill, or —
// joined to a hardware notch — the hardware notch carrying a pill's length out
// of each side, at the hole's depth. Folding a joined notch changes only its
// length: each side draws back into its own wall of the hole.
func (s Spec) CollapsedLength() float64 {
	if s.Flush() {
		return 2*s.carryLength(false) + s.holeGap()
	}
	return PillHeight
}

func (s Spec) CollapsedDepth() float64 {
	if s.Flush() {
		return s.Hardware.H
	}
	return PillWidth
}

// span is the length the panel reserves for the shape. Joined it is sized for
// two ring-carrying sides, so the hole lands on the panel's centre line even
// when the other side is shorter.
func (s Spec) span() float64 {
	if s.Flush() {
		return 2*s.carryLength(true) + s.holeGap()
	}
	return s.ExpandedLength()
}

// fit caps Profiles at the most rings whose shape fits in a panel span long,
// never fewer than one. PanelRect clamps the panel to the screen, but the
// rings kept their pitch: ten profiles down a 956pt edge ran half off the
// panel. The renderer draws min(slots, cells), so the rings that do not fit
// are simply not drawn.
//
// ponytail: capping hides the profiles past the fit; shrink the pitch first if
// a short edge with many profiles turns out to be common.
func (s Spec) fit(span float64) Spec {
	for s.Profiles > 1 && s.span() > span {
		s.Profiles--
	}
	return s
}

// ShapeParams is the resolved corner and shoulder for one state of the shape.
type ShapeParams struct {
	Corner float64 `json:"corner"`
	Curl   float64 `json:"curl"`
}

// Shape resolves the corner radius and shoulder for the shape in one state.
//
// The ORDER of the clamps is the thing to get right. The corner claims its
// radius first, out of half the depth; the shoulder then takes what is left.
// Clamping the corner by (depth - curl) instead — the obvious reading —
// collapses it to zero as soon as the shoulder is as wide as the body, which is
// exactly the folded pill: a 10pt shape comes out with square corners.
func (s Spec) Shape(expanded bool) ShapeParams {
	depth, length := s.CollapsedDepth(), s.CollapsedLength()
	if expanded {
		depth, length = s.ExpandedDepth(), s.ExpandedLength()
	}
	// Joined to a hardware notch the far ends are the notch's own curve — the
	// flare into the bezel and the corner below it — at the joined scale. The
	// ends at the hole are inside it, where nothing shows, so the one bar the
	// pair is drawn as needs no end of its own there.
	flare := CurlRadius * s.scale()
	cornerCap := CornerRadius * s.scale()
	wanted := max(0, min(cornerCap, depth/2))
	curl := max(0, min(flare, length/2, depth-wanted))
	corner := max(0, min(wanted, (length-2*curl)/2))
	return ShapeParams{Corner: corner, Curl: curl}
}

// Slack is the room reserved at each END of the panel, along the edge, so a
// hover card centred on the first or last ring still falls inside the panel.
func (s Spec) Slack() float64 { return s.cardAlong() / 2 }

// PanelRect is the panel's frame in screen coordinates. It depends only on the
// screen and the spec — NOT on whether the notch is expanded, which is the
// property this whole file exists to guarantee.
//
// screen is NSScreen.frame, deliberately not visibleFrame: anchoring to the
// visible frame moves the notch whenever the Dock is shown, hidden or moved.
//
// The rect is rounded — size up, origin to nearest — because AppKit rounding
// a fractional frame of its own accord makes a panel a fraction larger than
// asked for, and the overhang shows as a seam against the bezel.
func PanelRect(screen Rect, s Spec) Rect {
	if screen.W <= 0 || screen.H <= 0 {
		screen = fallbackScreen
	}
	length := s.span() + 2*s.Slack()
	depth := s.ExpandedDepth() + s.cardDepth() + BezelBleed

	if s.Edge.Vertical() {
		length, depth = min(length, screen.H), min(depth, screen.W)
		y := screen.Y + (screen.H-length)/2
		x := screen.X + screen.W - depth
		if s.Edge == EdgeLeft {
			x = screen.X
		}
		return round(Rect{X: x, Y: y, W: depth, H: length})
	}
	length, depth = min(length, screen.W), min(depth, screen.H)
	x := screen.X + (screen.W-length)/2
	y := screen.Y
	if s.Edge == EdgeTop {
		y = screen.Y + screen.H - depth
	}
	return round(Rect{X: x, Y: y, W: length, H: depth})
}

// place converts an (along, depth-from-bezel) box into a panel-local rect.
//
// along is measured from the panel's start of the long axis; for vertical
// edges that start is the TOP, because rings read top to bottom and macOS y
// grows upward. depth is measured from the bezel inward. This is the only
// function that knows axis orientation — everything above it works in
// (along, depth) and cannot get a flip wrong.
func place(panel Rect, e Edge, along, alongLen, depth, depthLen float64) Rect {
	switch e {
	case EdgeRight:
		return Rect{X: panel.W - depth - depthLen, Y: panel.H - along - alongLen, W: depthLen, H: alongLen}
	case EdgeLeft:
		return Rect{X: depth, Y: panel.H - along - alongLen, W: depthLen, H: alongLen}
	case EdgeTop:
		return Rect{X: along, Y: panel.H - depth - depthLen, W: alongLen, H: depthLen}
	default: // bottom
		return Rect{X: along, Y: depth, W: alongLen, H: depthLen}
	}
}

func alongSpan(panel Rect, e Edge) float64 {
	if e.Vertical() {
		return panel.H
	}
	return panel.W
}

// NotchRect is the drawn shape's rectangle in panel-local coordinates.
//
// Collapsed and expanded share a centre line along the edge, so folding never
// slides the notch along the bezel, and both sit flush against it. Joined to a
// hardware notch they share the hole instead: each side is pinned to its own
// wall and folds back into it.
func NotchRect(panel Rect, s Spec, expanded bool) Rect {
	s = s.fit(alongSpan(panel, s.Edge))
	length, depth := s.CollapsedLength(), s.CollapsedDepth()
	if expanded {
		length, depth = s.ExpandedLength(), s.ExpandedDepth()
	}
	span := alongSpan(panel, s.Edge)
	if s.Flush() {
		return place(panel, s.Edge, holeStart(panel, s)+CutoutOverlap-s.otherLength(expanded), length, 0, depth)
	}
	length = min(length, span)
	return place(panel, s.Edge, (span-length)/2, length, 0, depth)
}

// holeStart is where a hardware notch's left wall falls along the panel. The
// panel is centred on the screen and the hole is too, so the hole is centred
// in the panel — to within the rounding PanelRect applies, which the
// CutoutOverlap tucked into the hole absorbs many times over.
func holeStart(panel Rect, s Spec) float64 {
	return (alongSpan(panel, s.Edge) - s.Hardware.W) / 2
}

// WakeRect is the region that opens the notch while it is collapsed: the drawn
// pill grown by PillHotZone on every side, then clamped into the expanded
// shape.
//
// The clamp is load-bearing. If the wake region could reach outside the
// expanded shape, a pointer in that sliver would open the notch and be outside
// it immediately — the stutter that parked the previous implementation.
//
// Joined to a hardware notch the band is dropped: a band around it reaches
// well below the menu bar, across the title bar of any window tiled at screen
// centre, whose traffic lights would then open the notch on approach and
// disappear underneath it. What is left is the folded shape — the hardware's
// own notch and the pill either side of it — which is already a generous
// target, and reaching for the pair means reaching for the thing they are
// joined to.
func WakeRect(panel Rect, s Spec) Rect {
	pill := NotchRect(panel, s, false)
	band := PillHotZone
	if s.Flush() {
		band = 0
	}
	grown := Rect{X: pill.X - band, Y: pill.Y - band, W: pill.W + 2*band, H: pill.H + 2*band}
	return intersect(grown, NotchRect(panel, s, true))
}

// Cell is where one profile is drawn, in panel-local coordinates.
type Cell struct {
	// Ring is the ring's square box, RingDiameter on a side.
	Ring Rect `json:"ring"`
	// Label is the percentage's line box, beneath the ring on a side edge and
	// inward of it on a horizontal one. Zero with Spec.HidePercent.
	Label Rect `json:"label"`
	// Across marks a label drawn on the far side of a hardware notch from its
	// ring (see Spec.readsAcross): the larger size, held against the hole.
	Across bool `json:"across"`
	// Slot is the hit band: every point of the body belongs to exactly one
	// profile, so the pointer never crosses a strip that belongs to nobody on
	// its way from one ring to the next.
	Slot Rect `json:"slot"`
	// Card is the hover card's live region: body, tail, and the gap the tail
	// spans, from the shape's inner face outward.
	Card Rect `json:"card"`
	// Body is the card itself, CardWidth x CardHeight on screen.
	Body Rect `json:"body"`
	// Tail is the bounding box of the pointer between the body and the ring.
	// Its tip is the side facing the shape; the renderer draws the triangle.
	Tail Rect `json:"tail"`
}

// Cells lays out every profile. i=0 is the first in reading order: top on a
// side edge, left on a horizontal one.
func Cells(panel Rect, s Spec) []Cell {
	s = s.fit(alongSpan(panel, s.Edge))
	n := s.n()
	span := alongSpan(panel, s.Edge)
	// Everything laid along or across the body is drawn at the joined scale
	// (1 off a hardware notch), rings and labels included.
	sc := s.scale()
	bodyStart := (span-min(s.ExpandedLength(), span))/2 + CurlRadius
	if s.Flush() {
		// The rings ride the side right of the hole, measured from its wall:
		// what is buried inside the hole is length nobody sees.
		bodyStart = holeStart(panel, s) + s.Hardware.W + sc*CurlRadius
	}
	bodyEnd := bodyStart + sc*s.bodyLength()
	pitch := sc * (s.cellAlong() + CellSpacing)
	ringD := sc * RingDiameter
	depth := s.ExpandedDepth()

	out := make([]Cell, 0, n)
	for i := range n {
		ringAlong := bodyStart + sc*s.padStart() + float64(i)*pitch
		var ring, label Rect
		across := s.readsAcross()
		switch {
		case s.Edge.Vertical():
			d := sideRingMargin()
			ring = place(panel, s.Edge, ringAlong, RingDiameter, d, RingDiameter)
			label = place(panel, s.Edge, ringAlong+RingDiameter+RingLabelGap, PercentLineHeight, d-10, RingDiameter+20)
		case across:
			// On the other side of the hole, held against it by the ring's own
			// margin and level with the ring, which is centred in the depth.
			d := sc * sideRingMargin()
			ring = place(panel, s.Edge, ringAlong, ringD, d, ringD)
			h := sc * PercentAcrossSize * PercentLineHeight / PercentSize
			label = place(panel, s.Edge, holeStart(panel, s)-sc*(sideRingMargin()+ReadingAcrossWidth),
				sc*ReadingAcrossWidth, (depth-h)/2, h)
		default:
			d := sc * sideRingMargin()
			ring = place(panel, s.Edge, ringAlong, ringD, d, ringD)
			label = place(panel, s.Edge, ringAlong-10*sc, ringD+20*sc, d+sc*(RingDiameter+RingLabelGap), sc*PercentLineHeight)
		}

		// The hit band splits each gap between cells down the middle, and the
		// outermost bands run to the ends of the body.
		from := bodyStart
		if i > 0 {
			from = ringAlong - sc*CellSpacing/2
		}
		to := bodyEnd
		if i < n-1 {
			to = ringAlong + sc*(s.cellAlong()+CellSpacing/2)
		}
		slot := place(panel, s.Edge, from, to-from, 0, depth)

		// The card is centred on its ring and grows away from the bezel, from
		// the shape's inner face to the far side of the panel. Starting at the
		// face rather than at the tail tip is deliberate: the pointer travels
		// from the ring to the card through the gap, and a gap that belonged to
		// neither would go click-through and dismiss the card mid-travel.
		centre := ringAlong + ringD/2
		ca := s.cardAlong()
		cardAlong := clamp(centre-ca/2, 0, span-ca)
		panelDepth := panel.W
		if !s.Edge.Vertical() {
			panelDepth = panel.H
		}
		card := place(panel, s.Edge, cardAlong, ca, depth, panelDepth-depth)
		body := place(panel, s.Edge, cardAlong, ca, depth+TailGap+TailLength, s.cardDepth()-TailGap-TailLength)
		// The tail stays on its ring even when the body is clamped at an end
		// of the panel, so it always points at the right profile.
		tailAlong := clamp(centre-TailHeight/2, cardAlong, cardAlong+ca-TailHeight)
		tail := place(panel, s.Edge, tailAlong, TailHeight, depth+TailGap, TailLength)

		// Hidden percentages get no box at all: the renderer draws a label
		// only into a rect with area, so a zero rect is the whole signal.
		if s.HidePercent {
			label = Rect{}
		}
		out = append(out, Cell{Ring: ring, Label: label, Across: across, Slot: slot, Card: card, Body: body, Tail: tail})
	}
	return out
}

func intersect(a, b Rect) Rect {
	x0, y0 := max(a.X, b.X), max(a.Y, b.Y)
	x1, y1 := min(a.X+a.W, b.X+b.W), min(a.Y+a.H, b.Y+b.H)
	if x1 <= x0 || y1 <= y0 {
		return Rect{X: x0, Y: y0}
	}
	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

func clamp(v, lo, hi float64) float64 {
	if hi < lo {
		return lo
	}
	return min(max(v, lo), hi)
}

// round mirrors what AppKit would otherwise do, but in the direction we want:
// size up so the shape is never clipped, origin to nearest so the panel does
// not drift off the bezel.
func round(r Rect) Rect {
	return Rect{X: roundHalf(r.X), Y: roundHalf(r.Y), W: ceil(r.W), H: ceil(r.H)}
}

func roundHalf(v float64) float64 {
	if v < 0 {
		return -ceil(-v - 0.5)
	}
	return ceil(v - 0.5)
}

func ceil(v float64) float64 {
	i := float64(int64(v))
	if v > i {
		return i + 1
	}
	return i
}
