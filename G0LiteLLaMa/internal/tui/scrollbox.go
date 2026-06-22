package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"g0litellama/internal/tui/shapes"
)

// ScrollBox renders a vertically scrollable content area with a right-side
// scrollbar. When Pinned is true new content auto-scrolls to the bottom.
type ScrollBox struct {
	Lines     []string
	ViewLines int // visible row count (height in terminal lines)
	Offset    int // scroll offset (0 = top)
	Pinned    bool
}

// MaxOffset returns the maximum valid offset for the current content height.
func (s *ScrollBox) MaxOffset() int {
	n := len(s.Lines) - s.ViewLines
	if n < 0 {
		return 0
	}
	return n
}

// ClampOffset ensures Offset stays in [0, MaxOffset].
// If Offset goes to MaxOffset and Pinned is set, re-engages pinning.
func (s *ScrollBox) ClampOffset() {
	max := s.MaxOffset()
	if s.Offset > max {
		s.Offset = max
	}
	if s.Offset < 0 {
		s.Offset = 0
	}
}

// ScrollUp scrolls up by n lines. Detaches pin.
func (s *ScrollBox) ScrollUp(n int) {
	s.Pinned = false
	s.Offset += n
	s.ClampOffset()
}

// ScrollDown scrolls down by n lines. Re-engages pin if already at or
// reaching the bottom.
func (s *ScrollBox) ScrollDown(n int) {
	if s.Pinned || s.Offset >= s.MaxOffset() {
		s.Pinned = true
		s.Offset = s.MaxOffset()
		return
	}
	s.Offset -= n
	s.ClampOffset()
	if s.Offset == s.MaxOffset() {
		s.Pinned = true
	}
}

// ScrollToBottom pins the view to the latest content.
func (s *ScrollBox) ScrollToBottom() {
	s.Pinned = true
	s.Offset = s.MaxOffset()
}

// SetLines replaces the content lines. Re-pins content when Pinned is true.
func (s *ScrollBox) SetLines(lines []string) {
	s.Lines = lines
	if s.Pinned {
		s.Offset = s.MaxOffset()
	} else {
		s.ClampOffset()
	}
}

// AppendLine adds a single line. Re-pins content when Pinned is true.
func (s *ScrollBox) AppendLine(line string) {
	s.Lines = append(s.Lines, line)
	if s.Pinned {
		s.Offset = s.MaxOffset()
	}
}

// VisibleLines returns the slice of Lines visible at the current offset.
func (s *ScrollBox) VisibleLines() []string {
	start := s.Offset
	end := start + s.ViewLines
	if start >= len(s.Lines) {
		return nil
	}
	if end > len(s.Lines) {
		end = len(s.Lines)
	}
	return s.Lines[start:end]
}

const scrollBarWidth = 1

// View renders the visible lines with a right-side scrollbar.
// The rendered width is width+1 (content + scrollbar column).
func (s *ScrollBox) View(width int) string {
	s.ClampOffset()

	visible := s.VisibleLines()
	// Pad to fill ViewLines so the scrollbar aligns with the viewport.
	for len(visible) < s.ViewLines {
		visible = append(visible, "")
	}

	scrollCol := lipgloss.NewStyle().Width(scrollBarWidth).Align(lipgloss.Left)
	trackStyle := scrollCol.Background(lipgloss.Color("236"))
	thumbStyle := scrollCol.Background(lipgloss.Color("243"))

	total := len(s.Lines)
	if total < s.ViewLines {
		total = s.ViewLines
	}

	thumbHeight := thumbSize(s.ViewLines, total)
	thumbStart := thumbPos(s.Offset, s.ViewLines, total, thumbHeight)

	contentStyle := lipgloss.NewStyle().Width(width)

	var b strings.Builder
	for i := 0; i < s.ViewLines; i++ {
		// Content cell
		cell := ""
		if i < len(visible) {
			cell = visible[i]
		}
		b.WriteString(contentStyle.Render(cell))

		// Scrollbar cell
		if i >= thumbStart && i < thumbStart+thumbHeight {
			b.WriteString(thumbStyle.Render(" "))
		} else {
			b.WriteString(trackStyle.Render(" "))
		}

		if i < s.ViewLines-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// HitTargets returns the scrollbar hit targets positioned at (left, top).
// The track covers the full ViewLines height; the thumb covers its current
// position. Caller registers these into HitRegistry before rendering.
func (s *ScrollBox) HitTargets(left, top int) []HitTarget {
	s.ClampOffset()
	w := left + scrollBarWidth

	total := len(s.Lines)
	if total < s.ViewLines {
		total = s.ViewLines
	}
	thumbH := thumbSize(s.ViewLines, total)
	thumbStart := thumbPos(s.Offset, s.ViewLines, total, thumbH)

	trackRect := shapes.Rect{Row: top, Col: w, Rows: s.ViewLines, Cols: scrollBarWidth + 1}
	thumbRect := shapes.Rect{Row: top + thumbStart, Col: w, Rows: thumbH, Cols: scrollBarWidth + 1}

	return []HitTarget{
		{Rect: trackRect, Action: "scrollbar-track"},
		{Rect: thumbRect, Action: "scrollbar-thumb"},
	}
}

// ButtonHitArea returns the clickable region for the entire scrollbar column
// at a given viewport position. Caller uses this for mouse-wheel-area detection.
func (s *ScrollBox) ButtonHitArea(left, top int) shapes.Rect {
	return shapes.Rect{Row: top, Col: left + scrollBarWidth, Rows: s.ViewLines, Cols: scrollBarWidth}
}

func thumbSize(viewport, total int) int {
	if total <= viewport {
		return viewport
	}
	h := viewport * viewport / total
	if h < 1 {
		return 1
	}
	return h
}

func thumbPos(offset, viewport, total, thumbH int) int {
	maxOff := total - viewport
	if maxOff <= 0 {
		return 0
	}
	return offset * (viewport - thumbH) / maxOff
}
